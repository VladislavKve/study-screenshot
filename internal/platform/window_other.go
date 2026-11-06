//go:build !windows

package platform

// ActiveWindow на Linux/macOS без доп. разрешений — заглушка (источник «не определён»).
func ActiveWindow() (title string, rect [4]int, ok bool) {
	return "", [4]int{}, false
}
