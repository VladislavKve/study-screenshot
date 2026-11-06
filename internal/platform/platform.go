package platform

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// ScreenshotsDir — Pictures/StudyScreenshots (или «Изображения» на русской Windows через Known Folder — упрощённо Pictures).
func ScreenshotsDir() string {
	home, _ := os.UserHomeDir()
	// XDG / macOS / Windows: сначала стандартные Pictures
	candidates := []string{
		filepath.Join(home, "Pictures", "StudyScreenshots"),
		filepath.Join(home, "Изображения", "StudyScreenshots"),
		filepath.Join(home, "Pictures", "Снимки экрана"),
	}
	if runtime.GOOS == "windows" {
		if u := os.Getenv("USERPROFILE"); u != "" {
			candidates = append([]string{filepath.Join(u, "Pictures", "StudyScreenshots")}, candidates...)
		}
	}
	return candidates[0]
}

func ConfigDir() string {
	if d, err := os.UserConfigDir(); err == nil && d != "" {
		return filepath.Join(d, "StudyScreenshot")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "StudyScreenshot")
}

func HostName() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return h
}

func UserName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return os.Getenv("USERNAME")
}

func OSLabel() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

func SourceTag() string {
	switch runtime.GOOS {
	case "windows":
		return "win"
	case "darwin":
		return "mac"
	default:
		return "linux"
	}
}
