package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserFixURLAndRedditSample(t *testing.T) {
	raw := "https://www.reddit.com/r/IllegallySmolCats/comments/1x1rb7t/buckle_up_buckaroo/?utm_source=chatgpt.com"
	got, _, err := cleanURL(raw)
	if err != nil || got != "https://www.reddit.com/r/IllegallySmolCats/comments/1x1rb7t/buckle_up_buckaroo/" {
		t.Fatalf("wrong Reddit URL %q, %v", got, err)
	}
	raw = "https://www.youtube.com/watch?v=abc123&utm_source=share&feature=embed&UTM_MEDIUM=social"
	got, _, err = cleanURL(raw)
	if err != nil || strings.Contains(strings.ToLower(got), "utm_") || !strings.Contains(got, "v=abc123") || !strings.Contains(got, "feature=embed") {
		t.Fatalf("query lost or tracking kept: %q (%v)", got, err)
	}
	if canonicalJobKey("https://www.instagram.com/reel/abcdefgh/?utm_source=x&a=1") != canonicalJobKey("https://www.instagram.com/reel/abcdefgh/?a=1") {
		t.Fatal("UTM split duplicate detection")
	}
}
func TestUserFixHistoryThirtyAndQueuedSafe(t *testing.T) {
	dir := t.TempDir()
	a := newApp(dir, dir)
	for i := 0; i < 55; i++ {
		a.state.Jobs = append(a.state.Jobs, &Job{ID: fmt.Sprintf("old-%d", i), Status: "error", URL: "https://reddit.com/r/aww/comments/abc123/"})
	}
	a.state.Jobs = append(a.state.Jobs, &Job{ID: "queued-now", Status: "queued", URL: "https://reddit.com/r/aww/comments/abc124/"})
	a.saveHistory()
	if len(a.state.Jobs) != 31 || a.state.Jobs[len(a.state.Jobs)-1].ID != "queued-now" {
		t.Fatalf("wrong memory: %+v", a.state.Jobs)
	}
	b, err := os.ReadFile(filepath.Join(dir, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved []*Job
	if err = json.Unmarshal(b, &saved); err != nil || len(saved) != 30 {
		t.Fatalf("wrong disk history %d: %v", len(saved), err)
	}
}
func TestUserFixRedditErrorsDoNotPretendAuthentication(t *testing.T) {
	code, _, _ := classifyDownloadError("ERROR: [Reddit] Failed to parse JSON", "www.reddit.com")
	if code != "CC-106" {
		t.Fatalf("Reddit JSON classified %s", code)
	}
	code, _, _ = classifyDownloadError("ERROR: Account authentication is required", "www.reddit.com")
	if code != "CC-103" {
		t.Fatalf("login should be CC-103, got %s", code)
	}
}
