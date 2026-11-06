package main

import (
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

// showForm блокирует до закрытия диалога (вызывать из UI-потока Fyne).
func showForm(a fyne.App, cfg platform.Config) formResult {
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

	done := make(chan formResult, 1)
	finish := func(ok bool) {
		m := "screen"
		switch mode.Selected {
		case "Область":
			m = "area"
		case "Активное окно (Windows)":
			m = "window"
		}
		done <- formResult{Group: group.Text, S1: s1.Text, S2: s2.Text, Work: work.Text, Mode: m, OK: ok}
		w.Close()
	}

	form := widget.NewForm(
		widget.NewFormItem("Группа", group),
		widget.NewFormItem("Фамилия 1", s1),
		widget.NewFormItem("Фамилия 2", s2),
		widget.NewFormItem("Работа", work),
		widget.NewFormItem("Режим", mode),
	)
	btns := container.NewHBox(
		widget.NewButton("Отмена", func() { finish(false) }),
		widget.NewButton("Снять", func() {
			if group.Text == "" || s1.Text == "" {
				dialog.ShowInformation("Нужны данные", "Укажите группу и хотя бы одну фамилию.", w)
				return
			}
			finish(true)
		}),
	)
	w.SetContent(container.NewVBox(form, btns))
	w.SetCloseIntercept(func() { finish(false) })
	w.Show()
	return <-done
}
