package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func featurePost(a *App, path, data string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765"+path, strings.NewReader(data))
	req.Header.Set("Origin", "http://127.0.0.1:8765")
	req.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	a.serveHTTP(out, req)
	return out
}

func TestCanonicalVideoKeys(t *testing.T) {
	pairs := [][2]string{
		{"https://redd.it/abcdef", "https://www.reddit.com/r/aww/comments/ABCDEF/post?utm_source=share"},
		{"https://twitter.com/a/status/12345?s=20", "https://x.com/b/status/12345"},
		{"https://www.youtube.com/watch?v=abcDEF123&t=10", "https://youtu.be/abcDEF123"},
	}
	for _, pair := range pairs {
		if a, b := canonicalJobKey(pair[0]), canonicalJobKey(pair[1]); a != b {
			t.Errorf("not same: %q versus %q", a, b)
		}
	}
	if canonicalJobKey("https://reddit.com/r/aww/comments/abc111") == canonicalJobKey("https://reddit.com/r/aww/comments/abc222") {
		t.Fatal("different Reddit posts merged")
	}
}

func TestDuplicateDownloadRejectThenExplicitCopy(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(video, []byte("original video stays intact"), 0600); err != nil {
		t.Fatal(err)
	}
	original := "https://www.reddit.com/r/cats/comments/abc123/my_cat"
	a := newApp(dir, filepath.Join(dir, "downloads"))
	a.state.Jobs = []*Job{{ID: "done-1", URL: original, Status: "done", File: video, Title: "Kitten", Time: "2026-10-16 18:00"}}
	a.saveHistory()
	// Persistence must prevent duplicates even after the history list is trimmed.
	b := newApp(dir, filepath.Join(dir, "downloads"))
	b.state.Jobs = nil
	other := "https://www.reddit.com/r/dogs/comments/abc123/renamed?utm_source=test"
	response := featurePost(b, "/api/download", fmt.Sprintf(`{"urls":%q}`, other))
	if response.Code != 409 || !strings.Contains(response.Body.String(), "duplicates") {
		t.Fatalf("duplicate should need user confirmation, got %d %s", response.Code, response.Body.String())
	}
	response = featurePost(b, "/api/download", fmt.Sprintf(`{"urls":%q,"allow_duplicates":true}`, other))
	if response.Code != 200 {
		t.Fatalf("explicit redownload should be allowed: %s", response.Body.String())
	}
	if len(b.state.Jobs) != 1 || !b.state.Jobs[0].Redownload {
		t.Fatalf("should be a second COPY, got %+v", b.state.Jobs)
	}
	args := strings.Join(b.downloadArgs(b.state.Jobs[0]), " ")
	if !strings.Contains(args, b.state.Jobs[0].ID) {
		t.Fatal("copy output name should be unique: " + args)
	}
	read, _ := os.ReadFile(video)
	if string(read) != "original video stays intact" {
		t.Fatal("original file overwritten")
	}
	// If the previously downloaded file was deleted, a normal download is allowed.
	_ = os.Remove(video)
	c := newApp(dir, filepath.Join(dir, "downloads"))
	response = featurePost(c, "/api/download", fmt.Sprintf(`{"urls":%q}`, other))
	if response.Code != 200 {
		t.Fatalf("deleted file should not block, got %s", response.Body.String())
	}
}

func TestQueuedDuplicateSkipped(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	u := "https://x.com/alice/status/12345"
	first := featurePost(a, "/api/download", fmt.Sprintf(`{"urls":%q}`, u))
	if first.Code != 200 {
		t.Fatal(first.Body.String())
	}
	second := featurePost(a, "/api/download", fmt.Sprintf(`{"urls":%q}`, "https://twitter.com/bob/status/12345?s=20"))
	if second.Code != 200 || !strings.Contains(second.Body.String(), `"count":0`) {
		t.Fatalf("pending job must not duplicate: %s", second.Body.String())
	}
	if len(a.state.Jobs) != 1 {
		t.Fatalf("queued twice: %v", len(a.state.Jobs))
	}
}

func TestCancelQueuedPersistsAndCannotStart(t *testing.T) {
	dir := t.TempDir()
	a := newApp(dir, filepath.Join(dir, "downloads"))
	post := featurePost(a, "/api/download", `{"urls":"https://reddit.com/r/cats/comments/abc123/demo"}`)
	if post.Code != 200 {
		t.Fatal(post.Body.String())
	}
	id := a.state.Jobs[0].ID
	c := featurePost(a, "/api/cancel-download", fmt.Sprintf(`{"id":%q}`, id))
	if c.Code != 200 {
		t.Fatalf("cancel failed %s", c.Body.String())
	}
	a.runDownload(id)
	if j := a.getJob(id); j == nil || j.Status != "cancelled" {
		t.Fatalf("cancelled job restarted: %+v", j)
	}
	b := newApp(dir, filepath.Join(dir, "downloads"))
	if len(b.state.Jobs) != 1 || b.state.Jobs[0].Status != "cancelled" {
		t.Fatal("cancelled history lost")
	}
	// Cancel endpoint must not accept forged web origins.
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/cancel-download", strings.NewReader(fmt.Sprintf(`{"id":%q}`, id)))
	req.Header.Set("Origin", "https://other-site.com")
	w := httptest.NewRecorder()
	a.serveHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("cross-origin cancellation accepted")
	}
}

func TestCancelRunningProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux shell downloader stub")
	}
	dir := t.TempDir()
	tool := filepath.Join(dir, "fake-downloader")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho started > \""+filepath.Join(dir, "started")+"\"\nexec sleep 15\n"), 0755); err != nil {
		t.Fatal(err)
	}
	a := newApp(dir, filepath.Join(dir, "downloads"))
	a.ytDlp = tool
	a.state.Jobs = []*Job{{ID: "running", URL: "https://reddit.com/r/cats/comments/abcd12/demo", Host: "reddit.com", Status: "queued"}}
	finished := make(chan struct{})
	go func() { a.runDownload("running"); close(finished) }()
	deadline := time.After(4 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "started")); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("stub downloader did not start")
		default:
			time.Sleep(15 * time.Millisecond)
		}
	}
	r := featurePost(a, "/api/cancel-download", `{"id":"running"}`)
	if r.Code != 200 {
		t.Fatalf("cancel request rejected: %s", r.Body.String())
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("active process did not stop promptly")
	}
	if j := a.getJob("running"); j == nil || j.Status != "cancelled" {
		t.Fatalf("status overwritten after cancellation: %+v", j)
	}
}

func TestRetryFailedOncePerURL(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Jobs = []*Job{
		{ID: "newer", URL: "https://reddit.com/r/cats/comments/abc123/demo", Status: "error", Detail: "connection aborted"},
		{ID: "older", URL: "https://redd.it/abc123", Status: "error"},
		{ID: "other", URL: "https://x.com/a/status/8888", Status: "error"},
		{ID: "done", URL: "https://youtu.be/123xyz", Status: "done"},
		{ID: "cancelled", URL: "https://x.com/a/status/5555", Status: "cancelled"},
	}
	w := featurePost(a, "/api/retry-failed", `{}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"count":2`) {
		t.Fatalf("bad retry response %s", w.Body.String())
	}
	if j := a.getJob("newer"); j.Status != "queued" {
		t.Fatal("failed job not requeued")
	}
	if j := a.getJob("older"); j.Status != "error" {
		t.Fatal("duplicate failure retried")
	}
	w = featurePost(a, "/api/retry-failed", `{}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"count":0`) {
		t.Fatalf("duplicate retry again %s", w.Body.String())
	}
	if len(a.queue) != 2 {
		t.Fatalf("expected two jobs, got %d", len(a.queue))
	}
}

func TestUpdateReceiptRequiresVerifiedHealth(t *testing.T) {
	dir := t.TempDir()
	if readUpdateReceipt(dir) != nil {
		t.Fatal("nonexistent receipt")
	}
	if err := writeUpdateReceipt(dir, "verified", appVersion, "Healthy restart confirmed"); err != nil {
		t.Fatal(err)
	}
	a := newApp(dir, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765/api/state", nil)
	w := httptest.NewRecorder()
	a.serveHTTP(w, req)
	var st ClientState
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.UpdateReceipt == nil || st.UpdateReceipt.Version != appVersion || st.UpdateReceipt.Status != "verified" {
		t.Fatalf("receipt not shown %+v", st.UpdateReceipt)
	}
	if err := writeUpdateReceipt(dir, "rolled_back", appVersion, "Restored old version"); err != nil {
		t.Fatal(err)
	}
	if x := readUpdateReceipt(dir); x == nil || x.Status != "rolled_back" {
		t.Fatal("rollback invisible")
	}
}
