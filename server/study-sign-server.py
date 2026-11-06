#!/usr/bin/env python3
"""Study Screenshot sign server (local / box).

Protocols:
  v5 HMAC (compat): POST /sign {"v":5,"id":"<10 hex>","sha256":"<64 hex>","client":"..."}
      → {"ts":"...","sig":"<12 hex>","alg":"hmac-sha256"}
  v6 Ed25519: POST /sign {"v":6,"id":"<10 hex>","sha256":"<64 hex>",
                          "meta_sha256":"<64 hex>","client":"..."}
      → {"ts":"...","sig":"<base64>","alg":"ed25519","pub":"<base64>"}

GET  /health  → {"ok": true, ...}
GET  /pubkey  → {"alg":"ed25519","pub":"..."}
GET  /verify  → HTML form + ?query verification
POST /verify  → JSON {"ok": bool, ...}

Keys (local only; never touch Citadel):
  STUDY_SIGN_ED25519_KEY  — path to raw 32-byte seed or PEM (default: keys/ed25519.seed)
  STUDY_SIGN_KEY_FILE     — HMAC key for v5 (optional; if missing, v5 disabled)
"""
from __future__ import annotations

import base64
import hashlib
import hmac
import json
import os
import re
import secrets
import stat
import sys
import threading
import time
from datetime import datetime
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse
from zoneinfo import ZoneInfo

try:
    from cryptography.hazmat.primitives.asymmetric.ed25519 import (
        Ed25519PrivateKey,
        Ed25519PublicKey,
    )
    from cryptography.hazmat.primitives import serialization
except ImportError:
    sys.exit("нужен пакет cryptography: pip install cryptography")

BASE = os.path.dirname(os.path.abspath(__file__))
HOST = os.environ.get("STUDY_SIGN_HOST", "127.0.0.1")
PORT = int(os.environ.get("STUDY_SIGN_PORT", "8830"))
TZ = ZoneInfo("Europe/Moscow")
VERIFY_BASE = os.environ.get("STUDY_SIGN_VERIFY_BASE", f"http://{HOST}:{PORT}/verify")

ED25519_PATH = os.environ.get(
    "STUDY_SIGN_ED25519_KEY", os.path.join(BASE, "keys", "ed25519.seed")
)
HMAC_PATH = os.environ.get("STUDY_SIGN_KEY_FILE", "")

HEX10 = re.compile(r"^[0-9a-f]{10}$")
HEX64 = re.compile(r"^[0-9a-f]{64}$")
PER_IP, PER_IP_WINDOW = 20, 60.0
GLOBAL, GLOBAL_WINDOW = 300, 60.0


def _perms_ok(path):
    """На Unix требуем 600; на Windows/с флагом STUDY_SIGN_RELAX_PERMS — не проверяем."""
    if os.name != "posix" or os.environ.get("STUDY_SIGN_RELAX_PERMS") == "1":
        return True
    st = os.stat(path)
    return not (st.st_mode & (stat.S_IRWXG | stat.S_IRWXO))

def load_ed25519():
    os.makedirs(os.path.dirname(ED25519_PATH) or ".", exist_ok=True)
    if not os.path.exists(ED25519_PATH):
        seed = secrets.token_bytes(32)
        with open(ED25519_PATH, "wb") as f:
            f.write(seed)
        try:
            os.chmod(ED25519_PATH, 0o600)
        except OSError:
            pass
        print("generated", ED25519_PATH, flush=True)
    if not _perms_ok(ED25519_PATH):
        sys.exit(f"ключ {ED25519_PATH}: права должны быть 600")
    raw = open(ED25519_PATH, "rb").read()
    if b"BEGIN" in raw:
        priv = serialization.load_pem_private_key(raw, password=None)
        if not isinstance(priv, Ed25519PrivateKey):
            sys.exit("PEM не Ed25519")
        return priv
    if len(raw) == 32:
        return Ed25519PrivateKey.from_private_bytes(raw)
    if len(raw) == 64:
        return Ed25519PrivateKey.from_private_bytes(raw[:32])
    sys.exit(f"ключ {ED25519_PATH}: ожидается 32-байтный seed или PEM")


PRIV = load_ed25519()
PUB = PRIV.public_key()
PUB_B64 = base64.urlsafe_b64encode(
    PUB.public_bytes(
        encoding=serialization.Encoding.Raw,
        format=serialization.PublicFormat.Raw,
    )
).decode().rstrip("=")


def load_hmac():
    path = HMAC_PATH
    if not path:
        cand = os.path.join(BASE, "keys", "hmac.key")
        path = cand if os.path.exists(cand) else ""
    if not path or not os.path.exists(path):
        return None
    if not _perms_ok(path):
        sys.exit(f"ключ {path}: права должны быть 600")
    k = open(path, "rb").read().strip()
    if len(k) < 32:
        sys.exit(f"ключ {path}: слишком короткий")
    return k


HMAC_KEY = load_hmac()


def hmac_sign(v, sid, digest, ts):
    return hmac.new(HMAC_KEY, f"{v}|{sid}|{digest}|{ts}".encode(), hashlib.sha256).hexdigest()[:12]


def ed_message(v, sid, digest, meta, ts):
    return f"{v}|{sid}|{digest}|{meta}|{ts}".encode()


def ed_sign(v, sid, digest, meta, ts):
    sig = PRIV.sign(ed_message(v, sid, digest, meta, ts))
    return base64.urlsafe_b64encode(sig).decode().rstrip("=")


def ed_verify(v, sid, digest, meta, ts, sig_b64):
    pad = "=" * (-len(sig_b64) % 4)
    try:
        sig = base64.urlsafe_b64decode(sig_b64 + pad)
        PUB.verify(sig, ed_message(v, sid, digest, meta, ts))
        return True
    except Exception:
        return False


class Limiter:
    def __init__(self):
        self.lock = threading.Lock()
        self.ip = {}
        self.glob = []

    def allow(self, ip):
        now = time.monotonic()
        with self.lock:
            self.glob = [t for t in self.glob if now - t < GLOBAL_WINDOW]
            if len(self.glob) >= GLOBAL:
                return False
            q = [t for t in self.ip.get(ip, []) if now - t < PER_IP_WINDOW]
            if len(q) >= PER_IP:
                self.ip[ip] = q
                return False
            q.append(now)
            self.ip[ip] = q
            self.glob.append(now)
            if len(self.ip) > 5000:
                self.ip = {
                    k: v
                    for k, v in self.ip.items()
                    if v and now - v[-1] < PER_IP_WINDOW
                }
            return True


LIM = Limiter()
LOG_LOCK = threading.Lock()


def log(*parts):
    with LOG_LOCK:
        print(datetime.now(TZ).strftime("%Y-%m-%d %H:%M:%S"), *parts, flush=True)


def mask_ip(ip):
    if ":" in ip:
        h = ip.split(":")
        return ":".join(h[:3]) + ":x"
    p = ip.split(".")
    return ".".join(p[:3]) + ".x" if len(p) == 4 else "x"


def now_ts():
    return datetime.now(TZ).strftime("%Y-%m-%dT%H:%M:%S%z")


VERIFY_HTML = """<!doctype html>
<html lang="ru"><head><meta charset="utf-8"><title>Проверка штампа Study</title>
<style>
body{font-family:system-ui,sans-serif;max-width:720px;margin:2rem auto;padding:0 1rem}
code,textarea{font-family:ui-monospace,monospace;font-size:0.9rem}
textarea{width:100%;height:8rem}
.ok{color:#0a0}.bad{color:#a00}
label{display:block;margin-top:0.75rem}
</style></head><body>
<h1>Проверка заверения Study Screenshot</h1>
<p>Алгоритм: Ed25519 (v6). Вставьте поля из QR или URL.</p>
<form method="get" action="/verify">
<label>v <input name="v" value="6"></label>
<label>id <input name="id" size="16" value="{id}"></label>
<label>sha256 (изображение) <input name="sha256" size="68" value="{sha256}"></label>
<label>meta_sha256 <input name="meta_sha256" size="68" value="{meta}"></label>
<label>ts <input name="ts" size="32" value="{ts}"></label>
<label>sig <input name="sig" size="68" value="{sig}"></label>
<p><button type="submit">Проверить</button></p>
</form>
{result}
<p><small>Публичный ключ: <code>{pub}</code></small></p>
</body></html>
"""


class H(BaseHTTPRequestHandler):
    server_version = "study-sign/2"

    def log_message(self, *a):
        pass

    def client_ip(self):
        xff = self.headers.get("X-Forwarded-For", "")
        return xff.split(",")[0].strip() if xff else self.client_address[0]

    def reply_json(self, code, obj):
        body = json.dumps(obj, ensure_ascii=False).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.send_header("Access-Control-Allow-Origin", "*")
        if code == 429:
            self.send_header("Retry-After", "30")
        self.end_headers()
        self.wfile.write(body)

    def reply_html(self, code, html):
        body = html.encode()
        self.send_response(code)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_OPTIONS(self):
        self.send_response(204)
        self.send_header("Access-Control-Allow-Origin", "*")
        self.send_header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
        self.send_header("Access-Control-Allow-Headers", "Content-Type")
        self.end_headers()

    def do_GET(self):
        u = urlparse(self.path)
        path = u.path.rstrip("/") or "/"
        if path in ("/health", "/"):
            return self.reply_json(
                200,
                {
                    "ok": True,
                    "ts": now_ts(),
                    "alg": ["ed25519"] + (["hmac-sha256"] if HMAC_KEY else []),
                    "pub": PUB_B64,
                },
            )
        if path == "/pubkey":
            return self.reply_json(200, {"alg": "ed25519", "pub": PUB_B64})
        if path == "/verify":
            qs = parse_qs(u.query)
            get = lambda k: (qs.get(k) or [""])[0]
            v, sid, digest, meta, ts, sig = (
                get("v") or "6",
                get("id"),
                get("sha256"),
                get("meta_sha256") or get("meta"),
                get("ts"),
                get("sig"),
            )
            result = ""
            if sid and digest and ts and sig:
                try:
                    vi = int(v)
                except ValueError:
                    vi = 0
                ok = (
                    vi == 6
                    and HEX10.match(sid)
                    and HEX64.match(digest)
                    and (not meta or HEX64.match(meta))
                    and ed_verify(vi, sid, digest, meta or "", ts, sig)
                )
                if ok:
                    result = '<p class="ok"><b>Подпись верна</b> — снимок заверен сервером.</p>'
                else:
                    result = '<p class="bad"><b>Подпись неверна</b> или поля повреждены.</p>'
            html = VERIFY_HTML.format(
                id=sid,
                sha256=digest,
                meta=meta,
                ts=ts,
                sig=sig,
                pub=PUB_B64,
                result=result,
            )
            return self.reply_html(200, html)
        self.reply_json(404, {"error": "not found"})

    def do_POST(self):
        ip = self.client_ip()
        path = urlparse(self.path).path.rstrip("/")
        n = int(self.headers.get("Content-Length") or 0)
        if n <= 0 or n > 8192:
            return self.reply_json(413, {"error": "bad length"})
        raw = self.rfile.read(n)
        if path == "/verify":
            try:
                d = json.loads(raw.decode("utf-8"))
                ok = ed_verify(
                    int(d.get("v", 6)),
                    str(d.get("id")),
                    str(d.get("sha256")),
                    str(d.get("meta_sha256") or d.get("meta") or ""),
                    str(d.get("ts")),
                    str(d.get("sig")),
                )
                return self.reply_json(200, {"ok": ok})
            except Exception:
                return self.reply_json(400, {"error": "bad json", "ok": False})
        if path != "/sign":
            return self.reply_json(404, {"error": "not found"})
        if not LIM.allow(ip):
            log("429", mask_ip(ip))
            return self.reply_json(429, {"error": "too many requests"})
        try:
            d = json.loads(raw.decode("utf-8"))
            v = int(d.get("v"))
            sid = str(d.get("id"))
            digest = str(d.get("sha256"))
            meta = str(d.get("meta_sha256") or d.get("meta") or "")
            client = re.sub(r"[^\w./-]", "", str(d.get("client", "")))[:32]
        except Exception:
            log("400", mask_ip(ip), "bad json")
            return self.reply_json(400, {"error": "bad json"})
        if not HEX10.match(sid) or not HEX64.match(digest):
            log("400", mask_ip(ip), "bad fields")
            return self.reply_json(400, {"error": "bad fields"})
        ts = now_ts()
        if v == 5:
            if not HMAC_KEY:
                return self.reply_json(501, {"error": "hmac disabled"})
            log("200", mask_ip(ip), sid, client, "hmac")
            return self.reply_json(
                200, {"ts": ts, "sig": hmac_sign(v, sid, digest, ts), "alg": "hmac-sha256"}
            )
        if v == 6:
            if meta and not HEX64.match(meta):
                return self.reply_json(400, {"error": "bad meta_sha256"})
            sig = ed_sign(v, sid, digest, meta, ts)
            log("200", mask_ip(ip), sid, client, "ed25519")
            return self.reply_json(
                200,
                {
                    "ts": ts,
                    "sig": sig,
                    "alg": "ed25519",
                    "pub": PUB_B64,
                    "verify": f"{VERIFY_BASE}?v=6&id={sid}&sha256={digest}&meta_sha256={meta}&ts={ts}&sig={sig}",
                },
            )
        return self.reply_json(400, {"error": "unsupported v"})


if __name__ == "__main__":
    srv = ThreadingHTTPServer((HOST, PORT), H)
    srv.daemon_threads = True
    log("start", f"{HOST}:{PORT}", "ed25519", ED25519_PATH, "hmac", bool(HMAC_KEY))
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        pass
