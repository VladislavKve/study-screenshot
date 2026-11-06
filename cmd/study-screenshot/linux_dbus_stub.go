//go:build !linux

package main

func isWaylandSession() bool { return false }

func statusNotifierAvailable() bool { return true }

func tryPortalGlobalShortcuts(logf func(string, ...any), onActivate func()) error {
	return nil
}
