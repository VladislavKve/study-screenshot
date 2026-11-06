package sign_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"study.local/screenshot/internal/sign"
)

func TestE2EHashSignVerify(t *testing.T) {
	root := filepath.Join("..", "..", "server")
	script := filepath.Join(root, "study-sign-server.py")
	if _, err := os.Stat(script); err != nil {
		t.Skip("server script missing")
	}
	port := "18831"
	env := append(os.Environ(),
		"STUDY_SIGN_PORT="+port,
		"STUDY_SIGN_HOST=127.0.0.1",
		"STUDY_SIGN_ED25519_KEY="+filepath.Join(root, "keys", "ed25519.seed"),
		"STUDY_SIGN_KEY_FILE="+filepath.Join(root, "keys", "hmac.key"),
	)
	cmd := exec.Command("python3", script)
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get("http://127.0.0.1:" + port + "/health")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("server not up")
		}
		time.Sleep(50 * time.Millisecond)
	}

	pixels := []byte("fake-rgba-pixels-for-test-image")
	imgHash := sign.ImageDigest(pixels)
	meta := []byte(`{"group":"3ISP","work":"PR1"}`)
	mh := sha256.Sum256(meta)
	metaHash := hex.EncodeToString(mh[:])
	id := "abcdef0123"

	r, err := sign.Do("http://127.0.0.1:"+port+"/sign", sign.Request{
		V: 6, ID: id, SHA256: imgHash, MetaSHA256: metaHash, Client: "e2e/test",
	}, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if r.Alg != "ed25519" || r.Sig == "" {
		t.Fatalf("bad response %+v", r)
	}

	body, _ := json.Marshal(map[string]any{
		"v": 6, "id": id, "sha256": imgHash, "meta_sha256": metaHash, "ts": r.TS, "sig": r.Sig,
	})
	resp, err := http.Post("http://127.0.0.1:"+port+"/verify", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var vr struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(data, &vr); err != nil || !vr.OK {
		t.Fatalf("verify failed: %s", data)
	}

	body2, _ := json.Marshal(map[string]any{
		"v": 6, "id": id, "sha256": sign.ImageDigest([]byte("other")), "meta_sha256": metaHash, "ts": r.TS, "sig": r.Sig,
	})
	resp2, _ := http.Post("http://127.0.0.1:"+port+"/verify", "application/json", bytes.NewReader(body2))
	defer resp2.Body.Close()
	data2, _ := io.ReadAll(resp2.Body)
	var vr2 struct {
		OK bool `json:"ok"`
	}
	_ = json.Unmarshal(data2, &vr2)
	if vr2.OK {
		t.Fatal("expected verify fail on tamper")
	}
}
