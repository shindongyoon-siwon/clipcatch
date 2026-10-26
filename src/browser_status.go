package main

import (
	"math"
	"net/http"
	"os"
)

// This context key cannot be supplied by an untrusted JSON caller.
type browserOwnerContextKey struct{}

type browserJobView struct {
	ID        string  `json:"id"`
	Status    string  `json:"status"`
	Stage     string  `json:"stage"`
	Percent   float64 `json:"percent"`
	Mode      string  `json:"output_mode"`
	FileCount int     `json:"file_count"`
	ErrorCode string  `json:"error_code,omitempty"`
}

func (a *App) browserJobStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []string `json:"ids"`
	}
	if decodeBrowserJSON(w, r, &in) != nil || len(in.IDs) > 30 {
		writeError(w, 400, "Invalid status request")
		return
	}
	wanted := map[string]bool{}
	for _, id := range in.IDs {
		if len(id) < 1 || len(id) > 180 {
			writeError(w, 400, "Invalid job ID")
			return
		}
		wanted[id] = true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	result := []browserJobView{}
	for _, j := range a.state.Jobs {
		// Never expose other app jobs, paths, URLs, tokens or cookies to the extension.
		if !wanted[j.ID] || j.BrowserOwner == "" || !sameDigest(j.BrowserOwner, a.browser.Link.Digest) {
			continue
		}
		p := j.Percent
		if math.IsNaN(p) || math.IsInf(p, 0) {
			p = 0
		}
		p = math.Max(0, math.Min(100, p))
		count := 0
		if j.Status == "done" && nonempty(j.File) {
			count = 1
		}
		if len(j.Media) > 0 {
			count = 0
			for _, m := range j.Media {
				if nonempty(m.File) && m.Error == "" && (j.OutputMode != premiereMode || m.EditReady) {
					count++
				}
			}
		}
		result = append(result, browserJobView{ID: j.ID, Status: j.Status, Stage: j.Stage, Percent: p, Mode: j.OutputMode, FileCount: count, ErrorCode: j.ErrorCode})
	}
	writeJSON(w, 200, map[string]any{"app": runtimeAppID(), "channel": appChannel, "version": appVersion, "phase": a.state.Phase, "update_required": a.updateRequiredLocked(), "update_checking": a.requireStartupCheck && a.state.AppUpdate.Status == "checking", "capabilities": []string{"output-mode", "job-status"}, "jobs": result})
}
func (a *App) browserOpenDownloads(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if decodeBrowserJSON(w, r, &in) != nil {
		writeError(w, 400, "Invalid request")
		return
	}
	if e := os.MkdirAll(a.saveDir, 0755); e != nil {
		writeError(w, 500, "Cannot prepare download folder")
		return
	}
	if e := openFolder(a.saveDir); e != nil {
		writeError(w, 500, "Could not open the download folder")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// Called with a.mu held. Only return progress for jobs owned by this pairing.
func (a *App) ownedPendingBrowserJobs(key, owner string) []string {
	ids := []string{}
	if owner == "" {
		return ids
	}
	for _, j := range a.state.Jobs {
		if (j.Status == "queued" || j.Status == "downloading") && downloadJobKey(j) == key && sameDigest(owner, j.BrowserOwner) {
			ids = append(ids, j.ID)
		}
	}
	return ids
}
