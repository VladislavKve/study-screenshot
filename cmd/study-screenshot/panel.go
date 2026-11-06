package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// showControlPanel — маленькое окно с большой кнопкой «Снимок» (вместо/вместе с треем).
func (st *AppState) showControlPanel(hint string) {
	fyne.Do(func() {
		if st.panel != nil {
			st.panel.Show()
			st.panel.RequestFocus()
			return
		}
		w := st.fyne.NewWindow("Скриншот Study")
		w.SetFixedSize(true)
		w.Resize(fyne.NewSize(280, 160))
		w.CenterOnScreen()
		w.SetCloseIntercept(func() {
			// Скрываем, не выходим — чтобы не потерять процесс без трея
			w.Hide()
		})
		btn := widget.NewButton("Снимок", func() { st.shoot() })
		btn.Importance = widget.HighImportance
		about := widget.NewButton("О программе", func() { st.showAbout() })
		quit := widget.NewButton("Выход", func() {
			systrayQuitSafe()
			st.fyne.Quit()
		})
		msg := hint
		if msg == "" {
			msg = "Ctrl+Shift+S или кнопка ниже"
		}
		w.SetContent(container.NewVBox(
			widget.NewLabel(msg),
			btn,
			container.NewHBox(about, quit),
		))
		st.panel = w
		w.Show()
	})
}
