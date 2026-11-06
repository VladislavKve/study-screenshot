package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"time"

	"study.local/screenshot/internal/mark"
	"study.local/screenshot/internal/sign"
)

func main() {
	url := flag.String("sign-url", "http://127.0.0.1:8830/sign", "")
	out := flag.String("out", "testdata/out/roundtrip-v6.png", "")
	flag.Parse()
	img := image.NewRGBA(image.Rect(0, 0, 800, 500))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.RGBA{40, 60, 90, 255}}, image.Point{}, draw.Src)
	now := time.Now()
	ts := now.Format("2006-01-02T15:04:05-0700")
	students := []string{"Иванов", "Петров"}
	m := &mark.Mark{
		V: mark.VersionV6, OS: "linux/amd64", Group: "3ИСП-11-41", Students: students,
		Host: "box-test", OSUser: "student", TS: ts, Work: "ПР-тест", Source: "linux",
		Src: mark.Src{App: "stamptest"}, Wins: []string{}, Area: []int{}, AreaWins: []string{},
	}
	m.ID = mark.NewID(students, m.Group, m.Host, ts, img.Pix)
	imgHash := sign.ImageDigest(img.Pix)
	canon, _ := m.Canon()
	metaHash := sign.MetaDigest(canon)
	r, err := sign.Do(*url, sign.Request{V: 6, ID: m.ID, SHA256: imgHash, MetaSHA256: metaHash, Client: "stamptest"}, 5*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sign failed: %v\n", err)
		os.Exit(1)
	}
	sig := r.Sig
	if len(sig) > 12 {
		sig = sig[:12]
	}
	m.Srv = &mark.Srv{TS: r.TS, Sig: sig}
	stamped, js, err := mark.Stamp(img, m, mark.StampInput{Now: now, Signed: true})
	if err != nil {
		panic(err)
	}
	asc, _ := m.ASCIIJSON()
	wl, _, _ := mark.WindowsJSON(nil)
	data, err := mark.EncodePNG(stamped, js, asc, m.ID, wl)
	if err != nil {
		panic(err)
	}
	_ = os.MkdirAll(filepath.Dir(*out), 0o755)
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		panic(err)
	}
	fmt.Println("OK", *out, "id="+m.ID, "alg="+r.Alg)
}
