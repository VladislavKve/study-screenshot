package capture

import (
	"fmt"
	"image"

	"github.com/kbinani/screenshot"
)

// FullScreen снимает все мониторы в один виртуальный прямоугольник.
func FullScreen() (*image.RGBA, error) {
	n := screenshot.NumActiveDisplays()
	if n <= 0 {
		return nil, fmt.Errorf("нет активных дисплеев")
	}
	var bounds image.Rectangle
	for i := 0; i < n; i++ {
		bounds = bounds.Union(screenshot.GetDisplayBounds(i))
	}
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, err
	}
	return img, nil
}

// Rect снимает произвольную область экрана.
func Rect(r image.Rectangle) (*image.RGBA, error) {
	if r.Empty() {
		return nil, fmt.Errorf("пустая область")
	}
	return screenshot.CaptureRect(r)
}

// Crop вырезает область из уже снятого кадра.
func Crop(src *image.RGBA, r image.Rectangle) *image.RGBA {
	r = r.Intersect(src.Bounds())
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			out.Set(x, y, src.At(r.Min.X+x, r.Min.Y+y))
		}
	}
	return out
}
