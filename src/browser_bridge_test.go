package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testExtensionOrigin = "chrome-extension://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const testPostURL = "https://www.reddit.com/r/IllegallySmolCats/comments/1x1rb7t/buckle_up_buckaroo/?utm_source=chatgpt.com"
const testManifestURL = "https://v.redd.it/abcdefghijk12/DASHPlaylist.mpd?f=hd&sig=a%2Bb"

func browserRequest(a *App, path, origin, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765"+path, strings.NewReader(body))
	r.Header.Set("Origin", origin)
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	a.serveHTTP(w, r)
	return w
}
func browserTestPair(t *testing.T, a *App) string {
	t.Helper()
	r := browserRequest(a, "/api/browser-link/code", "http://127.0.0.1:8765", "", "{}")
	var c map[string]any
	if json.Unmarshal(r.Body.Bytes(), &c) != nil || r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	r = browserRequest(a, "/bridge/pair", testExtensionOrigin, "", fmt.Sprintf(`{"code":%q}`, c["code"]))
	var out map[string]any
	if json.Unmarshal(r.Body.Bytes(), &out) != nil || r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	token, _ := out["token"].(string)
	if len(token) != 64 {
		t.Fatal("invalid token")
	}
	return token
}
func TestBrowserMediaValidation(t *testing.T) {
	valid := browserMedia{PageURL: testPostURL, MediaURL: testManifestURL, Title: "Buckle up"}
	got, e := validateBrowserMedia(valid)
	if e != nil || strings.Contains(got.PageURL, "utm_") || got.MediaURL != testManifestURL {
		t.Fatalf("bad valid media: %+v %v", got, e)
	}
	bad := []string{"http://v.redd.it/abcdefghijk12/DASHPlaylist.mpd", "https://v.redd.it.evil.test/abcdefghijk12/DASHPlaylist.mpd", "https://user:secret@v.redd.it/abcdefghijk12/DASHPlaylist.mpd", "https://v.redd.it:444/abcdefghijk12/DASHPlaylist.mpd", "https://127.0.0.1/abcdefghijk12/DASHPlaylist.mpd", "https://v.redd.it/abcdefghijk12/../other/DASHPlaylist.mpd", "https://v.redd.it/abcdefghijk12/%44ASHPlaylist.mpd", "https://v.redd.it/abcdefghijk12/DASHPlaylist.mpd#fragment", "file:///tmp/x", "javascript:alert(1)", "https://v.redd.it/abcdefghijk12/file.json"}
	for _, u := range bad {
		v := valid
		v.MediaURL = u
		if _, e := validateBrowserMedia(v); e == nil {
			t.Errorf("accepted %q", u)
		}
	}
	for _, u := range []string{"https://reddit.com/r/cats/", "https://reddit.com.evil.test/r/aww/comments/abc123/test/", "https://x.com/u/status/123456"} {
		v := valid
		v.PageURL = u
		if _, e := validateBrowserMedia(v); e == nil {
			t.Errorf("accepted non-post %s", u)
		}
	}
	v := valid
	v.MediaURL = "https://v.redd.it/abcdefghijk12/DASH_720.mp4"
	if _, e := validateBrowserMedia(v); e == nil {
		t.Fatal("video-only silently allowed")
	}
	v.AllowSilent = true
	if _, e := validateBrowserMedia(v); e != nil {
		t.Fatal(e)
	}
	v.AllowSilent = false
	v.AudioURL = "https://v.redd.it/abcdefghijk12/DASH_AUDIO_128.mp4"
	if _, e := validateBrowserMedia(v); e != nil {
		t.Fatal(e)
	}
	v.AudioURL = "https://v.redd.it/othermedia123/DASH_AUDIO_128.mp4"
	if _, e := validateBrowserMedia(v); e == nil {
		t.Fatal("mixed unrelated audio/video")
	}
}
func TestBrowserPairingIsolationAndPersistence(t *testing.T) {
	dir := t.TempDir()
	a := newApp(dir, t.TempDir())
	token := browserTestPair(t, a)
	content, _ := os.ReadFile(a.browserLinkPath())
	if bytes.Contains(content, []byte(token)) {
		t.Fatal("persisted raw bearer")
	}
	for _, origin := range []string{"https://evil.test", "null", "http://127.0.0.1:8765", testExtensionOrigin + ".evil", ""} {
		w := browserRequest(a, "/bridge/download", origin, token, "{}")
		if w.Code != 403 {
			t.Fatalf("origin %q status %d", origin, w.Code)
		}
	}
	if w := browserRequest(a, "/bridge/download", testExtensionOrigin, "wrong", "{}"); w.Code != 401 {
		t.Fatal("bad key allowed")
	}
	if w := browserRequest(a, "/api/stop", testExtensionOrigin, token, "{}"); w.Code != 403 {
		t.Fatal("extension controlled unrelated privileged API")
	}
	b := newApp(dir, t.TempDir())
	b.state.Phase = "ready"
	payload, _ := json.Marshal(browserMedia{PageURL: testPostURL, MediaURL: testManifestURL, Title: "Kitten"})
	if w := browserRequest(b, "/bridge/download", testExtensionOrigin, token, string(payload)); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(b.state.Jobs) != 1 || !b.state.Jobs[0].BrowserSource {
		t.Fatal("job missing")
	}
	b.state.Jobs[0].Status = "error"
	b.saveHistory()
	hist, _ := os.ReadFile(filepath.Join(dir, "history.json"))
	if bytes.Contains(hist, []byte("v.redd.it")) || bytes.Contains(hist, []byte(token)) {
		t.Fatal("signed media or bearer leaked to history")
	}
	report := buildDiagnosticReport(b.state)
	if strings.Contains(report, token) || strings.Contains(report, "v.redd.it") {
		t.Fatal("secret diagnostic")
	}
	w := browserRequest(b, "/api/browser-link/disconnect", "http://127.0.0.1:8765", "", "{}")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := browserRequest(b, "/bridge/download", testExtensionOrigin, token, string(payload)); w.Code != 401 {
		t.Fatal("revoked key allowed")
	}
}
func TestBrowserPairCodeSingleUseExpiryAndCORS(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	code, _ := newBrowserSecret()
	a.browser.CodeDigest = tokenDigest(code)
	a.browser.Expires = time.Now().Add(time.Minute)
	body := fmt.Sprintf(`{"code":%q}`, code)
	if w := browserRequest(a, "/bridge/pair", testExtensionOrigin, "", body); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := browserRequest(a, "/bridge/pair", testExtensionOrigin, "", body); w.Code != 401 {
		t.Fatal("code reused")
	}
	a.browser.CodeDigest = tokenDigest(code)
	a.browser.Expires = time.Now().Add(-time.Second)
	if w := browserRequest(a, "/bridge/pair", testExtensionOrigin, "", body); w.Code != 401 {
		t.Fatal("expired code allowed")
	}
	r := httptest.NewRequest("OPTIONS", "http://127.0.0.1:8765/bridge/pair", nil)
	r.Header.Set("Origin", testExtensionOrigin)
	r.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	a.serveHTTP(w, r)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != testExtensionOrigin || w.Header().Get("Access-Control-Allow-Origin") == "*" {
		t.Fatal("bad CORS")
	}
	w = browserRequest(a, "/bridge/pair", testExtensionOrigin, "", `{"code":"x"} {"code":"y"}`)
	if w.Code != 400 {
		t.Fatal("trailing JSON allowed")
	}
}
func TestBrowserDownloadArgumentsAndTemporarySecrets(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	j := &Job{ID: "browser1", URL: testPostURL, Host: "www.reddit.com", BrowserSource: true}
	a.browser.Sources[j.ID] = browserMedia{PageURL: testPostURL, Title: "../../Kitten%(.s)|", MediaURL: testManifestURL}
	args, clean, e := a.browserDownloadArgs(j, a.downloadArgs(j))
	if e != nil {
		t.Fatal(e)
	}
	defer clean()
	joined := strings.Join(args, " ")
	if args[len(args)-1] != testManifestURL || strings.Contains(joined, testPostURL) || strings.Contains(joined, "--cookies") {
		t.Fatal("wrong browser path", args)
	}
	if !strings.Contains(joined, "--force-generic-extractor") || !strings.Contains(joined, "--no-overwrites") {
		t.Fatal("missing guard")
	}
	for i, x := range args {
		if x == "-o" {
			if filepath.Base(args[i+1]) != args[i+1] || strings.Contains(args[i+1], "../") {
				t.Fatal("unsafe output")
			}
		}
	}
	media := a.browser.Sources[j.ID]
	media.MediaURL = "https://v.redd.it/abcdefghijk12/DASH_720.mp4"
	media.AudioURL = "https://v.redd.it/abcdefghijk12/DASH_AUDIO_128.mp4"
	a.browser.Sources[j.ID] = media
	args, clean, e = a.browserDownloadArgs(j, a.downloadArgs(j))
	if e != nil {
		t.Fatal(e)
	}
	path := args[len(args)-1]
	if args[len(args)-2] != "--load-info-json" {
		t.Fatal("wrong direct pair path")
	}
	b, e := os.ReadFile(path)
	if e != nil || !bytes.Contains(b, []byte("browser-audio")) {
		t.Fatal("audio missing")
	}
	clean()
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("temporary media file retained")
	}
	delete(a.browser.Sources, j.ID)
	if _, _, e = a.browserDownloadArgs(j, a.downloadArgs(j)); e == nil || !strings.Contains(e.Error(), "CC-312") {
		t.Fatal("expired handoff silently used post extractor")
	}
}
func TestBrowserExtensionInstallationAndPrivacy(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	dir, e := a.installBrowserExtension()
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		t.Fatal("bad manifest")
	}
	for _, bad := range []string{`"cookies"`, `"debugger"`, `<all_urls>`, `"history"`, `"externally_connectable"`} {
		if bytes.Contains(b, []byte(bad)) {
			t.Fatal("excessive permission", bad)
		}
	}
	for _, name := range []string{"core.mjs", "background.js", "capture.js", "popup.js", "popup.html"} {
		if !nonempty(filepath.Join(dir, name)) {
			t.Fatal("missing extension file", name)
		}
	}
}
func TestBrowserProbeRealMuxedAndSilentFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local FFmpeg fixture test on Linux")
	}
	ffmpeg, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, e = exec.LookPath("ffprobe"); e != nil {
		t.Skip("ffprobe not installed")
	}
	d := t.TempDir()
	video := filepath.Join(d, "sample.mp4")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100", "-t", "0.4", "-c:v", "libx264", "-c:a", "aac", "-shortest", video)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("fixture: %v %s", e, out)
	}
	a := newApp(d, d)
	a.ffmpeg = ffmpeg
	v, au, e := a.inspectBrowserOutput(context.Background(), video)
	if e != nil || !v || !au {
		t.Fatalf("bad actual probe %v %v %v", v, au, e)
	}
	silent := filepath.Join(d, "silent.mp4")
	if out, e := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-i", video, "-an", "-c:v", "copy", silent).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	v, au, e = a.inspectBrowserOutput(context.Background(), silent)
	if e != nil || !v || au {
		t.Fatal("silent file falsely contains audio")
	}
}
