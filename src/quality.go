package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
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
		writeJSON(w, 200, map[string]any{"engine_ready": e == nil, "engine_bundled": len(bundledQualityEngine) > 0, "engine_size_mb": 46, "image_output": "PNG", "video_output": "H.264/AAC MP4", "limit_seconds": 120, "limit_side": 4096})
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
	cmd := hiddenCommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", script)
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
	if a.state.Phase != "ready" || a.newWorkBlockedLocked() {
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
	if a.state.Phase != "ready" || a.newWorkBlockedLocked() {
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
	defer a.finishQualityDeadline(ctx, id)
	fail := func(e error) {
		if ctx.Err() != nil || a.isJobCancelled(id) {
			return
		}
		a.mutateJob(id, func(x *Job) {
			x.Status = "error"
			x.Detail = qualityMessage(e)
			x.ErrorCode = qualityErrorCode(e)
			x.ErrorTitle = "AI / 미디어 처리 실패"
			x.ErrorAdvice = qualityMessage(e)
		})
	}
	if j.Task == "quality-engine" {
		a.mutateJob(id, func(x *Job) {
			x.Detail = "AI \ub3c4\uad6c \uc900\ube44 \uc911"
			if len(bundledQualityEngine) > 0 {
				x.Detail = "\ud3ec\ud568\ub41c AI \ud30c\uc77c \uac80\uc99d\u00b7\uc124\uce58 \uc911 (\ucd94\uac00 \ub2e4\uc6b4\ub85c\ub4dc \uc5c6\uc74c)"
			}
		})
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
		suffix := fmt.Sprintf("_resize%dx", o.Scale)
		if o.Mode == "ai" {
			suffix = fmt.Sprintf("_AI%dx", o.Scale)
		}
		stem := strings.TrimSuffix(q.Name, filepath.Ext(q.Name)) + suffix
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
			if o.Mode == "ai" {
				x.Detail = "AI \ubcf4\uc815\ubcf8 \uc800\uc7a5 \uc644\ub8cc \u00b7 \uc6d0\ubcf8 \ubcf4\uc874"
			} else {
				x.Detail = "\ud06c\uae30 \ubcc0\uacbd \uc644\ub8cc (AI \ubbf8\uc0ac\uc6a9)"
			}
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
func qualityErrorCode(e error) string {
	code := e.Error()
	if strings.HasPrefix(code, "quality-download-http-") {
		return "CC-AI-HTTP-" + strings.TrimPrefix(code, "quality-download-http-")
	}
	codes := map[string]string{"quality-engine": "CC-AI-MISSING", "quality-integrity": "CC-AI-INTEGRITY", "quality-archive": "CC-AI-ARCHIVE", "quality-download-network": "CC-AI-NETWORK", "quality-download-timeout": "CC-AI-TIMEOUT", "quality-download-redirect": "CC-AI-REDIRECT", "quality-disk": "CC-AI-DISK", "quality-gpu": "CC-AI-GPU"}
	if x := codes[code]; x != "" {
		return x
	}
	return "CC-QUALITY"
}
func qualityMessage(e error) string {
	if strings.HasPrefix(e.Error(), "quality-download-http-") {
		return "AI 설치 서버가 요청을 거부했습니다. HTTP " + strings.TrimPrefix(e.Error(), "quality-download-http-")
	}
	messages := map[string]string{
		"quality-ai-source-size":    "AI 보정 입력은 한 변 2048px, 300만 화소 이하입니다.",
		"quality-engine":            "AI 기능을 먼저 켜 주세요. 크기 변경으로 대체하지 않았습니다.",
		"quality-integrity":         "AI 파일 검증에 실패했습니다. 실행하지 않았습니다.",
		"quality-archive":           "AI 파일을 풀지 못했습니다. 원본은 변경하지 않았습니다.",
		"quality-gpu":               "AI 엔진 실행에 실패했습니다. GPU·Vulkan 드라이버·메모리를 확인해 주세요. AI 보정은 적용되지 않았습니다.",
		"quality-download":          "AI 설치 전송에 실패했습니다. 보안 설정을 끄지 말고 오류 코드를 확인해 주세요.",
		"quality-download-network":  "AI 서버에 연결하지 못했습니다. 네트워크 연결을 확인해 주세요.",
		"quality-download-timeout":  "AI 설치 연결 시간이 초과되었습니다. 다시 시도해 주세요.",
		"quality-download-redirect": "AI 서버의 이동 주소를 허용할 수 없습니다. 임의 사이트의 설치 파일은 사용하지 마세요.",
		"quality-disk":              "AI 도구를 PC에 저장하지 못했습니다. 디스크 공간과 폴더 권한을 확인해 주세요.",
		"quality-engine-backup":     "AI 이전 백업이 있어 교체를 멈췄습니다. 기존 도구를 삭제하지 않았습니다.",
		"quality-source-changed":    "원본이 바뀌었거나 선택이 만료되었습니다. 파일을 다시 선택해 주세요.",
		"quality-preview":           "현재 설정으로 비교본을 먼저 확인해 주세요.",
		"quality-duration":          "영상은 실제 길이 안에서 120초 이하 구간을 선택해 주세요.",
		"quality-size":              "결과는 한 변 4096px, 900만 화소 이하입니다. 배율을 줄여 주세요.",
		"quality-busy":              "진행 중인 처리를 완료하거나 취소해 주세요.",
		"quality-not-ready":         "도구 준비 후 다시 시도해 주세요.",
		"quality-windows":           "파일 선택·AI 설치는 Windows에서 사용해 주세요.",
		"quality-alpha":             "투명 이미지는 AI 보정을 지원하지 않습니다. 테두리 손상을 막기 위해 처리를 중단했습니다.",
		"quality-aspect":            "영상의 화면 비율 정보를 확인할 수 없습니다. 원본을 유지한 채 처리를 중단했습니다.",
		"quality-image-profile":     "색상 프로파일이 있어 처리를 멈췄습니다. 색상 확인 후 sRGB 사본을 사용해 주세요.",
		"quality-image-format":      "8비트 PNG/JPG/WebP를 선택해 주세요.",
	}
	if text := messages[e.Error()]; text != "" {
		return text
	}
	if strings.HasPrefix(e.Error(), "edit-") {
		return editErrorMessage(e)
	}
	return "처리·검증에 실패했습니다. 원본은 변경하지 않았습니다."
}
