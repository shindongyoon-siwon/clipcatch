package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExplicitBrowserPostModeOverridesWebButDoesNotMutatePreference(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Phase = "ready"
	a.state.OutputMode = originalMode
	token := browserTestPair(t, a)
	w := browserRequest(a, "/bridge/page", testExtensionOrigin, token, `{"page_url":"https://youtube.com/watch?v=abcdefghijk","output_mode":"premiere"}`)
	if w.Code != 200 {
		t.Fatalf("queue failed: %d %s", w.Code, w.Body.String())
	}
	if len(a.state.Jobs) != 1 || a.state.Jobs[0].OutputMode != premiereMode {
		t.Fatal("explicit MP4 mode not queued")
	}
	if a.state.OutputMode != originalMode {
		t.Fatal("extension overwrote web preference")
	}
	a.state.OutputMode = premiereMode
	w = browserRequest(a, "/bridge/page", testExtensionOrigin, token, `{"page_url":"https://youtube.com/watch?v=lmnopqrstuv","output_mode":"original"}`)
	if w.Code != 200 || a.state.Jobs[0].OutputMode != originalMode {
		t.Fatal("explicit original ignored")
	}
	if a.state.Jobs[1].OutputMode != premiereMode {
		t.Fatal("earlier queued snapshot changed")
	}
	if a.state.OutputMode != premiereMode {
		t.Fatal("original request changed web setting")
	}
}
func TestBrowserMediaModeOverridesAndRejectsUnknown(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Phase = "ready"
	a.state.OutputMode = originalMode
	token := browserTestPair(t, a)
	for i, mode := range []string{premiereMode, originalMode} {
		mediaID := []string{"trialfirst123", "trialsecond123"}[i]
		data, _ := json.Marshal(map[string]any{"page_url": "https://www.reddit.com/comments/abc123/", "media_url": "https://v.redd.it/" + mediaID + "/DASHPlaylist.mpd", "output_mode": mode, "separate": true})
		w := browserRequest(a, "/bridge/download", testExtensionOrigin, token, string(data))
		if w.Code != 202 || a.state.Jobs[0].OutputMode != mode {
			t.Fatalf("media mode %s: %d %s", mode, w.Code, w.Body.String())
		}
	}
	for _, route := range []string{"/bridge/download", "/bridge/page"} {
		w := browserRequest(a, route, testExtensionOrigin, token, `{"page_url":"https://www.reddit.com/comments/abc123/","output_mode":"shell:unsafe"}`)
		if w.Code != 400 {
			t.Fatalf("invalid mode accepted: %s %d", route, w.Code)
		}
	}
	if len(a.state.Jobs) != 2 {
		t.Fatal("invalid request added a job")
	}
}
func TestLegacyBrowserRequestsStillUseWebMode(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.OutputMode = originalMode
	token := browserTestPair(t, a)
	w := browserRequest(a, "/bridge/page", testExtensionOrigin, token, `{"page_url":"https://youtube.com/watch?v=abcdefghijk"}`)
	if w.Code != 200 || a.state.Jobs[0].OutputMode != originalMode {
		t.Fatal("omitted mode not backwards compatible")
	}
}
func TestTrialIsolatedPathsAndUpdaterBlocked(t *testing.T) {
	before := appChannel
	appChannel = "test"
	defer func() { appChannel = before }()
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	t.Setenv("CLIPCATCH_SAVEDIR", filepath.Join(root, "stable-do-not-touch"))
	t.Setenv("CLIPCATCH_TEST2_SAVEDIR", "")
	if runtimeAppID() != "clipcatch-test2" || runtimePort() != "18766" || runtimeDataName() != "ClipCatch-Test2" {
		t.Fatal("trial not isolated")
	}
	if runtimeSaveDir(root) != filepath.Join(root, "Downloads", "ClipCatch-Test2") {
		t.Fatal("stable save override leaked into trial")
	}
	target := installedExePath()
	if !strings.Contains(target, "ClipCatch-Test2") {
		t.Fatal("trial updater points at production")
	}
	a := newApp(t.TempDir(), t.TempDir())
	a.state.AppUpdate.Status = "current"
	w := httptest.NewRecorder()
	a.installAppUpdateHandler(w, httptest.NewRequest("POST", "/api/install-app-update", nil))
	if w.Code != 409 && w.Code != 400 {
		t.Fatal("unchecked install accepted")
	}
	a.downloadAndInstall(verifiedRelease{Version: "1.9.0", BinaryURL: "https://invalid.test"}, target)
	if _, e := os.Stat(filepath.Join(a.dataDir, "updates")); !os.IsNotExist(e) {
		t.Fatal("trial touched update files for a stable release")
	}
	h := httptest.NewRecorder()
	a.mux.ServeHTTP(h, httptest.NewRequest("GET", "/api/health", nil))
	if !strings.Contains(h.Body.String(), `"app":"clipcatch-test2"`) {
		t.Fatal("health cannot distinguish stable/test")
	}
}
func TestTrialPairRequiresTestExtension(t *testing.T) {
	old := appChannel
	appChannel = "test"
	defer func() { appChannel = old }()
	a := newApp(t.TempDir(), t.TempDir())
	w := browserRequest(a, "/bridge/pair", testExtensionOrigin, "", `{"code":"ignored"}`)
	if w.Code != 409 {
		t.Fatalf("production extension not rejected: %d", w.Code)
	}
	code, _ := newBrowserSecret()
	a.browser.CodeDigest = tokenDigest(code)
	a.browser.Expires = time.Now().Add(time.Minute)
	body, _ := json.Marshal(map[string]string{"code": code, "channel": "test"})
	w = browserRequest(a, "/bridge/pair", testExtensionOrigin, "", string(body))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"channel":"test"`) {
		t.Fatalf("trial pair failed: %d %s", w.Code, w.Body.String())
	}
}
