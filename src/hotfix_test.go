package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHotfixAuthenticatedPopupProgress(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Phase = "ready"
	token := browserTestPair(t, a)
	w := browserRequest(a, "/bridge/page", testExtensionOrigin, token, `{"page_url":"https://www.youtube.com/watch?v=abcdefghijk","output_mode":"premiere"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var accepted struct {
		IDs []string `json:"job_ids"`
	}
	if json.Unmarshal(w.Body.Bytes(), &accepted) != nil || len(accepted.IDs) != 1 {
		t.Fatal("missing queued ID", w.Body.String())
	}
	id := accepted.IDs[0]
	if a.state.Jobs[0].BrowserOwner == "" {
		t.Fatal("missing ownership")
	}
	a.state.Jobs = append(a.state.Jobs, &Job{ID: "other-job", File: "C:/secret/customer.mp4", URL: "https://secret.test/?token=private", Status: "done"})
	a.mutateJob(id, func(j *Job) { j.Status = "downloading"; j.Stage = "converting"; j.Percent = 42 })
	body, _ := json.Marshal(map[string]any{"ids": []string{id, "other-job"}})
	w = browserRequest(a, "/bridge/status", testExtensionOrigin, token, string(body))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var info struct {
		Jobs []browserJobView `json:"jobs"`
	}
	if json.Unmarshal(w.Body.Bytes(), &info) != nil || len(info.Jobs) != 1 || info.Jobs[0].Stage != "converting" || info.Jobs[0].Percent != 42 {
		t.Fatal("incorrect live progress", w.Body.String())
	}
	for _, secret := range []string{"C:/secret", "secret.test", "private", a.browser.Link.Digest, token} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("status leaked private state")
		}
	}
	one, two := filepath.Join(a.saveDir, "a.mp4"), filepath.Join(a.saveDir, "b.mp4")
	os.WriteFile(one, []byte("saved fixture"), 0600)
	os.WriteFile(two, []byte("saved fixture"), 0600)
	a.mutateJob(id, func(j *Job) {
		j.Status = "done"
		j.Stage = "ready"
		j.Media = []savedPostMedia{{File: one, EditReady: true}, {File: two, EditReady: true}}
	})
	w = browserRequest(a, "/bridge/status", testExtensionOrigin, token, string(body))
	json.Unmarshal(w.Body.Bytes(), &info)
	if info.Jobs[0].Status != "done" || info.Jobs[0].FileCount != 2 {
		t.Fatal("two-file completion not reported", w.Body.String())
	}
	a.mutateJob(id, func(j *Job) { j.Status = "error"; j.ErrorCode = "CC-PARTIAL"; j.Media[1].Error = "failed" })
	w = browserRequest(a, "/bridge/status", testExtensionOrigin, token, string(body))
	json.Unmarshal(w.Body.Bytes(), &info)
	if info.Jobs[0].Status != "error" || info.Jobs[0].FileCount != 1 {
		t.Fatal("partial failure falsely complete")
	}
}
func TestHotfixStatusAccessBoundaries(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	token := browserTestPair(t, a)
	for _, route := range []string{"/bridge/status", "/bridge/open-folder"} {
		if w := browserRequest(a, route, testExtensionOrigin, "invalid", `{}`); w.Code != http.StatusUnauthorized {
			t.Fatal("unauthorized endpoint", route, w.Code)
		}
		if w := browserRequest(a, route, "https://evil.test", token, `{}`); w.Code != 403 {
			t.Fatal("foreign origin", route, w.Code)
		}
	}
	w := browserRequest(a, "/bridge/open-folder", testExtensionOrigin, token, `{"path":"C:/private"}`)
	if w.Code != 400 {
		t.Fatal("arbitrary folder accepted")
	}
	w = browserRequest(a, "/bridge/status", testExtensionOrigin, token, `{"ids":[],"token":"other"}`)
	if w.Code != 400 {
		t.Fatal("unknown status fields accepted")
	}
}
func TestHotfixBundledAIExtractNoNetwork(t *testing.T) {
	if len(bundledQualityEngine) == 0 {
		t.Skip("requires bundled_ai build tag")
	}
	root := t.TempDir()
	// Invalid proxy would fail any external request; bundled install still succeeds.
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	if e := prepareQualityEngine(context.Background(), root); e != nil {
		t.Fatal(e)
	}
	exe, e := verifyQualityEngine(root)
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(exe)
	if e != nil || len(b) < 2 || string(b[:2]) != "MZ" {
		t.Fatal("not the Windows engine")
	}
	if e := prepareQualityEngine(context.Background(), root); e != nil {
		t.Fatal("repeated preparation failed", e)
	}
	for _, name := range []string{"input.jpg", "input2.jpg", "onepiece_demo.mp4"} {
		if _, e := os.Stat(filepath.Join(filepath.Dir(exe), name)); !os.IsNotExist(e) {
			t.Fatal("unnecessary sample extracted")
		}
	}
	if _, e := os.Stat(filepath.Join(filepath.Dir(exe), "LICENSE-Real-ESRGAN.txt")); e != nil {
		t.Fatal("missing license")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := obtainQualityArchive(ctx, filepath.Join(root, "cancel.zip")); !errors.Is(e, context.Canceled) {
		t.Fatal("cancel ignored")
	}
}
func TestHotfixSameSizeAIAndFailureDoNotResize(t *testing.T) {
	ff := testQualityFF(t)
	dir := t.TempDir()
	source := testQualityImage(t, dir, false)
	q, e := describeQualitySource(context.Background(), ff, source)
	if e != nil {
		t.Fatal(e)
	}
	o := qualityOptions{Mode: "ai", Scale: 1, Strength: .65}
	if e := validateQualityOptions(q, o); e != nil {
		t.Fatal(e)
	}
	dest := filepath.Join(dir, "no-ai.png")
	if e := renderQualityImage(context.Background(), ff, "/no/such/engine", source, dest, dir, o, q.Width, q.Height); e == nil {
		t.Fatal("missing AI unexpectedly succeeded")
	}
	if _, e := os.Stat(dest); !os.IsNotExist(e) {
		t.Fatal("AI failure silently produced a resize")
	}
}
func TestHotfixErrorCategoriesNotGenericPolicyBlame(t *testing.T) {
	for _, code := range []string{"quality-gpu", "quality-disk", "quality-integrity", "quality-download-network", "quality-download-http-403"} {
		e := errors.New(code)
		if qualityErrorCode(e) == "CC-QUALITY" || qualityMessage(e) == "" {
			t.Fatal("lost diagnostic", code)
		}
	}
}
