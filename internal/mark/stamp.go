package mark

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Шрифт плашки — тот же DejaVu Sans Bold, что на учебных ПК (лицензия Bitstream Vera: встраивать можно).
//
//go:embed DejaVuSans-Bold.ttf
var fontBytes []byte

var (
	fontOnce sync.Once
	fontFace *opentype.Font
	fontErr  error
)

func loadFont() (*opentype.Font, error) {
	fontOnce.Do(func() { fontFace, fontErr = opentype.Parse(fontBytes) })
	return fontFace, fontErr
}

// Line — строка плашки.
type line struct {
	text string
	size int
	col  color.RGBA
}

// StampInput — всё, что нужно для рисования (данные уже в Mark).
type StampInput struct {
	Now    time.Time // локальное время снимка
	Signed bool      // есть ли заверение сервера
}

// Stamp рисует плашку и QR на снимке, как stamp() в Linux-версии. Возвращает новую картинку
// (при маленьком снимке — с тёмной полосой снизу) и строку QR.
func Stamp(src image.Image, m *Mark, in StampInput) (*image.RGBA, []byte, error) {
	f, err := loadFont()
	if err != nil {
		return nil, nil, err
	}
	js, err := m.JSON()
	if err != nil {
		return nil, nil, err
	}
	b := src.Bounds()
	W, H := b.Dx(), b.Dy()
	im := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(im, im.Bounds(), src, b.Min, draw.Src)

	s := math.Max(1.0, float64(W)/1600)
	sz1, sz2 := int(34*s), int(20*s)
	pad := int(10 * s)

	var names []string
	for _, x := range m.Students {
		if x != "" {
			names = append(names, x)
		}
	}
	where := fmt.Sprintf("Компьютер %d", m.PC)
	if m.PC <= 0 {
		where = "Компьютер " + m.Host
	}
	l2 := fmt.Sprintf("Группа %s · %s", m.Group, where)
	if m.Work != "" {
		l2 += " · " + m.Work
	}
	var l3 string
	if in.Signed && m.Srv != nil {
		l3 = fmt.Sprintf("%s · ID %s · srv %s · заверено сервером", in.Now.Format("02.01.2006 15:04"), m.ID, m.Srv.Sig)
	} else {
		l3 = fmt.Sprintf("%s · ID %s · НЕ ЗАВЕРЕНО (нет связи с сервером)", in.Now.Format("02.01.2006 15:04"), m.ID)
	}
	st := Ellipsis(m.Src.Title, 50)
	srcline := "Источник: не определён"
	if m.Src.App != "?" && m.Src.App != "" {
		srcline = fmt.Sprintf("Источник: %s — %s", m.Src.App, st)
	}
	srcCol := color.RGBA{180, 255, 180, 255}
	if m.Suspect {
		srcline = "⚠ " + srcline + " (ПОДОЗРИТЕЛЬНО)"
		srcCol = color.RGBA{255, 90, 90, 255}
	}
	lines := []line{
		{strings.Join(names, " · "), sz1, color.RGBA{255, 230, 0, 255}},
		{l2, sz2, color.RGBA{255, 255, 255, 255}},
		{l3, sz2, color.RGBA{220, 220, 220, 255}},
		{srcline, sz2, srcCol},
	}
	faces := map[int]font.Face{}
	for _, sz := range []int{sz1, sz2} {
		face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: float64(sz), DPI: 72, Hinting: font.HintingNone})
		if err != nil {
			return nil, nil, err
		}
		faces[sz] = face
	}

	// QR: уровень Q, рамка 4 модуля, размер модуля = max(4, ceil(max(240,150s)/(modules+8)))
	qr, err := qrcode.New(string(js), qrcode.High)
	if err != nil {
		return nil, nil, fmt.Errorf("qr: %w", err)
	}
	n := len(qr.Bitmap()) // уже с рамкой (4 модуля с каждой стороны)
	box := int(math.Max(4, math.Ceil(math.Max(240, 150*s)/float64(n))))
	qi := qr.Image(-box)
	qw, qh := qi.Bounds().Dx(), qi.Bounds().Dy()

	// размеры плашки
	tw := 0.0
	th := 0
	for _, l := range lines {
		w := float64(font.MeasureString(faces[l.size], l.text)) / 64
		if w > tw {
			tw = w
		}
		th += l.size
	}
	w := int(tw) + 2*pad
	h := th + (len(lines)+1)*pad
	needW := w + qw + 3*pad
	needH := max(h, qh+6) + 2*pad
	if W < needW || H < needH { // маленький снимок: полоса снизу, чтобы плашка и QR не обрезались
		cv := image.NewRGBA(image.Rect(0, 0, max(W, needW), H+needH))
		draw.Draw(cv, cv.Bounds(), image.NewUniform(color.RGBA{40, 40, 40, 255}), image.Point{}, draw.Src)
		draw.Draw(cv, im.Bounds(), im, image.Point{}, draw.Src)
		im = cv
		W, H = cv.Bounds().Dx(), cv.Bounds().Dy()
	}
	qx, qy := W-qw-pad, H-qh-pad
	x0, y0 := qx-w-pad, H-h-pad
	if x0 < pad {
		x0 = pad
	}

	// плашка: скруглённый полупрозрачный прямоугольник с рамкой
	outline := color.RGBA{255, 215, 0, 255} // жёлтая — как «пару записали студенты»
	if !in.Signed {
		outline = color.RGBA{255, 140, 0, 255} // оранжевая — не заверено
	}
	if m.Suspect {
		outline = color.RGBA{255, 60, 60, 255}
	}
	radius := int(10 * s)
	bw := max(3, int(3*s))
	rr := image.Rect(x0, y0, x0+w, y0+h)
	fillRounded(im, rr, radius, color.NRGBA{15, 15, 15, 185})
	strokeRounded(im, rr, radius, bw, outline)

	y := y0 + pad
	for _, l := range lines {
		face := faces[l.size]
		sw := max(2, l.size/12)
		drawText(im, face, l.text, x0+pad, y, l.col, sw)
		y += l.size + pad
	}
	// чёрная подложка и сам QR
	draw.Draw(im, image.Rect(qx-3, qy-3, qx+qw+3, qy+qh+3), image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.Draw(im, image.Rect(qx, qy, qx+qw, qy+qh), qi, qi.Bounds().Min, draw.Src)
	return im, js, nil
}

// drawText рисует строку с чёрной обводкой толщиной sw (как stroke_width в PIL), верх текста в y.
func drawText(dst *image.RGBA, face font.Face, s string, x, y int, col color.RGBA, sw int) {
	met := face.Metrics()
	adv := font.MeasureString(face, s)
	tw := adv.Ceil() + 2*sw + 4
	th := (met.Ascent + met.Descent).Ceil() + 2*sw + 4
	mask := image.NewAlpha(image.Rect(0, 0, tw, th))
	d := &font.Drawer{Dst: mask, Src: image.NewUniform(color.Alpha{255}), Face: face,
		Dot: fixed.P(sw+2, sw+2+met.Ascent.Ceil())}
	d.DrawString(s)
	ox, oy := x-sw-2, y-sw-2
	black := image.NewUniform(color.Black)
	for dx := -sw; dx <= sw; dx++ {
		for dy := -sw; dy <= sw; dy++ {
			if dx*dx+dy*dy > sw*sw {
				continue
			}
			r := image.Rect(ox+dx, oy+dy, ox+dx+tw, oy+dy+th)
			draw.DrawMask(dst, r, black, image.Point{}, mask, image.Point{}, draw.Over)
		}
	}
	draw.DrawMask(dst, image.Rect(ox, oy, ox+tw, oy+th), image.NewUniform(col), image.Point{}, mask, image.Point{}, draw.Over)
}

func roundedMask(r image.Rectangle, radius int) *image.Alpha {
	w, h := r.Dx(), r.Dy()
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	rad := float64(radius)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cx, cy := float64(x)+0.5, float64(y)+0.5
			in := true
			switch {
			case cx < rad && cy < rad:
				in = math.Hypot(cx-rad, cy-rad) <= rad
			case cx > float64(w)-rad && cy < rad:
				in = math.Hypot(cx-(float64(w)-rad), cy-rad) <= rad
			case cx < rad && cy > float64(h)-rad:
				in = math.Hypot(cx-rad, cy-(float64(h)-rad)) <= rad
			case cx > float64(w)-rad && cy > float64(h)-rad:
				in = math.Hypot(cx-(float64(w)-rad), cy-(float64(h)-rad)) <= rad
			}
			if in {
				m.SetAlpha(x, y, color.Alpha{255})
			}
		}
	}
	return m
}

func fillRounded(dst *image.RGBA, r image.Rectangle, radius int, c color.Color) {
	m := roundedMask(r, radius)
	draw.DrawMask(dst, r, image.NewUniform(c), image.Point{}, m, image.Point{}, draw.Over)
}

func strokeRounded(dst *image.RGBA, r image.Rectangle, radius, width int, c color.Color) {
	outer := roundedMask(r, radius)
	inner := roundedMask(r.Inset(width), max(0, radius-width))
	for y := 0; y < outer.Rect.Dy(); y++ {
		for x := 0; x < outer.Rect.Dx(); x++ {
			ix, iy := x-width, y-width
			if ix >= 0 && iy >= 0 && ix < inner.Rect.Dx() && iy < inner.Rect.Dy() && inner.AlphaAt(ix, iy).A > 0 {
				outer.SetAlpha(x, y, color.Alpha{0})
			}
		}
	}
	draw.DrawMask(dst, r, image.NewUniform(c), image.Point{}, outer, image.Point{}, draw.Over)
}
