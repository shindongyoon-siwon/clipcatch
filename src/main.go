package main

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	appDefaultVersion = "1.9.0"
	appName           = "ClipCatch"
	ffmpegURL         = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
	ytDlpURL          = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe"
)

var appVersion = appDefaultVersion

//go:embed web/* browser_extension/*
var pages embed.FS

var (
	urlRE     = regexp.MustCompile(`https?://[^\s\"<>]+`)
	percentRE = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)%`)
	shaRE     = regexp.MustCompile(`(?i)[a-f0-9]{64}`)
)

type Job struct {
	Quality            *qualityTask     `json:"quality,omitempty"`
	ForceCompatible    bool             `json:"force_compatible,omitempty"`
	WholePost          bool             `json:"whole_post,omitempty"`
	MediaOverflow      bool             `json:"media_overflow,omitempty"`
	DownloadIncomplete bool             `json:"download_incomplete,omitempty"`
	Media              []savedPostMedia `json:"media,omitempty"`
	OutputMode         string           `json:"output_mode,omitempty"`
	Task               string           `json:"task,omitempty"`
	OriginalFile       string           `json:"original_file,omitempty"`
	EditReady          bool             `json:"edit_ready,omitempty"`
	Stage              string           `json:"stage,omitempty"`
	BrowserSource      bool             `json:"browser_source,omitempty"`
	ID                 string           `json:"id"`
	URL                string           `json:"url"`
	Host               string           `json:"host"`
	Title              string           `json:"title"`
	Status             string           `json:"status"`
	Percent            float64          `json:"percent"`
	Speed              string           `json:"speed"`
	ETA                string           `json:"eta"`
	File               string           `json:"file"`
	Detail             string           `json:"detail"`
	Time               string           `json:"time"`
	AuthRequired       bool             `json:"auth_required,omitempty"`
	Redownload         bool             `json:"redownload,omitempty"`
	ErrorCode          string           `json:"error_code,omitempty"`
	ErrorTitle         string           `json:"error_title,omitempty"`
	ErrorAdvice        string           `json:"error_advice,omitempty"`
}

type ClientState struct {
	OutputMode      string          `json:"output_mode"`
	Version         string          `json:"version"`
	Phase           string          `json:"phase"`
	SetupText       string          `json:"setup_text"`
	SetupPct        float64         `json:"setup_pct"`
	SetupError      string          `json:"setup_error"`
	SaveDir         string          `json:"save_dir"`
	Jobs            []*Job          `json:"jobs"`
	AppUpdate       UpdateInfo      `json:"app_update"`
	RedditAuthReady bool            `json:"reddit_auth_ready"`
	UpdateReceipt   *UpdateReceipt  `json:"update_receipt,omitempty"`
	Engine          EngineInfo      `json:"engine"`
	Diagnostics     DiagnosticState `json:"diagnostics"`
}

type App struct {
	qualitySources    map[string]qualitySource
	premiereSelecting bool
	browser           browserBridgeState
	mu                sync.Mutex
	persistMu         sync.Mutex
	state             ClientState
	dataDir           string
	toolDir           string
	saveDir           string
	ytDlp             string
	ffmpeg            string
	queue             chan string
	created           int64
	mux               *http.ServeMux
	server            *http.Server
	activeCmd         *exec.Cmd
	activeJobID       string
	activeCancel      context.CancelFunc
	activeContext     context.Context
	downloaded        map[string]downloadRecord
	latestRelease     verifiedRelease
	redditCookies     []byte
	engineMeta        EngineMetadata
}

func newApp(dataDir, saveDir string) *App {
	a := &App{
		dataDir:    dataDir,
		toolDir:    filepath.Join(dataDir, "tools"),
		saveDir:    saveDir,
		queue:      make(chan string, 150),
		downloaded: make(map[string]downloadRecord),
	}
	a.state = ClientState{
		Version:   appVersion,
		Phase:     "preparing",
		SetupText: "다운로드 도구 확인 중...",
		SaveDir:   saveDir,
		Jobs:      make([]*Job, 0),
		AppUpdate: UpdateInfo{Status: "checking", Detail: "업데이트 확인 중"},
	}
	a.initPremiere()
	a.initQuality()
	a.initBrowserBridge()
	// Existing unpacked extension: refresh bundled files in place after an app update.
	// Chrome/Edge still requires an explicit Reload or restart to activate new JS.
	if _, err := os.Stat(filepath.Join(a.dataDir, "browser-extension", "manifest.json")); err == nil {
		if _, err := a.installBrowserExtension(); err != nil {
			log.Println("Could not refresh existing ClipCatch extension:", err)
		}
	}
	a.loadEngineMetadata()
	a.state.Diagnostics = DiagnosticState{Status: "idle", Summary: "문제 해결에서 상태를 확인할 수 있어"}
	a.loadHistory()
	a.loadDownloadIndex()
	a.loadRememberedRedditAuth()
	a.removeOrphanedCookieFiles()
	a.routes()
	return a
}

func (a *App) routes() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, err := pages.ReadFile("web/index.html")
		if err != nil {
			http.Error(w, "UI not found", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(strings.ReplaceAll(strings.Replace(string(b), "</body>", "<script src=\"/premiere.js\"></script><script src=\"/quality.js\"></script></body>", 1), "__APP_VERSION__", appVersion)))
	})
	// Serve the app's distinctive tab icon from the executable (no extra files needed).
	for _, asset := range []struct{ path, mediaType string }{
		{"/favicon.svg", "image/svg+xml"},
	} {
		asset := asset
		mux.HandleFunc("GET "+asset.path, func(w http.ResponseWriter, r *http.Request) {
			data, err := pages.ReadFile("web" + asset.path)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", asset.mediaType)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(data)
		})
	}
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"app": "clipcatch", "version": appVersion})
	})
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		state := a.state
		state.Jobs = make([]*Job, len(a.state.Jobs))
		for i, j := range a.state.Jobs {
			copyJob := cloneDownloadJob(j)
			state.Jobs[i] = &copyJob
		}
		a.mu.Unlock()
		state.UpdateReceipt = readUpdateReceipt(a.dataDir)
		writeJSON(w, 200, state)
	})
	mux.HandleFunc("POST /api/download", a.downloadHandler)
	mux.HandleFunc("POST /api/cancel-download", a.cancelDownloadHandler)
	mux.HandleFunc("POST /api/retry-failed", a.retryFailedHandler)
	mux.HandleFunc("POST /api/update-tool", a.updateToolHandler)
	mux.HandleFunc("POST /api/repair-engine", a.repairEngineHandler)
	mux.HandleFunc("POST /api/rollback-engine", a.rollbackEngineHandler)
	mux.HandleFunc("POST /api/run-diagnostics", a.runDiagnosticsHandler)
	mux.HandleFunc("GET /api/diagnostic-report", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		result := buildDiagnosticReport(a.state)
		a.mu.Unlock()
		writeJSON(w, 200, map[string]string{"report": result})
	})
	mux.HandleFunc("POST /api/upload-reddit-cookies", a.uploadRedditCookiesHandler)
	mux.HandleFunc("POST /api/clear-reddit-cookies", a.clearRedditCookiesHandler)
	mux.HandleFunc("POST /api/check-app-update", a.checkAppUpdateHandler)
	mux.HandleFunc("POST /api/install-app-update", a.installAppUpdateHandler)
	mux.HandleFunc("POST /api/open-folder", func(w http.ResponseWriter, r *http.Request) {
		if err := os.MkdirAll(a.saveDir, 0755); err != nil {
			writeError(w, 500, "저장 폴더를 만들 수 없습니다.")
			return
		}
		if err := openFolder(a.saveDir); err != nil {
			writeError(w, 500, "폴더를 열지 못했습니다: "+err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/stop", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]bool{"ok": true})
		go func() {
			time.Sleep(250 * time.Millisecond)
			a.mu.Lock()
			active := a.activeCmd
			a.mu.Unlock()
			if active != nil && active.Process != nil {
				_ = active.Process.Kill()
			}
			if a.server != nil {
				_ = a.server.Close()
			}
		}()
	})
	a.browserRoutes(mux)
	mux.HandleFunc("GET /browser-link.js", func(w http.ResponseWriter, r *http.Request) {
		b, err := pages.ReadFile("web/browser-link.js")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(b)
	})
	a.premiereRoutes(mux)
	a.qualityRoutes(mux)
	a.mux = mux
}

func writeJSON(w http.ResponseWriter, code int, obj any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(obj)
}
func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (a *App) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; media-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	// The web page only controls the downloader listening on this machine.
	host := r.Host
	if !strings.HasPrefix(host, "127.0.0.1:") && !strings.HasPrefix(host, "localhost:") {
		http.Error(w, "Local access only", http.StatusForbidden)
		return
	}
	if a.serveBrowserBridge(w, r) {
		return
	}
	if r.Method == http.MethodPost {
		origin := r.Header.Get("Origin")
		if origin != "http://"+host {
			http.Error(w, "Invalid origin", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/api/download" && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeError(w, 415, "JSON 형식만 받을 수 있습니다.")
			return
		}
	}
	a.mux.ServeHTTP(w, r)
}

// stripUTMQueryParams drops utm_* tracking keys without changing the encoding
// or order of any non-tracking query parameter.
func stripUTMQueryParams(u *url.URL) {
	if u.RawQuery == "" {
		return
	}
	kept := make([]string, 0)
	for _, part := range strings.Split(u.RawQuery, "&") {
		name := strings.SplitN(part, "=", 2)[0]
		decoded, err := url.QueryUnescape(name)
		if err == nil && strings.HasPrefix(strings.ToLower(decoded), "utm_") {
			continue
		}
		kept = append(kept, part)
	}
	u.RawQuery = strings.Join(kept, "&")
}

func cleanURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(strings.TrimRight(raw, "),.;]"))
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", "", errors.New("http/https 영상 링크만 입력해 주세요")
	}
	if u.User != nil {
		return "", "", errors.New("계정 정보가 포함된 주소는 지원하지 않습니다")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".lan") || host == "host.docker.internal" {
		return "", "", errors.New("로컬 네트워크 주소는 지원하지 않습니다")
	}
	if ip := net.ParseIP(host); ip != nil {
		return "", "", errors.New("IP 주소 링크는 지원하지 않습니다")
	}
	if u.Port() != "" && u.Port() != "80" && u.Port() != "443" {
		return "", "", errors.New("기본 웹 포트 이외의 링크는 지원하지 않습니다")
	}
	if !approvedVideoHost(host) {
		return "", "", errors.New("회사 PC 안전판에서는 검증된 영상 사이트 링크만 지원해요")
	}
	if len(raw) > 4096 {
		return "", "", errors.New("링크가 너무 깁니다")
	}
	// A redd.it/<post_id> link is the same post as reddit.com/comments/<post_id>.
	// Canonicalizing it avoids the generic extractor and a needless redirect.
	if host == "redd.it" || host == "www.redd.it" {
		postID := strings.Trim(u.Path, "/")
		if regexp.MustCompile(`^[a-zA-Z0-9]{5,10}$`).MatchString(postID) {
			u.Host = "www.reddit.com"
			u.Path = "/comments/" + postID + "/"
			u.RawQuery = ""
			host = "www.reddit.com"
		}
	}
	stripUTMQueryParams(u)
	u.Fragment = ""
	return u.String(), host, nil
}

// Allow listed public video platforms only. Domain boundaries are checked.
func approvedVideoHost(host string) bool {
	allowed := []string{
		"reddit.com", "redd.it", "v.redd.it", "youtube.com", "youtu.be",
		"x.com", "twitter.com", "tiktok.com", "instagram.com",
		"facebook.com", "fb.watch", "vimeo.com", "streamable.com",
		"twitch.tv", "imgur.com",
	}
	for _, domain := range allowed {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func (a *App) downloadHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	var req struct {
		URLs            string `json:"urls"`
		AllowDuplicates bool   `json:"allow_duplicates"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, 400, "링크를 읽지 못했습니다.")
		return
	}
	matches := urlRE.FindAllString(req.URLs, -1)
	if len(matches) == 0 {
		writeError(w, 400, "영상 링크를 입력해 주세요.")
		return
	}
	if len(matches) > 10 {
		writeError(w, 400, "한 번에 링크를 최대 10개까지 추가할 수 있습니다.")
		return
	}
	type input struct{ u, h, key string }
	seen := map[string]bool{}
	inputs := make([]input, 0, len(matches))
	for _, m := range matches {
		u, h, err := cleanURL(m)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		key := requestedDownloadKey(u)
		if !seen[key] {
			seen[key] = true
			inputs = append(inputs, input{u, h, key})
		}
	}
	a.mu.Lock()
	var duplicates []duplicateInfo
	var pending int
	for _, it := range inputs {
		if a.hasQueuedDownloadLocked(it.key) {
			pending++
			continue
		}
		if duplicate, exists := a.existingDownloadLocked(it.key); exists && !req.AllowDuplicates {
			duplicate.URL = it.u
			duplicates = append(duplicates, duplicate)
		}
	}
	if len(duplicates) != 0 {
		a.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":      "이미 받은 영상이 있어. 다시 받으려면 확인해 줘.",
			"duplicates": duplicates,
		})
		return
	}
	a.trimHistoryLocked() // Keep pending and running jobs; trim only finished history.
	var added []string
	for _, it := range inputs {
		if a.hasQueuedDownloadLocked(it.key) {
			continue
		}
		_, isRedownload := a.existingDownloadLocked(it.key)
		a.created++
		id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), a.created)
		a.state.Jobs = append([]*Job{{
			WholePost:  isWholeVideoPost(it.u),
			OutputMode: a.state.OutputMode,
			ID:         id, URL: it.u, Host: it.h, Title: "제목 확인 중...", Status: "queued",
			Time: time.Now().Format("2006-01-02 15:04"), Detail: "다운로드 대기 중",
			Redownload: isRedownload,
		}}, a.state.Jobs...)
		added = append(added, id)
	}
	a.mu.Unlock()
	for _, id := range added {
		a.queue <- id
	}
	writeJSON(w, 200, map[string]any{"ok": true, "count": len(added), "skipped_pending": pending, "skipped_input": len(matches) - len(inputs)})
}

func (a *App) updateToolHandler(w http.ResponseWriter, r *http.Request) {
	if !a.beginEngineOperation(w, "stable", "update") {
		return
	}
	go a.replaceEngine("stable")
}
func (a *App) updateTool() { a.replaceEngine("stable") }

func (a *App) setSetup(phase, message, errText string, pct float64) {
	a.mu.Lock()
	a.state.Phase, a.state.SetupText, a.state.SetupError, a.state.SetupPct = phase, message, errText, pct
	a.mu.Unlock()
}

func (a *App) mutateJob(id string, fn func(*Job)) {
	a.mu.Lock()
	for _, j := range a.state.Jobs {
		if j.ID == id {
			fn(j)
			break
		}
	}
	a.mu.Unlock()
}
func (a *App) getJob(id string) *Job {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, j := range a.state.Jobs {
		if j.ID == id {
			cp := cloneDownloadJob(j)
			return &cp
		}
	}
	return nil
}

func (a *App) worker() {
	for id := range a.queue {
		for {
			if a.isJobCancelled(id) {
				break
			}
			a.mu.Lock()
			phase := a.state.Phase
			a.mu.Unlock()
			if phase == "ready" {
				break
			}
			if phase == "error" {
				a.mutateJob(id, func(j *Job) {
					if j.Status == "queued" {
						j.Status = "error"
						j.Detail = "프로그램 준비 실패: 설정 안내를 확인해 주세요"
					}
				})
				a.saveHistory()
				break
			}
			time.Sleep(400 * time.Millisecond)
		}
		if a.isJobCancelled(id) {
			continue
		}
		a.mu.Lock()
		phase := a.state.Phase
		a.mu.Unlock()
		if phase != "ready" {
			continue
		}
		a.runDownload(id)
		a.saveHistory()
	}
}

func (a *App) downloadArgs(j *Job) []string {
	return a.downloadArgsForAttempt(j, false)
}

func (a *App) downloadArgsForAttempt(j *Job, impersonate bool, forceIPv4 ...bool) []string {
	retries, fragmentRetries := "3", "3"
	if isRedditHost(j.Host) {
		retries, fragmentRetries = "6", "6"
	}
	args := []string{
		"--ignore-config", "--no-playlist", "--newline", "--progress",
		"--progress-template", "download:CLIP_PROGRESS|%(progress._percent_str)s|%(progress._speed_str)s|%(progress._eta_str)s",
		"--print", "before_dl:CLIP_TITLE|%(title)s",
		"--print", "after_move:CLIP_FILE|%(filepath)s",
		"--retries", retries, "--fragment-retries", fragmentRetries, "--socket-timeout", "30",
		"--merge-output-format", "mp4/mkv", "-f", "bv*+ba/b",
		"-P", a.saveDir,
		"-o", "%(title).120B [%(id)s].%(ext)s",
	}
	if j.Redownload {
		// The user explicitly chose another copy; never overwrite the original.
		args[len(args)-1] = "%(title).120B [%(id)s] (" + j.ID + ").%(ext)s"
	}
	if isRedditHost(j.Host) {
		// Retry video/CDN disconnects before falling back to a different route.
		// yt-dlp keeps .part files and continues partial downloads by default.
		args = append(args, "--retry-sleep", "http:2", "--retry-sleep", "fragment:2")
	}
	if a.ffmpeg != "" {
		args = append(args, "--ffmpeg-location", filepath.Dir(a.ffmpeg))
	}
	if len(forceIPv4) > 0 && forceIPv4[0] && isRedditHost(j.Host) {
		// An alternate network route can recover from a broken IPv6/CDN path.
		args = append(args, "--force-ipv4")
	}
	// This changes only the HTTP client fingerprint. It does NOT open Chrome,
	// read its cookie database or access any logged-in account.
	if impersonate && isRedditHost(j.Host) {
		args = append(args, "--impersonate", "chrome")
	}
	if j.OutputMode == premiereMode {
		// Resolution and frame rate outrank codec preference: never silently take a smaller AVC stream.
		args = append(args, "--format-sort-force", "-S", "hasvid,res,fps,hdr:12,lang,+vcodec:avc,+acodec:m4a")
	}
	return append(postDownloadArgs(j, args), j.URL)
}

func isRedditHost(host string) bool {
	host = strings.ToLower(host)
	for _, domain := range []string{"reddit.com", "redd.it"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// The remote peer may close a Reddit media request before sending a response.
// This is distinct from an authentication failure and should not prompt the
// user to export a browser session.
func isRedditConnectionDrop(msg string) bool {
	s := strings.ToLower(msg)
	for _, sign := range []string{
		"remotedisconnected", "remote end closed connection without response",
		"connection aborted", "connection reset by peer", "connectionreseterror",
		"unexpected eof", "read timed out", "connection timed out",
	} {
		if strings.Contains(s, sign) {
			return true
		}
	}
	return false
}

func isRedditAccessBlock(msg string) bool {
	s := strings.ToLower(msg)
	if strings.Contains(s, "429") || strings.Contains(s, "too many requests") {
		return false
	}
	return needsAuth(msg) || strings.Contains(s, "http error 403") ||
		strings.Contains(s, "403 forbidden") || strings.Contains(s, "403: blocked") ||
		strings.Contains(s, "failed to parse json")
}

// Explicitly avoid retries on 429 rate limits. No need to impersonate a browser
// on an ordinary network disconnect; forcing IPv4 is a safer first fallback.
func shouldRetryReddit(msg string) bool {
	s := strings.ToLower(msg)
	if strings.Contains(s, "429") || strings.Contains(s, "too many requests") {
		return false
	}
	return isRedditAccessBlock(msg) || isRedditConnectionDrop(msg)
}

func (a *App) runDownload(id string) {
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	var j *Job
	for _, item := range a.state.Jobs {
		if item.ID == id && item.Status == "queued" {
			item.Status, item.Percent, item.Detail = "downloading", 0, "영상 정보를 가져오는 중..."
			copyJob := *item
			j = &copyJob
			a.activeJobID, a.activeContext, a.activeCancel = id, ctx, cancel
			break
		}
	}
	a.mu.Unlock()
	if j == nil {
		cancel()
		return
	}
	defer func() {
		cancel()
		a.mu.Lock()
		if a.activeJobID == id {
			a.activeJobID, a.activeContext, a.activeCancel = "", nil, nil
		}
		a.mu.Unlock()
	}()

	if j.Task == "quality" || j.Task == "quality-engine" {
		a.runQualityTask(ctx, id, j)
		return
	}
	if j.Task == "convert" || (j.OutputMode == premiereMode && nonempty(j.OriginalFile) && !j.DownloadIncomplete) {
		a.finishPremiere(id, ctx)
		return
	}
	if j.BrowserSource {
		a.runBrowserJob(id, j)
		return
	}
	err, msg := a.runDownloadAttempt(id, j, false)
	if ctx.Err() != nil || a.isJobCancelled(id) {
		return
	}
	redditNeedsRetry := err != nil && isRedditHost(j.Host) && shouldRetryReddit(msg)
	initialAccessBlock := redditNeedsRetry && isRedditAccessBlock(msg)
	initialExplicitLogin := needsAuth(msg)
	if redditNeedsRetry {
		networkDrop := isRedditConnectionDrop(msg)
		a.mutateJob(id, func(j *Job) {
			if networkDrop {
				j.Detail = "레딧 영상 서버 연결 끊김. 다른 네트워크 방식으로 재시도 중..."
			} else {
				j.Detail = "레딧 접근 제한으로 다른 연결 방식을 자동 시도 중..."
			}
			j.Percent = 0
		})
		if networkDrop {
			err, msg = a.runDownloadAttemptIPv4(id, j)
		} else {
			err, msg = a.runDownloadAttempt(id, j, true)
		}
	}
	if ctx.Err() != nil || a.isJobCancelled(id) {
		return
	}
	// Reddit authentication is only used on access blocks, never network drops.
	if err != nil && isRedditHost(j.Host) && (initialAccessBlock || isRedditAccessBlock(msg)) {
		cookieData := a.copyRedditCookies()
		if len(cookieData) > 0 {
			a.mutateJob(id, func(j *Job) { j.Detail = "레딧 전용 로그인으로 마지막 재시도 중..."; j.Percent = 0 })
			err, msg = a.runDownloadWithRedditCookies(id, j, cookieData)
			for i := range cookieData {
				cookieData[i] = 0
			}
		}
	}
	if ctx.Err() != nil || a.isJobCancelled(id) {
		return
	}
	if err != nil {
		if msg == "" {
			msg = err.Error()
		}
		a.mutateJob(id, func(j *Job) {
			if j.Status == "cancelled" {
				return
			}
			j.Status = "error"
			// Generic 403/JSON failures are not evidence that the user must log in.
			j.AuthRequired = needsAuth(msg) || initialExplicitLogin
			j.ErrorCode, j.ErrorTitle, j.ErrorAdvice = classifyDownloadError(msg, j.Host)
			// Never persist raw stderr: sites may echo URLs or secret query tokens.
			j.Detail = j.ErrorTitle
			if j.ErrorCode == "CC-101" {
				j.Detail = "영상 서버가 연결을 끊었어. 원본 링크에서 재생되는지 확인해 줘."
			}
		})
		got := a.getJob(id)
		if got != nil && len(got.Media) > 0 {
			a.mutateJob(id, func(item *Job) { item.DownloadIncomplete = true })
			a.finishPostMedia(id, ctx, j.OutputMode == premiereMode)
		}
		return
	}
	a.mutateJob(id, func(item *Job) { item.DownloadIncomplete = false })
	got := a.getJob(id)
	if got != nil && (got.WholePost || len(got.Media) > 1) {
		a.finishPostMedia(id, ctx, j.OutputMode == premiereMode)
		return
	}
	if j.OutputMode == premiereMode {
		a.finishPremiere(id, ctx)
		return
	}
	a.mutateJob(id, func(j *Job) {
		if j.Status == "cancelled" {
			return
		}
		j.Status, j.Percent, j.Speed, j.ETA = "done", 100, "", ""
		j.Detail = "원본 스트림 다운로드 완료"
	})
}

func (a *App) runDownloadAttempt(id string, j *Job, impersonate bool, cookieFile ...string) (error, string) {
	return a.runDownloadAttemptNetwork(id, j, impersonate, false, cookieFile...)
}

func (a *App) runDownloadAttemptIPv4(id string, j *Job) (error, string) {
	return a.runDownloadAttemptNetwork(id, j, false, true)
}

func (a *App) runDownloadAttemptNetwork(id string, j *Job, impersonate, forceIPv4 bool, cookieFile ...string) (error, string) {
	args := a.downloadArgsForAttempt(j, impersonate, forceIPv4)
	if j.BrowserSource {
		var cleanup func()
		var err error
		args, cleanup, err = a.browserDownloadArgs(j, args)
		if err != nil {
			return err, err.Error()
		}
		defer cleanup()
	}
	if len(cookieFile) > 0 && isRedditHost(j.Host) {
		var err error
		args, err = addCookieArg(args, cookieFile[0])
		if err != nil {
			return err, ""
		}
	}
	a.mu.Lock()
	ctx := a.activeContext
	if a.activeJobID != id {
		ctx = nil
	}
	a.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err, "다운로드가 취소되었어."
	}
	cmd := exec.CommandContext(ctx, a.ytDlp, args...)
	cmd.Stdin = nil
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err, ""
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err, ""
	}
	if err := cmd.Start(); err != nil {
		return err, ""
	}
	a.mu.Lock()
	a.activeCmd = cmd
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		if a.activeCmd == cmd {
			a.activeCmd = nil
		}
		a.mu.Unlock()
	}()
	var errLines []string
	var linesMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 512*1024)
		for scanner.Scan() {
			a.parseOutput(id, scanner.Text())
		}
	}()
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 4096), 512*1024)
		for scanner.Scan() {
			s := strings.TrimSpace(scanner.Text())
			if s == "" {
				continue
			}
			linesMu.Lock()
			errLines = append(errLines, s)
			if len(errLines) > 14 {
				errLines = errLines[1:]
			}
			linesMu.Unlock()
			if strings.Contains(s, "CLIP_") {
				a.parseOutput(id, s)
			}
		}
	}()
	wg.Wait()
	err = cmd.Wait()
	if err != nil {
		linesMu.Lock()
		msg := strings.Join(errLines, "\n")
		linesMu.Unlock()
		return err, msg
	}
	return nil, ""
}

func (a *App) parseOutput(id, line string) {
	if a.isJobCancelled(id) {
		return
	}
	line = strings.TrimSpace(line)
	if idx := strings.Index(line, "CLIP_PROGRESS|"); idx >= 0 {
		fields := strings.SplitN(line[idx+len("CLIP_PROGRESS|"):], "|", 3)
		if len(fields) > 0 {
			pct := 0.0
			if match := percentRE.FindStringSubmatch(fields[0]); len(match) > 1 {
				pct, _ = strconv.ParseFloat(match[1], 64)
			}
			speed, eta := "", ""
			if len(fields) > 1 {
				speed = strings.TrimSpace(fields[1])
			}
			if len(fields) > 2 {
				eta = strings.TrimSpace(fields[2])
			}
			a.mutateJob(id, func(j *Job) {
				j.Percent, j.Speed, j.ETA = pct, speed, eta
				j.Detail = "영상 다운로드 중..."
			})
		}
	} else if idx := strings.Index(line, "CLIP_TITLE|"); idx >= 0 {
		title := strings.TrimSpace(line[idx+len("CLIP_TITLE|"):])
		if title != "" {
			a.mutateJob(id, func(j *Job) { j.Title = title })
		}
	} else if idx := strings.Index(line, "CLIP_FILE|"); idx >= 0 {
		file := strings.TrimSpace(line[idx+len("CLIP_FILE|"):])
		if file != "" {
			a.recordPostFile(id, file)
		}
	} else if strings.Contains(line, "[Merger]") || strings.Contains(line, "[VideoRemuxer]") {
		a.mutateJob(id, func(j *Job) { j.Detail = "영상과 소리를 합치는 중..." })
	}
}

func needsAuth(msg string) bool {
	s := strings.ToLower(msg)
	return strings.Contains(s, "account authentication is required") ||
		strings.Contains(s, "login required") ||
		strings.Contains(s, "sign in") ||
		strings.Contains(s, "login to") ||
		strings.Contains(s, "authentication required")
}

func friendlyError(msg string) string {
	s := strings.ToLower(msg)
	switch {
	case needsAuth(msg):
		return "레딧에서 로그인을 요구했어. 아래 [레딧 인증 연결]에 레딧 전용 cookies.txt를 등록한 뒤 재시도해 줘.\n" + tailMsg(msg)
	case isRedditConnectionDrop(msg):
		return "영상 서버가 연결을 끊었어. 다른 연결 방식으로 재시도했지만 실패했어. 원본 링크가 브라우저에서 정상 재생되는지 확인해 줘.\n" + tailMsg(msg)
	case strings.Contains(s, "403"), strings.Contains(s, "429"):
		return "사이트에서 접근을 제한했어. 레딧이라면 /comments/가 포함된 원본 게시물 주소로 시도해 줘.\n" + tailMsg(msg)
	case strings.Contains(s, "private"), strings.Contains(s, "not available"), strings.Contains(s, "unavailable"):
		return "영상이 비공개·삭제됐거나 해당 주소의 형식을 지원하지 않을 수 있어요.\n" + tailMsg(msg)
	default:
		return tailMsg(msg)
	}
}
func tailMsg(msg string) string {
	lines := strings.Split(strings.TrimSpace(msg), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.Contains(strings.ToLower(line), "error") {
			return cut(line, 360)
		}
	}
	return cut(strings.Join(lines, " "), 360)
}
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}
func (a *App) failJob(id, msg string) {
	a.mutateJob(id, func(j *Job) { j.Status = "error"; j.Detail = cut(msg, 520) })
}

func (a *App) bootstrap() {
	if runtime.GOOS != "windows" && os.Getenv("CLIPCATCH_SKIP_BOOTSTRAP") == "1" {
		a.ytDlp = os.Getenv("CLIPCATCH_YTDLP")
		a.ffmpeg = os.Getenv("CLIPCATCH_FFMPEG")
		a.setSetup("ready", "테스트 준비 완료", "", 100)
		return
	}
	// Recover interrupted engine replacement before checking for missing tools.
	if err := recoverInterruptedEngineFiles(a.toolDir); err != nil {
		a.setSetup("error", "다운로드 엔진 복구 실패", "문제 해결 화면에서 진단해 줘.", 0)
		return
	}
	if err := os.MkdirAll(a.toolDir, 0755); err != nil {
		a.setSetup("error", "설치 폴더 생성 실패", err.Error(), 0)
		return
	}
	if err := os.MkdirAll(a.saveDir, 0755); err != nil {
		a.setSetup("error", "저장 폴더 생성 실패", err.Error(), 0)
		return
	}
	exe := "yt-dlp"
	ffbin := "ffmpeg"
	if runtime.GOOS == "windows" {
		exe += ".exe"
		ffbin += ".exe"
	}
	if runtime.GOOS != "windows" {
		if found, err := exec.LookPath(exe); err == nil {
			a.ytDlp = found
		}
	}
	if a.ytDlp == "" && nonempty(filepath.Join(a.toolDir, exe)) {
		a.ytDlp = filepath.Join(a.toolDir, exe)
	}
	if a.ytDlp == "" {
		if runtime.GOOS != "windows" {
			a.setSetup("error", "yt-dlp를 찾지 못했습니다", "Windows용 배포판을 사용하거나 yt-dlp를 설치해 주세요.", 0)
			return
		}
		a.setSetup("preparing", "yt-dlp 공식 실행 파일을 설치 중...", "", 0)
		checksum, err := fetchYtDlpHash()
		if err != nil {
			a.setSetup("error", "yt-dlp 검증 정보 확인 실패", "네트워크를 확인해 주세요: "+err.Error(), 0)
			return
		}
		dest := filepath.Join(a.toolDir, exe)
		err = downloadVerified(ytDlpURL, dest, checksum, func(current, total int64) {
			a.setSetup("preparing", "yt-dlp 설치 중...", "", percent(current, total))
		})
		if err != nil {
			a.setSetup("error", "yt-dlp 다운로드 실패", err.Error(), 0)
			return
		}
		a.ytDlp = dest
	}
	a.setSetup("preparing", "FFmpeg를 확인하는 중...", "", 0)
	if runtime.GOOS != "windows" {
		if found, err := exec.LookPath(ffbin); err == nil {
			a.ffmpeg = found
		}
	}
	if a.ffmpeg == "" && nonempty(filepath.Join(a.toolDir, ffbin)) {
		a.ffmpeg = filepath.Join(a.toolDir, ffbin)
	}
	if a.ffmpeg == "" {
		if runtime.GOOS != "windows" {
			a.setSetup("error", "FFmpeg를 찾지 못했습니다", "FFmpeg를 설치해 주세요.", 0)
			return
		}
		a.setSetup("preparing", "FFmpeg 설치 준비 중... (첫 실행에서 약 100MB 다운로드)", "", 0)
		checksum, checksumErr := fetchSmallText(ffmpegURL + ".sha256")
		if checksumErr != nil || shaRE.FindString(checksum) == "" {
			a.setSetup("error", "FFmpeg 검증 정보 확인 실패", "FFmpeg 배포 사이트 연결에 실패했습니다. "+fmt.Sprint(checksumErr), 0)
			return
		}
		zipPath := filepath.Join(a.toolDir, "ffmpeg.zip")
		if err := downloadVerified(ffmpegURL, zipPath, shaRE.FindString(checksum), func(current, total int64) {
			a.setSetup("preparing", "FFmpeg 다운로드 중... (처음 한 번)", "", percent(current, total))
		}); err != nil {
			a.setSetup("error", "FFmpeg 다운로드 실패", err.Error(), 0)
			return
		}
		a.setSetup("preparing", "FFmpeg 압축을 해제하는 중...", "", 99)
		err := extractFFmpeg(zipPath, a.toolDir)
		_ = os.Remove(zipPath)
		if err != nil {
			a.setSetup("error", "FFmpeg 압축 해제 실패", err.Error(), 0)
			return
		}
		a.ffmpeg = filepath.Join(a.toolDir, ffbin)
	}
	a.setSetup("ready", "다운로드 준비 완료", "", 100)
	go a.engineCheckOnStartup()
}
func nonempty(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir() && fi.Size() > 0
}
func percent(now, total int64) float64 {
	if total <= 0 {
		return 0
	}
	v := float64(now) / float64(total) * 100
	if v > 100 {
		v = 100
	}
	return v
}
func fetchSmallText(link string) (string, error) {
	client := http.Client{Timeout: 35 * time.Second}
	resp, err := client.Get(link)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 100000))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
func fetchYtDlpHash() (string, error) {
	s, err := fetchSmallText("https://github.com/yt-dlp/yt-dlp/releases/latest/download/SHA2-256SUMS")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(s, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		if strings.TrimPrefix(strings.TrimPrefix(parts[len(parts)-1], "*"), "./") == "yt-dlp.exe" {
			if shaRE.MatchString(parts[0]) {
				return strings.ToLower(parts[0]), nil
			}
		}
	}
	return "", errors.New("공식 SHA256 목록에 yt-dlp.exe가 없습니다")
}
func downloadVerified(link, dest, hash string, progress func(int64, int64)) error {
	client := http.Client{Timeout: 30 * time.Minute}
	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ClipCatch/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("서버 응답 HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	h := sha256.New()
	writer := io.MultiWriter(f, h)
	buf := make([]byte, 256*1024)
	var n int64
	last := time.Time{}
	for {
		count, readErr := resp.Body.Read(buf)
		if count > 0 {
			if _, err := writer.Write(buf[:count]); err != nil {
				return err
			}
			n += int64(count)
			if n > 650*1024*1024 {
				return errors.New("도구 파일의 크기가 예상보다 큽니다")
			}
			if time.Since(last) > 250*time.Millisecond {
				progress(n, resp.ContentLength)
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	progress(n, resp.ContentLength)
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return errors.New("다운로드 크기가 일치하지 않습니다")
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), hash) {
		return errors.New("공식 파일의 SHA256 검증에 실패했습니다 (변조 또는 전송 오류)")
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	ok = true
	return nil
}
func extractFFmpeg(zipPath, dir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	required := map[string]bool{"ffmpeg.exe": false, "ffprobe.exe": false}
	for _, ent := range r.File {
		filename := strings.ToLower(filepath.Base(filepath.FromSlash(ent.Name)))
		if _, ok := required[filename]; !ok {
			continue
		}
		if !strings.Contains(strings.ToLower(filepath.ToSlash(ent.Name)), "/bin/") {
			continue
		}
		if ent.UncompressedSize64 > 320*1024*1024 {
			return errors.New("압축 파일 크기가 비정상적입니다")
		}
		in, err := ent.Open()
		if err != nil {
			return err
		}
		output := filepath.Join(dir, filename)
		out, err := os.Create(output + ".part")
		if err != nil {
			_ = in.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(in, 330*1024*1024))
		_ = in.Close()
		closeErr := out.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if err := os.Rename(output+".part", output); err != nil {
			return err
		}
		required[filename] = true
	}
	if !required["ffmpeg.exe"] {
		return errors.New("압축 파일에서 ffmpeg.exe를 찾지 못했습니다")
	}
	return nil
}

func (a *App) loadHistory() {
	data, err := os.ReadFile(filepath.Join(a.dataDir, "history.json"))
	if err != nil {
		return
	}
	var jobs []*Job
	if err = json.Unmarshal(data, &jobs); err != nil {
		return
	}
	for _, j := range jobs {
		if j != nil && (j.Status == "done" || j.Status == "error" || j.Status == "cancelled") {
			if clean, _, err := cleanURL(j.URL); err == nil {
				j.URL = clean
			}
			// Migrate auth errors from version 1.0 so the retry UI provides login guidance.
			if j.Status == "error" && needsAuth(j.Detail) {
				j.AuthRequired = true
			}
			a.state.Jobs = append(a.state.Jobs, j)
		}
		if len(a.state.Jobs) >= historyLimit {
			break
		}
	}
}

func openFolder(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}
func openBrowser(addr string) {
	if os.Getenv("CLIPCATCH_NO_BROWSER") == "1" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", addr)
	case "darwin":
		cmd = exec.Command("open", addr)
	default:
		cmd = exec.Command("xdg-open", addr)
	}
	_ = cmd.Start()
}

func main() {
	if len(os.Args) == 4 && os.Args[1] == "--clipcatch-apply-update" {
		applyClipCatchUpdate(os.Args[2:])
		return
	}
	if installAndRelaunch() {
		return
	}
	configBase, err := os.UserConfigDir()
	if err != nil {
		configBase = os.TempDir()
	}
	if runtime.GOOS == "windows" && os.Getenv("LOCALAPPDATA") != "" {
		configBase = os.Getenv("LOCALAPPDATA")
	}
	dataDir := filepath.Join(configBase, appName)
	_ = os.MkdirAll(dataDir, 0755)
	f, err := os.OpenFile(filepath.Join(dataDir, "clipcatch.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		log.SetOutput(f)
		defer f.Close()
	}
	log.Println("ClipCatch version", appVersion, "started")

	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	savedir := filepath.Join(home, "Downloads", "ClipCatch")
	if override := os.Getenv("CLIPCATCH_SAVEDIR"); override != "" {
		savedir = override
	}

	// On duplicate launch, reopen the existing instance instead of running twice.
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	testURL := "http://127.0.0.1:8765/api/health"
	if resp, err := client.Get(testURL); err == nil {
		var h map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&h)
		_ = resp.Body.Close()
		if h["app"] == "clipcatch" {
			openBrowser("http://127.0.0.1:8765/")
			return
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:8765")
	if err != nil {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		log.Println("Cannot start local web server:", err)
		return
	}
	a := newApp(dataDir, savedir)
	a.server = &http.Server{Handler: http.HandlerFunc(a.serveHTTP), ReadHeaderTimeout: 10 * time.Second}
	go a.bootstrap()
	go a.checkAppUpdate()
	go a.worker()
	addr := "http://" + listener.Addr().String() + "/"
	log.Println("Web UI running", addr)
	go func() { time.Sleep(300 * time.Millisecond); openBrowser(addr) }()
	if err := a.server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Println("Web server failure", err)
	}
	log.Println("ClipCatch closed")
}
