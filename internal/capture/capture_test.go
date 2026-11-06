package capture

import (
	"image"
	"image/color"
	"testing"
)

func TestCrop(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 80))
	src.Set(10, 10, color.RGBA{1, 2, 3, 255})
	out := Crop(src, image.Rect(5, 5, 20, 20))
	if out.Bounds().Dx() != 15 || out.Bounds().Dy() != 15 {
		t.Fatalf("size %v", out.Bounds())
	}
	r, g, b, a := out.At(5, 5).RGBA()
	if r>>8 != 1 || g>>8 != 2 || b>>8 != 3 || a>>8 != 255 {
		t.Fatalf("pixel %d %d %d %d", r, g, b, a)
	}
}
