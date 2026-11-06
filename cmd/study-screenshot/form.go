package main

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"study.local/screenshot/internal/platform"
)

type formResult struct {
	Group, S1, S2, Work, Mode string
	OK                        bool
}

// modeCode maps radio labels to capture mode codes.
func modeCode(label string) string {
	switch label {
	case "Область":
		return "area"
	case "Активное окно (Windows)":
		return "window"
	default:
		return "screen"
	}
}

// formFilled reports whether the form has the required fields.
func formFilled(group, s1 string) bool {
	return group != "" && s1 != ""
}

// openForm shows the work-data dialog on the Fyne UI thread and returns a
// channel that receives exactly one result when the user confirms or cancels.
// It must NOT block the UI thread: callers read the channel from another goroutine.
func openForm(a fyne.App, cfg platform.Config) <-chan formResult {
	done := make(chan formResult, 1)
	var once sync.Once
	finish := func(ok bool, group, s1, s2, work, modeLabel string) {
		once.Do(func() {
			done <- formResult{
				Group: group, S1: s1, S2: s2, Work: work,
				Mode: modeCode(modeLabel), OK: ok,
			}
		})
	}

	w := a.NewWindow("Скриншот — данные работы")
	w.Resize(fyne.NewSize(420, 320))
	w.CenterOnScreen()
	w.SetFixedSize(true)

	group := widget.NewEntry()
	group.SetText(cfg.Group)
	group.SetPlaceHolder("например 3ИСП-11-41")
	s1 := widget.NewEntry()
	s1.SetText(cfg.Students[0])
	s1.SetPlaceHolder("Фамилия 1")
	s2 := widget.NewEntry()
	s2.SetText(cfg.Students[1])
	s2.SetPlaceHolder("Фамилия 2 (необязательно)")
	work := widget.NewEntry()
	work.SetText(cfg.Work)
	work.SetPlaceHolder("Практическая работа …")

	mode := widget.NewRadioGroup([]string{
		"Весь экран",
		"Область",
		"Активное окно (Windows)",
	}, nil)
	switch cfg.Mode {
	case "area":
		mode.SetSelected("Область")
	case "window":
		mode.SetSelected("Активное окно (Windows)")
	default:
		mode.SetSelected("Весь экран")
	}

	form := widget.NewForm(
		widget.NewFormItem("Группа", group),
		widget.NewFormItem("Фамилия 1", s1),
		widget.NewFormItem("Фамилия 2", s2),
		widget.NewFormItem("Работа", work),
		widget.NewFormItem("Режим", mode),
	)
	btns := container.NewHBox(
		widget.NewButton("Отмена", func() {
			finish(false, group.Text, s1.Text, s2.Text, work.Text, mode.Selected)
			w.Close()
		}),
		widget.NewButton("Снять", func() {
			if !formFilled(group.Text, s1.Text) {
				dialog.ShowInformation("Нужны данные", "Укажите группу и хотя бы одну фамилию.", w)
				return
			}
			finish(true, group.Text, s1.Text, s2.Text, work.Text, mode.Selected)
			w.Close()
		}),
	)
	w.SetContent(container.NewVBox(form, btns))
	w.SetCloseIntercept(func() {
		finish(false, group.Text, s1.Text, s2.Text, work.Text, mode.Selected)
		w.Close()
	})
	w.Show()
	return done
}
