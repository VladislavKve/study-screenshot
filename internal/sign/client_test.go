package sign_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"study.local/screenshot/internal/sign"
)

func TestImageDigest(t *testing.T) {
	d := sign.ImageDigest([]byte("abc"))
	want := sha256.Sum256([]byte("abc"))
	if d != hex.EncodeToString(want[:]) {
		t.Fatalf("digest mismatch")
	}
}

func TestDoV6AgainstMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req sign.Request
		if err := json.Unmarshal(body, &req); err != nil || req.V != 6 {
			http.Error(w, `{"error":"bad"}`, 400)
			return
		}
		_ = json.NewEncoder(w).Encode(sign.Response{
			TS:  "2026-11-06T12:00:00+0300",
			Sig: strings.Repeat("A", 86),
			Alg: "ed25519",
			Pub: "pub",
		})
	}))
	defer srv.Close()
	r, err := sign.Do(srv.URL, sign.Request{
		V: 6, ID: "0123456789", SHA256: strings.Repeat("a", 64),
		MetaSHA256: strings.Repeat("b", 64), Client: "test",
	}, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if r.Alg != "ed25519" || len(r.Sig) < 40 {
		t.Fatalf("bad resp %+v", r)
	}
}
