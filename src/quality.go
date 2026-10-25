package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Private source paths can only enter via the OS chooser. Web requests reference
// random IDs, never arbitrary local paths or remote media URLs.
func (a *App) initQuality() {
	a.qualitySources = make(map[string]qualitySource)
	root := filepath.Join(a.dataDir, "quality-preview")
	if entries, e := os.ReadDir(root); e == nil {
		for _, d := range entries {
			if d.IsDir() && strings.HasPrefix(d.Name(), "q_") {
				_ = os.RemoveAll(filepath.Join(root, d.Name()))
			}
		}
	}
}
func (a *App) qualityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /quality.js", func(w http.ResponseWriter, r *http.Request) {
		b, e := pages.ReadFile("web/quality.js")
		if e != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("POST /api/quality/choose", a.chooseQualitySource)
	mux.HandleFunc("POST /api/quality/start", a.startQualityTask)
	mux.HandleFunc("POST /api/quality/install", a.queueQualityEngine)
	mux.HandleFunc("GET /api/quality/status", func(w http.ResponseWriter, r *http.Request) {
		_, e := verifyQualityEngine(a.dataDir)
		writeJSON(w, 200, map[string]any{"engine_ready": e == nil, "engine_size_mb": 46, "image_output": "PNG", "video_output": "H.264/AAC MP4", "limit_seconds": 120, "limit_side": 4096})
	})
	mux.HandleFunc("GET /api/quality/preview/{id}/{which}", a.serveQualityPreview)
}
func chooseLocalQualitySource(ctx context.Context) (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("quality-windows")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	script := `[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false);Add-Type -AssemblyName System.Windows.Forms;$d=New-Object System.Windows.Forms.OpenFileDialog;$d.Title='ClipCatch - Image or video';$d.Multiselect=$false;$d.CheckFileExists=$true;$d.Filter='Images and videos|*.png;*.jpg;*.jpeg;*.webp;*.mp4;*.mkv;*.mov;*.webm;*.m4v';if($d.ShowDialog() -eq [Windows.Forms.DialogResult]::OK){ConvertTo-Json -Compress -InputObject $d.FileName}else{'""'};$d.Dispose()`
	out := &cappedBuffer{Limit: 32768}
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", script)
	cmd.Stdout = out
	if e := cmd.Run(); e != nil {
		return "", e
	}
	var path string
	if len(out.Data) >= out.Limit || json.Unmarshal([]byte(strings.TrimPrefix(string(out.Data), "\ufeff")), &path) != nil {
		return "", errors.New("quality-input")
	}
	return path, nil
}
func (a *App) chooseQualitySource(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if premiereJSON(w, r, &in) != nil {
		writeError(w, 400, "Invalid request")
		return
	}
	a.mu.Lock()
	if a.premiereSelecting || a.state.Phase != "ready" {
		a.mu.Unlock()
		writeError(w, 409, qualityMessage(errors.New("quality-not-ready")))
		return
	}
	a.premiereSelecting = true
	ff := a.ffmpeg
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.premiereSelecting = false; a.mu.Unlock() }()
	path, e := chooseLocalQualitySource(r.Context())
	if e != nil {
		writeError(w, 400, qualityMessage(e))
		return
	}
	if path == "" {
		writeJSON(w, 200, map[string]any{"cancelled": true})
		return
	}
	q, e := describeQualitySource(r.Context(), ff, path)
	if e != nil {
		writeError(w, 400, qualityMessage(e))
		return
	}
	a.mu.Lock()
	if len(a.qualitySources) >= 20 {
		a.qualitySources = make(map[string]qualitySource)
	}
	a.qualitySources[q.ID] = q
	a.mu.Unlock()
	writeJSON(w, 200, q)
}
func (a *App) queueQualityEngine(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Consent bool `json:"consent"`
	}
	if premiereJSON(w, r, &in) != nil || !in.Consent {
		writeError(w, 400, "Explicit download consent required")
		return
	}
	if runtime.GOOS != "windows" {
		writeError(w, 400, qualityMessage(errors.New("quality-windows")))
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state.Phase != "ready" || a.state.AppUpdate.Status == "downloading" || a.state.AppUpdate.Status == "installing" {
		writeError(w, 409, qualityMessage(errors.New("quality-not-ready")))
		return
	}
	for _, j := range a.state.Jobs {
		if (j.Task == "quality" || j.Task == "quality-engine") && (j.Status == "queued" || j.Status == "downloading") {
			writeError(w, 409, qualityMessage(errors.New("quality-busy")))
			return
		}
	}
	id := fmt.Sprintf("qe_%d", time.Now().UnixNano())
	j := &Job{ID: id, Task: "quality-engine", Host: "PC", Title: "AI engine", Status: "queued", Time: time.Now().Format("2006-01-02 15:04"), Detail: "AI \ub3c4\uad6c \uc124\uce58 \ub300\uae30"}
	a.state.Jobs = append([]*Job{j}, a.state.Jobs...)
	select {
	case a.queue <- id:
		writeJSON(w, 202, map[string]string{"id": id})
	default:
		j.Status = "error"
		writeError(w, 429, "Queue full")
	}
}
func (a *App) startQualityTask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Source          string         `json:"source"`
		Options         qualityOptions `json:"options"`
		Preview         bool           `json:"preview"`
		ApprovedPreview string         `json:"approved_preview"`
	}
	if premiereJSON(w, r, &in) != nil {
		writeError(w, 400, "Invalid request")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	q, ok := a.qualitySources[in.Source]
	if !ok || !sameQualitySource(q) {
		writeError(w, 409, qualityMessage(errors.New("quality-source-changed")))
		return
	}
	if e := validateQualityOptions(q, in.Options); e != nil {
		writeError(w, 400, qualityMessage(e))
		return
	}
	if a.state.Phase != "ready" || a.state.AppUpdate.Status == "installing" || a.state.AppUpdate.Status == "downloading" {
		writeError(w, 409, qualityMessage(errors.New("quality-not-ready")))
		return
	}
	if in.Options.Mode == "ai" {
		if _, e := verifyQualityEngine(a.dataDir); e != nil {
			writeError(w, 409, qualityMessage(e))
			return
		}
	}
	task := &qualityTask{Options: in.Options, Source: q, Preview: in.Preview, Created: time.Now().Unix()}
	if !in.Preview {
		approved := false
		for _, j := range a.state.Jobs {
			if j.ID == in.ApprovedPreview && j.Task == "quality" && j.Quality != nil && j.Quality.Preview && j.Status == "done" && nonempty(j.File) && time.Now().Unix()-j.Quality.Created < 3600 && qualityFingerprint(*j.Quality) == qualityFingerprint(*task) {
				approved = true
			}
		}
		if !approved {
			writeError(w, 409, qualityMessage(errors.New("quality-preview")))
			return
		}
	}
	active := 0
	for _, j := range a.state.Jobs {
		if j.Status == "queued" || j.Status == "downloading" {
			active++
			if j.Task == "quality" || j.Task == "quality-engine" {
				writeError(w, 409, qualityMessage(errors.New("quality-busy")))
				return
			}
		}
	}
	if active >= 120 {
		writeError(w, 429, "Queue full")
		return
	}
	id := fmt.Sprintf("q_%d", time.Now().UnixNano())
	kind := "\ubcf4\uc815 \ubcf4\uae30"
	if !in.Preview {
		kind = "\ubcf4\uc815 \uc800\uc7a5"
	}
	j := &Job{ID: id, Task: "quality", Quality: task, Host: "PC", OriginalFile: q.Path, Title: q.Name + " - " + kind, Status: "queued", Time: time.Now().Format("2006-01-02 15:04"), Detail: kind + " \ub300\uae30"}
	a.trimHistoryLocked()
	a.state.Jobs = append([]*Job{j}, a.state.Jobs...)
	select {
	case a.queue <- id:
		writeJSON(w, 202, map[string]string{"id": id})
	default:
		j.Status = "error"
		writeError(w, 429, "Queue full")
	}
}
func (a *App) runQualityTask(ctx context.Context, id string, j *Job) {
	ctx, stop := context.WithTimeout(ctx, 2*time.Hour)
	defer stop()
	fail := func(e error) {
		if ctx.Err() != nil || a.isJobCancelled(id) {
			return
		}
		a.mutateJob(id, func(x *Job) { x.Status = "error"; x.Detail = qualityMessage(e); x.ErrorCode = "CC-QUALITY" })
	}
	if j.Task == "quality-engine" {
		a.mutateJob(id, func(x *Job) { x.Detail = "AI \ub3c4\uad6c \ub2e4\uc6b4\ub85c\ub4dc\u00b7\uac80\uc99d \uc911" })
		if e := installQualityEngine(ctx, a.dataDir); e != nil {
			fail(e)
			return
		}
		a.mutateJob(id, func(x *Job) {
			if x.Status != "cancelled" {
				x.Status = "done"
				x.Percent = 100
				x.Detail = "AI \ub3c4\uad6c \uc900\ube44 \uc644\ub8cc. \ube44\uad50\ubcf8\uc73c\ub85c GPU\u00b7\uacb0\uacfc\ub97c \ud655\uc778\ud574 \uc918."
			}
		})
		return
	}
	if j.Quality == nil || !sameQualitySource(j.Quality.Source) {
		fail(errors.New("quality-source-changed"))
		return
	}
	task := *j.Quality
	o := task.Options
	q := task.Source
	if e := validateQualityOptions(q, o); e != nil {
		fail(e)
		return
	}
	engine := ""
	if o.Mode == "ai" {
		var e error
		engine, e = verifyQualityEngine(a.dataDir)
		if e != nil {
			fail(e)
			return
		}
	}
	root := filepath.Join(a.dataDir, "quality-preview")
	if e := os.MkdirAll(root, 0700); e != nil {
		fail(e)
		return
	}
	work := filepath.Join(root, id)
	if e := os.Mkdir(work, 0700); e != nil {
		fail(e)
		return
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(work)
		}
	}()
	ext := ".png"
	if q.Kind == "video" {
		ext = ".mp4"
		if task.Preview && o.Seconds > 3 {
			o.Seconds = 3
		}
	}
	output := filepath.Join(work, "after"+ext)
	before := filepath.Join(work, "before"+ext)
	progress := func(p float64) {
		a.mutateJob(id, func(x *Job) {
			if x.Status != "cancelled" {
				x.Percent = p
				x.Detail = "\ud654\uc9c8 \ucc98\ub9ac \uc911 \u00b7 \uc6d0\ubcf8 \ubcf4\uc874"
			}
		})
	}
	progress(1)
	var e error
	if task.Preview {
		if q.Kind == "image" {
			e = qualityPNG(ctx, a.ffmpeg, q.Path, before, q.Width*o.Scale, q.Height*o.Scale)
		} else {
			base := o
			base.Mode = "resize"
			e = renderQualityVideo(ctx, a.ffmpeg, "", q, base, before, work, nil)
		}
		if e != nil {
			fail(e)
			return
		}
	}
	if q.Kind == "image" {
		e = renderQualityImage(ctx, a.ffmpeg, engine, q.Path, output, work, o, q.Width*o.Scale, q.Height*o.Scale)
	} else {
		e = renderQualityVideo(ctx, a.ffmpeg, engine, q, o, output, work, progress)
	}
	if e != nil {
		fail(e)
		return
	}
	progress(96)
	if e = validateQualityOutput(ctx, a.ffmpeg, output, q, o); e != nil {
		fail(e)
		return
	}
	if !sameQualitySource(q) {
		fail(errors.New("quality-source-changed"))
		return
	}
	if ctx.Err() != nil || a.isJobCancelled(id) {
		return
	}
	final := output
	if !task.Preview {
		stem := strings.TrimSuffix(q.Name, filepath.Ext(q.Name)) + fmt.Sprintf("_up%dx", o.Scale)
		final, e = publishQuality(ctx, output, a.saveDir, stem, ext)
		if e != nil {
			fail(e)
			return
		}
	} else {
		keep = true
	}
	a.mutateJob(id, func(x *Job) {
		if x.Status == "cancelled" {
			return
		}
		x.Status = "done"
		x.Percent = 100
		x.File = final
		x.EditReady = q.Kind == "video"
		t := *x.Quality
		t.Before = before
		x.Quality = &t
		if task.Preview {
			x.Detail = "\ube44\uad50\ubcf8 \uc900\ube44 \uc644\ub8cc \u00b7 \uc544\ub798\uc5d0\uc11c \ud655\uc778 \ud6c4 \uc800\uc7a5"
		} else {
			x.Detail = "\ubcf4\uc815\ubcf8 \uc800\uc7a5 \uc644\ub8cc \u00b7 \uc6d0\ubcf8 \ubcf4\uc874"
		}
	})
}
func (a *App) serveQualityPreview(w http.ResponseWriter, r *http.Request) {
	site := r.Header.Get("Sec-Fetch-Site")
	if site != "" && site != "same-origin" && site != "none" {
		http.Error(w, "Forbidden", 403)
		return
	}
	j := a.getJob(r.PathValue("id"))
	if j == nil || j.Task != "quality" || j.Quality == nil || !j.Quality.Preview || j.Status != "done" || time.Now().Unix()-j.Quality.Created > 3600 {
		http.NotFound(w, r)
		return
	}
	path := j.File
	if r.PathValue("which") == "before" {
		path = j.Quality.Before
	} else if r.PathValue("which") != "after" {
		http.NotFound(w, r)
		return
	}
	expected := filepath.Join(a.dataDir, "quality-preview", j.ID)
	if filepath.Dir(path) != expected {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	if j.Quality.Source.Kind == "image" {
		w.Header().Set("Content-Type", "image/png")
	} else {
		w.Header().Set("Content-Type", "video/mp4")
	}
	http.ServeFile(w, r, path)
}
func qualityMessage(e error) string {
	messages := map[string]string{
		"quality-engine":         "AI \ub3c4\uad6c\ub97c \uba3c\uc800 \uc900\ube44\ud574 \uc918.",
		"quality-integrity":      "AI \ub3c4\uad6c \ud30c\uc77c \uac80\uc99d\uc5d0 \uc2e4\ud328\ud588\uc5b4. \uc2e4\ud589\ud558\uc9c0 \uc54a\uc558\uc5b4.",
		"quality-gpu":            "AI \ucc98\ub9ac\uc5d0 \uc2e4\ud328\ud588\uc5b4. \uadf8\ub798\ud53d\uce74\ub4dc\u00b7Vulkan \ub4dc\ub77c\uc774\ubc84\u00b7\uba54\ubaa8\ub9ac\ub97c \ud655\uc778\ud574 \uc918. \ube44AI \ud655\ub300\ub294 \uc0ac\uc6a9\ud560 \uc218 \uc788\uc5b4.",
		"quality-download":       "AI \ub3c4\uad6c\ub97c \ubc1b\uc9c0 \ubabb\ud588\uc5b4. \ub124\ud2b8\uc6cc\ud06c\u00b7\ud68c\uc0ac \uc815\ucc45\ub97c \ud655\uc778\ud574 \uc918.",
		"quality-source-changed": "\uc6d0\ubcf8\uc774 \ubc14\ub00c\uc5c8\uac70\ub098 \uc120\ud0dd\uc774 \ub9cc\ub8cc\ub410\uc5b4. \ud30c\uc77c\uc744 \ub2e4\uc2dc \uace8\ub77c \uc918.",
		"quality-preview":        "\ud604\uc7ac \uc124\uc815\uc73c\ub85c \ube44\uad50\ubcf8\uc744 \uba3c\uc800 \ud655\uc778\ud574 \uc918.",
		"quality-duration":       "\uc601\uc0c1\uc740 \uc2e4\uc81c \uae38\uc774 \uc548\uc5d0\uc11c 120\ucd08 \uc774\ud558\uc758 \uad6c\uac04\uc744 \uace8\ub77c \uc918.",
		"quality-size":           "\uacb0\uacfc\ub294 \ud55c \ubcc0 4096px, 900\ub9cc \ud654\uc18c \uc774\ud558\ub85c \uc81c\ud55c\ub3fc. \ubc30\uc728\uc744 \uc904\uc5ec \uc918.",
		"quality-busy":           "\uc9c4\ud589 \uc911\uc778 \ud654\uc9c8 \ucc98\ub9ac\ub97c \uc644\ub8cc\ud558\uac70\ub098 \ucde8\uc18c\ud55c \ub4a4 \uc2dc\ub3c4\ud574 \uc918.",
		"quality-not-ready":      "\ub3c4\uad6c \uc900\ube44\u00b7\uc5c5\ub370\uc774\ud2b8\uac00 \ub05d\ub09c \ub4a4 \uc2dc\ub3c4\ud574 \uc918.",
		"quality-windows":        "\ud30c\uc77c \uc120\ud0dd\u00b7AI \ub3c4\uad6c \uc124\uce58\ub294 Windows\uc5d0\uc11c \uc0ac\uc6a9\ud574 \uc918.",
		"quality-alpha":          "\ud22c\uba85 \uc774\ubbf8\uc9c0\ub294 \ud14c\ub450\ub9ac \uc190\uc0c1\uc744 \ud53c\ud558\ub824\uace0 AI\ub97c \uc801\uc6a9\ud558\uc9c0 \uc54a\uc558\uc5b4. \uc6d0\ud615 \uc720\uc9c0 \ud655\ub300\ub97c \uc120\ud0dd\ud574 \uc918.",
		"quality-aspect":         "\ube44\uc815\ubc29\ud615 \ud53d\uc140 \uc601\uc0c1\uc740 \ud654\uba74 \ube44\uc728 \uc720\uc9c0\ub97c \uc704\ud574 \uc774\ubc88 \uae30\ub2a5\uc5d0\uc11c \uc81c\uc678\ud588\uc5b4.",
		"quality-image-profile":  "\uc0c9\uc0c1 \ud504\ub85c\ud30c\uc77c\uc774 \ud3ec\ud568\ub41c \uc774\ubbf8\uc9c0\uc57c. \uc0c9\uc0c1 \uc190\uc2e4\uc744 \ud53c\ud558\ub824\uace0 \ucc98\ub9ac\ud558\uc9c0 \uc54a\uc558\uc5b4. sRGB \uc0ac\ubcf8\uc744 \uc0ac\uc6a9\ud574 \uc918.",
		"quality-image-format":   "8\ube44\ud2b8 PNG/JPG/WebP\ub97c \uc120\ud0dd\ud574 \uc918. 16\ube44\ud2b8\u00b7\ud2b9\uc218 \uc0c9\uc0c1 \ud30c\uc77c\uc740 \uc6d0\ubcf8\uc744 \uc720\uc9c0\ud574 \uc918.",
	}
	if text := messages[e.Error()]; text != "" {
		return text
	}
	if strings.HasPrefix(e.Error(), "edit-") {
		return editErrorMessage(e)
	}
	return "\ucc98\ub9ac\u00b7\uac80\uc99d\uc5d0 \uc2e4\ud328\ud588\uc5b4. \uc6d0\ubcf8\uc740 \ubcc0\uacbd\ud558\uc9c0 \uc54a\uc558\uc5b4."
}
