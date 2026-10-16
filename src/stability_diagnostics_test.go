package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestErrorClassificationsGiveClearAction(t *testing.T) {
	cases := []struct{ raw, host, code string }{
		{"ERROR: unable to download video data: ('Connection aborted.', RemoteDisconnected('Remote end closed connection without response'))", "reddit.com", "CC-101"},
		{"HTTP Error 429: Too Many Requests", "reddit.com", "CC-102"},
		{"Account authentication is required", "www.reddit.com", "CC-103"},
		{"ERROR: HTTP Error 403: Forbidden", "x.com", "CC-104"},
		{"ERROR: video unavailable", "youtube.com", "CC-105"},
		{"ERROR: Unsupported URL", "reddit.com", "CC-106"},
		{"ERROR: ffmpeg not found", "youtube.com", "CC-107"},
		{"permission denied", "youtube.com", "CC-108"},
		{"Missing JavaScript runtime: please install Deno", "youtube.com", "CC-109"},
		{"Something very unusual happened", "vimeo.com", "CC-199"},
	}
	for _, tc := range cases {
		code, title, advice := classifyDownloadError(tc.raw, tc.host)
		if code != tc.code || title == "" || advice == "" {
			t.Errorf("raw %q => %s %s %s", tc.raw, code, title, advice)
		}
	}
}
func TestEngineSwapKeepsPreviousAndRollbackWorks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "yt-dlp.exe")
	next := filepath.Join(dir, "yt-dlp-next.exe")
	backup := filepath.Join(dir, "yt-dlp-backup.exe")
	for file, contents := range map[string]string{target: "MZstable", next: "MZnightly", backup: "MZprevious"} {
		if err := os.WriteFile(file, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := replaceEngineFiles(target, next, backup); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "MZnightly" {
		t.Fatalf("wrong target: %s", data)
	}
	data, _ = os.ReadFile(backup)
	if string(data) != "MZstable" {
		t.Fatalf("wrong rollback: %s", data)
	}
	if err := rollbackEngineFiles(target, backup); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(target)
	if string(data) != "MZstable" {
		t.Fatalf("rollback failed: %s", data)
	}
	data, _ = os.ReadFile(backup)
	if string(data) != "MZnightly" {
		t.Fatalf("new version not backed up: %s", data)
	}
}
func TestEngineMissingDownloadNeverDeletesWorkingFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "yt-dlp.exe")
	backup := filepath.Join(dir, "yt-dlp-backup.exe")
	_ = os.WriteFile(target, []byte("existing good binary"), 0600)
	_ = os.WriteFile(backup, []byte("older good binary"), 0600)
	if err := replaceEngineFiles(target, filepath.Join(dir, "missing"), backup); err == nil {
		t.Fatal("missing replacement accepted")
	}
	for file, contents := range map[string]string{target: "existing good binary", backup: "older good binary"} {
		data, _ := os.ReadFile(file)
		if string(data) != contents {
			t.Errorf("%s destroyed on failure", file)
		}
	}
}
func TestEngineChannelMetadataPersists(t *testing.T) {
	dir := t.TempDir()
	app := newApp(dir, filepath.Join(dir, "downloads"))
	meta := EngineMetadata{Current: "nightly", Backup: "stable"}
	if err := app.persistEngineMetadata(meta); err != nil {
		t.Fatal(err)
	}
	reloaded := newApp(dir, filepath.Join(dir, "downloads"))
	if reloaded.engineMeta.Current != "nightly" || reloaded.engineMeta.Backup != "stable" {
		t.Fatalf("metadata lost: %+v", reloaded.engineMeta)
	}
}
func TestDiagnosticsReportNeverLeaksUserData(t *testing.T) {
	secret := "SensitiveSecretA1B2C3"
	st := ClientState{Version: appVersion, Phase: "ready", Engine: EngineInfo{Channel: "stable", Status: "current"}, Diagnostics: DiagnosticState{Status: "error", Checks: []DiagnosticCheck{{Name: "저장 폴더", Status: "error", Message: secret}}}, Jobs: []*Job{{Status: "error", URL: "https://reddit.com/?token=" + secret, File: "C:\\Users\\" + secret + "\\video.mp4", Detail: secret, ErrorCode: "CC-101"}}}
	report := buildDiagnosticReport(st)
	for _, bad := range []string{secret, "reddit.com", "Users", "token", "video.mp4"} {
		if strings.Contains(report, bad) {
			t.Fatalf("diagnostic report leaked %s: %s", bad, report)
		}
	}
	if !strings.Contains(report, "CC-101") || !strings.Contains(report, appVersion) {
		t.Fatalf("report lacks useful information: %s", report)
	}
}
func TestDiagnosticsEndpointAndSameOrigin(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Jobs = []*Job{{Status: "error", ErrorCode: "CC-108", URL: "https://reddit.com/?private=secret"}}
	get := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765/api/diagnostic-report", nil)
	w := httptest.NewRecorder()
	a.serveHTTP(w, get)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var res map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res["report"], "CC-108") || strings.Contains(res["report"], "private=secret") {
		t.Fatal(res)
	}
	post := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/run-diagnostics", strings.NewReader("{}"))
	post.Header.Set("Origin", "https://evil.test")
	w = httptest.NewRecorder()
	a.serveHTTP(w, post)
	if w.Code != 403 {
		t.Fatalf("cross origin accepted: %d", w.Code)
	}
}
func TestRepairControlsUI(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765/", nil)
	a.serveHTTP(w, req)
	html := w.Body.String()
	for _, term := range []string{"id=\"runDiagnostics\"", "id=\"helpPanel\"", "id=\"repairEngineNightly\"", "id=\"rollbackEngine\"", "id=\"copyDiagnostics\"", "/api/run-diagnostics", "/api/repair-engine", "/api/rollback-engine", "/api/diagnostic-report", "문제 해결 열기"} {
		if !strings.Contains(html, term) {
			t.Errorf("missing UI feature %q", term)
		}
	}
}

func TestRestoreEngineAfterInterruptedReplace(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "yt-dlp.exe")
	backup := filepath.Join(dir, "yt-dlp-backup.exe")
	previous := backup + ".previous"
	if err := os.WriteFile(backup, []byte("MZold"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(previous, []byte("MZolder"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := recoverInterruptedEngineFiles(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dest)
	if string(b) != "MZold" {
		t.Errorf("no restoration: %q", b)
	}
	b, _ = os.ReadFile(backup)
	if string(b) != "MZolder" {
		t.Errorf("previous backup lost: %q", b)
	}
}
