package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserPageHandoffUsesExistingQueue(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Phase = "ready"
	token := browserTestPair(t, a)
	for _, link := range []string{"https://www.youtube.com/watch?v=abcdefghijk&utm_source=share", "https://www.instagram.com/reel/AbCdEF0123/", "https://x.com/cats/status/1234567890123456789"} {
		body, _ := json.Marshal(map[string]any{"page_url": link})
		w := browserRequest(a, "/bridge/page", testExtensionOrigin, token, string(body))
		if w.Code != 200 {
			t.Fatalf("could not queue %s: %d %s", link, w.Code, w.Body.String())
		}
	}
	if len(a.state.Jobs) != 3 {
		t.Fatalf("wanted 3 queued posts, got %d", len(a.state.Jobs))
	}
	for _, j := range a.state.Jobs {
		if strings.Contains(j.URL, "utm_source") {
			t.Fatal("tracking not removed")
		}
		if j.BrowserSource {
			t.Fatal("ordinary page must use post extractor")
		}
	}
	if !strings.Contains(a.state.Jobs[2].URL, "youtube.com/watch?v=abcdefghijk") {
		t.Fatal("wrong queued link")
	}
}
func TestBrowserPageHandoffAuthAndAllowlist(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	token := browserTestPair(t, a)
	good := `{"page_url":"https://www.youtube.com/watch?v=abcdefghijk"}`
	if w := browserRequest(a, "/bridge/page", testExtensionOrigin, "wrong", good); w.Code != 401 {
		t.Fatal("invalid bearer accepted")
	}
	if w := browserRequest(a, "/bridge/page", "https://attacker.example", token, good); w.Code != 403 {
		t.Fatal("foreign origin accepted")
	}
	for _, bad := range []string{`{"page_url":"http://127.0.0.1:9999/internal"}`, `{"page_url":"https://example.com/other"}`, `{"page_url":"https://user:pass@youtube.com/watch?v=abcdefghijk"}`, `{"page_url":"http://www.youtube.com/watch?v=abcdefghijk"}`, `{"page_url":"https://www.youtube.com/watch?v=abcdefghijk","unused":true}`} {
		if w := browserRequest(a, "/bridge/page", testExtensionOrigin, token, bad); w.Code != 400 {
			t.Fatalf("bad request: %s %d", bad, w.Code)
		}
	}
	if len(a.state.Jobs) != 0 {
		t.Fatal("rejected request queued")
	}
}
func TestMultiExtensionEmbeddedAndRefreshed(t *testing.T) {
	root := t.TempDir()
	a := newApp(root, t.TempDir())
	folder, err := a.installBrowserExtension()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(folder, "manifest.json"))
	if err != nil || !strings.Contains(string(manifest), `"version": "1.9.4"`) {
		t.Fatalf("embedded extension not found: %s %v", manifest, err)
	}
	original, err := os.ReadFile(filepath.Join(folder, "background.js"))
	if err != nil || !strings.Contains(string(original), `'/bridge/page'`) {
		t.Fatal("missing multisite handoff")
	}
	if data, e := os.ReadFile(filepath.Join(folder, "discover.js")); e != nil || len(data) == 0 {
		t.Fatal("feed discovery script not bundled")
	}
	if err := os.WriteFile(filepath.Join(folder, "manifest.json"), []byte(`{"version":"old"}`), 0600); err != nil {
		t.Fatal(err)
	}
	_ = newApp(root, t.TempDir())
	after, err := os.ReadFile(filepath.Join(folder, "manifest.json"))
	if err != nil || !strings.Contains(string(after), `"version": "1.9.4"`) {
		t.Fatal("installed extension not refreshed")
	}
}
