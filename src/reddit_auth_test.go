package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func redditCookiesFixture() string {
	return "# Netscape HTTP Cookie File\n" +
		".google.com\tTRUE\t/\tTRUE\t1893456000\tSID\tgoogle-secret\n" +
		"#HttpOnly_.reddit.com\tTRUE\t/\tTRUE\t1893456000\treddit_session\treddit-secret\n" +
		".reddit.com.evil.org\tTRUE\t/\tTRUE\t1893456000\tfoo\tevildoer\n"
}
func TestRedditCookieFilter(t *testing.T) {
	clean, err := keepOnlyRedditCookies([]byte(redditCookiesFixture()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(clean, []byte("reddit_session\treddit-secret")) {
		t.Fatal("reddit session stripped")
	}
	for _, secret := range []string{"google-secret", "evildoer", "google.com", "evil.org"} {
		if bytes.Contains(clean, []byte(secret)) {
			t.Fatal("other-site secret retained: " + secret)
		}
	}
	if _, err := keepOnlyRedditCookies([]byte("# Netscape HTTP Cookie File\n.google.com\tTRUE\t/\tTRUE\t1\tSID\tfoo\n")); err == nil {
		t.Fatal("accepted non-reddit credentials")
	}
}
func TestRedditCookieUploadAndClear(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	body := new(bytes.Buffer)
	mw := multipart.NewWriter(body)
	fp, err := mw.CreateFormFile("cookies", "reddit-cookies.txt")
	if err != nil {
		t.Fatal(err)
	}
	fp.Write([]byte(redditCookiesFixture()))
	mw.WriteField("remember", "no")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/upload-reddit-cookies", body)
	req.Header.Set("Origin", "http://127.0.0.1:8765")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	out := httptest.NewRecorder()
	a.serveHTTP(out, req)
	if out.Code != 200 || !a.state.RedditAuthReady {
		t.Fatalf("upload failed: %d %s", out.Code, out.Body.String())
	}
	if strings.Contains(string(a.copyRedditCookies()), "google-secret") {
		t.Fatal("non-reddit cookies retained")
	}
	if _, err := os.Stat(a.cookieStorePath()); err == nil {
		t.Fatal("unencrypted / unexpected persistent cookies")
	}
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/clear-reddit-cookies", strings.NewReader("{}"))
	req.Header.Set("Origin", "http://127.0.0.1:8765")
	out = httptest.NewRecorder()
	a.serveHTTP(out, req)
	if out.Code != 200 || a.state.RedditAuthReady || len(a.copyRedditCookies()) > 0 {
		t.Fatal("cookie clearing failed")
	}
}
func TestRedditAuthOnlyForRedditAndDeleted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub uses Unix shell")
	}
	tmp := t.TempDir()
	exe := filepath.Join(tmp, "fake-dlp")
	cookieDir := filepath.Join(tmp, "cookies-used")
	script := "#!/bin/sh\n" +
		"while [ $# -gt 0 ]; do\n" +
		" if [ \"$1\" = \"--cookies\" ]; then\n" +
		"  test -f \"$2\" || exit 12\n" +
		"  grep -q reddit_session \"$2\" || exit 14\n" +
		"  grep -q google-secret \"$2\" && exit 16\n" +
		"  cp \"$2\" \"" + cookieDir + "\"\n" +
		"  printf 'CLIP_TITLE|test\\n'; printf 'CLIP_FILE|/tmp/test.mp4\\n'; exit 0\n" +
		" fi\n" +
		" shift\n" +
		"done\nexit 10\n"
	if err := os.WriteFile(exe, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	a := newApp(tmp, tmp)
	a.ytDlp = exe
	sanitized, _ := keepOnlyRedditCookies([]byte(redditCookiesFixture()))
	if err, _ := a.runDownloadWithRedditCookies("id", &Job{URL: "https://reddit.com/r/aww/comments/a/test", Host: "reddit.com"}, sanitized); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cookieDir); err != nil {
		t.Fatal("cookie not passed to reddit")
	}
	if files, _ := filepath.Glob(filepath.Join(tmp, "reddit-auth-session-*.txt")); len(files) > 0 {
		t.Fatal("secret temp file not cleaned")
	}
	if _, msg := a.runDownloadWithRedditCookies("id", &Job{URL: "https://youtube.com/watch?v=a", Host: "youtube.com"}, sanitized); msg != "" {
		t.Fatal("unexpected secret path")
	}
}
func TestRedditAndUpdateControlsExist(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765/", nil)
	w := httptest.NewRecorder()
	a.serveHTTP(w, req)
	for _, text := range []string{"redditCookieFile", "rememberRedditAuth", "/api/upload-reddit-cookies", "installAppUpdate", "/api/install-app-update"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatalf("missing control: %s", text)
		}
	}
	if strings.Contains(w.Body.String(), "authBrowser") {
		t.Fatal("Chrome-wide authentication reenabled")
	}
}
