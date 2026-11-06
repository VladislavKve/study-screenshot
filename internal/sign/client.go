// Package sign — заверение снимка сервером.
// v5: HMAC-SHA256[:12]; v6: Ed25519 (base64url). На сервер уходят только хеши.
package sign

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

// Request — тело POST /sign.
type Request struct {
	V          int    `json:"v"`
	ID         string `json:"id"`
	SHA256     string `json:"sha256"`                // хеш изображения (сырые пиксели или PNG до штампа)
	MetaSHA256 string `json:"meta_sha256,omitempty"` // sha256 канонических метаданных
	Client     string `json:"client,omitempty"`
}

// Response — ответ сервера.
type Response struct {
	TS     string `json:"ts"`
	Sig    string `json:"sig"`
	Alg    string `json:"alg,omitempty"`
	Pub    string `json:"pub,omitempty"`
	Verify string `json:"verify,omitempty"`
	Err    string `json:"error,omitempty"`
}

var (
	hex12 = regexp.MustCompile(`^[0-9a-f]{12}$`)
	b64re = regexp.MustCompile(`^[A-Za-z0-9_-]{40,}$`)
)

// ImageDigest — SHA-256 сырых RGBA-пикселей (как NewID).
func ImageDigest(pixels []byte) string {
	h := sha256.Sum256(pixels)
	return hex.EncodeToString(h[:])
}

// MetaDigest — SHA-256 канонического JSON метаданных.
func MetaDigest(canon []byte) string {
	h := sha256.Sum256(canon)
	return hex.EncodeToString(h[:])
}

// Do отправляет хеши; ошибка — нет сети / сервер отказал.
func Do(url string, req Request, timeout time.Duration) (*Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	c := &http.Client{Timeout: timeout}
	resp, err := c.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	var r Response
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("ответ сервера %d: %s", resp.StatusCode, bytes.TrimSpace(data))
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("сервер: %d %s", resp.StatusCode, r.Err)
	}
	if r.TS == "" || r.Sig == "" {
		return nil, fmt.Errorf("сервер вернул неполный ответ")
	}
	if req.V == 5 && !hex12.MatchString(r.Sig) {
		return nil, fmt.Errorf("неверная HMAC-подпись")
	}
	if req.V == 6 && !b64re.MatchString(r.Sig) {
		return nil, fmt.Errorf("неверная Ed25519-подпись")
	}
	return &r, nil
}
