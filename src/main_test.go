package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCleanURL(t *testing.T) {
	u, host, err := cleanURL("https://x.com/KaptanHindustan/status/2106785096879792408?s=20")
	if err != nil || host != "x.com" || !strings.Contains(u, "?s=20") {
		t.Fatal(u, host, err)
	}
	fail := []string{
		"file:///C:/foo", "http://localhost:8080", "http://127.0.0.1:80/a",
		"http://192.168.0.1/a", "http://host.local/a", "http://fake.internal/a", "http://u:pw@example.com/foo", "https://foo.com:1234/file",
	}
	for _, raw := range fail {
		if _, _, err = cleanURL(raw); err == nil {
			t.Errorf("should reject %q", raw)
		}
	}
}

func TestHTTPAdd(t *testing.T) {
	tmp := t.TempDir()
	app := newApp(tmp, filepath.Join(tmp, "downloads"))
	body := `{"urls":"https://x.com/account/status/111\nhttps://www.reddit.com/r/funny/comments/abc/xyz"}`
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/download", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:8765")
	w := httptest.NewRecorder()
	app.serveHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765/api/state", nil)
	w = httptest.NewRecorder()
	app.serveHTTP(w, req)
	var data ClientState
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Jobs) != 2 || data.Jobs[0].Host != "www.reddit.com" || data.Jobs[1].Host != "x.com" {
		t.Fatalf("bad queue: %#v", data.Jobs)
	}
}

func TestHTTPRejectCrossOrigin(t *testing.T) {
	app := newApp(t.TempDir(), t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/download", strings.NewReader(`{"urls":"https://x.com/a/status/1"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://attacker.example")
	w := httptest.NewRecorder()
	app.serveHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %v", w.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/download", strings.NewReader(`{"urls":"https://x.com/a/status/1"}`))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Origin", "http://127.0.0.1:8765")
	w = httptest.NewRecorder()
	app.serveHTTP(w, req)
	if w.Code != 415 {
		t.Fatalf("expected 415, got %v", w.Code)
	}
}

func TestRunnerOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub test on Linux")
	}
	tmp := t.TempDir()
	tool := filepath.Join(tmp, "fake-dlp")
	shell := `#!/bin/sh
printf 'CLIP_TITLE|귀여운 고양이\n'
printf 'CLIP_PROGRESS| 49.8%%|2.4MiB/s|00:03\n'
printf 'CLIP_PROGRESS|100.0%%|2.4MiB/s|00:00\n'
printf 'CLIP_FILE|/tmp/cat_video.mp4\n'
exit 0
`
	if err := os.WriteFile(tool, []byte(shell), 0755); err != nil {
		t.Fatal(err)
	}
	app := newApp(tmp, filepath.Join(tmp, "downloads"))
	app.ytDlp = tool
	j := &Job{ID: "id1", URL: "https://x.com/a/status/1", Status: "queued"}
	app.state.Jobs = []*Job{j}
	app.runDownload("id1")
	got := app.getJob("id1")
	if got.Status != "done" || got.Percent != 100 || got.Title != "귀여운 고양이" || got.File != "/tmp/cat_video.mp4" {
		t.Fatalf("bad result: %+v", got)
	}
}

func TestVerifiedDownload(t *testing.T) {
	content := []byte("fake tool bytes")
	checksum := sha256.Sum256(content)
	testserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "15")
		w.Write(content)
	}))
	defer testserver.Close()
	target := filepath.Join(t.TempDir(), "yt-dlp.exe")
	if err := downloadVerified(testserver.URL, target, hex.EncodeToString(checksum[:]), func(now, total int64) {}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(b, content) {
		t.Fatal(err, string(b))
	}
	err = downloadVerified(testserver.URL, target, "0000000000000000000000000000000000000000000000000000000000000000", func(now, total int64) {})
	if err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestExtractFFmpeg(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "ff.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, entry := range []string{"ffmpeg-test-essentials_build/bin/ffmpeg.exe", "ffmpeg-test-essentials_build/bin/ffprobe.exe", "../../badfile.exe"} {
		w, err := zw.Create(entry)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, "test")
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if err := extractFFmpeg(archive, tmp); err != nil {
		t.Fatal(err)
	}
	if !nonempty(filepath.Join(tmp, "ffmpeg.exe")) || !nonempty(filepath.Join(tmp, "ffprobe.exe")) {
		t.Fatal("missing extracted bin")
	}
	if nonempty(filepath.Join(tmp, "badfile.exe")) {
		t.Fatal("unsafe extraction")
	}
}

func TestUpdateBlockedWhenQueued(t *testing.T) {
	app := newApp(t.TempDir(), t.TempDir())
	app.state.Phase = "ready"
	app.state.Jobs = []*Job{{ID: "queued", Status: "queued"}}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/update-tool", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:8765")
	w := httptest.NewRecorder()
	app.serveHTTP(w, req)
	if w.Code != 409 {
		t.Fatalf("want 409, got %d", w.Code)
	}
}

// Security regression tests for the corporate-integrated Reddit-only build.

func TestCorporateRestrictedDomains(t *testing.T) {
	allowed := []string{"https://www.reddit.com/r/aww/comments/id/title", "https://x.com/user/status/123", "https://youtu.be/abc", "https://m.youtube.com/watch?v=abc", "https://www.instagram.com/reel/abc"}
	denied := []string{"https://youtube.com.attacker.com/a", "https://reddit.com.evil.example/video", "https://intranet.company.test/video.mp4", "https://127.0.0.1/video.mp4", "https://evil.example/redirect", "https://www.reddit.com:444/a"}
	for _, u := range allowed {
		if _, _, err := cleanURL(u); err != nil {
			t.Errorf("should allow %s: %v", u, err)
		}
	}
	for _, u := range denied {
		if _, _, err := cleanURL(u); err == nil {
			t.Errorf("should reject %s", u)
		}
	}
}
func TestCorporateNoBrowserCredentials(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	args := strings.Join(a.downloadArgs(&Job{URL: "https://www.reddit.com/r/aww/comments/abc/cat"}), " ")
	for _, bad := range []string{"--cookies", "--cookies-from-browser", "--exec", "--config-location"} {
		if strings.Contains(args, bad) {
			t.Fatalf("unsafe argument %q in %q", bad, args)
		}
	}
	for _, api := range []string{"/api/upload-cookies", "/api/clear-cookies"} {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765"+api, nil)
		req.Header.Set("Origin", "http://127.0.0.1:8765")
		w := httptest.NewRecorder()
		a.serveHTTP(w, req)
		if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s active: %d", api, w.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/download", strings.NewReader(`{"urls":"https://www.reddit.com/r/aww/comments/abc/cat","auth_browser":"chrome"}`))
	req.Header.Set("Origin", "http://127.0.0.1:8765")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.serveHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unexpected legacy cookie input acceptance: %d", w.Code)
	}
}
func TestCorporateRejectMissingOrigin(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/stop", nil)
	w := httptest.NewRecorder()
	a.serveHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing Origin was allowed: %d", w.Code)
	}
}
func TestCorporateHTML(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765/", nil)
	w := httptest.NewRecorder()
	a.serveHTTP(w, req)
	html := w.Body.String()
	if w.Code != 200 || !strings.Contains(html, "ClipCatch") || !strings.Contains(html, "v"+appVersion) {
		t.Fatalf("safe page error %d", w.Code)
	}
	for _, forbidden := range []string{"authBrowser", "--cookies-from-browser", "cookies-session-"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("forbidden UI element: %s", forbidden)
		}
	}
}
func TestCorporateLoginErrorHonest(t *testing.T) {
	msg := friendlyError("ERROR: [Reddit] 1witx57: Account authentication is required")
	if !strings.Contains(msg, "로그인") || !strings.Contains(msg, "cookies.txt") {
		t.Fatal(msg)
	}
}
func TestFaviconRoutes(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	for url, typ := range map[string]string{"/favicon.svg": "image/svg+xml"} {
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765"+url, nil)
		w := httptest.NewRecorder()
		a.serveHTTP(w, req)
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), typ) {
			t.Errorf("%s: %d %s", url, w.Code, w.Header().Get("Content-Type"))
		}
	}
}
