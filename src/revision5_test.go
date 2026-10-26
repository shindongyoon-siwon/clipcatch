package main

import (
	"context"
	"crypto/sha256"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRevision5MandatoryUpdateBlocksOnlyNewWork(t *testing.T) {
	oldC, oldV := appChannel, appVersion
	appChannel = "test"
	appVersion = "1.9.1-test.5"
	defer func() { appChannel, appVersion = oldC, oldV }()
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Phase = "ready"
	appChannel = "stable"
	token := browserTestPair(t, a)
	appChannel = "test"
	a.state.AppUpdate.Status = "error"
	if a.updateRequiredLocked() {
		t.Fatal("offline check failure locked app")
	}
	a.latestRelease, _ = parseTrialRelease(trialReleaseFixture("1.9.1-test.6"))
	a.state.AppUpdate.Status = "available"
	a.state.Jobs = []*Job{{ID: "existing", Status: "downloading"}}
	for _, path := range []string{"/api/download", "/api/premiere/choose", "/api/quality/start", "/api/retry-failed"} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:18766"+path, strings.NewReader(`{}`))
		r.Header.Set("Origin", "http://127.0.0.1:18766")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.serveHTTP(w, r)
		if w.Code != 409 || !strings.Contains(w.Body.String(), "CC-UPDATE-REQUIRED") {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	w := browserRequest(a, "/bridge/page", testExtensionOrigin, token, `{"page_url":"https://youtube.com/watch?v=abcdefghijk"}`)
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
	if a.state.Jobs[0].Status != "downloading" {
		t.Fatal("existing work interrupted")
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:18766/api/state", nil)
	w = httptest.NewRecorder()
	a.serveHTTP(w, r)
	if !strings.Contains(w.Body.String(), `"update_required":true`) {
		t.Fatal(w.Body.String())
	}
	a.state.AppUpdate.Status = "error"
	if !a.updateRequiredLocked() {
		t.Fatal("known update forgotten after failure")
	}
	a.latestRelease, _ = parseTrialRelease(trialReleaseFixture(appVersion))
	if a.updateRequiredLocked() {
		t.Fatal("current release blocked")
	}
}
func TestRevision5WorkspaceIsolation(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	j := &Job{ID: "a", OutputMode: premiereMode, Status: "queued"}
	a.state.Jobs = []*Job{j}
	if e := a.prepareManagedDownload(j); e != nil {
		t.Fatal(e)
	}
	if a.downloadRoot(j) == a.saveDir {
		t.Fatal("source mixed into outputs")
	}
	if !strings.Contains(strings.Join(a.downloadArgs(j), " "), j.DownloadDir) {
		t.Fatal("downloader ignored private path")
	}
	old := j.DownloadDir
	j.Status = "error"
	a.finishManagedDownload(j.ID)
	if _, e := os.Stat(old); e != nil {
		t.Fatal("failed source removed")
	}
	for _, task := range []string{"convert", "quality"} {
		other := &Job{ID: task, Task: task, OutputMode: premiereMode}
		if e := a.prepareManagedDownload(other); e != nil || other.DownloadDir != "" {
			t.Fatal("local source staged")
		}
	}
	original := &Job{ID: "original", OutputMode: originalMode}
	a.prepareManagedDownload(original)
	if a.downloadRoot(original) != a.saveDir {
		t.Fatal("original mode changed")
	}
	j.DownloadDir = a.saveDir
	j.Status = "done"
	a.finishManagedDownload(j.ID)
	if _, e := os.Stat(a.saveDir); e != nil {
		t.Fatal("user folder deleted")
	}
}
func TestRevision5RealMP4OnlyAndLosslessCopy(t *testing.T) {
	if os.Getenv("CLIPCATCH_EDIT_MEDIA_TEST") != "1" || runtime.GOOS == "windows" {
		t.Skip("local real FFmpeg")
	}
	ff, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Skip(e)
	}
	for _, mode := range []string{"mkv", "mp4", "multiple"} {
		t.Run(mode, func(t *testing.T) {
			a := newApp(t.TempDir(), t.TempDir())
			a.ffmpeg = ff
			a.state.Phase = "ready"
			j := &Job{ID: "download", OutputMode: premiereMode, Status: "downloading"}
			a.state.Jobs = []*Job{j}
			fixture := filepath.Join(t.TempDir(), "source."+map[bool]string{true: "mp4", false: "mkv"}[mode == "mp4"])
			args := []string{"-y", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=128x96:rate=10", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "0.4"}
			if mode == "mp4" {
				args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac")
			} else {
				args = append(args, "-c:v", "libvpx-vp9", "-c:a", "libopus")
			}
			if b, e := exec.Command(ff, append(args, fixture)...).CombinedOutput(); e != nil {
				t.Fatal(e, string(b))
			}
			original, _ := os.ReadFile(fixture)
			hash := sha256.Sum256(original)
			if e := a.prepareManagedDownload(j); e != nil {
				t.Fatal(e)
			}
			dir := j.DownloadDir
			count := 1
			if mode == "multiple" {
				count = 2
				j.WholePost = true
			}
			for n := 0; n < count; n++ {
				p := filepath.Join(dir, string(rune('a'+n))+filepath.Ext(fixture))
				os.WriteFile(p, original, 0600)
				a.recordPostFile(j.ID, p)
			}
			a.finishPremiere(j.ID, context.Background())
			a.finishManagedDownload(j.ID)
			got := a.getJob(j.ID)
			if got.Status != "done" || !got.EditReady {
				t.Fatalf("%+v", got)
			}
			files, _ := os.ReadDir(a.saveDir)
			if len(files) != count {
				t.Fatalf("wanted %d MP4 files, got %d", count, len(files))
			}
			for _, f := range files {
				if filepath.Ext(f.Name()) != ".mp4" {
					t.Fatal(f.Name())
				}
				p := filepath.Join(a.saveDir, f.Name())
				info, e := inspectEditMedia(context.Background(), ff, p)
				if e != nil {
					t.Fatal(e)
				}
				plan, e := planEdit(info)
				if e != nil || plan.Strategy != "remux" {
					t.Fatal(e, plan.Strategy)
				}
				if mode == "mp4" {
					output, _ := os.ReadFile(p)
					if sha256.Sum256(output) != hash {
						t.Fatal("compatible MP4 bytes changed")
					}
				}
			}
			if _, e := os.Stat(dir); !os.IsNotExist(e) {
				t.Fatal("managed sources not cleaned", e)
			}
			after, _ := os.ReadFile(fixture)
			if sha256.Sum256(after) != hash {
				t.Fatal("user original changed")
			}
			if got.OriginalFile != "" {
				t.Fatal("stale original reference")
			}
		})
	}
}
