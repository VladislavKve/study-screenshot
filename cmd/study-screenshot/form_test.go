package main

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"study.local/screenshot/internal/platform"
)

func TestModeCode(t *testing.T) {
	cases := map[string]string{
		"Весь экран":                 "screen",
		"Область":                    "area",
		"Активное окно (Windows)":    "window",
		"":                           "screen",
		"unknown":                    "screen",
	}
	for in, want := range cases {
		if got := modeCode(in); got != want {
			t.Fatalf("modeCode(%q)=%q want %q", in, got, want)
		}
	}
}

func TestFormFilled(t *testing.T) {
	if formFilled("", "Иванов") {
		t.Fatal("empty group should fail")
	}
	if formFilled("Г1", "") {
		t.Fatal("empty surname should fail")
	}
	if !formFilled("ТЕСТ", "Тест") {
		t.Fatal("both set should pass")
	}
}

// openForm must return a channel without waiting for the user (no UI-thread deadlock).
func TestOpenFormReturnsImmediately(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	cfg := platform.Config{Group: "ТЕСТ", Students: [2]string{"Тест", ""}, Work: "Проверка", Mode: "screen"}
	started := time.Now()
	ch := openForm(a, cfg)
	if time.Since(started) > 500*time.Millisecond {
		t.Fatalf("openForm blocked for %v — must not wait on UI/user", time.Since(started))
	}
	if ch == nil {
		t.Fatal("nil channel")
	}
	// Cancel via close-intercept path is hard in test driver; ensure channel is receive-ready shape.
	select {
	case <-ch:
		t.Fatal("form completed without user action")
	case <-time.After(50 * time.Millisecond):
		// expected: still waiting for user → channel open, UI not deadlocked
	}
}
