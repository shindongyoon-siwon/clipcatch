package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Downloads remain recognizable even after the visible history is trimmed.
// Nothing here reads browser profiles or changes the downloaded video files.
type downloadRecord struct {
	File  string `json:"file"`
	Title string `json:"title"`
	Time  string `json:"time"`
	Stamp int64  `json:"stamp"`
}

type duplicateInfo struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Time  string `json:"time"`
}

var (
	redditPostRE = regexp.MustCompile(`(?i)/comments/([a-z0-9]{5,12})(?:/|$)`)
	statusPostRE = regexp.MustCompile(`(?i)/status/(\d+)`)
)

func canonicalJobKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	p := strings.TrimRight(u.Path, "/")
	switch {
	case host == "reddit.com" || host == "redd.it" || strings.HasSuffix(host, ".reddit.com"):
		if m := redditPostRE.FindStringSubmatch(p); m != nil {
			return "reddit:" + strings.ToLower(m[1])
		}
		if host == "redd.it" && regexp.MustCompile(`(?i)^/[a-z0-9]{5,12}$`).MatchString(p) {
			return "reddit:" + strings.ToLower(strings.TrimPrefix(p, "/"))
		}
	case host == "x.com" || host == "twitter.com" || strings.HasSuffix(host, ".twitter.com"):
		if m := statusPostRE.FindStringSubmatch(p); m != nil {
			return "x:" + m[1]
		}
	case host == "youtube.com" || strings.HasSuffix(host, ".youtube.com"):
		if id := u.Query().Get("v"); id != "" {
			return "youtube:" + id
		}
		if strings.HasPrefix(p, "/shorts/") {
			return "youtube:" + strings.TrimPrefix(p, "/shorts/")
		}
	case host == "youtu.be":
		if p != "" {
			return "youtube:" + strings.TrimPrefix(p, "/")
		}
	}
	// Non-post URLs retain their query to avoid combining genuinely different videos.
	return host + strings.ToLower(p) + "?" + u.RawQuery
}

func (a *App) loadDownloadIndex() {
	data, err := os.ReadFile(filepath.Join(a.dataDir, "download-index.json"))
	if err != nil {
		return
	}
	var records map[string]downloadRecord
	if json.Unmarshal(data, &records) == nil && len(records) <= 20000 {
		a.downloaded = records
	}
}

func (a *App) existingDownloadLocked(key string) (duplicateInfo, bool) {
	for _, j := range a.state.Jobs {
		if canonicalJobKey(j.URL) != key || j.Status != "done" || !nonempty(j.File) {
			continue
		}
		return duplicateInfo{URL: j.URL, Title: j.Title, Time: j.Time}, true
	}
	if record, ok := a.downloaded[key]; ok && nonempty(record.File) {
		return duplicateInfo{Title: record.Title, Time: record.Time}, true
	}
	return duplicateInfo{}, false
}

func (a *App) hasQueuedDownloadLocked(key string) bool {
	for _, j := range a.state.Jobs {
		if (j.Status == "queued" || j.Status == "downloading") && canonicalJobKey(j.URL) == key {
			return true
		}
	}
	return false
}

// History and the durable duplicate index share one writer to prevent races
// between job cancellation, download completion and app shutdown.
func (a *App) saveHistory() {
	a.persistMu.Lock()
	defer a.persistMu.Unlock()
	a.mu.Lock()
	history := make([]*Job, 0, len(a.state.Jobs))
	if a.downloaded == nil {
		a.downloaded = make(map[string]downloadRecord)
	}
	for _, j := range a.state.Jobs {
		if j.Status == "done" && nonempty(j.File) {
			key := canonicalJobKey(j.URL)
			// Newer completed copies take precedence over older copies.
			if r, exists := a.downloaded[key]; !exists || r.Time <= j.Time {
				a.downloaded[key] = downloadRecord{File: j.File, Title: j.Title, Time: j.Time, Stamp: time.Now().Unix()}
			}
		}
		if j.Status == "done" || j.Status == "error" || j.Status == "cancelled" {
			if len(history) < 50 {
				cp := *j
				history = append(history, &cp)
			}
		}
	}
	// Bound the disk index to avoid unbounded growth during daily content work.
	if len(a.downloaded) > 5000 {
		type entry struct {
			key string
			at  int64
		}
		times := make([]entry, 0, len(a.downloaded))
		for key, r := range a.downloaded {
			times = append(times, entry{key, r.Stamp})
		}
		sort.Slice(times, func(i, j int) bool { return times[i].at > times[j].at })
		for _, old := range times[5000:] {
			delete(a.downloaded, old.key)
		}
	}
	index := make(map[string]downloadRecord, len(a.downloaded))
	for k, v := range a.downloaded {
		index[k] = v
	}
	a.mu.Unlock()
	if err := os.MkdirAll(a.dataDir, 0700); err != nil {
		return
	}
	if data, err := json.MarshalIndent(history, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(a.dataDir, "history.json"), data, 0600)
	}
	if data, err := json.Marshal(index); err == nil {
		_ = os.WriteFile(filepath.Join(a.dataDir, "download-index.json"), data, 0600)
	}
}

func (a *App) isJobCancelled(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, j := range a.state.Jobs {
		if j.ID == id {
			return j.Status == "cancelled"
		}
	}
	return true // Deleted/unrecognized jobs must not run.
}

func (a *App) cancelDownloadHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	var input struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ID == "" {
		writeError(w, 400, "취소할 다운로드를 선택해 줘.")
		return
	}
	a.mu.Lock()
	found := false
	var stop func()
	var cmd *exec.Cmd
	for _, j := range a.state.Jobs {
		if j.ID != input.ID {
			continue
		}
		if j.Status != "queued" && j.Status != "downloading" {
			break
		}
		found = true
		j.Status = "cancelled"
		j.Detail = "사용자가 다운로드를 취소했어."
		j.Speed, j.ETA = "", ""
		if a.activeJobID == input.ID {
			stop = a.activeCancel
			cmd = a.activeCmd
		}
		break
	}
	a.mu.Unlock()
	if !found {
		writeError(w, 409, "이미 완료되었거나 취소할 수 없는 다운로드야.")
		return
	}
	if cmd != nil && cmd.Process != nil && runtime.GOOS == "windows" {
		// Kill yt-dlp with its FFmpeg child tree before the parent exits.
		_ = exec.Command("taskkill.exe", "/T", "/F", "/PID", stringPID(cmd.Process.Pid)).Run()
	}
	if stop != nil {
		stop()
	}
	a.saveHistory()
	writeJSON(w, 200, map[string]any{"ok": true, "id": input.ID, "status": "cancelled"})
}

func (a *App) retryFailedHandler(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	active := map[string]bool{}
	for _, j := range a.state.Jobs {
		if j.Status == "queued" || j.Status == "downloading" {
			active[canonicalJobKey(j.URL)] = true
		}
	}
	var queued []string
	var skipped int
	for _, j := range a.state.Jobs {
		if j.Status != "error" {
			continue
		}
		key := canonicalJobKey(j.URL)
		if active[key] {
			skipped++
			continue
		}
		active[key] = true
		j.Status, j.Percent, j.Detail = "queued", 0, "실패한 영상 다시 시도 대기 중"
		j.Speed, j.ETA, j.File = "", "", ""
		j.AuthRequired = false
		j.Time = time.Now().Format("2006-01-02 15:04")
		queued = append(queued, j.ID)
	}
	a.mu.Unlock()
	for _, id := range queued {
		a.queue <- id
	}
	writeJSON(w, 200, map[string]any{"ok": true, "count": len(queued), "skipped": skipped})
}

func stringPID(n int) string {
	return strconv.Itoa(n)
}
