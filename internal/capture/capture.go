package capture

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kbinani/screenshot"
)

// FullScreen снимает экран: сначала kbinani (X11/Win/mac), затем портал/gnome-screenshot на Wayland.
func FullScreen() (*image.RGBA, error) {
	img, err := captureKbinani()
	if err == nil && img != nil {
		return img, nil
	}
	kbinaniErr := err

	if runtime.GOOS == "linux" {
		if img, err2 := capturePortalOrGnome(); err2 == nil {
			return img, nil
		} else if kbinaniErr == nil {
			kbinaniErr = err2
		} else {
			kbinaniErr = fmt.Errorf("%v; fallback: %w", kbinaniErr, err2)
		}
	}
	if kbinaniErr == nil {
		kbinaniErr = fmt.Errorf("захват экрана не удался")
	}
	return nil, kbinaniErr
}

func captureKbinani() (*image.RGBA, error) {
	n := screenshot.NumActiveDisplays()
	if n <= 0 {
		return nil, fmt.Errorf("нет активных дисплеев (kbinani)")
	}
	var bounds image.Rectangle
	for i := 0; i < n; i++ {
		bounds = bounds.Union(screenshot.GetDisplayBounds(i))
	}
	return screenshot.CaptureRect(bounds)
}

// Rect — область через kbinani (на Wayland область лучше выделять из полного кадра).
func Rect(r image.Rectangle) (*image.RGBA, error) {
	if r.Empty() {
		return nil, fmt.Errorf("пустая область")
	}
	img, err := screenshot.CaptureRect(r)
	if err == nil {
		return img, nil
	}
	full, err2 := FullScreen()
	if err2 != nil {
		return nil, err
	}
	return Crop(full, r), nil
}

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

func isWayland() bool {
	if strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland") {
		return true
	}
	return os.Getenv("WAYLAND_DISPLAY") != ""
}

func capturePortalOrGnome() (*image.RGBA, error) {
	dir, err := os.MkdirTemp("", "study-shot-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "shot.png")

	// 1) gnome-screenshot (часто уже разрешён в GNOME)
	if p, err := exec.LookPath("gnome-screenshot"); err == nil {
		cmd := exec.Command(p, "-f", out)
		if err := cmd.Run(); err == nil {
			if img, err2 := loadPNG(out); err2 == nil {
				return img, nil
			}
		}
	}
	// 2) spectacle / scrot / import — запасные
	for _, cand := range [][]string{
		{"spectacle", "-b", "-n", "-o", out},
		{"grim", out},
	} {
		if _, err := exec.LookPath(cand[0]); err != nil {
			continue
		}
		if err := exec.Command(cand[0], cand[1:]...).Run(); err == nil {
			if img, err2 := loadPNG(out); err2 == nil {
				return img, nil
			}
		}
	}
	// 3) xdg-desktop-portal Screenshot через gdbus (интерактивный диалог)
	if img, err := captureViaPortal(out); err == nil {
		return img, nil
	}
	return nil, fmt.Errorf("wayland: ни gnome-screenshot, ни portal Screenshot не сработали (сессия wayland=%v)", isWayland())
}

func loadPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := im.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			rgba.Set(x, y, im.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return rgba, nil
}

// captureViaPortal — org.freedesktop.portal.Screenshot; ждём файл до 30с.
func captureViaPortal(outPath string) (*image.RGBA, error) {
	if _, err := exec.LookPath("gdbus"); err != nil {
		return nil, fmt.Errorf("нет gdbus")
	}
	// Non-interactive attempt: interactive=false may be denied; still try.
	cmd := exec.Command("gdbus", "call", "--session",
		"--dest", "org.freedesktop.portal.Desktop",
		"--object-path", "/org/freedesktop/portal/desktop",
		"--method", "org.freedesktop.portal.Screenshot.Screenshot",
		"", "{'interactive': <false>, 'handle_token': <'studyshot'>}",
	)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("portal Screenshot: %v %s", err, raw)
	}
	s := string(raw)
	// Response is async via Request; some portals write uri in signal.
	// Fallback: poll common screenshot dirs / ask gnome again.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if img, err := loadPNG(outPath); err == nil {
			return img, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = s
	return nil, fmt.Errorf("portal Screenshot: нет файла (%s)", strings.TrimSpace(s))
}
