package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type clipRoundTripper func(*http.Request) (*http.Response, error)

func (f clipRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestClipTimesExact(t *testing.T) {
	for raw, want := range map[string]float64{"0": 0, "12.345": 12.345, "01:02.500": 62.5, "1:02:03": 3723} {
		got, e := clipTime(raw)
		if e != nil || got != want {
			t.Fatalf("%s = %v %v", raw, got, e)
		}
	}
	for _, bad := range []string{"", "-1", "1:60", "1:02:60", "NaN", "1e3", " 2", "1.2:03", "1:2:3:4", "0.1234"} {
		if _, e := clipTime(bad); e == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
func TestClipDomainBoundaries(t *testing.T) {
	for _, good := range []string{"https://rr1.googlevideo.com/videoplayback?a=1", "https://video.twimg.com/a.mp4", "https://v.redd.it/abc/1.mp4", "https://s.cdninstagram.com/a"} {
		if !clipMediaURL(good) {
			t.Fatal(good)
		}
	}
	for _, bad := range []string{"http://video.twimg.com/a", "https://twimg.com.evil.test/a", "https://user:pass@v.redd.it/a", "https://localhost/a", "https://127.0.0.1/a", "https://v.redd.it:8443/a"} {
		if clipMediaURL(bad) {
			t.Fatal(bad)
		}
	}
}
func TestClipServerIgnoringRangeRejected(t *testing.T) {
	reads := atomic.Int64{}
	c := &http.Client{Transport: clipRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: &clipCountingReader{reader: strings.NewReader(strings.Repeat("x", 9000)), count: &reads}}, nil
	})}
	if _, e := clipRangeProbe(context.Background(), c, "https://v.redd.it/a.mp4", http.Header{}); e == nil {
		t.Fatal("200 must fail")
	}
	if reads.Load() != 0 {
		t.Fatal("read the complete-response body", reads.Load())
	}
}

type clipCountingReader struct {
	reader io.Reader
	count  *atomic.Int64
}

func (r *clipCountingReader) Read(p []byte) (int, error) {
	n, e := r.reader.Read(p)
	r.count.Add(int64(n))
	return n, e
}
func (r *clipCountingReader) Close() error { return nil }
func TestClipRangeGateRechecksAndBlocksFullResponse(t *testing.T) {
	reads := atomic.Int64{}
	calls := 0
	c := &http.Client{Transport: clipRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: 206, Header: http.Header{"Content-Range": []string{"bytes 1-1/1000000"}}, Body: io.NopCloser(strings.NewReader("x")), ContentLength: 1}, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: &clipCountingReader{reader: strings.NewReader(strings.Repeat("x", 10000)), count: &reads}}, nil
	})}
	n, e := clipRangeProbe(context.Background(), c, "https://v.redd.it/a.mp4", http.Header{})
	if e != nil {
		t.Fatal(e)
	}
	g, e := newClipGate(context.Background(), c)
	if e != nil {
		t.Fatal(e)
	}
	defer g.close()
	u := g.add(&clipResource{URL: "https://v.redd.it/a.mp4", Headers: http.Header{}, Size: n, limit: n * 8 / 10}, ".mp4")
	resp, e := http.Get(u)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 409 || g.failure() == nil || reads.Load() != 0 {
		t.Fatal("allowed full response", resp.StatusCode, reads.Load())
	}
}
func TestClipHLSNoWholePlaylistFallback(t *testing.T) {
	text := "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2,\na.ts\n#EXTINF:2,\nb.ts\n#EXTINF:2,\nc.ts\n#EXT-X-ENDLIST\n"
	p, e := clipParsePlaylist(text, "https://video.twimg.com/v/index.m3u8")
	if e != nil {
		t.Fatal(e)
	}
	segs, e := clipSubset(p, 2.25, 3.5)
	if e != nil || len(segs) != 1 || !strings.HasSuffix(segs[0].URL, "b.ts") {
		t.Fatal(segs, e)
	}
	if _, e = clipSubset(p, 0.1, 5.9); e == nil {
		t.Fatal("must not fetch all segments")
	}
	for _, bad := range []string{strings.Replace(text, "#EXT-X-ENDLIST", "", 1), "#EXT-X-KEY:METHOD=AES-128\n" + text, strings.Replace(text, "b.ts", "https://127.0.0.1/private", 1)} {
		if _, e = clipParsePlaylist(bad, "https://video.twimg.com/v/index.m3u8"); e == nil {
			t.Fatal("unsafe manifest accepted")
		}
	}
}
func TestClipQualityAndDuplicateIdentity(t *testing.T) {
	m := clipMetadata{Formats: []clipFormat{{ID: "v1", URL: "https://video.twimg.com/a", Width: 1920, Height: 1080, VCodec: "h264", ACodec: "aac"}, {ID: "v2", URL: "https://video.twimg.com/b", Width: 720, Height: 1280, VCodec: "h264", ACodec: "aac"}}}
	q := clipQualities(m)
	if fmt.Sprint(q) != "[1080 720]" {
		t.Fatal(q)
	}
	f, _, e := clipSelect(m, 720)
	if e != nil || f.ID != "v2" {
		t.Fatal(f, e)
	}
	if _, _, e = clipSelect(m, 480); e == nil {
		t.Fatal("silently changed selected quality")
	}
	a := &Job{URL: "https://youtube.com/watch?v=123", Clip: &clipSpec{Start: 1, End: 3, Quality: 720}, FormatSelector: "v1"}
	b := cloneDownloadJob(a)
	b.Clip.Start = 2
	if a.Clip.Start != 1 || downloadJobKey(a) == downloadJobKey(&b) {
		t.Fatal("time alias/duplicate collision")
	}
	raw, _ := json.Marshal(a)
	if strings.Contains(string(raw), "Token") || strings.Contains(string(raw), "googlevideo") {
		t.Fatal(string(raw))
	}
}
func TestSectionCandidateIsolation(t *testing.T) {
	old := appChannel
	appChannel = "section-test"
	defer func() { appChannel = old }()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("CLIPCATCH_SAVEDIR", "/production")
	t.Setenv("CLIPCATCH_TEST2_SAVEDIR", "/old-test")
	if runtimeAppID() != "clipcatch-section-test" || runtimePort() != "18767" || runtimeDataName() != "ClipCatch-SectionTest" || strings.Contains(runtimeSaveDir("/home"), "production") {
		t.Fatal("not isolated")
	}
	if validateBuildIdentity(buildIdentity{App: runtimeAppID(), Channel: appChannel, Version: "2.1.0-test.1"}) != nil {
		t.Fatal("identity")
	}
	a := newApp(t.TempDir(), t.TempDir())
	if check, _ := a.beginAppUpdateCheck(true, true); check != nil {
		t.Fatal("automatic update must be off")
	}
	w := httptest.NewRecorder()
	a.installAppUpdateHandler(w, httptest.NewRequest("POST", "/api/install-app-update", nil))
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
	b, _ := pages.ReadFile("browser_extension/background.js")
	s := string(sectionExtensionAsset("background.js", b))
	if !strings.Contains(s, "app!=='clipcatch-section-test'") || !strings.Contains(s, "port!=='18767'") {
		t.Fatal("helper cross-channel")
	}
}
func TestClipPlanOwnerAndExpiryAndNoFallback(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Phase = "ready"
	a.state.AppUpdate.Status = "current"
	a.clips.plans["p"] = &clipPlan{Token: "p", URL: "https://www.youtube.com/watch?v=BaW_jenozKc", Owner: "other-owner", At: time.Now(), Meta: clipMetadata{Duration: 60}, Choices: map[int]clipChoice{720: {Quality: 720, Status: "supported", Selector: "f1"}}}
	payload := `{"token":"p","quality":720,"partial":true,"start":"10","end":"15","output_mode":"premiere","allow_duplicates":false}`
	w := httptest.NewRecorder()
	a.clipQueue(w, httptest.NewRequest("POST", "/api/clip-queue", strings.NewReader(payload)))
	if w.Code != 409 || len(a.state.Jobs) != 0 {
		t.Fatal("owner bypass")
	}
	a.clips.plans["p"].Owner = "web"
	a.clips.plans["p"].Choices[720] = clipChoice{Status: "unsupported", Notice: "range", Selector: "f1"}
	w = httptest.NewRecorder()
	a.clipQueue(w, httptest.NewRequest("POST", "/api/clip-queue", strings.NewReader(payload)))
	if w.Code != 409 || len(a.state.Jobs) != 0 {
		t.Fatal("partial fell back to full")
	}
}

// Real FFmpeg + real loopback gate + instrumented origin fixture, NOT live
// YouTube/Instagram/X/Reddit. Enabled on Linux and native Windows CI.
func TestClipActualMediaNoFullTransfer(t *testing.T) {
	if os.Getenv("CLIPCATCH_SECTION_MEDIA_TEST") != "1" {
		t.Skip("set CLIPCATCH_SECTION_MEDIA_TEST=1")
	}
	ff, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "long.mp4")
	args := []string{"-hide_banner", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=24:duration=90", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=90", "-c:v", "libx264", "-preset", "ultrafast", "-crf", "18", "-g", "48", "-threads", "2", "-c:a", "aac", "-movflags", "+faststart", "-shortest", "-y", source}
	if out, e := exec.Command(ff, args...).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	data, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	n := int64(len(data))
	received := atomic.Int64{}
	client := &http.Client{Transport: clipRoundTripper(func(r *http.Request) (*http.Response, error) {
		var a, b int64
		if _, e := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &a, &b); e != nil || a < 0 || b < a || b >= n {
			t.Errorf("unbounded origin request: %s", r.Header.Get("Range"))
			return nil, errClipRange
		}
		received.Add(b - a + 1)
		return &http.Response{StatusCode: 206, Header: http.Header{"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", a, b, n)}}, Body: io.NopCloser(bytes.NewReader(data[a : b+1])), ContentLength: b - a + 1, Request: r}, nil
	})}
	a := newApp(filepath.Join(dir, "settings"), filepath.Join(dir, "out"))
	a.ffmpeg = ff
	a.clips.client = client
	fmtV := clipFormat{ID: "v1", URL: "https://fixture.googlevideo.com/long.mp4", Ext: "mp4", Protocol: "https", Width: 640, Height: 360, FPS: 24, VCodec: "h264", ACodec: "aac"}
	p := &clipPlan{Token: "fixture", Owner: "web", At: time.Now(), Meta: clipMetadata{Title: "known-source", Duration: 90}, Choices: map[int]clipChoice{360: {Quality: 360, Status: "supported", Video: clipInput{Format: fmtV, Size: n}, Selector: "v1"}}}
	a.clips.plans[p.Token] = p
	a.state.Jobs = []*Job{{ID: "section-fixture", URL: "https://www.youtube.com/watch?v=fixture", Status: "queued", Task: "section", Clip: &clipSpec{Start: 60.25, End: 62.75, Quality: 360, Token: p.Token}, OutputMode: "premiere"}}
	started := time.Now()
	a.runDownload("section-fixture")
	j := a.getJob("section-fixture")
	if j.Status != "done" {
		t.Fatalf("job=%+v transferred=%d/%d", j, received.Load(), n)
	}
	info, e := inspectEditMedia(context.Background(), ff, j.File)
	if e != nil {
		t.Fatal(e)
	}
	duration, _ := strconv.ParseFloat(info.Format.Duration, 64)
	if duration < 2.45 || duration > 2.65 {
		t.Fatalf("wrong duration %f", duration)
	}
	if received.Load() >= n {
		t.Fatalf("whole source transferred: %d/%d", received.Load(), n)
	}
	t.Logf("actual clip %.3fs; origin bytes %d/%d; elapsed %.3fs", duration, received.Load(), n, time.Since(started).Seconds())
}

func TestClipActualHLSOnlySelectedSegments(t *testing.T) {
	if os.Getenv("CLIPCATCH_SECTION_MEDIA_TEST") != "1" {
		t.Skip("real media opt-in")
	}
	ff, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	manifest := filepath.Join(dir, "index.m3u8")
	args := []string{"-hide_banner", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=30", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=30", "-c:v", "libx264", "-preset", "ultrafast", "-g", "48", "-sc_threshold", "0", "-threads", "2", "-c:a", "aac", "-f", "hls", "-hls_time", "2", "-hls_list_size", "0", "-hls_playlist_type", "vod", "-hls_segment_filename", filepath.Join(dir, "seg%03d.ts"), "-y", manifest}
	if out, e := exec.Command(ff, args...).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	data, e := os.ReadFile(manifest)
	if e != nil {
		t.Fatal(e)
	}
	list, e := clipParsePlaylist(string(data), "https://video.twimg.com/parts/index.m3u8")
	if e != nil {
		t.Fatal(e)
	}
	requested := map[string]int{}
	var mu sync.Mutex
	client := &http.Client{Transport: clipRoundTripper(func(r *http.Request) (*http.Response, error) {
		name := filepath.Base(r.URL.Path)
		mu.Lock()
		requested[name]++
		mu.Unlock()
		b, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil {
			return nil, e
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b)), Request: r}, nil
	})}
	a := newApp(filepath.Join(dir, "config"), filepath.Join(dir, "out"))
	a.ffmpeg = ff
	a.clips.client = client
	f := clipFormat{ID: "hls", URL: "https://video.twimg.com/parts/index.m3u8", Ext: "mp4", Protocol: "m3u8_native", Width: 320, Height: 180, FPS: 24, VCodec: "h264", ACodec: "aac"}
	a.clips.plans["hls"] = &clipPlan{Token: "hls", Owner: "web", At: time.Now(), Meta: clipMetadata{Title: "hls-fixture", Duration: 30}, Choices: map[int]clipChoice{180: {Status: "supported", Video: clipInput{Format: f, List: list}}}}
	a.state.Jobs = []*Job{{ID: "hls-job", Status: "queued", Task: "section", URL: "https://x.com/example/status/123", Clip: &clipSpec{Start: 10.25, End: 12.75, Quality: 180, Token: "hls"}}}
	a.runDownload("hls-job")
	j := a.getJob("hls-job")
	if j.Status != "done" {
		t.Fatalf("HLS job=%+v", j)
	}
	wanted, _ := clipSubset(list, 10.25, 12.75)
	allowed := map[string]bool{}
	for _, s := range wanted {
		u, _ := url.Parse(s.URL)
		allowed[filepath.Base(u.Path)] = true
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requested) != len(allowed) {
		t.Fatal(requested, allowed)
	}
	for name := range requested {
		if !allowed[name] {
			t.Fatal("unselected segment fetched", name)
		}
	}
	t.Logf("selected %d/%d segments; output verified", len(requested), len(list.Segments))
}

func TestSectionHelperDoesNotPairWithProduction(t *testing.T) {
	old := appChannel
	appChannel = "section-test"
	defer func() { appChannel = old }()
	for _, name := range []string{"background.js", "popup.js", "manifest.json"} {
		raw, e := pages.ReadFile("browser_extension/" + name)
		if e != nil {
			t.Fatal(e)
		}
		got := string(sectionExtensionAsset(name, raw))
		if name == "background.js" {
			for _, part := range []string{"channel:'section-test'", "app!=='clipcatch-section-test'", "port!=='18767'"} {
				if !strings.Contains(got, part) {
					t.Fatal("missing isolated helper identity", part)
				}
			}
			if strings.Contains(got, "channel:'stable'") {
				t.Fatal("production channel retained")
			}
		}
		if name == "manifest.json" && !strings.Contains(got, "ClipCatch Browser SECTION TEST") {
			t.Fatal("helper label")
		}
	}
}
