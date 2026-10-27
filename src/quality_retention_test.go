package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func retentionApp(t *testing.T) *App {
	t.Helper()
	a := newApp(t.TempDir(), t.TempDir())
	a.ffmpeg = testQualityFF(t)
	a.state.Phase = "ready"
	return a
}

// Exercise the same browser-upload and queue handlers used by the batch UI.
// Resize fixtures isolate result lifetime from machine-specific GPU inference.
func retentionInputs(t *testing.T, a *App, n int) ([]qualitySource, map[string][]byte) {
	t.Helper()
	originals := make(map[string][]byte)
	var sources []qualitySource
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("image-%02d.png", i)
		data := fixtureImageBytes(t)
		original := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(original, data, 0600); err != nil {
			t.Fatal(err)
		}
		originals[original] = data
		w := uploadFixture(t, a, name, data, "http://127.0.0.1:8765")
		if w.Code != 201 {
			t.Fatal(i, w.Code, w.Body.String())
		}
		var q qualitySource
		if err := json.Unmarshal(w.Body.Bytes(), &q); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, a.qualitySources[q.ID])
	}
	return sources, originals
}

func retentionStatus(t *testing.T, a *App, id, expected string) *Job {
	t.Helper()
	j := a.getJob(id)
	if j == nil || j.Status != expected {
		t.Fatalf("job %s: want %s; got %+v", id, expected, j)
	}
	return j
}

func retentionCheckFiles(t *testing.T, files map[string][]byte) {
	t.Helper()
	for path, want := range files {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("file changed or disappeared: %s: %v", path, err)
		}
	}
}

func retentionCancel(t *testing.T, a *App, id string) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"id": id})
	w := browserRequest(a, "/api/cancel-download", "http://127.0.0.1:8765", "", string(b))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestUnsavedWorkspaceFiftyPreviewsSaveAll(t *testing.T) {
	a := retentionApp(t)
	sources, originals := retentionInputs(t, a, qualitySourceLimit)
	options := qualityOptions{Mode: "resize", Scale: 2, Strength: .65}
	var previews, saves []string
	for _, q := range sources {
		previews = append(previews, qualityRequest(t, a, q, options, true, ""))
	}
	for _, id := range previews {
		a.runReadyJob(id) // Includes the production saveHistory/trim path.
	}
	outputs := make(map[string][]byte)
	for i, id := range previews {
		j := retentionStatus(t, a, id, "done")
		// A slow batch may take more than an hour; current results must survive.
		a.mutateJob(id, func(j *Job) { j.Quality.Created = time.Now().Add(-2 * time.Hour).Unix() })
		w := httptest.NewRecorder()
		a.serveHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8765/api/quality/preview/"+id+"/after", nil))
		if w.Code != 200 {
			t.Fatal("preview became unavailable", i, w.Code)
		}
		data, err := os.ReadFile(j.File)
		if err != nil {
			t.Fatal(err)
		}
		outputs[id] = data
		saves = append(saves, qualityRequest(t, a, sources[i], options, false, id))
	}
	for _, id := range saves {
		a.runReadyJob(id)
	}
	written := make(map[string][]byte)
	for i, id := range saves {
		j := retentionStatus(t, a, id, "done")
		retentionStatus(t, a, previews[i], "done")
		if _, exists := written[j.File]; exists {
			t.Fatal("two items share an output file")
		}
		written[j.File] = outputs[previews[i]]
		if got := qualityRequest(t, a, sources[i], options, false, previews[i]); got != id {
			t.Fatal("repeated save created a duplicate output job")
		}
	}
	if len(written) != 50 {
		t.Fatal("not all images saved", len(written))
	}
	retentionCheckFiles(t, written)
	retentionCheckFiles(t, originals)
	w := httptest.NewRecorder()
	a.serveHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8765/api/state", nil))
	var state ClientState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil || len(state.Jobs) != 100 {
		t.Fatal("batch UI cannot see all 50 previews and save results", err, len(state.Jobs))
	}
	// Removing items is explicit. It releases pins, not user originals/results.
	for _, q := range sources {
		b, _ := json.Marshal(map[string]string{"id": q.ID})
		w := browserRequest(a, "/api/quality/release", "http://127.0.0.1:8765", "", string(b))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	a.saveHistory()
	if len(a.qualitySources) != 0 || len(a.state.Jobs) > historyLimit {
		t.Fatal("workspace removal did not release protected history")
	}
	retentionCheckFiles(t, originals)
	retentionCheckFiles(t, written)
	t.Log("50 real PNG previews -> 50 exact saved outputs; >1h session retention; original/output preservation; removal releases pins")
}

func TestUnsavedWorkspaceFiftyMixedFailuresCancellationAndRetry(t *testing.T) {
	a := retentionApp(t)
	sources, originals := retentionInputs(t, a, 50)
	options := qualityOptions{Mode: "resize", Scale: 2, Strength: .65}
	var previews []string
	for _, q := range sources {
		previews = append(previews, qualityRequest(t, a, q, options, true, ""))
	}
	retentionCancel(t, a, previews[1])
	ff := a.ffmpeg
	a.ffmpeg = filepath.Join(t.TempDir(), "missing-fixture-ffmpeg")
	a.runReadyJob(previews[0])
	a.ffmpeg = ff
	for _, id := range previews[2:] {
		a.runReadyJob(id)
	}
	retentionStatus(t, a, previews[0], "error")
	retentionStatus(t, a, previews[1], "cancelled")
	for _, id := range previews[2:] {
		retentionStatus(t, a, id, "done")
	}
	for i := 0; i < 2; i++ {
		previews[i] = qualityRequest(t, a, sources[i], options, true, "")
		a.runReadyJob(previews[i])
		retentionStatus(t, a, previews[i], "done")
	}
	var saves []string
	for i, q := range sources {
		saves = append(saves, qualityRequest(t, a, q, options, false, previews[i]))
	}
	retentionCancel(t, a, saves[1])
	saveDir := a.saveDir
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("existing file"), 0600); err != nil {
		t.Fatal(err)
	}
	a.saveDir = blocked
	a.runReadyJob(saves[0])
	a.saveDir = saveDir
	for _, id := range saves[2:] {
		a.runReadyJob(id)
	}
	retentionStatus(t, a, saves[0], "error")
	retentionStatus(t, a, saves[1], "cancelled")
	for _, id := range saves[2:] {
		retentionStatus(t, a, id, "done")
	}
	for i := 0; i < 2; i++ {
		retentionStatus(t, a, previews[i], "done")
		saves[i] = qualityRequest(t, a, sources[i], options, false, previews[i])
		a.runReadyJob(saves[i])
	}
	for i, id := range saves {
		j := retentionStatus(t, a, id, "done")
		p := retentionStatus(t, a, previews[i], "done")
		want, err := os.ReadFile(p.File)
		if err != nil {
			t.Fatal(err)
		}
		retentionCheckFiles(t, map[string][]byte{j.File: want})
	}
	retentionCheckFiles(t, originals)
	retentionCheckFiles(t, map[string][]byte{blocked: []byte("existing file")})
	t.Log("50 items: preview failure/cancel + save failure/cancel retained for retry; all 50 eventually saved; other items unaffected")
}

func TestUnsavedWorkspaceProtectionIsBoundedAndSessionScoped(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	for i := 0; i < 50; i++ {
		q := qualitySource{ID: fmt.Sprint(i), Path: fmt.Sprintf("source-%d", i)}
		a.qualitySources[q.ID] = q
		for attempt := 0; attempt < 10; attempt++ {
			previewID := fmt.Sprintf("q_%d_%d", i, attempt)
			p := &Job{ID: previewID, Task: "quality", Status: "done", Quality: &qualityTask{Source: q, Preview: true, Created: time.Now().Add(-2 * time.Hour).Unix()}}
			s := &Job{ID: "save_" + previewID, Task: "quality", Status: "error", Quality: &qualityTask{Source: q, ReusePreview: previewID}}
			a.state.Jobs = append([]*Job{s, p}, a.state.Jobs...)
		}
	}
	for i := 0; i < 60; i++ {
		a.state.Jobs = append([]*Job{{ID: fmt.Sprintf("download-%d", i), Status: "done"}}, a.state.Jobs...)
	}
	a.saveHistory()
	if len(a.state.Jobs) != 2*qualitySourceLimit+historyLimit {
		t.Fatal("unbounded retention or dropped current workspace entries", len(a.state.Jobs))
	}
	b, err := os.ReadFile(filepath.Join(a.dataDir, "history.json"))
	var disk []*Job
	if err != nil || json.Unmarshal(b, &disk) != nil || len(disk) != historyLimit {
		t.Fatal("ordinary on-disk history limit changed", len(disk), err)
	}
	old := retentionStatus(t, a, "q_0_9", "done")
	if !a.qualityPreviewAvailable(old) {
		t.Fatal("current unsaved result expired")
	}
	a.mu.Lock()
	delete(a.qualitySources, "0")
	a.mu.Unlock()
	if a.qualityPreviewAvailable(old) {
		t.Fatal("released result incorrectly exempt from expiry")
	}
	a.mu.Lock()
	clear(a.qualitySources)
	a.trimHistoryLocked()
	a.mu.Unlock()
	if len(a.state.Jobs) != historyLimit {
		t.Fatal("released sources kept their workspace pins")
	}
}

func TestUnsavedWorkspaceSavePinsExactOlderPreview(t *testing.T) {
	a := retentionApp(t)
	sources, originals := retentionInputs(t, a, 1)
	q := sources[0]
	o := qualityOptions{Mode: "resize", Scale: 2}
	oldID := qualityRequest(t, a, q, o, true, "")
	a.runReadyJob(oldID)
	newOptions := o
	newOptions.Scale = 4
	newID := qualityRequest(t, a, q, newOptions, true, "")
	a.runReadyJob(newID)
	for i := 0; i < 40; i++ {
		a.state.Jobs = append([]*Job{{ID: fmt.Sprintf("recent-%d", i), Status: "done"}}, a.state.Jobs...)
	}
	// Validation sees the requested old preview; the accepted save must pin it
	// before its enqueue operation trims ordinary history.
	savedID := qualityRequest(t, a, q, o, false, oldID)
	retentionStatus(t, a, oldID, "done")
	a.mutateJob(oldID, func(j *Job) { j.Quality.Created = time.Now().Add(-2 * time.Hour).Unix() })
	a.saveHistory()
	a.runReadyJob(savedID)
	retentionStatus(t, a, savedID, "done")
	retentionStatus(t, a, oldID, "done")
	retentionStatus(t, a, newID, "done")
	retentionCheckFiles(t, originals)
}

func TestUnsavedWorkspaceRejectsChangedSourceAndSettings(t *testing.T) {
	a := retentionApp(t)
	sources, _ := retentionInputs(t, a, 1)
	q := sources[0]
	o := qualityOptions{Mode: "resize", Scale: 2}
	id := qualityRequest(t, a, q, o, true, "")
	a.runReadyJob(id)
	a.mutateJob(id, func(j *Job) { j.Quality.Created = time.Now().Add(-2 * time.Hour).Unix() })
	wrong := o
	wrong.Scale = 4
	b, _ := json.Marshal(map[string]any{"source": q.ID, "options": wrong, "preview": false, "approved_preview": id})
	w := browserRequest(a, "/api/quality/start", "http://127.0.0.1:8765", "", string(b))
	if w.Code != 409 {
		t.Fatal("changed options bypassed preview verification")
	}
	if err := os.Chtimes(q.Path, time.Now(), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(map[string]any{"source": q.ID, "options": o, "preview": false, "approved_preview": id})
	w = browserRequest(a, "/api/quality/start", "http://127.0.0.1:8765", "", string(b))
	if w.Code != 409 {
		t.Fatal("changed source bypassed preview verification")
	}
	// Old history is not enough to create a new selected source after a restart.
	b, _ = json.Marshal(map[string]string{"id": q.ID})
	w = browserRequest(a, "/api/quality/release", "http://127.0.0.1:8765", "", string(b))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if a.qualityPreviewAvailable(a.getJob(id)) {
		t.Fatal("released expired preview remained available")
	}
}

func TestUnsavedWorkspaceConcurrentHistoryAndPreviewChecks(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	q := qualitySource{ID: "source", Path: "unchanged"}
	a.qualitySources[q.ID] = q
	a.state.Jobs = []*Job{{ID: "q_preview", Task: "quality", Status: "done", Quality: &qualityTask{Source: q, Preview: true, Created: time.Now().Add(-2 * time.Hour).Unix()}}}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 25; i++ {
			a.saveHistory()
		}
	}()
	for i := 0; i < 25; i++ {
		if !a.qualityPreviewAvailable(a.getJob("q_preview")) {
			t.Error("current preview disappeared during history update")
		}
	}
	<-done
}
