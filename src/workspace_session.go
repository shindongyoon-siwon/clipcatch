package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

const workspaceCloseGrace = 15 * time.Second

type workspaceStream struct {
	open     chan struct{}
	replaced chan struct{}
}

// One idle local connection per page replaces browser heartbeat polling.
// Hidden/minimized tabs keep their connection. A disconnect (close, refresh or
// crash) only arms the grace period; it never immediately cancels user work.
func (a *App) workspaceEvents(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	origin, site := r.Header.Get("Origin"), r.Header.Get("Sec-Fetch-Site")
	if !validWorkspaceID(id) || (origin != "" && origin != "http://"+r.Host) || (site != "" && site != "same-origin") {
		writeError(w, http.StatusForbidden, "Invalid workspace origin or identity")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "Streaming unsupported")
		return
	}
	c := &workspaceStream{open: make(chan struct{}, 1), replaced: make(chan struct{})}
	a.mu.Lock()
	if a.stopping {
		a.mu.Unlock()
		writeError(w, 503, "Workspace stopping")
		return
	}
	if a.workspaceStreams == nil {
		a.workspaceStreams = make(map[string]*workspaceStream)
	}
	old := a.workspaceStreams[id]
	if old == nil && len(a.workspaceStreams) >= 32 {
		a.mu.Unlock()
		writeError(w, 429, "Too many workspaces")
		return
	}
	if old != nil {
		close(old.replaced)
	}
	a.workspaceStreams[id] = c
	a.workspaceClients[id] = workspaceClient{Seen: time.Now()}
	a.workspaceWasOpened = true
	a.cancelWorkspaceIdleLocked()
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		// An older connection's delayed close must not remove its replacement.
		if a.workspaceStreams[id] != c {
			return
		}
		delete(a.workspaceStreams, id)
		delete(a.workspaceClients, id)
		if len(a.workspaceStreams) == 0 && !a.stopping {
			a.armWorkspaceIdleLocked(a.workspaceGrace())
		}
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := fmt.Fprint(w, "event: ready\ndata: {}\n\n"); err != nil {
		return
	}
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-a.workerStop:
			return
		case <-c.replaced:
			return
		case <-c.open:
			if _, err := fmt.Fprint(w, "event: open-workspace\ndata: {}\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
func (a *App) workspaceGrace() time.Duration {
	if a.workspaceCloseDelay > 0 {
		return a.workspaceCloseDelay
	}
	return workspaceCloseGrace
}
func (a *App) cancelWorkspaceIdleLocked() {
	a.workspaceIdleSerial++
	if a.workspaceIdleTimer != nil {
		a.workspaceIdleTimer.Stop()
		a.workspaceIdleTimer = nil
	}
}
func (a *App) armWorkspaceIdleLocked(delay time.Duration) {
	a.cancelWorkspaceIdleLocked()
	serial := a.workspaceIdleSerial
	a.workspaceIdleTimer = time.AfterFunc(delay, func() { a.closeIdleWorkspace(serial) })
}
func (a *App) closeIdleWorkspace(serial uint64) {
	a.mu.Lock()
	if a.stopping || serial != a.workspaceIdleSerial || len(a.workspaceStreams) != 0 {
		a.mu.Unlock()
		return
	}
	// Old tabs surviving a version upgrade may still use heartbeat messages.
	legacyOpen := false
	for _, c := range a.workspaceClients {
		if time.Since(c.Seen) < 90*time.Second {
			legacyOpen = true
			break
		}
	}
	// Do not interrupt active downloads, uploads, processing, tool preparation,
	// installation or rollback. Recheck locally only while such work is pending.
	if legacyOpen || a.pendingTasksLocked() > 0 || a.state.Phase == "preparing" || updateInProgress(a.state.AppUpdate.Status) {
		a.armWorkspaceIdleLocked(time.Second)
		a.mu.Unlock()
		return
	}
	a.stopping = true
	server := a.server
	a.mu.Unlock()
	log.Println("Last workspace closed; pending work complete; exiting")
	a.stopWork()
	if server != nil {
		_ = server.Close()
	}
}
