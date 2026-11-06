package main

import (
	"image"
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// selectRegion — оверлей поверх снимка: выделите мышью, Enter — OK, Esc — отмена.
func selectRegion(a fyne.App, shot image.Image) image.Rectangle {
	done := make(chan image.Rectangle, 1)
	w := a.NewWindow("Выделите область")
	w.SetFullScreen(true)

	bg := canvas.NewImageFromImage(shot)
	bg.FillMode = canvas.ImageFillContain
	overlay := newDragOverlay(
		func(r image.Rectangle) { done <- r; w.Close() },
		func() { done <- image.Rectangle{}; w.Close() },
	)
	w.SetContent(container.NewStack(bg, overlay))
	w.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		switch e.Name {
		case fyne.KeyEscape:
			overlay.cancel()
		case fyne.KeyReturn, fyne.KeyEnter:
			overlay.confirm()
		}
	})
	w.Show()
	return <-done
}

type dragOverlay struct {
	widget.BaseWidget
	mu                 sync.Mutex
	dragging           bool
	x0, y0, x1, y1     float32
	onOK               func(image.Rectangle)
	onCancel           func()
}

func newDragOverlay(onOK func(image.Rectangle), onCancel func()) *dragOverlay {
	d := &dragOverlay{onOK: onOK, onCancel: onCancel}
	d.ExtendBaseWidget(d)
	return d
}

func (d *dragOverlay) CreateRenderer() fyne.WidgetRenderer {
	stroke := canvas.NewRectangle(color.NRGBA{R: 255, G: 215, B: 0, A: 40})
	stroke.StrokeColor = color.NRGBA{R: 255, G: 215, B: 0, A: 255}
	stroke.StrokeWidth = 2
	hint := canvas.NewText("Выделите область мышью · Enter — готово · Esc — отмена", color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	hint.TextSize = 18
	return &dragRenderer{d: d, stroke: stroke, hint: hint, objs: []fyne.CanvasObject{stroke, hint}}
}

func (d *dragOverlay) cancel() { d.onCancel() }
func (d *dragOverlay) confirm() {
	d.mu.Lock()
	r := image.Rect(int(min32(d.x0, d.x1)), int(min32(d.y0, d.y1)), int(max32(d.x0, d.x1)), int(max32(d.y0, d.y1)))
	d.mu.Unlock()
	if r.Dx() < 8 || r.Dy() < 8 {
		d.onCancel()
		return
	}
	d.onOK(r)
}

func (d *dragOverlay) MouseDown(e *desktop.MouseEvent) {
	d.mu.Lock()
	d.dragging = true
	d.x0, d.y0, d.x1, d.y1 = e.Position.X, e.Position.Y, e.Position.X, e.Position.Y
	d.mu.Unlock()
	d.Refresh()
}
func (d *dragOverlay) MouseUp(e *desktop.MouseEvent) {
	d.mu.Lock()
	d.dragging = false
	d.x1, d.y1 = e.Position.X, e.Position.Y
	d.mu.Unlock()
	d.Refresh()
}
func (d *dragOverlay) MouseMoved(e *desktop.MouseEvent) {
	d.mu.Lock()
	act := d.dragging
	if act {
		d.x1, d.y1 = e.Position.X, e.Position.Y
	}
	d.mu.Unlock()
	if act {
		d.Refresh()
	}
}
func (d *dragOverlay) MouseIn(*desktop.MouseEvent) {}
func (d *dragOverlay) MouseOut()                   {}
func (d *dragOverlay) Cursor() desktop.Cursor      { return desktop.CrosshairCursor }

type dragRenderer struct {
	d      *dragOverlay
	stroke *canvas.Rectangle
	hint   *canvas.Text
	objs   []fyne.CanvasObject
}

func (r *dragRenderer) Layout(size fyne.Size) {
	r.hint.Move(fyne.NewPos(16, 16))
	r.hint.Resize(r.hint.MinSize())
	r.d.mu.Lock()
	x0, y0, x1, y1 := r.d.x0, r.d.y0, r.d.x1, r.d.y1
	r.d.mu.Unlock()
	x, y := min32(x0, x1), min32(y0, y1)
	r.stroke.Move(fyne.NewPos(x, y))
	r.stroke.Resize(fyne.NewSize(abs32(x1-x0), abs32(y1-y0)))
}
func (r *dragRenderer) MinSize() fyne.Size               { return fyne.NewSize(100, 100) }
func (r *dragRenderer) Objects() []fyne.CanvasObject     { return r.objs }
func (r *dragRenderer) Destroy()                         {}
func (r *dragRenderer) Refresh()                         { r.Layout(r.d.Size()); canvas.Refresh(r.stroke); canvas.Refresh(r.hint) }

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
func abs32(a float32) float32 {
	if a < 0 {
		return -a
	}
	return a
}
