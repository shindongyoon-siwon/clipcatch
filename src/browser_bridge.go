package main

import (
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

// Pairing material is never returned by /api/state, history, or diagnostics.
// Only a digest is persisted; the extension retains its own bearer token.
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
	PageURL         string `json:"page_url"`
	Title           string `json:"title"`
	MediaURL        string `json:"media_url"`
	AudioURL        string `json:"audio_url,omitempty"`
	AllowSilent     bool   `json:"allow_silent"`
	AllowDuplicates bool   `json:"allow_duplicates"`
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
	// Delete only our temporary request files left by an interrupted download.
	files, _ := filepath.Glob(filepath.Join(a.dataDir, "browser-media-*.json"))
	for _, f := range files {
		_ = os.Remove(f)
	}
}
func decodeBrowserJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errors.New("invalid browser request")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("invalid trailing data")
	}
	return nil
}

// The caller has already validated the loopback Host. Only these two extension
// routes receive CORS permission, and all writes require a one-use code or key.
func (a *App) serveBrowserBridge(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/bridge/pair" && r.URL.Path != "/bridge/download" {
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
		writeError(w, 401, "ClipCatch\uc5d0\uc11c \uc0c8 \uc5f0\uacb0 \ucf54\ub4dc\ub97c \ub9cc\ub4e4\uc5b4 \ub2e4\uc2dc \uc5f0\uacb0\ud574 \uc918.")
		return true
	}
	a.queueBrowserMedia(w, r)
	return true
}
func (a *App) pairBrowser(w http.ResponseWriter, r *http.Request, origin string) {
	var in struct {
		Code string `json:"code"`
	}
	if decodeBrowserJSON(w, r, &in) != nil {
		writeError(w, 400, "Invalid pairing request")
		return
	}
	token, err := newBrowserSecret()
	if err != nil {
		writeError(w, 500, "Cannot create local pairing key")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Now().After(a.browser.Expires) || !sameDigest(tokenDigest(strings.TrimSpace(in.Code)), a.browser.CodeDigest) {
		writeError(w, 401, "\uc5f0\uacb0 \ucf54\ub4dc\uac00 \ud2c0\ub838\uac70\ub098 \ub9cc\ub8cc\ub410\uc5b4. ClipCatch\uc5d0\uc11c \uc0c8\ub85c \ub9cc\ub4e4\uc5b4 \uc918.")
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
	writeJSON(w, 200, map[string]any{"token": token, "version": appVersion, "protocol": 1})
}
func (a *App) browserRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/browser-link/status", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		paired := a.browser.Link.Origin != ""
		a.mu.Unlock()
		writeJSON(w, 200, map[string]any{"paired": paired, "protocol": 1, "endpoint": "http://" + r.Host, "extension_path": filepath.Join(a.dataDir, "browser-extension")})
	})
	mux.HandleFunc("POST /api/browser-link/code", func(w http.ResponseWriter, r *http.Request) {
		code, err := newBrowserSecret()
		if err != nil {
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
		path, err := a.installBrowserExtension()
		if err != nil {
			writeError(w, 500, "\ud655\uc7a5 \ud30c\uc77c\uc744 \uc900\ube44\ud558\uc9c0 \ubabb\ud588\uc5b4.")
			return
		}
		err = openFolder(path)
		writeJSON(w, 200, map[string]any{"ok": true, "path": path, "opened": err == nil})
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
		return in, errors.New("\uc74c\uc131 \uc8fc\uc18c\uac00 \uc5c6\uc5b4. \uc601\uc0c1\uc744 \uc7ac\uc0dd\ud55c \ub4a4 \ub2e4\uc2dc \ucc3e\uac70\ub098, \uc6d0\ub798 \ubb34\uc74c\uc778 \uacbd\uc6b0\ub9cc \ubb34\uc74c \uc800\uc7a5\uc744 \ud5c8\uc6a9\ud574 \uc918.")
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
func (a *App) queueBrowserMedia(w http.ResponseWriter, r *http.Request) {
	var in browserMedia
	if decodeBrowserJSON(w, r, &in) != nil {
		writeError(w, 400, "Invalid media request")
		return
	}
	var err error
	if in, err = validateBrowserMedia(in); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	key := canonicalJobKey(in.PageURL)
	a.mu.Lock()
	if a.state.Phase != "ready" {
		a.mu.Unlock()
		writeError(w, 409, "ClipCatch \ub2e4\uc6b4\ub85c\ub4dc \uc900\ube44\uac00 \ub05d\ub09c \ub4a4 \ub2e4\uc2dc \ubcf4\ub0b4 \uc918.")
		return
	}
	if a.hasQueuedDownloadLocked(key) {
		a.mu.Unlock()
		writeJSON(w, 200, map[string]any{"ok": true, "already_queued": true})
		return
	}
	_, dup := a.existingDownloadLocked(key)
	if dup && !in.AllowDuplicates {
		a.mu.Unlock()
		writeJSON(w, 409, map[string]any{"error": "\uc774\ubbf8 \ubc1b\uc740 \uc601\uc0c1\uc774\uc57c. \ubcc4\ub3c4 \uc0ac\ubcf8\uc73c\ub85c \ub2e4\uc2dc \ubc1b\uc744\uae4c?", "duplicate": true})
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
	a.trimHistoryLocked()
	a.browser.Sources[id] = in
	a.state.Jobs = append([]*Job{{ID: id, URL: in.PageURL, Host: "www.reddit.com", Title: in.Title, Status: "queued", Time: time.Now().Format("2006-01-02 15:04"), Detail: "\ube0c\ub77c\uc6b0\uc800\uc5d0\uc11c \ubcf4\ub0b8 \uc601\uc0c1 \ub300\uae30 \uc911", Redownload: dup, BrowserSource: true}}, a.state.Jobs...)
	a.mu.Unlock()
	// Keep the handler bounded even if another request fills the queue concurrently.
	select {
	case a.queue <- id:
		writeJSON(w, 202, map[string]any{"ok": true, "id": id})
	default:
		a.mutateJob(id, func(j *Job) {
			j.Status = "error"
			j.Detail = "\ub300\uae30\uc5f4\uc774 \uac00\ub4dd \ucc3c. \ub2e4\uc2dc \ubcf4\ub0b4 \uc918."
		})
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
