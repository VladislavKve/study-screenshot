// Скриншот со штампом и QR (Study) — Windows / Linux / macOS.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"fyne.io/systray"
	"golang.design/x/hotkey"

	"study.local/screenshot/internal/capture"
	"study.local/screenshot/internal/mark"
	"study.local/screenshot/internal/platform"
	"study.local/screenshot/internal/sign"
)

var version = "0.1.2"

const defaultSignURL = "https://77-91-72-31.sslip.io/study-sign/sign"

type AppState struct {
	fyne    fyne.App
	cfg     platform.Config
	busy    atomic.Bool
	log     *log.Logger
	signURL string
	panel   fyne.Window
	once    bool
}

func main() {
	signURLFlag := flag.String("sign-url", "", "URL службы подписи (POST /sign)")
	once := flag.Bool("once", false, "один снимок и выход (для ярлыка GNOME / теста)")
	forcePanel := flag.Bool("panel", false, "показать окно с кнопкой «Снимок» (без расчёта на трей)")
	flag.Parse()

	cfg := platform.LoadConfig()
	if *signURLFlag != "" {
		cfg.SignURL = *signURLFlag
	}
	url := cfg.SignURL
	if url == "" {
		url = defaultSignURL
	}

	fa := app.NewWithID("study.local.screenshot")
	st := &AppState{fyne: fa, cfg: cfg, log: openLog(), signURL: url, once: *once}
	st.log.Printf("старт %s %s sign=%s once=%v wayland=%v", version, runtime.GOOS, url, *once, isWaylandSession())

	if *once {
		// Важно: не вызывать Quit до завершения shootBlocking — иначе fyne.Do зависает.
		go func() {
			time.Sleep(150 * time.Millisecond)
			st.shootBlocking()
			fyne.Do(func() { fa.Quit() })
		}()
		fa.Run()
		return
	}

	needPanel := *forcePanel
	if runtime.GOOS == "linux" && !statusNotifierAvailable() {
		st.log.Printf("трей: StatusNotifierWatcher не найден — панель с кнопкой «Снимок»")
		needPanel = true
	}

	if !needPanel {
		go func() {
			trayRunning.Store(true)
			systray.Run(st.onTrayReady, func() { trayRunning.Store(false) })
		}()
	} else {
		go func() {
			time.Sleep(200 * time.Millisecond)
			hint := "Нет значка в трее (GNOME). Жмите «Снимок»."
			if isWaylandSession() {
				hint += "\nWayland: глобальный Ctrl+Shift+S через X11 не работает.\nЯрлык GNOME → команда: study-screenshot -once"
			}
			st.showControlPanel(hint)
		}()
	}

	go st.registerHotkey()
	fa.Run()
}

func (st *AppState) onTrayReady() {
	systray.SetIcon(iconPNG)
	systray.SetTitle("Скриншот")
	systray.SetTooltip("Скриншот со штампом — Ctrl+Shift+S")
	mShoot := systray.AddMenuItem("Сделать снимок", "Ctrl+Shift+S")
	mPanel := systray.AddMenuItem("Показать панель", "")
	mOpen := systray.AddMenuItem("Открыть папку снимков", "")
	systray.AddSeparator()
	mAbout := systray.AddMenuItem("О программе", "")
	mQuit := systray.AddMenuItem("Выход", "")
	go func() {
		for {
			select {
			case <-mShoot.ClickedCh:
				st.shoot()
			case <-mPanel.ClickedCh:
				st.showControlPanel("Ctrl+Shift+S или кнопка «Снимок»")
			case <-mOpen.ClickedCh:
				_ = openDir(platform.ScreenshotsDir())
			case <-mAbout.ClickedCh:
				st.showAbout()
			case <-mQuit.ClickedCh:
				systrayQuitSafe()
				st.fyne.Quit()
				return
			}
		}
	}()
}

func (st *AppState) registerHotkey() {
	if isWaylandSession() {
		err := tryPortalGlobalShortcuts(st.log.Printf, func() { st.shoot() })
		if err != nil {
			st.log.Printf("hotkey wayland: %v", err)
		}
		// X11 grab на чистом Wayland обычно бесполезен — пробуем на случай XWayland
	}
	hk := hotkey.New([]hotkey.Modifier{hotkey.ModCtrl, hotkey.ModShift}, hotkey.KeyS)
	if err := hk.Register(); err != nil {
		st.log.Printf("hotkey X11/Win: %v", err)
		if isWaylandSession() {
			st.log.Printf("подсказка: Настройки → Клавиатура → свои комбинации → Ctrl+Shift+S → study-screenshot -once")
		}
		return
	}
	st.log.Printf("hotkey Ctrl+Shift+S ок (X11/Win/mac)")
	for range hk.Keydown() {
		st.shoot()
	}
}

// shoot — асинхронный снимок (трей / кнопка / hotkey).
func (st *AppState) shoot() {
	if !st.busy.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer st.busy.Store(false)
		st.doShoot()
	}()
}

// shootBlocking — для -once: ждёт завершения формы и сохранения.
func (st *AppState) shootBlocking() {
	if !st.busy.CompareAndSwap(false, true) {
		return
	}
	defer st.busy.Store(false)
	st.doShoot()
}

func (st *AppState) doShoot() {
	res := st.showFormSync()
	if !res.OK {
		return
	}
	st.cfg.Group = mark.Clean(res.Group, 20)
	st.cfg.Students = [2]string{mark.Clean(res.S1, 30), mark.Clean(res.S2, 30)}
	st.cfg.Work = mark.Clean(res.Work, 30)
	st.cfg.Mode = res.Mode
	_ = platform.SaveConfig(st.cfg)

	time.Sleep(400 * time.Millisecond)
	full, err := capture.FullScreen()
	if err != nil {
		st.fail("Не удалось сделать снимок", err)
		return
	}
	img := full
	var area []int
	switch res.Mode {
	case "area":
		r := st.selectRegionSync(full)
		if r.Empty() {
			st.alert("Снимок отменён", "Область не выделена.")
			return
		}
		r = r.Intersect(full.Bounds())
		if r.Dx() < 8 || r.Dy() < 8 {
			st.fail("Область слишком маленькая", fmt.Errorf("%v", r))
			return
		}
		img = capture.Crop(full, r)
		area = []int{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}
	case "window":
		title, rect, ok := platform.ActiveWindow()
		if !ok || rect[2] <= 16 || rect[3] <= 16 {
			st.fail("Активное окно", fmt.Errorf("недоступно на этой ОС или окно слишком маленькое"))
			return
		}
		r := image.Rect(rect[0], rect[1], rect[0]+rect[2], rect[1]+rect[3]).Intersect(full.Bounds())
		img = capture.Crop(full, r)
		area = []int{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}
		_ = title
	}
	if err := st.finish(img, area); err != nil {
		st.fail("Ошибка снимка", err)
	}
}

// showFormSync shows the form on the Fyne UI thread and waits for the result
// on this (non-UI) goroutine. Never blocks the UI thread.
func (st *AppState) showFormSync() formResult {
	ch := make(chan (<-chan formResult), 1)
	fyne.Do(func() {
		ch <- openForm(st.fyne, st.cfg)
	})
	return <-(<-ch)
}

// selectRegionSync opens the region overlay on the UI thread and waits for the
// result off the UI thread.
func (st *AppState) selectRegionSync(shot image.Image) image.Rectangle {
	ch := make(chan (<-chan image.Rectangle), 1)
	fyne.Do(func() {
		ch <- openRegionSelect(st.fyne, shot)
	})
	return <-(<-ch)
}

func (st *AppState) finish(img *image.RGBA, area []int) error {
	now := time.Now()
	ts := now.Format("2006-01-02T15:04:05-0700")
	host := platform.HostName()
	students := []string{st.cfg.Students[0], st.cfg.Students[1]}
	title, _, _ := platform.ActiveWindow()
	src := mark.Src{App: "?", Title: mark.Trunc(mark.Sanitize(title), 60)}
	if title != "" {
		src.App = "window"
	}
	m := &mark.Mark{
		V: mark.VersionV6, OS: platform.OSLabel(), Group: st.cfg.Group, Students: students,
		PC: 0, Host: host, OSUser: platform.UserName(), TS: ts, Work: st.cfg.Work,
		Source: platform.SourceTag(), Src: src, Wins: []string{}, Suspect: false,
		Area: area, AreaWins: []string{}, WL: "",
	}
	m.ID = mark.NewID(students, m.Group, host, ts, img.Pix)

	imgDigest := sign.ImageDigest(img.Pix)
	canon, err := m.Canon()
	if err != nil {
		return err
	}
	metaDigest := sign.MetaDigest(canon)

	resp, err := sign.Do(st.signURL, sign.Request{
		V: mark.VersionV6, ID: m.ID, SHA256: imgDigest, MetaSHA256: metaDigest,
		Client: platform.SourceTag() + "/" + version,
	}, 6*time.Second)
	if err != nil {
		return fmt.Errorf("нет связи с сервером подписи: %w", err)
	}
	m.Srv = &mark.Srv{TS: resp.TS, Sig: shortSig(resp.Sig)}

	stamped, js, err := mark.Stamp(img, m, mark.StampInput{Now: now, Signed: true})
	if err != nil {
		return err
	}
	js = enrichJSON(js, resp, imgDigest, metaDigest)
	asc, _ := m.ASCIIJSON()
	wlJSON, _, _ := mark.WindowsJSON(nil)
	data, err := mark.EncodePNG(stamped, js, asc, m.ID, wlJSON)
	if err != nil {
		return err
	}
	dir := platform.ScreenshotsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := mark.FileName(students, 0, host, now.Format("20060102-150405"), m.ID)
	out := filepath.Join(dir, name)
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return err
	}
	st.log.Printf("снимок id=%s file=%s", m.ID, out)
	if !st.once {
		st.alert("Скриншот сохранён", name+"\n"+dir)
	} else {
		st.log.Printf("once: готово %s", out)
	}
	return nil
}

func enrichJSON(js []byte, resp *sign.Response, imgHash, metaHash string) []byte {
	var obj map[string]any
	if err := json.Unmarshal(js, &obj); err != nil {
		return js
	}
	obj["alg"] = resp.Alg
	obj["pub"] = resp.Pub
	obj["img_sha256"] = imgHash
	obj["meta_sha256"] = metaHash
	obj["sig_full"] = resp.Sig
	if resp.Verify != "" {
		obj["verify"] = resp.Verify
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return js
	}
	return out
}

func shortSig(sig string) string {
	if len(sig) <= 12 {
		return sig
	}
	return sig[:12]
}

func (st *AppState) fail(what string, err error) {
	st.log.Printf("%s: %v", what, err)
	if st.once {
		return
	}
	st.alert(what, err.Error())
}

func (st *AppState) alert(title, body string) {
	fyne.Do(func() {
		w := st.fyne.NewWindow(title)
		w.SetContent(container.NewVBox(
			widget.NewLabel(body),
			widget.NewButton("OK", func() { w.Close() }),
		))
		w.Resize(fyne.NewSize(420, 160))
		w.CenterOnScreen()
		w.Show()
	})
}

func (st *AppState) showAbout() {
	extra := ""
	if isWaylandSession() {
		extra = "\nWayland: горячая клавиша — через ярлык GNOME (study-screenshot -once) или кнопку «Снимок»."
	}
	st.alert("О программе", fmt.Sprintf("Скриншот Study %s\n%s/%s\nПодпись: Ed25519 через %s%s", version, runtime.GOOS, runtime.GOARCH, st.signURL, extra))
}

func openLog() *log.Logger {
	dir := platform.ConfigDir()
	_ = os.MkdirAll(dir, 0o700)
	p := filepath.Join(dir, "log.txt")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return log.New(os.Stderr, "", log.LstdFlags)
	}
	return log.New(f, "", log.LstdFlags)
}

func openDir(dir string) error {
	_ = os.MkdirAll(dir, 0o755)
	switch runtime.GOOS {
	case "windows":
		return execLook("explorer", dir)
	case "darwin":
		return execLook("open", dir)
	default:
		return execLook("xdg-open", dir)
	}
}
