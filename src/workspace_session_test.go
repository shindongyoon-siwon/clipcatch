package main

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func sessionApp(t *testing.T) (*App, *httptest.Server) {
	t.Helper()
	a := discoveryApp(t)
	a.workspaceCloseDelay = 80 * time.Millisecond
	s := httptest.NewServer(http.HandlerFunc(a.serveHTTP))
	a.server = s.Config
	t.Cleanup(func() { a.stopWork(); s.Close() })
	return a, s
}
func connectSession(t *testing.T, base, id string) io.ReadCloser {
	t.Helper()
	response, err := http.Get(base + "/api/workspace/events?id=" + id)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil || line != "event: ready\n" {
		t.Fatal(line, err)
	}
	return response.Body
}
func awaitSessionCount(t *testing.T, a *App, n int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		got := len(a.workspaceStreams)
		a.mu.Unlock()
		if got == n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("expected %d connections", n)
}
func aliveSession(t *testing.T, a *App) {
	t.Helper()
	select {
	case <-a.workerStop:
		t.Fatal("app stopped with a live tab or pending work")
	default:
	}
}
func TestLastTabOnlyAndHiddenTabsNeverExpire(t *testing.T) {
	a, s := sessionApp(t)
	first := connectSession(t, s.URL, strings.Repeat("a", 32))
	second := connectSession(t, s.URL, strings.Repeat("b", 32))
	a.mu.Lock()
	for id, c := range a.workspaceClients {
		c.Seen = time.Now().Add(-3 * time.Hour)
		a.workspaceClients[id] = c
	}
	a.mu.Unlock()
	first.Close()
	awaitSessionCount(t, a, 1)
	time.Sleep(120 * time.Millisecond)
	aliveSession(t, a)
	second.Close()
	select {
	case <-a.workerStop:
	case <-time.After(2 * time.Second):
		t.Fatal("last tab did not close app")
	}
}
func TestReloadAndDuplicateConnectionDoNotTriggerExit(t *testing.T) {
	a, s := sessionApp(t)
	id := strings.Repeat("a", 32)
	first := connectSession(t, s.URL, id)
	replacement := connectSession(t, s.URL, id)
	first.Close()
	time.Sleep(120 * time.Millisecond)
	aliveSession(t, a)
	replacement.Close()
	awaitSessionCount(t, a, 0)
	fresh := connectSession(t, s.URL, strings.Repeat("b", 32))
	defer fresh.Close()
	time.Sleep(150 * time.Millisecond)
	aliveSession(t, a)
}
func TestLastTabWaitsForPendingTasks(t *testing.T) {
	for _, status := range []string{"queued", "downloading", "upload", "installing", "preparing"} {
		t.Run(status, func(t *testing.T) {
			a, s := sessionApp(t)
			a.mu.Lock()
			switch status {
			case "upload":
				a.qualityUploads = 1
			case "installing":
				a.state.AppUpdate.Status = "installing"
			case "preparing":
				a.state.Phase = "preparing"
			default:
				a.state.Jobs = []*Job{{ID: "keep", Status: status}}
			}
			a.mu.Unlock()
			page := connectSession(t, s.URL, strings.Repeat("c", 32))
			page.Close()
			time.Sleep(180 * time.Millisecond)
			aliveSession(t, a)
			a.mu.Lock()
			a.qualityUploads = 0
			a.state.AppUpdate.Status = "current"
			a.state.Phase = "ready"
			for _, j := range a.state.Jobs {
				if j.Status != status {
					t.Error("job cancelled before completion")
				}
				j.Status = "done"
			}
			a.mu.Unlock()
			select {
			case <-a.workerStop:
			case <-time.After(2 * time.Second):
				t.Fatal("did not stop after work completed")
			}
		})
	}
}
func TestWorkspaceEventsRejectCrossOrigin(t *testing.T) {
	a, s := sessionApp(t)
	for _, site := range []string{"cross-site", "same-site"} {
		req, _ := http.NewRequest("GET", s.URL+"/api/workspace/events?id="+strings.Repeat("a", 32), nil)
		req.Header.Set("Sec-Fetch-Site", site)
		r, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		r.Body.Close()
		if r.StatusCode != 403 {
			t.Fatal("cross-origin workspace accepted", site, r.StatusCode)
		}
	}
	aliveSession(t, a)
}
