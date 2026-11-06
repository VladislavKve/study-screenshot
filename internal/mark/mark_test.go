package mark

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"os"
	"os/exec"
	"runtime"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sample() *Mark {
	return &Mark{V: 5, OS: "Windows 11 Pro 23H2 (22631)", Group: "2исп9-82", Students: []string{"Иванов", "Петров"},
		PC: 0, Host: "DESKTOP-K7QX2", OSUser: "vanya", TS: "2026-11-03T14:05:07+0300", Work: "ПР 6", Source: "win", ID: "6fed6aac36",
		Src:  Src{App: "Chrome", Title: `6.Видео уроки "Cisco" <b>&amp; STP \ путь`, Exe: `C:\Program Files\Google\Chrome\Application\chrome.exe`},
		Wins: []string{"Chrome", "Cisco Packet Tracer"}, Suspect: false, Area: nil, AreaWins: []string{}, WL: "f6943629e15f",
		Srv: &Srv{TS: "2026-11-03T14:05:08+0300", Sig: "0123456789ab"}}
}

// Канонический JSON Go должен байт в байт совпадать с json.dumps(..., ensure_ascii=False, sort_keys=True, separators=(',',':'))
// над теми же данными — иначе study-mark-verify.py не сойдётся с подписью.
func TestCanonMatchesPython(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("нет python3")
	}
	m := sample()
	m.Src.Title = Sanitize(m.Src.Title + "\x08\x0c\u2028\x7fй\xff")
	js, _ := m.JSON()
	canon, _ := m.Canon()
	script := `import sys,json
d=json.loads(sys.stdin.read())
S=('v','os','group','students','pc','host','os_user','ts','work','source','id','src','wins','suspect','area','area_wins','wl')
sys.stdout.write(json.dumps({k:d.get(k) for k in S},ensure_ascii=False,sort_keys=True,separators=(',',':')))`
	cmd := exec.Command(py, "-c", script)
	cmd.Stdin = bytes.NewReader(js)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, canon) {
		t.Fatalf("canon отличается:\n go: %s\n py: %s", canon, out)
	}
	if bytes.Contains(canon, []byte(`"srv"`)) {
		t.Fatal("srv не должен входить в подписываемые поля")
	}
	if !bytes.Contains(js, []byte(`<b>&amp;`)) {
		t.Fatalf("HTML-экранирование не должно включаться: %s", js)
	}
}

func TestASCIIJSONMatchesPython(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("python console encoding on Windows CI is not UTF-8")
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("нет python3")
	}
	m := sample()
	m.Work = "😀 ПР" // суррогатная пара
	js, _ := m.JSON()
	asc, _ := m.ASCIIJSON()
	cmd := exec.Command(py, "-c", `import sys,json; sys.stdout.write(json.dumps(json.loads(sys.stdin.read()),ensure_ascii=True,separators=(',',':')))`)
	cmd.Stdin = bytes.NewReader(js)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, asc) {
		t.Fatalf("ascii отличается:\n go: %s\n py: %s", asc, out)
	}
}

func TestFieldOrderAndNulls(t *testing.T) {
	m := sample()
	m.Srv = nil
	js, _ := m.JSON()
	s := string(js)
	if !strings.HasPrefix(s, `{"v":5,"os":"`) || !strings.HasSuffix(s, `"srv":null}`) {
		t.Fatalf("порядок полей: %s", s)
	}
	if !strings.Contains(s, `"area":null`) || !strings.Contains(s, `"area_wins":[]`) {
		t.Fatalf("area/area_wins: %s", s)
	}
}

func TestClean(t *testing.T) {
	if got := Clean("  Ива,нов |  Пет\tров  ", 30); got != "Ива нов Пет ров" {
		t.Fatalf("clean: %q", got)
	}
	if got := Clean("абвгдеёжзий", 5); got != "абвгд" {
		t.Fatalf("trunc: %q", got)
	}
	if got := Ellipsis("абвгдеёжзий", 5); got != "абвг…" {
		t.Fatalf("ellipsis: %q", got)
	}
}

func TestFileName(t *testing.T) {
	got := FileName([]string{"Иванов", "Петров-Сидоров"}, 0, "DESKTOP-K7QX2", "20261103-140507", "6fed6aac36")
	if got != "Иванов-Петров-Сидоров_DESKTOP-K7QX2_20261103-140507_6fed6aac36.png" {
		t.Fatal(got)
	}
	got = FileName([]string{"Иванов", ""}, 3, "studymost3", "20261103-140507", "abc")
	if got != "Иванов_ПК3_20261103-140507_abc.png" {
		t.Fatal(got)
	}
	got = FileName([]string{"Иванов Ив.", "x/y"}, 0, "DESKTOP", "t", "i")
	if got != "Иванов_Ив_-x_y_DESKTOP_t_i.png" {
		t.Fatal(got)
	}
}

func TestSuspect(t *testing.T) {
	cases := []struct {
		app, exe, class, title string
		want                   bool
	}{
		{"Chrome", `C:\...\chrome.exe`, "Chrome_WidgetWin_1", "Packet Tracer tutorial - YouTube", false},
		{"Chrome", `C:\...\chrome.exe`, "Chrome_WidgetWin_1", "IMG_2031.jpg (1920×1080)", true},
		{"Telegram", `C:\...\Telegram.exe`, "Qt5...", "Telegram", true},
		{"Max", `C:\Users\x\AppData\Local\Max\Max.exe`, "Chrome_WidgetWin_1", "Max", true},
		{"PacketTracer", `C:\...\PacketTracer.exe`, "Qt", "Cisco Packet Tracer", false},
		{"Code", `C:\...\Code.exe`, "Chrome_WidgetWin_1", "main.py - Visual Studio Code", false},
		{"Фотографии", `C:\Windows\System32\ApplicationFrameHost.exe`, "ApplicationFrameWindow", "IMG_1.png – Фотографии", true},
		{"ApplicationFrameHost", `C:\Windows\System32\ApplicationFrameHost.exe`, "ApplicationFrameWindow", "Калькулятор", false},
		{"Проводник", `C:\Windows\explorer.exe`, "CabinetWClass", "Загрузки", true},
		{"mspaint", `C:\Windows\System32\mspaint.exe`, "MSPaintApp", "Безымянный - Paint", true},
		{"notepad", `C:\Windows\System32\notepad.exe`, "Notepad", "report.txt", false},
	}
	for _, c := range cases {
		if got := IsSuspect(c.app, c.exe, c.class, c.title); got != c.want {
			t.Errorf("%s/%s: %v, ждали %v", c.app, c.title, got, c.want)
		}
	}
}

func TestSrcFields(t *testing.T) {
	wins := []Window{
		{App: "Chrome", Title: "x.jpg", Exe: "chrome.exe", Rect: [4]int{0, 0, 800, 600}, Suspect: true},
		{App: "PacketTracer", Rect: [4]int{700, 0, 800, 600}},
		{App: "Chrome", Rect: [4]int{0, 0, 10, 10}},
		{App: "Word", Minimized: true},
	}
	src, short, sus, aw := SrcFields(&wins[1], wins, nil)
	if src.App != "PacketTracer" || sus || len(aw) != 0 {
		t.Fatalf("src=%+v sus=%v aw=%v", src, sus, aw)
	}
	if strings.Join(short, ",") != "Chrome,PacketTracer" {
		t.Fatalf("wins=%v", short)
	}
	_, _, sus, aw = SrcFields(&wins[1], wins, []int{750, 10, 100, 100})
	if strings.Join(aw, ",") != "Chrome,PacketTracer" {
		t.Fatalf("area_wins=%v", aw)
	}
	if !sus { // Chrome (верхнее окно над областью) подозрителен → suspect
		t.Fatal("ожидали suspect по перекрывающему окну")
	}
	src, _, _, _ = SrcFields(nil, nil, nil)
	if src.App != "?" {
		t.Fatal(src)
	}
}

// Полный путь: штамп → PNG с чанками → PIL читает StudyMark/StudyWindows, zbarimg читает QR,
// study-mark-verify.py понимает v5.
func TestStampPNGRoundTrip(t *testing.T) {
	base := image.NewRGBA(image.Rect(0, 0, 1366, 768))
	for y := 0; y < 768; y++ {
		for x := 0; x < 1366; x++ {
			base.Set(x, y, color.RGBA{uint8(x / 6), uint8(y / 4), 120, 255})
		}
	}
	m := sample()
	m.Srv = nil
	wl, h, _ := WindowsJSON([]Window{{App: "Chrome", Title: "t", Rect: [4]int{0, 0, 1, 1}}})
	m.WL = h
	im, js, err := Stamp(base, m, StampInput{Now: time.Date(2026, 11, 3, 14, 5, 7, 0, time.FixedZone("MSK", 3*3600)), Signed: false})
	if err != nil {
		t.Fatal(err)
	}
	asc, _ := m.ASCIIJSON()
	pngb, err := EncodePNG(im, js, asc, m.ID, wl)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("..", "..", "testdata", "out")
	os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, "roundtrip.png")
	if err := os.WriteFile(p, pngb, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("нет python3")
	}
	// skip if Pillow missing
	if out, err := exec.Command("python3", "-c", "from PIL import Image").CombinedOutput(); err != nil {
		t.Skipf("нет Pillow: %v %s", err, out)
	}
	out, err := exec.Command("python3", "-c", `import sys,json
from PIL import Image
t=Image.open(sys.argv[1]).text
d=json.loads(t['StudyMark']); w=json.loads(t['StudyWindows'])
assert d['v']==5 and d['srv'] is None and t['StudyID']==d['id'], d
assert json.loads(t['StudyMarkASCII'])==d
print(len(w), d['group'])`, p).CombinedOutput()
	if err != nil {
		t.Fatalf("PIL: %v %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "1 2исп9-82" {
		t.Fatalf("PIL: %s", out)
	}
	if zb, err := exec.LookPath("zbarimg"); err == nil {
		out, err := exec.Command(zb, "-q", "--raw", "-Sbinary", p).Output()
		if err != nil {
			t.Fatalf("zbarimg: %v", err)
		}
		var d map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(out), &d); err != nil || d["id"] != m.ID {
			t.Fatalf("QR не читается: %v %s", err, out)
		}
		if !bytes.Equal(bytes.TrimSpace(out), js) {
			t.Fatalf("QR != JSON:\n%s\n%s", out, js)
		}
	}
}
