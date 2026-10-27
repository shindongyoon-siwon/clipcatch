package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

type workspaceClient struct {
	Seen     time.Time
	Sequence uint64
	Ack      uint64
}

func (a *App) workspaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /lifecycle.js", func(w http.ResponseWriter, r *http.Request) {
		b, e := pages.ReadFile("web/lifecycle.js")
		if e != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("POST /api/workspace/heartbeat", a.workspaceHeartbeat)
	mux.HandleFunc("POST /api/workspace/closed", a.workspaceClosed)
	mux.HandleFunc("POST /api/workspace/open", a.workspaceOpen)
	mux.HandleFunc("GET /api/workspace/status", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		active := a.pendingTasksLocked()
		stopping := a.stopping
		a.mu.Unlock()
		writeJSON(w, 200, map[string]any{"active": active, "stopping": stopping})
	})
}
func (a *App) pendingTasksLocked() int {
	n := a.qualityUploads
	for _, j := range a.state.Jobs {
		if j != nil && (j.Status == "queued" || j.Status == "downloading") {
			n++
		}
	}
	if a.premiereSelecting {
		n++
	}
	return n
}
func validWorkspaceID(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
func (a *App) workspaceHeartbeat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID  string `json:"id"`
		Ack uint64 `json:"ack"`
	}
	if premiereJSON(w, r, &in) != nil || !validWorkspaceID(in.ID) {
		writeError(w, 400, "Invalid workspace identity")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for id, c := range a.workspaceClients {
		if now.Sub(c.Seen) > 90*time.Second {
			delete(a.workspaceClients, id)
		}
	}
	c, exists := a.workspaceClients[in.ID]
	if !exists && len(a.workspaceClients) >= 32 {
		writeError(w, 429, "Too many workspaces")
		return
	}
	c.Seen = now
	if in.Ack == c.Sequence {
		c.Ack = in.Ack
	}
	a.workspaceClients[in.ID] = c
	writeJSON(w, 200, map[string]any{"ok": true, "sequence": c.Sequence, "stopping": a.stopping})
}
func (a *App) workspaceClosed(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if premiereJSON(w, r, &in) != nil || !validWorkspaceID(in.ID) {
		writeError(w, 400, "Invalid workspace identity")
		return
	}
	a.mu.Lock()
	delete(a.workspaceClients, in.ID)
	a.mu.Unlock()
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (a *App) workspaceOpen(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if premiereJSON(w, r, &in) != nil {
		writeError(w, 400, "Invalid request")
		return
	}
	a.mu.Lock()
	now := time.Now()
	id := ""
	var newest time.Time
	for k, c := range a.workspaceClients {
		if now.Sub(c.Seen) <= 90*time.Second && c.Seen.After(newest) {
			id = k
			newest = c.Seen
		}
	}
	if id != "" {
		a.workspaceSequence++
		c := a.workspaceClients[id]
		c.Sequence = a.workspaceSequence
		a.workspaceClients[id] = c
	}
	reused := id != ""
	if !reused {
		reused = now.Sub(a.workspaceOpening) < 3*time.Second
		if !reused {
			a.workspaceOpening = now
		}
	}
	a.mu.Unlock()
	writeJSON(w, 200, map[string]bool{"reused": reused})
}
func reopenWorkspace(base string) {
	req, e := http.NewRequest("POST", base+"/api/workspace/open", strings.NewReader("{}"))
	if e == nil {
		req.Header.Set("Origin", base)
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 2 * time.Second}
		if resp, e := client.Do(req); e == nil {
			var out struct {
				Reused bool `json:"reused"`
			}
			err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&out)
			resp.Body.Close()
			if resp.StatusCode == 200 && err == nil && out.Reused {
				return
			}
		}
	}
	openBrowser(base + "/")
}
func waitForWorkspace(base string, timeout time.Duration) bool {
	client := &http.Client{Timeout: 400 * time.Millisecond}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, e := client.Get(base + "/api/health")
		if e == nil {
			var h struct {
				App     string `json:"app"`
				Version string `json:"version"`
				Channel string `json:"channel"`
			}
			err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&h)
			resp.Body.Close()
			if resp.StatusCode == 200 && err == nil {
				if h.App != runtimeAppID() || h.Version != appVersion || h.Channel != appChannel {
					return false
				}
				page, e := client.Get(base + "/")
				if e == nil {
					page.Body.Close()
					if page.StatusCode == 200 {
						return true
					}
				}
			}
		}
		time.Sleep(80 * time.Millisecond)
	}
	return false
}
func (a *App) stopHandler(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Force bool `json:"force"`
	}
	if premiereJSON(w, r, &in) != nil {
		writeError(w, 400, "Invalid shutdown request")
		return
	}
	a.mu.Lock()
	active := a.pendingTasksLocked()
	if active > 0 && !in.Force {
		a.mu.Unlock()
		writeJSON(w, 409, map[string]any{"error": "Active work requires confirmation", "active": active})
		return
	}
	a.stopping = true
	a.mu.Unlock()
	writeJSON(w, 200, map[string]bool{"ok": true, "stopping": true})
	go func() {
		time.Sleep(250 * time.Millisecond)
		a.stopWork()
		if a.server != nil {
			_ = a.server.Close()
		}
	}()
}
