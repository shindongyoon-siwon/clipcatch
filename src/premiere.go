package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const premiereMode = "premiere"
const originalMode = "original"

func (a *App) initPremiere() {
	a.state.OutputMode = premiereMode
	b, e := os.ReadFile(filepath.Join(a.dataDir, "output-preference.json"))
	if e == nil && len(b) < 2048 {
		var in struct {
			Mode string `json:"mode"`
		}
		if json.Unmarshal(b, &in) == nil && (in.Mode == premiereMode || in.Mode == originalMode) {
			a.state.OutputMode = in.Mode
		}
	}
	// Our private incomplete outputs only; user sources are never stored here.
	files, _ := filepath.Glob(filepath.Join(a.dataDir, "edit-work", "convert-*.part.mp4"))
	for _, f := range files {
		_ = os.Remove(f)
	}
}
func (a *App) premiereRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /premiere.js", func(w http.ResponseWriter, r *http.Request) {
		b, e := pages.ReadFile("web/premiere.js")
		if e != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("POST /api/premiere/settings", a.setOutputMode)
	mux.HandleFunc("POST /api/premiere/convert", a.convertHistoryFile)
	mux.HandleFunc("POST /api/premiere/choose", a.chooseEditFiles)
}
func premiereJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("JSON required")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	var more any
	if d.Decode(&more) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
func (a *App) setOutputMode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode string `json:"mode"`
	}
	if premiereJSON(w, r, &in) != nil || (in.Mode != premiereMode && in.Mode != originalMode) {
		writeError(w, 400, "\uc800\uc7a5 \ubc29\uc2dd\uc744 \ud655\uc778\ud574 \uc918.")
		return
	}
	a.persistMu.Lock()
	defer a.persistMu.Unlock()
	b, _ := json.Marshal(in)
	p := filepath.Join(a.dataDir, "output-preference.json")
	if os.MkdirAll(a.dataDir, 0700) != nil || os.WriteFile(p+".new", b, 0600) != nil {
		writeError(w, 500, "\uc124\uc815\uc744 \uc800\uc7a5\ud558\uc9c0 \ubabb\ud588\uc5b4.")
		return
	}
	if e := os.Rename(p+".new", p); e != nil {
		os.Remove(p + ".new")
		writeError(w, 500, "\uc124\uc815 \uc800\uc7a5 \uc2e4\ud328")
		return
	}
	a.mu.Lock()
	a.state.OutputMode = in.Mode
	a.mu.Unlock()
	writeJSON(w, 200, map[string]any{"ok": true, "mode": in.Mode})
}
func (a *App) convertHistoryFile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID   string `json:"id"`
		Safe bool   `json:"safe"`
	}
	if premiereJSON(w, r, &in) != nil || in.ID == "" {
		writeError(w, 400, "\ubcc0\ud658\ud560 \uc601\uc0c1\uc744 \uc120\ud0dd\ud574 \uc918.")
		return
	}
	j := a.getJob(in.ID)
	if j == nil || j.Status == "queued" || j.Status == "downloading" {
		writeError(w, 409, "\uc791\uc5c5\uc774 \ub05d\ub09c \uc601\uc0c1\ub9cc \ubcc0\ud658\ud560 \uc218 \uc788\uc5b4.")
		return
	}
	if len(j.Media) > 1 {
		items := []map[string]string{}
		for _, item := range j.Media {
			if !in.Safe && item.EditReady && nonempty(item.File) {
				continue
			}
			source := item.Original
			if !nonempty(source) {
				source = item.File
			}
			id, err := a.queueEditFile(source, j.URL, in.Safe)
			result := map[string]string{"id": id, "name": filepath.Base(source)}
			if err != nil {
				result["error"] = editErrorMessage(err)
			}
			items = append(items, result)
		}
		writeJSON(w, 202, map[string]any{"ok": true, "items": items})
		return
	}
	source := j.OriginalFile
	if source == "" {
		source = j.File
	}
	id, e := a.queueEditFile(source, j.URL, in.Safe)
	if e != nil {
		writeError(w, 409, editErrorMessage(e))
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "id": id})
}
func (a *App) queueEditFile(source, url string, force ...bool) (string, error) {
	safe := len(force) > 0 && force[0]
	if !filepath.IsAbs(source) {
		return "", errors.New("edit-input")
	}
	st, e := os.Stat(source)
	if e != nil || !st.Mode().IsRegular() || st.Size() == 0 {
		return "", errors.New("edit-input")
	}
	ext := strings.ToLower(filepath.Ext(source))
	switch ext {
	case ".mkv", ".mp4", ".webm", ".mov", ".m4v", ".avi", ".ts", ".m2ts":
	default:
		return "", errors.New("edit-input")
	}
	a.mu.Lock()
	if a.state.Phase != "ready" || a.newWorkBlockedLocked() {
		a.mu.Unlock()
		return "", errors.New("edit-not-ready")
	}
	active := 0
	for _, j := range a.state.Jobs {
		if j.Status == "queued" || j.Status == "downloading" {
			active++
			if j.Task == "convert" && j.OriginalFile == source && j.ForceCompatible == safe {
				a.mu.Unlock()
				return j.ID, nil
			}
		}
	}
	if active >= 120 {
		a.mu.Unlock()
		return "", errors.New("edit-queue")
	}
	a.created++
	id := fmt.Sprintf("pc_%d_%d", time.Now().UnixNano(), a.created)
	a.trimHistoryLocked()
	j := &Job{ForceCompatible: safe, ID: id, URL: url, Host: "PC \uc601\uc0c1", Title: filepath.Base(source), Status: "queued", Detail: "\ud3b8\uc9d1\uc6a9 MP4 \ubcc0\ud658 \ub300\uae30", Time: time.Now().Format("2006-01-02 15:04"), Task: "convert", OutputMode: premiereMode, OriginalFile: source}
	a.state.Jobs = append([]*Job{j}, a.state.Jobs...)
	a.mu.Unlock()
	select {
	case a.queue <- id:
		return id, nil
	default:
		a.mutateJob(id, func(j *Job) { j.Status = "error"; j.Detail = "\ub300\uae30\uc5f4\uc774 \uac00\ub4dd \ucc3c" })
		return "", errors.New("edit-queue")
	}
}

// File paths come only from an explicit native chooser or an existing job ID,
// never from arbitrary filesystem paths in a web request.
func (a *App) chooseEditFiles(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Safe bool `json:"safe"`
	}
	if premiereJSON(w, r, &in) != nil {
		writeError(w, 400, "Invalid request")
		return
	}
	a.mu.Lock()
	if a.premiereSelecting {
		a.mu.Unlock()
		writeError(w, 409, "\uc5f4\ub824 \uc788\ub294 \ud30c\uc77c \uc120\ud0dd\ucc3d\uc744 \ud655\uc778\ud574 \uc918.")
		return
	}
	a.premiereSelecting = true
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.premiereSelecting = false; a.mu.Unlock() }()
	files, e := chooseLocalEditFiles(r.Context())
	if e != nil {
		writeError(w, 400, "\ud30c\uc77c \uc120\ud0dd\ucc3d\uc744 \uc5f4\uc9c0 \ubabb\ud588\uc5b4. \ub2e4\uc6b4\ub85c\ub4dc \ub0b4\uc5ed\uc758 [\ud3b8\uc9d1\uc6a9 MP4 \ub9cc\ub4e4\uae30]\ub3c4 \uc0ac\uc6a9\ud560 \uc218 \uc788\uc5b4.")
		return
	}
	if len(files) > 10 {
		writeError(w, 400, "\ud55c \ubc88\uc5d0 \ucd5c\ub300 10\uac1c\uae4c\uc9c0 \uc120\ud0dd\ud574 \uc918.")
		return
	}
	results := []map[string]string{}
	for _, f := range files {
		id, err := a.queueEditFile(f, "", in.Safe)
		result := map[string]string{"name": filepath.Base(f), "id": id}
		if err != nil {
			result["error"] = editErrorMessage(err)
		}
		results = append(results, result)
	}
	writeJSON(w, 200, map[string]any{"ok": true, "items": results})
}
func chooseLocalEditFiles(parent context.Context) ([]string, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("Windows native chooser required")
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	// Fixed script, no user input is interpolated into PowerShell.
	script := `[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false);Add-Type -AssemblyName System.Windows.Forms;$d=New-Object System.Windows.Forms.OpenFileDialog;$d.Title='ClipCatch - Select videos';$d.Multiselect=$true;$d.CheckFileExists=$true;$d.Filter='Video files|*.mkv;*.mp4;*.webm;*.mov;*.m4v;*.avi;*.ts;*.m2ts';if($d.ShowDialog() -eq [Windows.Forms.DialogResult]::OK){ConvertTo-Json -Compress -InputObject @($d.FileNames)}else{'[]'};$d.Dispose()`
	out := &cappedBuffer{Limit: 65536}
	cmd := hiddenCommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", script)
	cmd.Stdout = out
	if e := cmd.Run(); e != nil {
		return nil, e
	}
	var paths []string
	if len(out.Data) >= out.Limit || json.Unmarshal([]byte(strings.TrimPrefix(string(out.Data), "\ufeff")), &paths) != nil {
		return nil, errors.New("invalid chooser result")
	}
	return paths, nil
}
func editStageText(stage string) string {
	switch stage {
	case "probe":
		return "\uc601\uc0c1\u00b7\uc18c\ub9ac \ud615\uc2dd \ud655\uc778 \uc911"
	case "remux":
		return "MP4\ub85c \ud3ec\uc7a5 \ubcc0\uacbd \uc911 \u00b7 \uc7ac\uc555\ucd95 \uc5c6\uc74c"
	case "audio":
		return "\uc18c\ub9ac\ub9cc AAC\ub85c \ubcc0\ud658 \uc911 \u00b7 \uc601\uc0c1\uc740 \uadf8\ub300\ub85c"
	case "video":
		return "\uc601\uc0c1\ub9cc H.264\ub85c \ubcc0\ud658 \uc911 \u00b7 \uc6d0\ubcf8 \ubcf4\uc874"
	case "video-audio":
		return "H.264\u00b7AAC MP4 \ubcc0\ud658 \uc911 \u00b7 \uc6d0\ubcf8 \ubcf4\uc874"
	case "decode":
		return "\uc601\uc0c1\uacfc \uc18c\ub9ac \uc804\uccb4 \uc7ac\uc0dd \uac80\uc0ac \uc911"
	case "safe":
		return "\ud638\ud658\uc131 \uc7ac\ubcc0\ud658 \uc911 \u00b7 \uace0\uc815 \ud504\ub808\uc784 H.264/AAC"
	case "verify":
		return "MP4 \ud615\uc2dd\u00b7\uae38\uc774\u00b7\uc74c\uc131 \ud655\uc778 \uc911"
	default:
		return "\ud3b8\uc9d1\uc6a9 MP4 \ud655\uc778 \uc911"
	}
}
func editErrorMessage(e error) string {
	switch e.Error() {
	case "edit-hdr":
		return "HDR \uc601\uc0c1\uc740 \uc0c9\uc0c1 \ubcc0\ud615\uc744 \ud53c\ud558\uae30 \uc704\ud574 \uc790\ub3d9 \ubcc0\ud658\ud558\uc9c0 \uc54a\uc558\uc5b4. \uc6d0\ubcf8 \ud30c\uc77c\uc740 \ubcf4\uc874\ub3fc."
	case "edit-decode":
		return "\uc601\uc0c1\uc774\ub098 \uc74c\uc131\uc5d0 \uc190\uc0c1 \uac00\ub2a5\uc131\uc774 \uc788\uc5b4. \uc6d0\ubcf8\uc744 \ub2e4\uc2dc \ubc1b\uc544 \uc918."
	case "edit-interlaced", "edit-rotation", "edit-rate":
		return "\ud504\ub808\uc784\u00b7\ud68c\uc804\u00b7\uc778\ud130\ub808\uc774\uc2a4 \ud615\uc2dd\uc744 \uc548\uc804\ud558\uac8c \ubcc0\ud658\ud560 \uc218 \uc5c6\uc5b4. \uc6d0\ubcf8\uc740 \ubcf4\uc874\ub3fc."
	case "edit-input":
		return "\uc6d0\ubcf8 \uc601\uc0c1 \ud30c\uc77c\uc744 \ucc3e\uac70\ub098 \uc77d\uc744 \uc218 \uc5c6\uc5b4."
	case "edit-no-video":
		return "\ud30c\uc77c\uc5d0 \ubcc0\ud658\ud560 \uc601\uc0c1\uc774 \uc5c6\uc5b4."
	case "edit-storage":
		return "MP4 \uc800\uc7a5\uc5d0 \uc2e4\ud328\ud588\uc5b4. \ub514\uc2a4\ud06c \uc6a9\ub7c9\u00b7\uc800\uc7a5 \uad8c\ud55c\uc744 \ud655\uc778\ud574 \uc918. \uc6d0\ubcf8\uc740 \ubcf4\uc874\ub3fc."
	case "edit-not-ready":
		return "\ub2e4\uc6b4\ub85c\ub4dc \ub3c4\uad6c \uc900\ube44\ub098 \uc5c5\ub370\uc774\ud2b8\uac00 \ub05d\ub09c \ub4a4 \uc2dc\ub3c4\ud574 \uc918."
	case "edit-tools", "edit-probe":
		return "\uc601\uc0c1 \ud615\uc2dd\uc744 \ud655\uc778\ud558\uc9c0 \ubabb\ud588\uc5b4. FFmpeg/FFprobe \ub3c4\uad6c\uc640 \uc6d0\ubcf8 \ud30c\uc77c\uc744 \ud655\uc778\ud574 \uc918."
	case "edit-queue":
		return "\uc791\uc5c5 \ub300\uae30\uc5f4\uc774 \uac00\ub4dd \ucc3c. \uc9c4\ud589 \uc911\uc778 \uc791\uc5c5\uc774 \ub05d\ub098\uba74 \ub2e4\uc2dc \uc2dc\ub3c4\ud574 \uc918."
	case "edit-multiple-video":
		return "\uc601\uc0c1 \ud2b8\ub799\uc774 \uc5ec\ub7ec \uac1c\uc778 \ud30c\uc77c\uc740 \uc790\ub3d9 \ubcc0\ud658\ud558\uc9c0 \uc54a\uc558\uc5b4. \uc6d0\ubcf8\uc740 \ubcf4\uc874\ub3fc."
	case "edit-limit":
		return "\uc790\ub3d9 \ubcc0\ud658 \ud06c\uae30\ub098 \ud2b8\ub799 \uc218 \ubc94\uc704\ub97c \ub118\uc5c8\uc5b4. \uc6d0\ubcf8\uc740 \ubcf4\uc874\ub3fc."
	default:
		return "MP4 \ubcc0\ud658\u00b7\uac80\uc99d\uc5d0 \uc2e4\ud328\ud588\uc5b4. \uc644\ub8cc\ub85c \ucc98\ub9ac\ud558\uc9c0 \uc54a\uc558\uace0 \uc6d0\ubcf8\uc740 \ubcf4\uc874\ud588\uc5b4."
	}
}
func (a *App) finishPremiere(id string, ctx context.Context) {
	j := a.getJob(id)
	if j == nil {
		return
	}
	if len(j.Media) > 1 || (j.WholePost && len(j.Media) > 0) {
		a.finishPostMedia(id, ctx, true)
		return
	}
	source := j.OriginalFile
	if source == "" {
		source = j.File
	}
	a.mutateJob(id, func(j *Job) {
		j.OriginalFile = source
		j.EditReady = false
		j.ErrorCode = ""
		j.ErrorTitle = ""
		j.ErrorAdvice = ""
		j.AuthRequired = false
	})
	a.mu.Lock()
	ffmpeg := a.ffmpeg
	a.mu.Unlock()
	result, e := a.makeDownloadEditCopy(j, ctx, ffmpeg, source, a.saveDir, filepath.Join(a.dataDir, "edit-work"), func(stage string, pct float64) {
		a.mutateJob(id, func(j *Job) {
			if j.Status != "cancelled" {
				j.Status = "downloading"
				j.Stage = "converting"
				j.Percent = pct
				j.Speed = ""
				j.ETA = ""
				j.Detail = editStageText(stage)
			}
		})
	}, func(cmd *exec.Cmd) {
		a.mu.Lock()
		if a.activeJobID == id {
			a.activeCmd = cmd
		}
		a.mu.Unlock()
	}, j.ForceCompatible)
	if ctx.Err() != nil || a.isJobCancelled(id) {
		return
	}
	if e != nil {
		a.mutateJob(id, func(j *Job) {
			if j.Status == "cancelled" {
				return
			}
			j.Status = "error"
			j.Stage = ""
			j.File = source
			j.ErrorCode = "CC-401"
			j.ErrorTitle = editErrorMessage(e)
			j.ErrorAdvice = "\uc6d0\ubcf8\uc740 \uc0ad\uc81c\ud558\uc9c0 \uc54a\uc558\uc5b4. [\ud3b8\uc9d1\uc6a9 MP4 \ub9cc\ub4e4\uae30]\ub85c \ub2e4\uc2dc \uc2dc\ub3c4\ud560 \uc218 \uc788\uc5b4."
			j.Detail = j.ErrorTitle
		})
		return
	}
	a.mutateJob(id, func(j *Job) {
		if j.Status == "cancelled" {
			return
		}
		j.File = result.File
		if len(j.Media) == 1 {
			j.Media[0].File = result.File
			j.Media[0].EditReady = true
		}
		j.EditReady = true
		j.Stage = ""
		j.Status = "done"
		j.Percent = 100
		j.Speed = ""
		j.ETA = ""
		j.Detail = "\ud3b8\uc9d1\uc6a9 MP4 \uc900\ube44 \uc644\ub8cc \u00b7 \uc6d0\ubcf8 \ubcf4\uc874"
		if result.Strategy == "already" {
			j.Detail = "\uc774\ubbf8 H.264\u00b7AAC MP4 \ud615\uc2dd\uc774\uc57c \u00b7 \uc7ac\uc555\ucd95 \uc548 \ud568"
		} else if result.Strategy == "remux" {
			j.Detail = "MP4 \ud3ec\uc7a5 \ubcc0\uacbd \uc644\ub8cc \u00b7 \uc7ac\uc555\ucd95 \uc5c6\uc74c"
		}
	})
}
