package main

import (
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
)

func TestWholePostURLScope(t *testing.T) {
	for _, u := range []string{"https://x.com/cats/status/1234", "https://twitter.com/cats/status/1234/", "https://www.instagram.com/p/AbC123/", "https://www.instagram.com/reel/AbC123/"} {
		if !isWholeVideoPost(u) {
			t.Fatal("rejected post", u)
		}
	}
	for _, u := range []string{"https://x.com/home", "https://x.com/cats/status/1234/video/2", "https://instagram.com/cats/", "https://youtube.com/watch?v=abcdefghijk&list=PL123", "https://youtube.com/playlist?list=PL123", "https://x.com.evil.test/cats/status/1234", "https://x.com/cats/status/1234/evil", "file:///tmp/a"} {
		if isWholeVideoPost(u) {
			t.Fatal("expanded non-post", u)
		}
	}
}
func TestWholePostFlagsAndSelectedVideo(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	for _, u := range []string{"https://x.com/u/status/1234", "https://www.instagram.com/p/AbC123/"} {
		args := a.downloadArgs(&Job{URL: u, WholePost: true, OutputMode: premiereMode})
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "--no-playlist") || !strings.Contains(joined, "--yes-playlist") || !strings.Contains(joined, "playlist_index") || !strings.Contains(joined, "--no-overwrites") {
			t.Fatal(args)
		}
		if args[len(args)-1] != u {
			t.Fatal("URL must stay last")
		}
	}
	args := strings.Join(a.downloadArgs(&Job{URL: "https://youtube.com/watch?v=abcdefghijk&list=PL123", WholePost: true}), " ")
	if !strings.Contains(args, "--no-playlist") || strings.Contains(args, "--yes-playlist") {
		t.Fatal("YouTube queue widened", args)
	}
	args = strings.Join(a.downloadArgs(&Job{URL: "https://x.com/u/status/1234/video/2"}), " ")
	if !strings.Contains(args, "--no-playlist") {
		t.Fatal("selected X clip widened")
	}
}
func TestPostQueueDoesNotMistakeLegacyOneFileAsWholePost(t *testing.T) {
	d := t.TempDir()
	f := filepath.Join(d, "legacy.mp4")
	os.WriteFile(f, []byte("legacy"), 0600)
	a := newApp(t.TempDir(), d)
	u := "https://x.com/u/status/1234"
	a.state.Jobs = []*Job{{ID: "old", URL: u, Status: "done", File: f}}
	body, _ := json.Marshal(map[string]string{"urls": u})
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/download", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://127.0.0.1:8765")
	w := httptest.NewRecorder()
	a.serveHTTP(w, r)
	if w.Code != 200 || !a.state.Jobs[0].WholePost {
		t.Fatal(w.Code, w.Body.String())
	}
	if downloadJobKey(a.state.Jobs[0]) == canonicalJobKey(u) {
		t.Fatal("legacy key reused")
	}
}
func TestAllPostFilesRecordedAndCopied(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Jobs = []*Job{{ID: "j", Status: "downloading"}}
	a.parseOutput("j", "CLIP_FILE|/tmp/one.mp4")
	a.parseOutput("j", "CLIP_FILE|/tmp/two.mp4")
	a.parseOutput("j", "CLIP_FILE|/tmp/one.mp4")
	j := a.getJob("j")
	if len(j.Media) != 2 {
		t.Fatal(j.Media)
	}
	j.Media[0].File = "changed"
	if a.getJob("j").Media[0].File == "changed" {
		t.Fatal("shared job slice")
	}
	for i := 0; i < 110; i++ {
		a.recordPostFile("j", fmt.Sprintf("/tmp/file-%d.mp4", i))
	}
	if got := a.getJob("j"); len(got.Media) != 100 || !got.MediaOverflow {
		t.Fatal("unbounded collection", got)
	}
}
func postFixture(t *testing.T, codec, audio, ext string) (string, string, string) {
	t.Helper()
	if os.Getenv("CLIPCATCH_EDIT_MEDIA_TEST") != "1" {
		t.Skip("real media fixture opt-in")
	}
	ff, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Fatal(e)
	}
	d := t.TempDir()
	path := filepath.Join(d, "fixture."+ext)
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10", "-f", "lavfi", "-i", "sine=frequency=300:sample_rate=48000", "-t", "0.3", "-c:v", codec, "-c:a", audio, path}
	out, e := exec.Command(ff, args...).CombinedOutput()
	if e != nil {
		t.Fatal(e, string(out))
	}
	return ff, d, path
}
func TestTwoActualVideosBothPreparedAndPreserved(t *testing.T) {
	ff, d, p1 := postFixture(t, "libx264", "aac", "mkv")
	p2 := filepath.Join(d, "second.mkv")
	if out, e := exec.Command(ff, "-v", "error", "-i", p1, "-c:v", "libvpx-vp9", "-c:a", "libopus", p2).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	a := newApp(t.TempDir(), d)
	a.ffmpeg = ff
	a.state.Phase = "ready"
	j := &Job{ID: "album", URL: "https://x.com/u/status/1234", Status: "downloading", WholePost: true, OutputMode: premiereMode}
	a.state.Jobs = []*Job{j}
	a.recordPostFile(j.ID, p1)
	a.recordPostFile(j.ID, p2)
	a.finishPostMedia(j.ID, context.Background(), true)
	got := a.getJob(j.ID)
	if got.Status != "done" || !got.EditReady || len(got.Media) != 2 {
		t.Fatalf("%+v", got)
	}
	for _, m := range got.Media {
		if !nonempty(m.Original) || !nonempty(m.File) || m.File == m.Original || !m.EditReady {
			t.Fatalf("%+v", m)
		}
	}
	a.saveHistory()
	b := newApp(a.dataDir, d)
	restored := b.getJob(j.ID)
	if restored == nil || len(restored.Media) != 2 || !allSavedPostFiles(restored) {
		t.Fatal("media history lost")
	}
	key := downloadJobKey(restored)
	if _, ok := b.existingDownloadLocked(key); !ok {
		t.Fatal("complete album not deduplicated")
	}
	os.Remove(restored.Media[1].File)
	if _, ok := b.existingDownloadLocked(key); ok {
		t.Fatal("missing second file treated as complete")
	}
}
func TestAlbumPartialFailureNeverMarkedComplete(t *testing.T) {
	ff, d, p := postFixture(t, "libx264", "aac", "mp4")
	a := newApp(t.TempDir(), d)
	a.ffmpeg = ff
	a.state.Jobs = []*Job{{ID: "partial", URL: "https://x.com/u/status/1234", Status: "downloading", WholePost: true, OutputMode: premiereMode, DownloadIncomplete: true}}
	a.recordPostFile("partial", p)
	a.finishPostMedia("partial", context.Background(), true)
	j := a.getJob("partial")
	if j.Status != "error" || !nonempty(j.Media[0].File) || !nonempty(p) {
		t.Fatalf("%+v", j)
	}
	a.saveHistory()
	if _, ok := a.existingDownloadLocked(downloadJobKey(j)); ok {
		t.Fatal("partial album marked downloaded")
	}
}
func TestAlbumRejectsExecutablesAndExternalPaths(t *testing.T) {
	ff, d, p := postFixture(t, "libx264", "aac", "mp4")
	bad := filepath.Join(d, "fake.exe")
	os.WriteFile(bad, []byte("MZbad"), 0600)
	outside := filepath.Join(t.TempDir(), "other.mp4")
	os.WriteFile(outside, []byte("not video"), 0600)
	a := newApp(t.TempDir(), d)
	a.ffmpeg = ff
	a.state.Jobs = []*Job{{ID: "bad", Status: "downloading", WholePost: true}}
	for _, s := range []string{p, bad, outside} {
		a.recordPostFile("bad", s)
	}
	a.finishPostMedia("bad", context.Background(), false)
	j := a.getJob("bad")
	if j.Status != "error" || j.Media[0].Error != "" || j.Media[1].Error == "" || j.Media[2].Error == "" {
		t.Fatalf("%+v", j)
	}
	if !nonempty(bad) || !nonempty(outside) {
		t.Fatal("untrusted original was deleted")
	}
}
func TestAlbumRunnerConvertsEveryFileNotJustLast(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture runner")
	}
	ff, d, p := postFixture(t, "libx264", "aac", "mkv")
	two := filepath.Join(d, "second.mkv")
	data, _ := os.ReadFile(p)
	os.WriteFile(two, data, 0600)
	engine := filepath.Join(d, "fixture-engine")
	script := fmt.Sprintf("#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do if [ \"$1\" = \"-P\" ]; then shift; dest=$1; fi; shift; done\ncp %q \"$dest/a.mkv\"\ncp %q \"$dest/b.mkv\"\nprintf 'CLIP_FILE|%%s\\n' \"$dest/a.mkv\" \"$dest/b.mkv\"\n", p, two)
	os.WriteFile(engine, []byte(script), 0700)
	a := newApp(t.TempDir(), d)
	a.ffmpeg = ff
	a.ytDlp = engine
	a.state.Phase = "ready"
	a.state.Jobs = []*Job{{ID: "runner", URL: "https://x.com/u/status/1234", Host: "x.com", Status: "queued", WholePost: true, OutputMode: premiereMode}}
	a.runDownload("runner")
	j := a.getJob("runner")
	if j.Status != "done" || len(j.Media) != 2 || !j.Media[0].EditReady || !j.Media[1].EditReady {
		t.Fatalf("%+v", j)
	}
}

func TestAlbumKeysRespectInstagramCaseAndIgnoreSelectedSlide(t *testing.T) {
	a := requestedDownloadKey("https://www.instagram.com/p/AbC123/?img_index=1")
	b := requestedDownloadKey("https://instagram.com/p/AbC123/?img_index=2")
	if a != b {
		t.Fatal("same Instagram post split by slide query")
	}
	if a == requestedDownloadKey("https://instagram.com/p/abc123/") {
		t.Fatal("case-sensitive shortcode collapsed")
	}
}
func TestCancelledAlbumDoesNotResumeConversion(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Jobs = []*Job{{ID: "cancel", Status: "cancelled", WholePost: true, Media: []savedPostMedia{{Original: "/not/run.mp4"}}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.finishPostMedia("cancel", ctx, true)
	if a.getJob("cancel").Status != "cancelled" {
		t.Fatal("cancelled work resumed")
	}
}
