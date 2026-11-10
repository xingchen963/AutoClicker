# AutoClicker GUI v20

Windows 10/11 定时鼠标点击器源码。

## 功能

- F8：锁定当前鼠标坐标
- F9：开始定时点击
- F10：停止
- Ctrl+Alt+F8 / F9 / F10：备用全局热键
- 设置点击间隔（毫秒）、每轮点击次数
- 左键 / 右键切换
- 显示累计点击次数和最近点击时间
- 点击时将鼠标临时移动到锁定坐标，完成后恢复原位置

## 编译

需要 Go（Windows AMD64）：

```powershell
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -trimpath -ldflags="-s -w -H windowsgui" -o AutoClicker_GUI_v20.exe .\AutoClicker_GUI_v20.go
```

该项目使用 Go 标准库和 Windows Win32 API，不需要额外第三方 Go 依赖。

注意：源码是 GUI 自动点击器；它只模拟鼠标输入，不会改变网页课件计时或服务器学习记录。
