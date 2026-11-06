// Package mark — метка «Скриншот» v5 для Windows-версии: поля QR, канонический JSON
// для подписи, идентификатор снимка, признак «подозрительно». Чистый Go, без Win32:
// собирается и тестируется на сервере.
//
// v5 повторяет поля v4 (deploy/study-screenshot.sh), но вместо iface/mac — os (версия Windows),
// source = "win", а подпись ставит сервер: srv = {ts, sig}. Без сети srv = null («не заверено»).
package mark

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const Version = 5

// Src — активное окно в момент нажатия горячей клавиши.
type Src struct {
	App   string `json:"app"`
	Title string `json:"title"`
	Exe   string `json:"exe"`
}

// Srv — заверение сервера: его время и HMAC-подпись (12 hex).
type Srv struct {
	TS  string `json:"ts"`
	Sig string `json:"sig"`
}

// Mark — содержимое QR и метаданных PNG (StudyMark). Порядок полей = порядок в JSON.
type Mark struct {
	V        int      `json:"v"`
	OS       string   `json:"os"`
	Group    string   `json:"group"`
	Students []string `json:"students"`
	PC       int      `json:"pc"`
	Host     string   `json:"host"`
	OSUser   string   `json:"os_user"`
	TS       string   `json:"ts"`
	Work     string   `json:"work"`
	Source   string   `json:"source"`
	ID       string   `json:"id"`
	Src      Src      `json:"src"`
	Wins     []string `json:"wins"`
	Suspect  bool     `json:"suspect"`
	Area     []int    `json:"area"`
	AreaWins []string `json:"area_wins"`
	WL       string   `json:"wl"`
	Srv      *Srv     `json:"srv"`
}

// Signed — поля под подписью (как SIGNED в Linux-версии: всё, кроме самой подписи).
var Signed = []string{"v", "os", "group", "students", "pc", "host", "os_user", "ts", "work", "source", "id",
	"src", "wins", "suspect", "area", "area_wins", "wl"}

// Window — одно окно для полного списка (StudyWindows в PNG), как в v3/v4.
type Window struct {
	App       string `json:"app"`
	AppID     string `json:"app_id"`
	WMClass   string `json:"wm_class"`
	Title     string `json:"title"`
	Exe       string `json:"exe"`
	PID       int    `json:"pid"`
	Rect      [4]int `json:"rect"`
	Minimized bool   `json:"minimized"`
	Suspect   bool   `json:"suspect"`
}

// Clean — как clean() в Linux-версии: без запятых и «|», одиночные пробелы, не длиннее n рун.
// Дополнительно убирает управляющие символы, U+2028/2029 и битый UTF-8: тогда канонический JSON
// в Go и в Python (study-mark-verify.py) совпадает байт в байт.
func Clean(s string, n int) string {
	s = Sanitize(strings.NewReplacer(",", " ", "|", " ").Replace(s))
	s = strings.Join(strings.Fields(s), " ")
	return Trunc(s, n)
}

// Sanitize убирает символы, которые Go и Python экранируют по-разному.
func Sanitize(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20, r == 0x7f, r == 0x2028, r == 0x2029:
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Trunc — первые n рун (как срез строки в Python).
func Trunc(s string, n int) string {
	if n <= 0 {
		return ""
	}
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}

// Ellipsis — как `st if len(st) <= 50 else st[:49] + '…'`.
func Ellipsis(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return Trunc(s, n-1) + "…"
}

// marshal — компактный JSON без HTML-экранирования (как json.dumps(ensure_ascii=False, separators=(',',':'))).
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// JSON — строка для QR и iTXt StudyMark (порядок полей как в структуре).
func (m *Mark) JSON() ([]byte, error) { return marshal(m) }

// Canon — канонический JSON подписываемых полей: ключи по алфавиту, компактно, без ensure_ascii.
// Совпадает с canon() Linux-версии и с тем, что считает study-mark-verify.py.
func (m *Mark) Canon() ([]byte, error) {
	full, err := marshal(m)
	if err != nil {
		return nil, err
	}
	// Через any: map[string]any кодируется с ключами по алфавиту на всех уровнях (как sort_keys=True);
	// UseNumber сохраняет числа как есть.
	dec := json.NewDecoder(bytes.NewReader(full))
	dec.UseNumber()
	var all map[string]any
	if err := dec.Decode(&all); err != nil {
		return nil, err
	}
	sub := map[string]any{}
	for _, k := range Signed {
		sub[k] = all[k] // отсутствующее → nil → null
	}
	return marshal(sub)
}

// Digest — sha256(Canon) hex: это и уходит на сервер подписи.
func (m *Mark) Digest() (string, error) {
	c, err := m.Canon()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(c)
	return hex.EncodeToString(h[:]), nil
}

// ASCIIJSON — как json.dumps(d, ensure_ascii=True, separators=(',',':')) для tEXt StudyMarkASCII.
func (m *Mark) ASCIIJSON() ([]byte, error) {
	js, err := marshal(m)
	if err != nil {
		return nil, err
	}
	return ASCIIEscape(js), nil
}

// ASCIIEscape переводит не-ASCII руны в \uXXXX (с суррогатными парами), как ensure_ascii в Python.
func ASCIIEscape(js []byte) []byte {
	var b bytes.Buffer
	for _, r := range string(js) {
		if r < 0x7f {
			b.WriteRune(r)
			continue
		}
		if r > 0xffff {
			r -= 0x10000
			fmt.Fprintf(&b, "\\u%04x\\u%04x", 0xd800+(r>>10), 0xdc00+(r&0x3ff))
		} else {
			fmt.Fprintf(&b, "\\u%04x", r)
		}
	}
	return b.Bytes()
}

// NewID — идентификатор снимка, как в Linux-версии: sha256("ф1/ф2|группа|host|ts|" + sha256(пиксели))[:10].
func NewID(students []string, group, host, ts string, pixels []byte) string {
	ph := sha256.Sum256(pixels)
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|", strings.Join(students, "/"), group, host, ts)
	h.Write(ph[:])
	return hex.EncodeToString(h.Sum(nil))[:10]
}

// WindowsJSON — полный список окон для iTXt StudyWindows; WL — sha256 этой строки[:12].
func WindowsJSON(ws []Window) ([]byte, string, error) {
	if ws == nil {
		ws = []Window{}
	}
	js, err := marshal(ws)
	if err != nil {
		return nil, "", err
	}
	h := sha256.Sum256(js)
	return js, hex.EncodeToString(h[:])[:12], nil
}

// ---- «Подозрительно»: снимали просмотрщик картинок / мессенджер / браузер с картинкой (как в Linux-версии, плюс Windows-приложения) ----

var (
	viewers = regexp.MustCompile(`(?i)photos|photoviewer|photo viewer|фотографии|i_view|irfan|xnview|faststone|honeyview|acdsee|imageglass|nomacs|` +
		`gimp|krita|pinta|paint\.?net|paintdotnet|mspaint|\bpaint\b|photoshop|lightroom|` +
		`cabinetwclass|explorer\.exe|nautilus|` +
		`telegram|whatsapp|\bmax\b|max\.exe|\bvk\b|vk\.exe|discord|element|signal|zapzap|skype|viber|messenger|teams`)
	browsers = regexp.MustCompile(`(?i)firefox|chrom|yandex|browser\.exe|brave|opera|vivaldi|epiphany|msedge|\bedge\b|iexplore`)
	imgTitle = regexp.MustCompile(`(?i)\.(jpe?g|png|webp|heic|gif|bmp|tiff?)\b|фото|photo|image|изображени|скриншот|screenshot|снимок|` +
		`max\.ru|web\.max|telegram|whatsapp|vk\.com|вконтакте|mail\.ru|яндекс\s*диск|disk\.yandex|drive\.google|` +
		`\d{3,5}\s*[×x]\s*\d{3,5}`)
	safeApps = regexp.MustCompile(`(?i)study-?screenshot|packet|\bcode\b|pgadmin|terminal|cmd\.exe|powershell|conhost|libreoffice|soffice|` +
		`calculator|notepad|winword|excel|powerpnt|virtualbox|vmware|idea|pycharm|devenv`)
)

// IsSuspect — по идентификаторам окна (app, exe, класс) и заголовку.
func IsSuspect(app, exe, class, title string) bool {
	// Класс окна в проверке браузера не участвует: Electron-приложения (Max, VK, Discord)
	// тоже носят класс Chrome_WidgetWin_1.
	ident := app + " " + exe + " " + class
	if browsers.MatchString(app + " " + exe) {
		return imgTitle.MatchString(title)
	}
	if safeApps.MatchString(ident) {
		return false
	}
	return viewers.MatchString(ident) || viewers.MatchString(title) && strings.Contains(strings.ToLower(exe), "applicationframehost")
}

// SrcFields — поля src/wins/suspect/area/area_wins по снимку окон (аналог src_fields в Linux-версии).
// focus — активное окно (может быть nil), wins — все окна верхнего уровня сверху вниз,
// area — [x,y,w,h] снятой области в экранных пикселях (nil — весь экран).
func SrcFields(focus *Window, wins []Window, area []int) (src Src, short []string, suspect bool, areaWins []string) {
	src = Src{App: "?"}
	if focus != nil {
		src = Src{App: focus.App, Title: Trunc(Sanitize(focus.Title), 60), Exe: focus.Exe}
		if src.App == "" {
			src.App = "?"
		}
		suspect = focus.Suspect
	}
	short = []string{}
	areaWins = []string{}
	for _, w := range wins {
		if w.Minimized {
			continue
		}
		if w.App != "" && !contains(short, w.App) {
			short = append(short, w.App)
		}
		if area != nil && len(area) == 4 {
			x, y, aw, ah := area[0], area[1], area[2], area[3]
			if w.Rect[0] < x+aw && x < w.Rect[0]+w.Rect[2] && w.Rect[1] < y+ah && y < w.Rect[1]+w.Rect[3] {
				if !contains(areaWins, w.App) {
					areaWins = append(areaWins, w.App)
				}
			}
		}
	}
	if area != nil && len(areaWins) > 0 {
		for _, w := range wins {
			if !w.Minimized && w.App == areaWins[0] && w.Suspect {
				suspect = true
			}
		}
	}
	if len(short) > 8 {
		short = short[:8]
	}
	if len(areaWins) > 6 {
		areaWins = areaWins[:6]
	}
	return
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// SafeName — имя файла: `<Фамилии>_ПК<N>_<yyyymmdd-HHMMSS>_<id>.png`, как в Linux-версии
// (re.sub(r'[^\w-]+', '_', ...)); без номера ПК вместо ПК<N> — имя компьютера.
func FileName(students []string, pc int, host, stamp, id string) string {
	var names []string
	for _, s := range students {
		if s != "" {
			names = append(names, s)
		}
	}
	safe := nonWord.ReplaceAllString(strings.Join(names, "-"), "_")
	where := fmt.Sprintf("ПК%d", pc)
	if pc <= 0 {
		where = nonWord.ReplaceAllString(host, "_")
	}
	return fmt.Sprintf("%s_%s_%s_%s.png", safe, where, stamp, id)
}

var nonWord = regexp.MustCompile(`[^\pL\pN_-]+`)
