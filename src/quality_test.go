package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testQualityImage(t *testing.T, dir string, transparent bool) string {
	t.Helper()
	p := filepath.Join(dir, "input.png")
	im := image.NewNRGBA(image.Rect(0, 0, 31, 23))
	for y := 0; y < 23; y++ {
		for x := 0; x < 31; x++ {
			a := uint8(255)
			if transparent && x < 5 {
				a = 0
			}
			im.SetNRGBA(x, y, color.NRGBA{uint8(x * 7), uint8(y * 9), 70, a})
		}
	}
	f, e := os.Create(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = png.Encode(f, im); e != nil {
		t.Fatal(e)
	}
	f.Close()
	return p
}
func testQualityFF(t *testing.T) string {
	t.Helper()
	if os.Getenv("CLIPCATCH_QUALITY_TEST") != "1" {
		t.Skip("real media opt-in")
	}
	ff, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Fatal(e)
	}
	return ff
}
func TestQualityOptionGuards(t *testing.T) {
	q := qualitySource{Kind: "video", Width: 1920, Height: 1080, Duration: 1000}
	good := qualityOptions{Mode: "ai", Scale: 2, Strength: .35, Seconds: 10}
	if e := validateQualityOptions(q, good); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []qualityOptions{{Mode: "ai", Scale: 4, Strength: .35, Seconds: 10}, {Mode: "ai", Scale: 2, Strength: .9, Seconds: 10}, {Mode: "resize", Scale: 2, Seconds: 121}, {Mode: "resize", Scale: 2, Start: 999, Seconds: 2}, {Mode: "remote", Scale: 2, Seconds: 1}} {
		if validateQualityOptions(q, bad) == nil {
			t.Fatal("bad options accepted", bad)
		}
	}
}
func TestQualityArchiveGuard(t *testing.T) {
	for _, name := range []string{"../bad.exe", "/absolute.exe", "C:/evil.exe", "folder/../../evil.exe", "folder\\..\\evil.exe"} {
		dir := t.TempDir()
		p := filepath.Join(dir, "x.zip")
		f, _ := os.Create(p)
		z := zip.NewWriter(f)
		w, _ := z.Create(name)
		w.Write([]byte("test"))
		z.Close()
		f.Close()
		if extractQualityEngine(p, filepath.Join(dir, "out")) == nil {
			t.Fatal("unsafe archive accepted", name)
		}
	}
	for _, u := range []string{"http://github.com/test", "https://github.com.evil.test/test", "https://user@github.com/test", "https://127.0.0.1/x", "https://github.com:1234/x"} {
		if qualityDownloadAllowed(u) {
			t.Fatal(u)
		}
	}
}
func TestQualityPreviewRequiredAndSourceChange(t *testing.T) {
	ff := testQualityFF(t)
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Phase = "ready"
	a.ffmpeg = ff
	source := testQualityImage(t, t.TempDir(), false)
	q, e := describeQualitySource(context.Background(), ff, source)
	if e != nil {
		t.Fatal(e)
	}
	a.qualitySources[q.ID] = q
	payload := map[string]any{"source": q.ID, "options": qualityOptions{Mode: "resize", Scale: 2}, "preview": false}
	b, _ := json.Marshal(payload)
	w := browserRequest(a, "/api/quality/start", "http://127.0.0.1:8765", "", string(b))
	if w.Code != 409 {
		t.Fatal("save before preview allowed", w.Code, w.Body.String())
	}
	payload["preview"] = true
	b, _ = json.Marshal(payload)
	w = browserRequest(a, "/api/quality/start", "http://127.0.0.1:8765", "", string(b))
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	id := a.state.Jobs[0].ID
	a.runDownload(id)
	j := a.getJob(id)
	if j.Status != "done" || !nonempty(j.File) || !nonempty(j.Quality.Before) {
		t.Fatalf("preview not complete: %+v", j)
	}
	payload["preview"] = false
	payload["approved_preview"] = id
	b, _ = json.Marshal(payload)
	w = browserRequest(a, "/api/quality/start", "http://127.0.0.1:8765", "", string(b))
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	id = a.state.Jobs[0].ID
	a.runDownload(id)
	j = a.getJob(id)
	if j.Status != "done" || !strings.HasSuffix(j.File, "_resize2x.png") {
		t.Fatalf("save failed %+v", j)
	}
	if !nonempty(source) {
		t.Fatal("original missing")
	}
	a.saveHistory()
	if len(a.downloaded) != 0 {
		t.Fatal("upscaled file polluted download dedup index")
	}
	os.Chtimes(source, time.Now().Add(time.Second), time.Now().Add(time.Second))
	payload["preview"] = true
	b, _ = json.Marshal(payload)
	w = browserRequest(a, "/api/quality/start", "http://127.0.0.1:8765", "", string(b))
	if w.Code != 409 {
		t.Fatal("changed source accepted")
	}
}
func TestQualityRealResizeImageAndAlpha(t *testing.T) {
	ff := testQualityFF(t)
	for _, alpha := range []bool{false, true} {
		for _, factor := range []int{2, 4} {
			t.Run(string(rune('0'+factor)), func(t *testing.T) {
				dir := t.TempDir()
				source := testQualityImage(t, dir, alpha)
				before, _ := qualityFileHash(source)
				q, e := describeQualitySource(context.Background(), ff, source)
				if e != nil {
					t.Fatal(e)
				}
				if q.Width != 31 || q.Height != 23 {
					t.Fatal("image padded or squeezed", q)
				}
				o := qualityOptions{Mode: "resize", Scale: factor}
				out := filepath.Join(dir, "out.png")
				if e = renderQualityImage(context.Background(), ff, "", source, out, dir, o, q.Width*factor, q.Height*factor); e != nil {
					t.Fatal(e)
				}
				if e = validateQualityOutput(context.Background(), ff, out, q, o); e != nil {
					t.Fatal(e)
				}
				after, _ := qualityFileHash(source)
				if before != after {
					t.Fatal("source changed")
				}
				if alpha && !pngHasTransparency(out) {
					t.Fatal("alpha lost")
				}
			})
		}
	}
}
func qualityFixtureVideo(t *testing.T, ff, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "fixture.mp4")
	out, e := exec.Command(ff, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=32x24:rate=10", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "0.5", "-c:v", "libx264", "-threads", "1", "-c:a", "aac", p).CombinedOutput()
	if e != nil {
		t.Fatal(e, string(out))
	}
	return p
}
func TestQualityRealVideo(t *testing.T) {
	ff := testQualityFF(t)
	dir := t.TempDir()
	path := qualityFixtureVideo(t, ff, dir)
	q, e := describeQualitySource(context.Background(), ff, path)
	if e != nil {
		t.Fatal(e)
	}
	o := qualityOptions{Mode: "resize", Scale: 2, Start: .1, Seconds: .3}
	out := filepath.Join(dir, "resized.mp4")
	before, _ := qualityFileHash(path)
	if e = renderQualityVideo(context.Background(), ff, "", q, o, out, dir, nil); e != nil {
		t.Fatal(e)
	}
	if e = validateQualityOutput(context.Background(), ff, out, q, o); e != nil {
		t.Fatal(e)
	}
	after, _ := qualityFileHash(path)
	if before != after {
		t.Fatal("source changed")
	}
}
func TestQualityRealAIImageAndVideo(t *testing.T) {
	engine := os.Getenv("CLIPCATCH_AI_ENGINE_TEST")
	if engine == "" {
		t.Skip("requires official real engine; never a mock inference claim")
	}
	ff := testQualityFF(t)
	dir := t.TempDir()
	source := testQualityImage(t, dir, false)
	q, e := describeQualitySource(context.Background(), ff, source)
	if e != nil {
		t.Fatal(e)
	}
	o := qualityOptions{Mode: "ai", Scale: 2, Strength: .35}
	out := filepath.Join(dir, "image-ai.png")
	if e = renderQualityImage(context.Background(), ff, engine, source, out, dir, o, q.Width*2, q.Height*2); e != nil {
		t.Fatal("AI image", e)
	}
	if e = validateQualityOutput(context.Background(), ff, out, q, o); e != nil {
		t.Fatal(e)
	}
	// Exercise actual frame inference, original audio reattachment and CFR output.
	vdir := t.TempDir()
	v := qualityFixtureVideo(t, ff, vdir)
	q, e = describeQualitySource(context.Background(), ff, v)
	if e != nil {
		t.Fatal(e)
	}
	o.Seconds = .2
	out = filepath.Join(vdir, "video-ai.mp4")
	if e = renderQualityVideo(context.Background(), ff, engine, q, o, out, vdir, nil); e != nil {
		t.Fatal("AI video", e)
	}
	if e = validateQualityOutput(context.Background(), ff, out, q, o); e != nil {
		t.Fatal(e)
	}
}
