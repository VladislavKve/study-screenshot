package sign_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"study.local/screenshot/internal/sign"
)

func pythonBin(t *testing.T) string {
	t.Helper()
	var cands []string
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			cands = append(cands, p)
		}
	}
	for _, p := range cands {
		cmd := exec.Command(p, "-c", "from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey")
		if cmd.Run() == nil {
			return p
		}
	}
	if len(cands) == 0 {
		t.Skip("нет python3/python")
	}
	t.Skip("python без cryptography (pip install cryptography)")
	return ""
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return port
}

func ensureKeys(t *testing.T, dir string) (ed, hmac string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	ed = filepath.Join(dir, "ed25519.seed")
	hmac = filepath.Join(dir, "hmac.key")
	for _, p := range []string{ed, hmac} {
		if st, err := os.Stat(p); err == nil && st.Size() >= 32 {
			_ = os.Chmod(p, 0o600)
			continue
		}
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return ed, hmac
}

func TestE2EHashSignVerify(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("e2e sign-server spawn is flaky on macOS CI runners")
	}
	py := pythonBin(t)
	root := filepath.Join("..", "..", "server")
	script := filepath.Join(root, "study-sign-server.py")
	if _, err := os.Stat(script); err != nil {
		t.Skip("server script missing")
	}
	ed, hmacKey := ensureKeys(t, filepath.Join(root, "keys"))
	port := freePort(t)
	env := append(os.Environ(),
		"STUDY_SIGN_PORT="+port,
		"STUDY_SIGN_HOST=127.0.0.1",
		"STUDY_SIGN_ED25519_KEY="+ed,
		"STUDY_SIGN_KEY_FILE="+hmacKey,
		"STUDY_SIGN_RELAX_PERMS=1",
		"PYTHONUTF8=1",
		"PYTHONIOENCODING=utf-8",
	)
	var stderr bytes.Buffer
	cmd := exec.Command(py, script)
	cmd.Env = env
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	deadline := time.Now().Add(20 * time.Second)
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
			t.Fatalf("server not up; stderr=%s", stderr.String())
		}
		time.Sleep(100 * time.Millisecond)
	}

	pixels := []byte("fake-rgba-pixels-for-test-image")
	imgHash := sign.ImageDigest(pixels)
	meta := []byte(`{"group":"3ISP","work":"PR1"}`)
	mh := sha256.Sum256(meta)
	metaHash := hex.EncodeToString(mh[:])
	id := "abcdef0123"

	r, err := sign.Do("http://127.0.0.1:"+port+"/sign", sign.Request{
		V: 6, ID: id, SHA256: imgHash, MetaSHA256: metaHash, Client: "e2e/test",
	}, 5*time.Second)
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
	resp2, err := http.Post("http://127.0.0.1:"+port+"/verify", "application/json", bytes.NewReader(body2))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	data2, _ := io.ReadAll(resp2.Body)
	var vr2 struct {
		OK bool `json:"ok"`
	}
	_ = json.Unmarshal(data2, &vr2)
	if vr2.OK {
		t.Fatal("expected verify fail on tamper")
	}
	_ = runtime.GOOS
}
