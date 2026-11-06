package main

import (
	"sync/atomic"

	"fyne.io/systray"
)

var trayRunning atomic.Bool

func systrayQuitSafe() {
	if trayRunning.Load() {
		systray.Quit()
	}
}
