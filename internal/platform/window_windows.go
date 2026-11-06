//go:build windows

package platform

import (
	"syscall"
	"unsafe"
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

type RECT struct{ Left, Top, Right, Bottom int32 }

// ActiveWindow возвращает заголовок и прямоугольник активного окна.
func ActiveWindow() (title string, rect [4]int, ok bool) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "", [4]int{}, false
	}
	buf := make([]uint16, 512)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), 512)
	title = syscall.UTF16ToString(buf)
	var r RECT
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	rect = [4]int{int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top)}
	return title, rect, true
}
