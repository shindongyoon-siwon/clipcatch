package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyManualInstallerMustBeOfficial(t *testing.T) {
	tmp := t.TempDir()
	exe := filepath.Join(tmp, "ClipCatch.exe")
	data := []byte("MZ" + strings.Repeat("x", 300))
	if err := os.WriteFile(exe, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	correct := hex.EncodeToString(sum[:])
	rel := verifiedRelease{Version: appVersion}
	if err := verifyManualInstaller(exe, rel, correct); err != nil {
		t.Fatalf("official exe denied: %v", err)
	}
	if err := verifyManualInstaller(exe, rel, strings.Repeat("a", 64)); err == nil {
		t.Fatal("tampered executable accepted")
	}
	rel.Version = "99.0.0"
	if err := verifyManualInstaller(exe, rel, correct); err == nil {
		t.Fatal("different release version accepted")
	}
	rel.Version = appVersion
	if err := os.WriteFile(exe, []byte("ZZwrong executable"), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := sha256File(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyManualInstaller(exe, rel, a); err == nil {
		t.Fatal("non-PE executable accepted")
	}
}

func TestInstalledBinaryMatches(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "download.exe")
	b := filepath.Join(dir, "install.exe")
	_ = os.WriteFile(a, []byte("same"), 0600)
	_ = os.WriteFile(b, []byte("same"), 0600)
	if !installedBinaryMatches(a, b) {
		t.Fatal("same binary considered different")
	}
	_ = os.WriteFile(b, []byte("different"), 0600)
	if installedBinaryMatches(a, b) {
		t.Fatal("different binary considered identical")
	}
}

func TestManualRepairPreservesActiveDownload(t *testing.T) {
	app := newApp(t.TempDir(), t.TempDir())
	app.state.Jobs = []*Job{{Status: "downloading"}}
	server := httptest.NewServer(http.HandlerFunc(app.serveHTTP))
	defer server.Close()
	if err := stopExistingInstanceForUpgradeAt(server.URL); err == nil || !strings.Contains(err.Error(), "다운로드") {
		t.Fatalf("should prevent stopping active job: %v", err)
	}
	// Confirm that the /api/stop endpoint was not called and state remains accessible.
	resp, err := http.Get(server.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "downloading") {
		t.Fatal("active state lost")
	}
}

func TestManualRepairSafeStop(t *testing.T) {
	app := newApp(t.TempDir(), t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(app.serveHTTP))
	defer server.Close()
	if err := stopExistingInstanceForUpgradeAt(server.URL); err != nil {
		t.Fatalf("safe app stop failed: %v", err)
	}
}

func TestUIUpdateStatusIsVisible(t *testing.T) {
	data, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"id=\"appCheckFeedback\"", "프로그램 업데이트 확인", "최신 버전 v"} {
		if !strings.Contains(string(data), fragment) {
			t.Fatalf("missing update feedback %q", fragment)
		}
	}
}
