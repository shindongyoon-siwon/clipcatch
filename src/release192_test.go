package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStableBuildIdentityRejectsMixedChannels(t *testing.T) {
	for _, b := range []buildIdentity{{App: "clipcatch", Channel: "stable", Version: "1.9.2"}, {App: "clipcatch-test2", Channel: "test", Version: "1.9.1-test.5"}} {
		if e := validateBuildIdentity(b); e != nil {
			t.Fatal(e)
		}
	}
	for _, b := range []buildIdentity{{App: "clipcatch", Channel: "stable", Version: "1.9.1-test.5"}, {App: "clipcatch-test2", Channel: "test", Version: "1.9.2"}, {App: "clipcatch", Channel: "test", Version: "1.9.1-test.5"}} {
		if validateBuildIdentity(b) == nil {
			t.Fatal("mixed identity accepted", b)
		}
	}
}
func TestKnownUpdatePersistsAndKeepsReleaseNotes(t *testing.T) {
	dir := t.TempDir()
	a := newApp(dir, t.TempDir())
	input := releaseFixture("99.1.0")
	input.Body = "MP4 and download improvements\n<script>alert(1)</script>\x00" + strings.Repeat("x", 13000)
	rel, e := parseGithubRelease(input)
	if e != nil {
		t.Fatal(e)
	}
	if len([]rune(rel.Notes)) > 12000 || strings.ContainsRune(rel.Notes, 0) {
		t.Fatal("unbounded notes")
	}
	a.saveRequiredUpdate(rel)
	b := newApp(dir, t.TempDir())
	b.state.AppUpdate.Status = "error"
	if !b.updateRequiredLocked() {
		t.Fatal("restart lost mandatory update")
	}
	b.setUpdate(UpdateInfo{Status: "error", Detail: "offline"})
	if b.state.AppUpdate.Notes != rel.Notes || b.state.AppUpdate.Latest != rel.Version {
		t.Fatal("notes lost on failure")
	}
	for _, path := range []string{"/api/download", "/api/retry-failed", "/api/premiere/choose", "/api/premiere/convert", "/api/quality/choose", "/api/quality/start", "/api/quality/install"} {
		request := httptest.NewRequest("POST", "http://127.0.0.1:8765"+path, strings.NewReader(`{}`))
		request.Header.Set("Origin", "http://127.0.0.1:8765")
		request.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		b.serveHTTP(w, request)
		if w.Code != 409 || !strings.Contains(w.Body.String(), "CC-UPDATE-REQUIRED") {
			t.Fatal(path, w.Code, w.Body.String())
		}
	}
	token := browserTestPair(t, b)
	if w := browserRequest(b, "/bridge/page", testExtensionOrigin, token, `{"page_url":"https://youtube.com/watch?v=abcdefghijk"}`); w.Code != 409 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:8765/api/state", nil)
	w := httptest.NewRecorder()
	b.serveHTTP(w, r)
	var st ClientState
	if json.Unmarshal(w.Body.Bytes(), &st) != nil || st.AppUpdate.Notes != rel.Notes || !st.UpdateRequired {
		t.Fatal("state missing changelog")
	}
	// A successful official check retracting the update can release the gate.
	same, _ := parseGithubRelease(releaseFixture(appVersion))
	a.saveRequiredUpdate(same)
	c := newApp(dir, t.TempDir())
	c.state.AppUpdate.Status = "error"
	if c.updateRequiredLocked() {
		t.Fatal("old requirement not cleared")
	}
	// A local cache cannot point an installer at a foreign site or trial channel.
	rel.BinaryURL = "https://evil.example/tool.exe"
	raw, _ := json.Marshal(rel)
	os.WriteFile(filepath.Join(dir, "required-update.json"), raw, 0600)
	if newApp(dir, t.TempDir()).updateRequiredLocked() {
		t.Fatal("untrusted release cache accepted")
	}
}
func TestStartupCheckBlocksBeforeNewWork(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.requireStartupCheck = true
	a.state.Phase = "ready"
	if !a.newWorkBlockedLocked() {
		t.Fatal("startup check bypass")
	}
	a.state.AppUpdate.Status = "error"
	if a.newWorkBlockedLocked() {
		t.Fatal("unknown offline state is not an available update")
	}
}
func TestSchedulerSeparateLanesAndBounds(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	started := make(chan string, 10)
	release := make(chan struct{})
	done := make(chan struct{})
	var mu sync.Mutex
	active := map[string]int{}
	maximum := map[string]int{}
	for _, id := range []string{"q0", "q1", "d0", "d1", "d2"} {
		task := ""
		if strings.HasPrefix(id, "q") {
			task = "quality"
		}
		a.state.Jobs = append(a.state.Jobs, &Job{ID: id, Task: task, Status: "queued"})
		a.queue <- id
	}
	go func() {
		a.dispatchJobs(func(id string) {
			lane := string(id[0])
			mu.Lock()
			active[lane]++
			if active[lane] > maximum[lane] {
				maximum[lane] = active[lane]
			}
			mu.Unlock()
			started <- id
			select {
			case <-release:
			case <-a.workerStop:
			}
			mu.Lock()
			active[lane]--
			mu.Unlock()
		})
		close(done)
	}()
	observed := map[string]bool{}
	for i := 0; i < 3; i++ {
		select {
		case id := <-started:
			observed[id] = true
		case <-time.After(3 * time.Second):
			a.stopWork()
			t.Fatal("downloads blocked behind AI")
		}
	}
	if !observed["q0"] || !observed["d0"] || !observed["d1"] {
		a.stopWork()
		t.Fatal(observed)
	}
	select {
	case extra := <-started:
		a.stopWork()
		t.Fatal("worker cap exceeded", extra)
	default:
	}
	close(release)
	a.stopWork()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("workers did not stop")
	}
	mu.Lock()
	defer mu.Unlock()
	if maximum["d"] != 2 || maximum["q"] != 1 {
		t.Fatal(maximum)
	}
}
func TestCancelOneParallelJobNotOthers(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	c1, x1 := context.WithCancel(context.Background())
	defer x1()
	c2, x2 := context.WithCancel(context.Background())
	defer x2()
	a.activeRuns["one"] = &jobExecution{ctx: c1, cancel: x1}
	a.activeRuns["two"] = &jobExecution{ctx: c2, cancel: x2}
	a.state.Jobs = []*Job{{ID: "one", Status: "downloading"}, {ID: "two", Status: "downloading"}}
	req := httptest.NewRequest("POST", "/api/cancel-download", strings.NewReader(`{"id":"one"}`))
	w := httptest.NewRecorder()
	a.cancelDownloadHandler(w, req)
	if w.Code != 200 || c1.Err() == nil || c2.Err() != nil || a.getJob("two").Status != "downloading" {
		t.Fatal("job cancellation leaked")
	}
	a.stopWork()
	if c2.Err() == nil {
		t.Fatal("shutdown did not cancel remaining work")
	}
}
func TestParallelFixtureDownloadsKeepBothMP4s(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getenv("CLIPCATCH_EDIT_MEDIA_TEST") != "1" {
		t.Skip("opt-in POSIX media fixture")
	}
	ff, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Skip(e)
	}
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Phase = "ready"
	a.state.AppUpdate.Status = "current"
	a.ffmpeg = ff
	source := filepath.Join(t.TempDir(), "source.mp4")
	if b, e := exec.Command(ff, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=128x96:rate=10", "-t", "0.5", "-c:v", "libx264", "-pix_fmt", "yuv420p", source).CombinedOutput(); e != nil {
		t.Fatal(e, string(b))
	}
	script := fmt.Sprintf("#!/bin/sh\nwhile [ \"$#\" -gt 0 ]; do if [ \"$1\" = '-P' ]; then shift; dest=$1; fi; shift; done\ncp %q \"$dest/fixture.mp4\"\nsleep 0.1\nprintf 'CLIP_FILE|%%s\\n' \"$dest/fixture.mp4\"\n", source)
	engine := filepath.Join(a.dataDir, "fake-download")
	os.WriteFile(engine, []byte(script), 0700)
	a.ytDlp = engine
	for _, id := range []string{"one", "two", "three"} {
		a.state.Jobs = append(a.state.Jobs, &Job{ID: id, Status: "queued", URL: "https://youtube.com/watch?v=" + id, OutputMode: premiereMode})
		a.queue <- id
	}
	finished := make(chan struct{})
	go func() { a.worker(); close(finished) }()
	defer func() { a.stopWork(); <-finished }()
	end := time.Now().Add(15 * time.Second)
	for time.Now().Before(end) {
		complete := 0
		for _, id := range []string{"one", "two", "three"} {
			j := a.getJob(id)
			if j.Status == "error" {
				t.Fatalf("%s: %+v", id, j)
			}
			if j.Status == "done" && j.DownloadDir == "" {
				complete++
			}
		}
		if complete == 3 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	paths := map[string]bool{}
	for _, id := range []string{"one", "two", "three"} {
		j := a.getJob(id)
		if j.Status != "done" || !j.EditReady || j.DownloadDir != "" {
			t.Fatalf("%+v", j)
		}
		paths[j.File] = true
	}
	if len(paths) != 3 {
		t.Fatal("parallel outputs overwritten", paths)
	}
	for p := range paths {
		if _, e := inspectEditMedia(context.Background(), ff, p); e != nil {
			t.Fatal(e)
		}
	}
}
func TestDownloadFragmentsBounded(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	args := strings.Join(a.downloadArgs(&Job{URL: "https://youtube.com/watch?v=abcdefghijk"}), " ")
	if !strings.Contains(args, "--concurrent-fragments 2") || !strings.Contains(args, "--ignore-config") {
		t.Fatal(args)
	}
}

func TestReadVerifiedExecutableBuildIdentity(t *testing.T) {
	if os.Getenv("CLIPCATCH_BUILD_ID_TEST") != "1" {
		t.Skip("opt-in native executable identity")
	}
	name := "ClipCatch"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	if out, e := exec.Command("go", "build", "-trimpath", "-ldflags=-X main.appVersion=1.9.2 -X main.appChannel=stable", "-o", binary, ".").CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	b, e := fileBuildIdentity(binary)
	if e != nil || b.Channel != "stable" || b.Version != "1.9.2" || b.App != "clipcatch" {
		t.Fatal(b, e)
	}
	if e := verifyInstallerIdentity(binary, "1.9.3"); e == nil {
		t.Fatal("wrong version accepted")
	}
}
