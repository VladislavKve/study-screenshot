# Study Screenshot — кроссплатформенная утилита скриншотов со штампом и QR

Стек: **Go** (один код для Windows / Linux / macOS) + локальный сервер подписи на **Python (Ed25519)**.

Студент запускает утилиту, жмёт **Ctrl+Shift+S** (или пункт в трее), вводит группу / до двух фамилий / название работы, получает PNG со штампом и QR. Без связи с сервером подписи снимок **не сохраняется** (ошибка).

## Что найдено на Citadel (только чтение)

| Путь | Назначение |
|------|------------|
| `/home/mosi/Documents/studyPCcontrol/study-screenshot-win/` | Прототип Windows (Go 1.26, Win32 tray/hotkey/dialog) |
| `…/dist/StudyScreenshot.exe` | Сборка от 03.11 |
| `/home/mosi/Documents/studyPCcontrol/deploy/study-sign-server.py` | HMAC v5 служба подписи (не включена) |
| `…/deploy/study-sign.service` | systemd-юнит |
| `…/deploy/study-screenshot.sh` | Старая Linux-утилита (bash/Python/zenity) |
| `/home/mosi/Documents/KS54/tasks/STUDY-SCREENSHOT-WINDOWS-20261103.md` | ТЗ |

Копии лежат в `ref/` (Citadel **не менялся**).

## Возможности (v0.2 cross)

- Трей: `fyne.io/systray`
- Горячая клавиша: `Ctrl+Shift+S` (`golang.design/x/hotkey`)
- Форма (русский UI): группа, фамилия 1/2, работа, режим (экран / область / активное окно)
- Снимок: `github.com/kbinani/screenshot`
- Область: полноэкранный оверлей Fyne
- Активное окно: WinAPI (только Windows)
- Штамп + QR: пакет `internal/mark` (из Windows-прототипа)
- Подпись v6: клиент шлёт SHA-256 изображения + SHA-256 канонических метаданных → сервер возвращает Ed25519 + время; QR содержит `verify` URL
- Сохранение: `Pictures/StudyScreenshots/`
- Конфиг: `config.json` в user config dir; флаг `-sign-url`

## Публичный сервер подписи (HOSTKEY RU, тест)

- Health: https://77-91-72-31.sslip.io/study-sign/health
- Sign:   https://77-91-72-31.sslip.io/study-sign/sign
- Verify: https://77-91-72-31.sslip.io/study-sign/verify

В клиенте это значение по умолчанию (`defaultSignURL`). Переопределение: флаг `-sign-url` или `sign_url` в config.json.

## Сервер подписи (только на box / локально)

```bash
cd server
# ключи уже в keys/ (600) — НЕ с Citadel
STUDY_SIGN_PORT=8830 python3 study-sign-server.py
# GET /health  POST /sign  GET|POST /verify  GET /pubkey
```

Включать `study-sign.service` / править Caddy на Citadel — **только после «да» Влада**.

## Сборка

```bash
./scripts/build.sh
# артефакты:
#   dist/study-screenshot-linux-amd64
#   dist/StudyScreenshot-windows-amd64.exe
```

macOS: собирайте через GitHub Actions (`.github/workflows/build.yml`) — нужен CGO и SDK macOS. Репозиторий этим workflow **не создавался и не пушился**.

## Тесты

```bash
go test ./internal/...
# e2e: hash → sign → verify (+ негатив на подмену хеша)
go run ./cmd/stamptest -sign-url http://127.0.0.1:8830/sign
```

## Ограничения

- **Wayland**: захват экрана зависит от portal/XWayland; на чистом Wayland без portal может не работать (нужен PipeWire portal / X11).
- **macOS**: требуется разрешение «Запись экрана» в Системных настройках; глобальная горячая клавиша — Accessibility.
- **Активное окно**: полноценно только Windows; на Linux/macOS режим недоступен.
- **Область**: координаты оверлея при `ImageFillContain` на мультимониторе могут требовать калибровки.
- Exe не подписан (SmartScreen).
- Без интернета до сервера подписи — ошибка, штамп не ставится.

## Нужно от Влада

1. «да» на включение службы подписи + Caddy на Citadel (или отдельный хост).
2. ПК преподавателя для теста Windows-сборки.
3. Нужен ли сразу Mac-тест.


## Wayland / GNOME (v0.1.2)

На чистом Wayland глобальный **Ctrl+Shift+S** через X11-grab (`golang.design/x/hotkey`) обычно **не работает**.

Что делает утилита:
- если нет StatusNotifierWatcher (типичный GNOME) — показывает маленькое окно с кнопкой **«Снимок»** (`-panel` принудительно);
- захват экрана: kbinani → иначе `gnome-screenshot` / `grim` / portal;
- режим `-once` — один снимок и выход (для ярлыка).

Ярлык GNOME: Параметры → Клавиатура → Свои комбинации → команда `study-screenshot -once`, клавиши Ctrl+Shift+S.

v0.1.2: форма и выбор области больше не блокируют UI-поток Fyne (исправлен deadlock «Is Not Responding» после «Снимок»).
