package main

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	SW_SHOW             = 5
	CW_USEDEFAULT       = 0x80000000
	WM_DESTROY          = 0x0002
	WM_CLOSE            = 0x0010
	WM_PAINT            = 0x000F
	WM_TIMER            = 0x0113
	WM_LBUTTONDOWN      = 0x0201
	WM_KEYDOWN          = 0x0100
	WM_SYSKEYDOWN       = 0x0104
	WM_CHAR             = 0x0102
	WM_HOTKEY           = 0x0312
	VK_BACK             = 0x08
	VK_RETURN           = 0x0D
	VK_F8               = 0x77
	VK_F9               = 0x78
	VK_F10              = 0x79
	MOD_ALT             = 0x0001
	MOD_CONTROL         = 0x0002
	MOD_NOREPEAT        = 0x4000
	HOTKEY_LOCK         = 1001
	HOTKEY_START        = 1002
	HOTKEY_STOP         = 1003
	HOTKEY_LOCK_BACKUP  = 1011
	HOTKEY_START_BACKUP = 1012
	HOTKEY_STOP_BACKUP  = 1013
	TIMER_REFRESH       = 7001
	WM_APP_CLICK_OK     = 0x8001
	WM_APP_CLICK_FAIL   = 0x8002
	MOUSEEVENTF_MOVE        = 0x0001
	MOUSEEVENTF_LEFTDOWN    = 0x0002
	MOUSEEVENTF_LEFTUP      = 0x0004
	MOUSEEVENTF_RIGHTDOWN   = 0x0008
	MOUSEEVENTF_RIGHTUP     = 0x0010
	MOUSEEVENTF_VIRTUALDESK = 0x4000
	MOUSEEVENTF_ABSOLUTE    = 0x8000
	COLOR_WINDOW  = 5
	TRANSPARENT   = 1
	DT_LEFT       = 0x00000000
	DT_CENTER     = 0x00000001
	DT_VCENTER    = 0x00000004
	DT_SINGLELINE = 0x00000020
	DT_NOPREFIX   = 0x00000800
	SM_XVIRTUALSCREEN  = 76
	SM_YVIRTUALSCREEN  = 77
	SM_CXVIRTUALSCREEN = 78
	SM_CYVIRTUALSCREEN = 79
)

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }
type MSG struct {
	Hwnd uintptr
	Message uint32
	WParam, LParam uintptr
	Time uint32
	Pt POINT
	LPrivate uint32
}
type WNDCLASSEXW struct {
	CbSize uint32
	Style uint32
	LpfnWndProc uintptr
	CbClsExtra, CbWndExtra int32
	HInstance, HIcon, HCursor, HbrBackground uintptr
	LpszMenuName, LpszClassName uintptr
	HIconSm uintptr
}
type PAINTSTRUCT struct {
	Hdc uintptr
	Erase int32
	RcPaint RECT
	Restore, IncUpdate int32
	Reserved [32]byte
}
type MOUSEINPUT struct {
	Dx, Dy int32
	MouseData, DwFlags, Time uint32
	DwExtraInfo uintptr
}
type INPUT struct { Type uint32; Mi MOUSEINPUT }
type UIState struct {
	main, font uintptr
	cursorX, cursorY int32
	lockedX, lockedY int32
	locked, running, right bool
	intervalMs uint32
	count int
	intervalText, countText string
	stopCh chan struct{}
	activeField int
	status, hotkeyStatus string
	totalClicks uint64
	lastClickText string
}

var state UIState
var registeredHotkeys = map[int]bool{}
var (
	user32 = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32 = syscall.NewLazyDLL("gdi32.dll")
	RegisterClassExW = user32.NewProc("RegisterClassExW")
	CreateWindowExW = user32.NewProc("CreateWindowExW")
	DefWindowProcW = user32.NewProc("DefWindowProcW")
	GetMessageW = user32.NewProc("GetMessageW")
	TranslateMessage = user32.NewProc("TranslateMessage")
	DispatchMessageW = user32.NewProc("DispatchMessageW")
	PostQuitMessage = user32.NewProc("PostQuitMessage")
	DestroyWindow = user32.NewProc("DestroyWindow")
	ShowWindow = user32.NewProc("ShowWindow")
	UpdateWindow = user32.NewProc("UpdateWindow")
	InvalidateRect = user32.NewProc("InvalidateRect")
	BeginPaint = user32.NewProc("BeginPaint")
	EndPaint = user32.NewProc("EndPaint")
	GetClientRect = user32.NewProc("GetClientRect")
	GetCursorPos = user32.NewProc("GetCursorPos")
	SetCursorPos = user32.NewProc("SetCursorPos")
	SendInput = user32.NewProc("SendInput")
	GetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	RegisterHotKey = user32.NewProc("RegisterHotKey")
	UnregisterHotKey = user32.NewProc("UnregisterHotKey")
	PostMessageW = user32.NewProc("PostMessageW")
	GetSystemMetrics = user32.NewProc("GetSystemMetrics")
	SetTimer = user32.NewProc("SetTimer")
	KillTimer = user32.NewProc("KillTimer")
	MessageBoxW = user32.NewProc("MessageBoxW")
	CreateFontW = gdi32.NewProc("CreateFontW")
	DeleteObject = gdi32.NewProc("DeleteObject")
	SelectObject = gdi32.NewProc("SelectObject")
	SetBkMode = gdi32.NewProc("SetBkMode")
	SetTextColor = gdi32.NewProc("SetTextColor")
	CreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	FillRect = user32.NewProc("FillRect")
	DrawTextW = user32.NewProc("DrawTextW")
	GetSysColorBrush = user32.NewProc("GetSysColorBrush")
	SetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	SetFocus = user32.NewProc("SetFocus")
)

func u16(s string) *uint16 { v := utf16.Encode([]rune(s + "\x00")); return &v[0] }
func hiword(v uintptr) uint16 { return uint16((v >> 16) & 0xffff) }
func loword(v uintptr) uint16 { return uint16(v & 0xffff) }
func hinst() uintptr { r, _, _ := GetModuleHandleW.Call(0); return r }
func def(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr { r, _, _ := DefWindowProcW.Call(hwnd, uintptr(msg), wp, lp); return r }
func rgb(r, g, b byte) uintptr { return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16) }
func font(size int32, weight uint32) uintptr {
	h, _, _ := CreateFontW.Call(uintptr(uint32(-size)), 0, 0, 0, uintptr(weight), 0, 0, 0, 134, 0, 0, 5, 0, uintptr(unsafe.Pointer(u16("Microsoft YaHei UI"))))
	return h
}
func drawText(hdc uintptr, rect RECT, text string, align uint32, color uintptr, bold bool) {
	f := state.font
	if bold { f = font(18, 700) }
	old, _, _ := SelectObject.Call(hdc, f)
	SetBkMode.Call(hdc, TRANSPARENT)
	SetTextColor.Call(hdc, color)
	r := rect
	utf := utf16.Encode([]rune(text))
	if len(utf) == 0 {
		empty := uint16(0)
		DrawTextW.Call(hdc, uintptr(unsafe.Pointer(&empty)), 0, uintptr(unsafe.Pointer(&r)), uintptr(align|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX))
	} else {
		DrawTextW.Call(hdc, uintptr(unsafe.Pointer(&utf[0])), uintptr(len(utf)), uintptr(unsafe.Pointer(&r)), uintptr(align|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX))
	}
	SelectObject.Call(hdc, old)
	if bold { DeleteObject.Call(f) }
}
func fill(hdc uintptr, r RECT, color uintptr) {
	b, _, _ := CreateSolidBrush.Call(color)
	FillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), b)
	DeleteObject.Call(b)
}
func border(hdc uintptr, r RECT, color uintptr) {
	fill(hdc, RECT{r.Left, r.Top, r.Right, r.Top+1}, color)
	fill(hdc, RECT{r.Left, r.Bottom-1, r.Right, r.Bottom}, color)
	fill(hdc, RECT{r.Left, r.Top, r.Left+1, r.Bottom}, color)
	fill(hdc, RECT{r.Right-1, r.Top, r.Right, r.Bottom}, color)
}
func button(hdc uintptr, r RECT, label string, enabled bool) {
	bg := rgb(245,245,245); if !enabled { bg = rgb(225,225,225) }
	fill(hdc,r,bg); border(hdc,r,rgb(150,150,150))
	drawText(hdc,r,label,DT_CENTER,rgb(20,20,20),false)
}
func textField(hdc uintptr, r RECT, value string, active bool) {
	fill(hdc,r,rgb(255,255,255))
	c := rgb(150,150,150); if active { c = rgb(30,90,180) }
	border(hdc,r,c)
	drawText(hdc,RECT{r.Left+10,r.Top,r.Right-10,r.Bottom},value,DT_LEFT,rgb(20,20,20),false)
}
func layoutClick(r RECT, x,y int32) bool { return x>=r.Left && x<=r.Right && y>=r.Top && y<=r.Bottom }
func screenPoint() (int32,int32) { var p POINT; GetCursorPos.Call(uintptr(unsafe.Pointer(&p))); return p.X,p.Y }

func registerHotkey(hwnd uintptr,id int,vk uintptr) bool {
	r,_,_ := RegisterHotKey.Call(hwnd,uintptr(id),MOD_NOREPEAT,vk)
	if r==0 { r,_,_ = RegisterHotKey.Call(hwnd,uintptr(id),0,vk) }
	if r!=0 { registeredHotkeys[id]=true; return true }; return false
}
func registerHotkeyMods(hwnd uintptr,id int,modifiers,vk uintptr) bool {
	r,_,_ := RegisterHotKey.Call(hwnd,uintptr(id),modifiers,vk)
	if r==0 && modifiers&uintptr(MOD_NOREPEAT)!=0 { r,_,_ = RegisterHotKey.Call(hwnd,uintptr(id),modifiers&^uintptr(MOD_NOREPEAT),vk) }
	if r!=0 { registeredHotkeys[id]=true; return true }; return false
}
func yesNo(v bool) string { if v { return "正常" }; return "失败" }
func lockPoint() {
	x,y := screenPoint(); state.lockedX=x; state.lockedY=y; state.locked=true
	state.status=fmt.Sprintf("已锁定坐标：X = %d    Y = %d",x,y)
	InvalidateRect.Call(state.main,0,1)
}
func parseSettings() bool {
	ms,err := strconv.ParseUint(strings.TrimSpace(state.intervalText),10,32)
	if err!=nil || ms<1 || ms>86400000 { state.status="参数错误：点击间隔必须是 1 ～ 86400000 毫秒"; return false }
	cnt,err := strconv.Atoi(strings.TrimSpace(state.countText))
	if err!=nil || cnt<1 || cnt>1000 { state.status="参数错误：每次点击次数必须是 1 ～ 1000"; return false }
	state.intervalMs=uint32(ms); state.count=cnt; return true
}
func start() {
	if state.running { return }
	if !state.locked { state.status="请先按 F8 锁定目标坐标"; InvalidateRect.Call(state.main,0,1); return }
	if !parseSettings() { InvalidateRect.Call(state.main,0,1); return }
	state.running=true; stopCh:=make(chan struct{}); state.stopCh=stopCh
	x,y:=state.lockedX,state.lockedY; count,right,interval:=state.count,state.right,state.intervalMs
	go clickLoop(stopCh,x,y,count,right,interval,state.main)
	state.status="● 自动点击中"; InvalidateRect.Call(state.main,0,1)
}
func stop() {
	if state.running && state.stopCh!=nil { close(state.stopCh); state.stopCh=nil }
	state.running=false; state.status="● 已停止"; InvalidateRect.Call(state.main,0,1)
}
func clickLoop(stopCh <-chan struct{},x,y int32,count int,right bool,intervalMs uint32,hwnd uintptr) {
	ticker:=time.NewTicker(time.Duration(intervalMs)*time.Millisecond); defer ticker.Stop()
	for {
		select {
		case <-stopCh: return
		case <-ticker.C:
			errCode,completed:=clickAt(stopCh,x,y,count,right)
			if completed>0 { PostMessageW.Call(hwnd,WM_APP_CLICK_OK,uintptr(completed),0) }
			if errCode!=0 { PostMessageW.Call(hwnd,WM_APP_CLICK_FAIL,uintptr(errCode),0); return }
		}
	}
}
func absolutePoint(x,y int32) (int32,int32) {
	vx,_,_:=GetSystemMetrics.Call(SM_XVIRTUALSCREEN); vy,_,_:=GetSystemMetrics.Call(SM_YVIRTUALSCREEN)
	vw,_,_:=GetSystemMetrics.Call(SM_CXVIRTUALSCREEN); vh,_,_:=GetSystemMetrics.Call(SM_CYVIRTUALSCREEN)
	return normalizeAxis(x,int32(vx),int32(vw)),normalizeAxis(y,int32(vy),int32(vh))
}
func normalizeAxis(value,origin,extent int32) int32 {
	if extent<=1 { return 0 }
	n:=(int64(value-origin)*65535)/int64(extent-1)
	if n<0 { n=0 }; if n>65535 { n=65535 }; return int32(n)
}
func clickAt(stopCh <-chan struct{},x,y int32,count int,right bool) (int,int) {
	down,up:=uint32(MOUSEEVENTF_LEFTDOWN),uint32(MOUSEEVENTF_LEFTUP)
	if right { down,up=uint32(MOUSEEVENTF_RIGHTDOWN),uint32(MOUSEEVENTF_RIGHTUP) }
	tx,ty:=absolutePoint(x,y)
	for i:=0;i<count;i++ {
		select { case <-stopCh: return 0,i; default: }
		var old POINT
		if r,_,_:=GetCursorPos.Call(uintptr(unsafe.Pointer(&old))); r==0 { return 1,i }
		inputs:=[3]INPUT{
			{Type:0,Mi:MOUSEINPUT{Dx:tx,Dy:ty,DwFlags:uint32(MOUSEEVENTF_MOVE|MOUSEEVENTF_ABSOLUTE|MOUSEEVENTF_VIRTUALDESK)}},
			{Type:0,Mi:MOUSEINPUT{DwFlags:down}},
			{Type:0,Mi:MOUSEINPUT{DwFlags:up}},
		}
		inserted,_,_:=SendInput.Call(uintptr(len(inputs)),uintptr(unsafe.Pointer(&inputs[0])),unsafe.Sizeof(INPUT{}))
		if inserted!=uintptr(len(inputs)) {
			upInput:=INPUT{Type:0,Mi:MOUSEINPUT{DwFlags:up}}
			SendInput.Call(1,uintptr(unsafe.Pointer(&upInput)),unsafe.Sizeof(INPUT{}))
			SetCursorPos.Call(uintptr(int32(old.X)),uintptr(int32(old.Y)))
			return 2,i
		}
		time.Sleep(30*time.Millisecond)
		if r,_,_:=SetCursorPos.Call(uintptr(int32(old.X)),uintptr(int32(old.Y))); r==0 { return 3,i+1 }
		if i+1<count { time.Sleep(20*time.Millisecond) }
	}
	return 0,count
}

func onPaint(hwnd uintptr) {
	var ps PAINTSTRUCT
	hdc,_,_:=BeginPaint.Call(hwnd,uintptr(unsafe.Pointer(&ps)))
	var rc RECT; GetClientRect.Call(hwnd,uintptr(unsafe.Pointer(&rc)))
	fill(hdc,rc,rgb(248,249,251))
	header:=RECT{24,18,rc.Right-24,70}; fill(hdc,header,rgb(229,236,247))
	drawText(hdc,header,"定时鼠标点击器",DT_LEFT,rgb(20,35,55),true)
	drawText(hdc,RECT{26,72,rc.Right-26,104},"不移动窗口也能设置；F8 锁定，F9 开始，F10 停止",DT_LEFT,rgb(90,90,90),false)
	drawText(hdc,RECT{28,118,220,150},"当前鼠标位置",DT_LEFT,rgb(30,30,30),false)
	drawText(hdc,RECT{28,148,rc.Right/2-10,188},fmt.Sprintf("X = %d",state.cursorX),DT_LEFT,rgb(25,25,25),false)
	drawText(hdc,RECT{rc.Right/2+10,148,rc.Right-28,188},fmt.Sprintf("Y = %d",state.cursorY),DT_LEFT,rgb(25,25,25),false)
	locked:=RECT{28,202,rc.Right-28,244}
	lc:=rgb(238,238,238); if state.locked { lc=rgb(231,244,235) }; fill(hdc,locked,lc)
	lt:="已锁定坐标：尚未锁定"; lcolor:=rgb(100,100,100)
	if state.locked { lt=fmt.Sprintf("已锁定坐标：  X = %d    Y = %d",state.lockedX,state.lockedY); lcolor=rgb(25,110,55) }
	drawText(hdc,locked,lt,DT_LEFT,lcolor,false)
	button(hdc,RECT{28,258,rc.Right-28,310},"F8  锁定当前鼠标位置",true)
	drawText(hdc,RECT{28,336,230,370},"点击间隔（毫秒）",DT_LEFT,rgb(30,30,30),false)
	textField(hdc,RECT{240,326,rc.Right-28,372},state.intervalText,state.activeField==1)
	drawText(hdc,RECT{28,396,230,430},"每次点击次数",DT_LEFT,rgb(30,30,30),false)
	textField(hdc,RECT{240,386,rc.Right-28,432},state.countText,state.activeField==2)
	drawText(hdc,RECT{28,456,230,490},"鼠标按键",DT_LEFT,rgb(30,30,30),false)
	rightText:="左键（点击此处切换）"; if state.right { rightText="右键" }
	textField(hdc,RECT{240,446,rc.Right-28,492},rightText,false)
	button(hdc,RECT{28,520,(rc.Right-44)/2,575},"开始（F9）",true)
	button(hdc,RECT{(rc.Right+16)/2,520,rc.Right-28,575},"停止（F10）",true)
	status:=RECT{28,592,rc.Right-28,624}; sc:=rgb(238,238,238); if state.running { sc=rgb(231,244,235) }; fill(hdc,status,sc)
	scolor:=rgb(40,40,40); if state.running { scolor=rgb(25,110,55) }
	drawText(hdc,status,"状态："+state.status+"    "+state.hotkeyStatus,DT_LEFT,scolor,false)
	drawText(hdc,RECT{28,630,rc.Right-28,660},fmt.Sprintf("累计点击：%d 次    最近一次点击：%s",state.totalClicks,state.lastClickText),DT_LEFT,rgb(45,45,45),false)
	EndPaint.Call(hwnd,uintptr(unsafe.Pointer(&ps)))
}
func proc(hwnd uintptr,msg uint32,wp,lp uintptr) uintptr {
	switch msg {
	case WM_PAINT: onPaint(hwnd); return 0
	case WM_TIMER:
		if wp==TIMER_REFRESH { state.cursorX,state.cursorY=screenPoint(); InvalidateRect.Call(hwnd,0,0); return 0 }
	case WM_APP_CLICK_OK:
		state.totalClicks+=uint64(wp); state.lastClickText=time.Now().Format("15:04:05"); InvalidateRect.Call(hwnd,0,0); return 0
	case WM_APP_CLICK_FAIL:
		state.running=false
		if state.stopCh!=nil { close(state.stopCh); state.stopCh=nil }
		switch wp {
		case 1: state.status="点击失败：无法读取鼠标位置"
		case 2: state.status="点击失败：Windows 未接受完整输入；请检查权限/目标程序"
		case 3: state.status="点击已发送，但无法恢复鼠标位置"
		default: state.status="点击失败：未知输入错误"
		}
		InvalidateRect.Call(hwnd,0,1); return 0
	case WM_HOTKEY:
		switch wp {
		case HOTKEY_LOCK,HOTKEY_LOCK_BACKUP: lockPoint()
		case HOTKEY_START,HOTKEY_START_BACKUP: start()
		case HOTKEY_STOP,HOTKEY_STOP_BACKUP: stop()
		}; return 0
	case WM_KEYDOWN,WM_SYSKEYDOWN:
		switch wp {
		case VK_F8: lockPoint(); return 0
		case VK_F9: start(); return 0
		case VK_F10: stop(); return 0
		case VK_BACK:
			if state.activeField==1 && len(state.intervalText)>0 { state.intervalText=state.intervalText[:len(state.intervalText)-1] }
			if state.activeField==2 && len(state.countText)>0 { state.countText=state.countText[:len(state.countText)-1] }
			InvalidateRect.Call(hwnd,0,1); return 0
		case VK_RETURN: state.activeField=0; InvalidateRect.Call(hwnd,0,1); return 0
		}
	case WM_CHAR:
		if wp>='0' && wp<='9' {
			if state.activeField==1 && len(state.intervalText)<8 { state.intervalText+=string(rune(wp)) }
			if state.activeField==2 && len(state.countText)<4 { state.countText+=string(rune(wp)) }
			InvalidateRect.Call(hwnd,0,1); return 0
		}
	case WM_LBUTTONDOWN:
		SetFocus.Call(hwnd)
		x:=int32(int16(loword(lp))); y:=int32(int16(hiword(lp)))
		var rc RECT; GetClientRect.Call(hwnd,uintptr(unsafe.Pointer(&rc)))
		if layoutClick(RECT{28,258,rc.Right-28,310},x,y) { lockPoint(); return 0 }
		if layoutClick(RECT{28,326,rc.Right-28,372},x,y) { state.activeField=1; InvalidateRect.Call(hwnd,0,1); return 0 }
		if layoutClick(RECT{28,386,rc.Right-28,432},x,y) { state.activeField=2; InvalidateRect.Call(hwnd,0,1); return 0 }
		if layoutClick(RECT{240,446,rc.Right-28,492},x,y) { state.right=!state.right; InvalidateRect.Call(hwnd,0,1); return 0 }
		if layoutClick(RECT{28,520,(rc.Right-44)/2,575},x,y) { start(); return 0 }
		if layoutClick(RECT{(rc.Right+16)/2,520,rc.Right-28,575},x,y) { stop(); return 0 }
		state.activeField=0; InvalidateRect.Call(hwnd,0,1); return 0
	case WM_CLOSE: stop(); DestroyWindow.Call(hwnd); return 0
	case WM_DESTROY:
		KillTimer.Call(hwnd,TIMER_REFRESH)
		for id,ok:=range registeredHotkeys { if ok { UnregisterHotKey.Call(hwnd,uintptr(id)) } }
		if state.font!=0 { DeleteObject.Call(state.font); state.font=0 }
		PostQuitMessage.Call(0); return 0
	}
	return def(hwnd,msg,wp,lp)
}
func main() {
	runtime.LockOSThread()
	defer func() { if r:=recover(); r!=nil { MessageBoxW.Call(0,uintptr(unsafe.Pointer(u16(fmt.Sprintf("程序发生异常：%v",r)))),uintptr(unsafe.Pointer(u16("定时鼠标点击器"))),0x10) } }()
	SetProcessDPIAware.Call()
	state=UIState{intervalText:"1000",countText:"1",status:"● 已停止",lastClickText:"暂无"}
	state.font=font(18,400)
	cls:=u16("AutoClickerGuiV20")
	sysBrush,_,_:=GetSysColorBrush.Call(COLOR_WINDOW)
	wc:=WNDCLASSEXW{CbSize:uint32(unsafe.Sizeof(WNDCLASSEXW{})),LpfnWndProc:syscall.NewCallback(proc),HInstance:hinst(),HbrBackground:sysBrush,HCursor:0,LpszClassName:uintptr(unsafe.Pointer(cls))}
	if r,_,_:=RegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r==0 {
		MessageBoxW.Call(0,uintptr(unsafe.Pointer(u16("窗口类注册失败。"))),uintptr(unsafe.Pointer(u16("定时鼠标点击器"))),0x10); return
	}
	hwnd,_,_:=CreateWindowExW.Call(0,uintptr(unsafe.Pointer(cls)),uintptr(unsafe.Pointer(u16("定时鼠标点击器"))),WS_OVERLAPPEDWINDOW,CW_USEDEFAULT,CW_USEDEFAULT,820,740,0,0,wc.HInstance,0)
	if hwnd==0 { MessageBoxW.Call(0,uintptr(unsafe.Pointer(u16("创建窗口失败。"))),uintptr(unsafe.Pointer(u16("定时鼠标点击器"))),0x10); return }
	state.main=hwnd
	lockOK:=registerHotkey(hwnd,HOTKEY_LOCK,VK_F8)
	startOK:=registerHotkey(hwnd,HOTKEY_START,VK_F9)
	stopOK:=registerHotkey(hwnd,HOTKEY_STOP,VK_F10)
	backupMods:=uintptr(MOD_CONTROL|MOD_ALT|MOD_NOREPEAT)
	lockBackupOK:=registerHotkeyMods(hwnd,HOTKEY_LOCK_BACKUP,backupMods,VK_F8)
	startBackupOK:=registerHotkeyMods(hwnd,HOTKEY_START_BACKUP,backupMods,VK_F9)
	stopBackupOK:=registerHotkeyMods(hwnd,HOTKEY_STOP_BACKUP,backupMods,VK_F10)
	backupOK:=lockBackupOK&&startBackupOK&&stopBackupOK
	state.hotkeyStatus=fmt.Sprintf("热键：F8 %s  F9 %s  F10 %s；备用 Ctrl+Alt 组合 %s",yesNo(lockOK),yesNo(startOK),yesNo(stopOK),map[bool]string{true:"可用",false:"部分失败"}[backupOK])
	ShowWindow.Call(hwnd,SW_SHOW); UpdateWindow.Call(hwnd); SetTimer.Call(hwnd,TIMER_REFRESH,250,0)
	var msg MSG
	for {
		r,_,_:=GetMessageW.Call(uintptr(unsafe.Pointer(&msg)),0,0,0)
		if int32(r)==-1 { MessageBoxW.Call(hwnd,uintptr(unsafe.Pointer(u16("Windows 消息循环失败。"))),uintptr(unsafe.Pointer(u16("定时鼠标点击器"))),0x10); break }
		if r==0 { break }
		TranslateMessage.Call(uintptr(unsafe.Pointer(&msg))); DispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
