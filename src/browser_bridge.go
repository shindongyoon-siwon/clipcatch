package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type browserLink struct {
	Origin string `json:"origin"`
	Digest string `json:"digest"`
}
type browserBridgeState struct {
	Link       browserLink
	CodeDigest string
	Expires    time.Time
	Sources    map[string]browserMedia
}
type browserMedia struct {
	OutputMode      string `json:"output_mode,omitempty"`
	PageURL         string `json:"page_url"`
	Title           string `json:"title"`
	MediaURL        string `json:"media_url"`
	AudioURL        string `json:"audio_url,omitempty"`
	AllowSilent     bool   `json:"allow_silent"`
	AllowDuplicates bool   `json:"allow_duplicates"`
	Separate        bool   `json:"separate,omitempty"`
}

var extensionOriginRE = regexp.MustCompile(`^chrome-extension://[a-p]{32}$`)
var redditMediaPathRE = regexp.MustCompile(`^/([a-z0-9]{5,40})/([A-Za-z0-9_-]+\.(?:mpd|m3u8|mp4))$`)
var redditPagePathRE = regexp.MustCompile(`(?i)^/(?:(?:r|user)/[^/]+/)?comments/[a-z0-9]{5,12}(?:/|$)`)

func tokenDigest(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func sameDigest(a, b string) bool {
	return len(a) == 64 && len(b) == 64 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func newBrowserSecret() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func (a *App) browserLinkPath() string { return filepath.Join(a.dataDir, "browser-link.json") }
func (a *App) initBrowserBridge() {
	a.browser.Sources = make(map[string]browserMedia)
	if b, e := os.ReadFile(a.browserLinkPath()); e == nil && len(b) < 4096 {
		var link browserLink
		if json.Unmarshal(b, &link) == nil && extensionOriginRE.MatchString(link.Origin) && len(link.Digest) == 64 {
			a.browser.Link = link
		}
	}
	files, _ := filepath.Glob(filepath.Join(a.dataDir, "browser-media-*.json"))
	for _, f := range files {
		_ = os.Remove(f)
	}
}
func decodeBrowserJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return errors.New("invalid browser request")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("invalid trailing data")
	}
	return nil
}
func (a *App) serveBrowserBridge(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/bridge/pair" && r.URL.Path != "/bridge/download" && r.URL.Path != "/bridge/page" && r.URL.Path != "/bridge/status" && r.URL.Path != "/bridge/open-folder" {
		return false
	}
	origin := r.Header.Get("Origin")
	if !extensionOriginRE.MatchString(origin) {
		writeError(w, 403, "Invalid extension origin")
		return true
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	if r.Method == "OPTIONS" {
		if r.Header.Get("Access-Control-Request-Method") != "POST" {
			writeError(w, 405, "POST only")
			return true
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		w.WriteHeader(204)
		return true
	}
	if r.Method != "POST" {
		writeError(w, 405, "POST only")
		return true
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeError(w, 415, "JSON only")
		return true
	}
	if r.URL.Path == "/bridge/pair" {
		a.pairBrowser(w, r, origin)
		return true
	}
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	a.mu.Lock()
	ok := origin == a.browser.Link.Origin && sameDigest(tokenDigest(bearer), a.browser.Link.Digest)
	a.mu.Unlock()
	if !ok {
		writeError(w, 401, "ClipCatch에서 새 연결 코드를 만들어 다시 연결해 줘.")
		return true
	}
	if a.updateGateRequest(w, r) {
		return true
	}
	if r.URL.Path == "/bridge/status" {
		a.browserJobStatus(w, r)
	} else if r.URL.Path == "/bridge/open-folder" {
		a.browserOpenDownloads(w, r)
	} else if r.URL.Path == "/bridge/page" {
		a.queueBrowserPage(w, r)
	} else {
		a.queueBrowserMedia(w, r)
	}
	return true
}
func (a *App) pairBrowser(w http.ResponseWriter, r *http.Request, origin string) {
	var in struct {
		Code    string `json:"code"`
		Channel string `json:"channel,omitempty"`
	}
	if decodeBrowserJSON(w, r, &in) != nil {
		writeError(w, 400, "Invalid pairing request")
		return
	}
	if isTrialBuild() && in.Channel != "test" {
		writeError(w, 409, "테스트판은 ClipCatch Browser TEST2와만 연결됩니다.")
		return
	}
	token, e := newBrowserSecret()
	if e != nil {
		writeError(w, 500, "Cannot create local pairing key")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Now().After(a.browser.Expires) || !sameDigest(tokenDigest(strings.TrimSpace(in.Code)), a.browser.CodeDigest) {
		writeError(w, 401, "연결 코드가 틀렸거나 만료됐어. ClipCatch에서 새로 만들어 줘.")
		return
	}
	link := browserLink{Origin: origin, Digest: tokenDigest(token)}
	b, _ := json.Marshal(link)
	if os.MkdirAll(a.dataDir, 0700) != nil || os.WriteFile(a.browserLinkPath(), b, 0600) != nil {
		writeError(w, 500, "Cannot save local pairing")
		return
	}
	a.browser.Link = link
	a.browser.CodeDigest = ""
	a.browser.Expires = time.Time{}
	writeJSON(w, 200, map[string]any{"token": token, "version": appVersion, "protocol": 1, "channel": appChannel})
}
func (a *App) browserRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/browser-link/status", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		paired := a.browser.Link.Origin != ""
		a.mu.Unlock()
		writeJSON(w, 200, map[string]any{"paired": paired, "protocol": 1, "endpoint": "http://" + r.Host, "extension_path": filepath.Join(a.dataDir, "browser-extension")})
	})
	mux.HandleFunc("POST /api/browser-link/code", func(w http.ResponseWriter, r *http.Request) {
		code, e := newBrowserSecret()
		if e != nil {
			writeError(w, 500, "Cannot generate pairing code")
			return
		}
		a.mu.Lock()
		a.browser.CodeDigest = tokenDigest(code)
		a.browser.Expires = time.Now().Add(5 * time.Minute)
		a.mu.Unlock()
		writeJSON(w, 200, map[string]any{"code": code, "expires_seconds": 300, "endpoint": "http://" + r.Host})
	})
	mux.HandleFunc("POST /api/browser-link/disconnect", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if e := os.Remove(a.browserLinkPath()); e != nil && !os.IsNotExist(e) {
			writeError(w, 500, "Cannot remove local pairing file")
			return
		}
		a.browser.Link = browserLink{}
		a.browser.CodeDigest = ""
		a.browser.Expires = time.Time{}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/browser-link/install", func(w http.ResponseWriter, r *http.Request) {
		p, e := a.installBrowserExtension()
		if e != nil {
			writeError(w, 500, "확장 파일을 준비하지 못했어.")
			return
		}
		e = openFolder(p)
		writeJSON(w, 200, map[string]any{"ok": true, "path": p, "opened": e == nil})
	})
}
func (a *App) installBrowserExtension() (string, error) {
	dir := filepath.Join(a.dataDir, "browser-extension")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	entries, e := pages.ReadDir("browser_extension")
	if e != nil {
		return "", e
	}
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		b, e := pages.ReadFile("browser_extension/" + ent.Name())
		if e != nil {
			return "", e
		}
		if e = os.WriteFile(filepath.Join(dir, ent.Name()), b, 0600); e != nil {
			return "", e
		}
	}
	return dir, nil
}
func validateBrowserMediaURL(raw string) (string, string, error) {
	if len(raw) > 4096 {
		return "", "", errors.New("media URL too long")
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "v.redd.it" || u.User != nil || u.Fragment != "" || u.RawPath != "" {
		return "", "", errors.New("only direct HTTPS v.redd.it media is supported")
	}
	m := redditMediaPathRE.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", errors.New("unsupported Reddit media path")
	}
	return m[1], strings.ToLower(filepath.Ext(m[2])), nil
}
func validateBrowserMedia(in browserMedia) (browserMedia, error) {
	if in.OutputMode != "" {
		if _, ok := selectedOutputMode(in.OutputMode, premiereMode); !ok {
			return in, errors.New("저장 형식을 확인해 주세요.")
		}
	}
	clean, host, e := cleanURL(in.PageURL)
	if e != nil {
		return in, e
	}
	u, _ := url.Parse(clean)
	if !(host == "reddit.com" || strings.HasSuffix(host, ".reddit.com")) || !redditPagePathRE.MatchString(u.Path) {
		return in, errors.New("open a Reddit post, not a feed")
	}
	id, ext, e := validateBrowserMediaURL(in.MediaURL)
	if e != nil {
		return in, e
	}
	if in.AudioURL != "" {
		aid, aext, e := validateBrowserMediaURL(in.AudioURL)
		if e != nil || aid != id || aext != ".mp4" || ext != ".mp4" {
			return in, errors.New("audio and video must belong to the same Reddit media")
		}
		if in.MediaURL == in.AudioURL {
			return in, errors.New("audio and video URLs must differ")
		}
	}
	if ext == ".mp4" && in.AudioURL == "" && !in.AllowSilent {
		return in, errors.New("음성 주소가 없어. 영상을 재생한 뒤 다시 찾거나, 원래 무음인 경우만 무음 저장을 허용해 줘.")
	}
	if len([]rune(in.Title)) > 300 {
		return in, errors.New("title too long")
	}
	in.PageURL = clean
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		in.Title = "Reddit video"
	}
	return in, nil
}
func (a *App) queueBrowserPage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PageURL         string `json:"page_url"`
		AllowDuplicates bool   `json:"allow_duplicates"`
		OutputMode      string `json:"output_mode,omitempty"`
	}
	if decodeBrowserJSON(w, r, &in) != nil {
		writeError(w, 400, "영상 링크를 확인하지 못했어.")
		return
	}
	clean, _, e := cleanURL(in.PageURL)
	if e != nil || !strings.HasPrefix(clean, "https://") {
		writeError(w, 400, "지원하는 공개 영상 게시물 주소만 사용할 수 있어.")
		return
	}
	payload, e := json.Marshal(map[string]any{"urls": clean, "allow_duplicates": in.AllowDuplicates, "output_mode": in.OutputMode})
	if e != nil {
		writeError(w, 500, "Cannot prepare request")
		return
	}
	a.mu.Lock()
	owner := a.browser.Link.Digest
	a.mu.Unlock()
	local := r.Clone(context.WithValue(r.Context(), browserOwnerContextKey{}, owner))
	local.Body = io.NopCloser(strings.NewReader(string(payload)))
	a.downloadHandler(w, local)
}
func (a *App) queueBrowserMedia(w http.ResponseWriter, r *http.Request) {
	var in browserMedia
	if decodeBrowserJSON(w, r, &in) != nil {
		writeError(w, 400, "Invalid media request")
		return
	}
	var e error
	if in, e = validateBrowserMedia(in); e != nil {
		writeError(w, 400, e.Error())
		return
	}
	mediaID, _, _ := validateBrowserMediaURL(in.MediaURL)
	key := canonicalJobKey(in.PageURL)
	if in.Separate {
		key = selectedRedditKey(in.PageURL, mediaID)
	}
	a.mu.Lock()
	if a.newWorkBlockedLocked() {
		a.mu.Unlock()
		writeError(w, 409, "\uC5C5\uB370\uC774\uD2B8 \uC644\uB8CC \uD6C4 \uB2E4\uC2DC \uC2DC\uB3C4\uD574 \uC8FC\uC138\uC694.")
		return
	}
	if a.state.Phase != "ready" {
		a.mu.Unlock()
		writeError(w, 409, "ClipCatch 다운로드 준비가 끝난 뒤 다시 보내 줘.")
		return
	}
	if a.hasQueuedDownloadLocked(key) {
		ids := a.ownedPendingBrowserJobs(key, a.browser.Link.Digest)
		a.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true, "already_queued": true, "job_ids": ids})
		return
	}
	_, dup := a.existingDownloadLocked(key)
	if dup && !in.AllowDuplicates {
		a.mu.Unlock()
		writeJSON(w, 409, map[string]any{"error": "이미 받은 영상이야. 별도 사본으로 다시 받을까?", "duplicate": true})
		return
	}
	active := 0
	for _, j := range a.state.Jobs {
		if j.Status == "queued" || j.Status == "downloading" {
			active++
		}
	}
	if active >= 120 {
		a.mu.Unlock()
		writeError(w, 429, "Too many queued downloads")
		return
	}
	a.created++
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), a.created)
	if in.Separate {
		id = fmt.Sprintf("bm_%s_%d_%d", mediaID, time.Now().UnixNano(), a.created)
	}
	a.trimHistoryLocked()
	a.browser.Sources[id] = in
	outputMode, _ := selectedOutputMode(in.OutputMode, a.state.OutputMode)
	a.state.Jobs = append([]*Job{{BrowserOwner: a.browser.Link.Digest, OutputMode: outputMode, ID: id, URL: in.PageURL, Host: "www.reddit.com", Title: in.Title, Status: "queued", Time: time.Now().Format("2006-01-02 15:04"), Detail: "브라우저에서 보낸 영상 대기 중", Redownload: dup, BrowserSource: true}}, a.state.Jobs...)
	a.mu.Unlock()
	select {
	case a.queue <- id:
		writeJSON(w, 202, map[string]any{"ok": true, "id": id})
	default:
		a.mutateJob(id, func(j *Job) { j.Status = "error"; j.Detail = "대기열이 가득 참. 다시 보내 줘." })
		a.saveHistory()
		writeError(w, 429, "Queue is full")
	}
}
func safeBrowserFilename(title string) string {
	title = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*%`, r) {
			return '_'
		}
		return r
	}, title)
	title = strings.Trim(title, " .")
	if title == "" {
		title = "Reddit video"
	}
	return cut(title, 80)
}
func (a *App) browserDownloadArgs(j *Job, args []string) ([]string, func(), error) {
	a.mu.Lock()
	media, ok := a.browser.Sources[j.ID]
	a.mu.Unlock()
	cleanup := func() {}
	if !ok {
		return nil, cleanup, errors.New("CC-312: browser media expired; capture again")
	}
	var err error
	if media, err = validateBrowserMedia(media); err != nil {
		return nil, cleanup, err
	}
	id, ext, _ := validateBrowserMediaURL(media.MediaURL)
	out := append([]string(nil), args[:len(args)-1]...)
	name := safeBrowserFilename(media.Title) + " [" + id + "]"
	if j.Redownload {
		name += " (" + j.ID + ")"
	}
	for n := range out {
		if out[n] == "-o" && n+1 < len(out) {
			out[n+1] = name + ".%(ext)s"
		}
	}
	out = append(out, "--no-overwrites", "--add-header", "Referer:https://www.reddit.com/")
	if ext != ".mp4" {
		return append(out, "--force-generic-extractor", media.MediaURL), cleanup, nil
	}
	formats := []map[string]any{{"format_id": "browser-video", "url": media.MediaURL, "ext": "mp4", "vcodec": "unknown", "acodec": "none"}}
	if media.AudioURL != "" {
		formats = append(formats, map[string]any{"format_id": "browser-audio", "url": media.AudioURL, "ext": "m4a", "vcodec": "none", "acodec": "unknown"})
	} else {
		for n := range out {
			if out[n] == "-f" && n+1 < len(out) {
				out[n+1] = "bestvideo/best"
			}
		}
	}
	info := map[string]any{"id": id, "title": media.Title, "webpage_url": media.PageURL, "extractor": "ClipCatchBrowser", "formats": formats}
	if err = os.MkdirAll(a.dataDir, 0700); err != nil {
		return nil, cleanup, err
	}
	f, err := os.CreateTemp(a.dataDir, "browser-media-*.json")
	if err != nil {
		return nil, cleanup, err
	}
	cleanup = func() { _ = os.Remove(f.Name()) }
	err = json.NewEncoder(f).Encode(info)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		cleanup()
		return nil, func() {}, errors.New("cannot prepare media request")
	}
	return append(out, "--load-info-json", f.Name()), cleanup, nil
}
