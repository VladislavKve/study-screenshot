//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func isWaylandSession() bool {
	if strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland") {
		return true
	}
	return os.Getenv("WAYLAND_DISPLAY") != ""
}

// statusNotifierAvailable — есть ли StatusNotifierWatcher (трей на GNOME часто отсутствует).
func statusNotifierAvailable() bool {
	if _, err := exec.LookPath("gdbus"); err != nil {
		// без gdbus считаем, что трей «может быть» — попробуем systray
		return true
	}
	out, err := exec.Command("gdbus", "call", "--session",
		"--dest", "org.freedesktop.DBus",
		"--object-path", "/org/freedesktop/DBus",
		"--method", "org.freedesktop.DBus.NameHasOwner",
		"org.kde.StatusNotifierWatcher",
	).CombinedOutput()
	if err == nil && strings.Contains(string(out), "true") {
		return true
	}
	out, err = exec.Command("gdbus", "call", "--session",
		"--dest", "org.freedesktop.DBus",
		"--object-path", "/org/freedesktop/DBus",
		"--method", "org.freedesktop.DBus.NameHasOwner",
		"org.freedesktop.StatusNotifierWatcher",
	).CombinedOutput()
	return err == nil && strings.Contains(string(out), "true")
}

// tryPortalGlobalShortcuts — лучший effort через gdbus; при неудаче возвращает ошибку.
// Полноценная сессия GlobalShortcuts требует Keep-Alive Request — здесь только probe + лог инструкции.
func tryPortalGlobalShortcuts(logf func(string, ...any), onActivate func()) error {
	if !isWaylandSession() {
		return fmt.Errorf("не wayland")
	}
	if _, err := exec.LookPath("gdbus"); err != nil {
		return fmt.Errorf("нет gdbus")
	}
	// Проверяем, есть ли интерфейс GlobalShortcuts у портала
	out, err := exec.Command("gdbus", "introspect", "--session",
		"--dest", "org.freedesktop.portal.Desktop",
		"--object-path", "/org/freedesktop/portal/desktop",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("portal introspect: %w", err)
	}
	if !strings.Contains(string(out), "GlobalShortcuts") {
		return fmt.Errorf("портал без GlobalShortcuts (нужен xdg-desktop-portal ≥ 1.18 + реализация)")
	}
	logf("wayland: GlobalShortcuts есть в портале, но постоянная привязка Ctrl+Shift+S требует desktop-файла и разрешения пользователя")
	logf("wayland: fallback — кнопка «Снимок» в панели; либо ярлык GNOME: настройки → клавиатура → своя комбинация → команда «study-screenshot -once»")
	_ = onActivate
	_ = time.Second
	return fmt.Errorf("автопривязка GlobalShortcuts не настроена (используйте панель или ярлык GNOME)")
}
