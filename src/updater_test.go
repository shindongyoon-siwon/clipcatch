package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func releaseFixture(version string) githubRelease {
	var r githubRelease
	r.TagName = "v" + version
	r.Assets = []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
		Digest             string `json:"digest"`
	}{
		{Name: "ClipCatch.exe", BrowserDownloadURL: "https://github.com/" + clipCatchReleaseRepo + "/releases/download/v" + version + "/ClipCatch.exe", Size: 4000000, Digest: "sha256:" + strings.Repeat("a", 64)},
		{Name: "SHA256SUMS", BrowserDownloadURL: "https://github.com/" + clipCatchReleaseRepo + "/releases/download/v" + version + "/SHA256SUMS", Size: 80},
	}
	return r
}
func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.3.0", "1.3.1", -1}, {"1.3.0", "1.3.0", 0}, {"1.10.0", "1.9.9", 1}, {"2.0.0", "1.99.99", 1},
	}
	for _, tt := range cases {
		if got := compareSemver(tt.a, tt.b); got != tt.want {
			t.Errorf("compareSemver(%s,%s)=%d want %d", tt.a, tt.b, got, tt.want)
		}
	}
	if _, err := parseSemver("v1.2.x"); err == nil {
		t.Fatal("invalid version accepted")
	}
}
func TestReleaseParseSafety(t *testing.T) {
	r := releaseFixture("1.3.1")
	got, err := parseGithubRelease(r)
	if err != nil || got.Version != "1.3.1" || got.Digest != strings.Repeat("a", 64) || got.BinaryURL == "" {
		t.Fatalf("got %+v, %v", got, err)
	}
	r.Assets[0].BrowserDownloadURL = "https://evil.example/download.exe"
	if _, err := parseGithubRelease(r); err == nil {
		t.Fatal("release from wrong host allowed")
	}
	r = releaseFixture("1.3.1")
	r.Assets[0].Size = 12
	if _, err := parseGithubRelease(r); err == nil {
		t.Fatal("tiny file accepted")
	}
	r = releaseFixture("1.3.1")
	r.Assets[0].Digest = ""
	r.Assets = r.Assets[:1]
	if _, err := parseGithubRelease(r); err == nil {
		t.Fatal("missing checksum accepted")
	}
	r = releaseFixture("1.3.1")
	r.Prerelease = true
	if _, err := parseGithubRelease(r); err == nil {
		t.Fatal("prerelease accepted")
	}
}
func TestReadAppChecksum(t *testing.T) {
	h := strings.Repeat("1", 64)
	got, err := readAppChecksum(h + "  ClipCatch.exe\n")
	if err != nil || got != h {
		t.Fatalf("hash=%s err=%v", got, err)
	}
	if _, err := readAppChecksum(h + "  wrong.exe\n"); err == nil {
		t.Fatal("wrong filename accepted")
	}
}
func TestAppUpdateEndpoint(t *testing.T) {
	app := newApp(t.TempDir(), t.TempDir())
	app.setUpdate(UpdateInfo{Status: "available", Latest: "1.3.1", Detail: "새 버전"})
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765/api/state", nil)
	w := httptest.NewRecorder()
	app.serveHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	var s ClientState
	if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil || s.AppUpdate.Status != "available" {
		t.Fatalf("state %+v err %v", s.AppUpdate, err)
	}
	r = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/install-app-update", strings.NewReader("{}"))
	r.Header.Set("Origin", "http://127.0.0.1:8765")
	w = httptest.NewRecorder()
	app.serveHTTP(w, r)
	if w.Code == http.StatusOK {
		t.Fatal("should not install update without Windows and registered release")
	}
}
func TestAppHTMLHasUpdateAndVersion(t *testing.T) {
	app := newApp(t.TempDir(), t.TempDir())
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765/", nil)
	w := httptest.NewRecorder()
	app.serveHTTP(w, r)
	html := w.Body.String()
	for _, term := range []string{"v" + appVersion, "id=\"installAppUpdate\"", "/api/install-app-update", "/api/check-app-update"} {
		if !strings.Contains(html, term) {
			t.Fatalf("UI missing %q", term)
		}
	}
	if strings.Contains(html, "__APP_VERSION__") {
		t.Fatal("unfilled app version placeholder")
	}
}
