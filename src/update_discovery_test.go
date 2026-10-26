package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func discoveryRelease(t *testing.T, version string) verifiedRelease {
	t.Helper()
	rel, err := parseGithubRelease(releaseFixture(version))
	if err != nil {
		t.Fatal(err)
	}
	return rel
}

func discoveryHint(t *testing.T, version string) updateHint {
	t.Helper()
	data, _ := json.Marshal(buildIdentity{App: "clipcatch", Channel: "stable", Version: version, ShareURL: stableShareURL})
	hint, err := parseStableUpdateHint(data)
	if err != nil {
		t.Fatal(err)
	}
	return hint
}

func discoveryApp(t *testing.T) *App {
	t.Helper()
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Phase = "ready"
	a.state.AppUpdate = UpdateInfo{Status: "current"}
	// Every discovery test has per-app providers, including unexpected paths.
	a.updateHooks = updateCheckHooks{
		now: func() time.Time { return time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC) },
		probe: func(context.Context) (updateHint, error) {
			return updateHint{}, errors.New("fixture probe not configured")
		},
		fetch: func(context.Context) (verifiedRelease, error) {
			return verifiedRelease{}, errors.New("fixture release not configured")
		},
	}
	return a
}

func runDiscovery(t *testing.T, a *App) {
	t.Helper()
	check, reason := a.beginAppUpdateCheck(true, false)
	if check == nil {
		t.Fatalf("automatic check not claimed: %s", reason)
	}
	a.runAppUpdateCheck(check)
}

func advanceDiscovery(a *App) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.updateCheck.nextAutomatic.Add(time.Nanosecond)
	a.updateHooks.now = func() time.Time { return now }
}

func discoveryRequest(a *App, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api/check-app-update", strings.NewReader(body))
	r.Header.Set("Origin", "http://127.0.0.1:8765")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.serveHTTP(w, r)
	return w
}

func TestStableUpdateHintCannotSupplyInstallerOrCrossChannels(t *testing.T) {
	valid := buildIdentity{App: "clipcatch", Channel: "stable", Version: "9.9.1", ShareURL: stableShareURL}
	data, _ := json.Marshal(valid)
	if hint, err := parseStableUpdateHint(data); err != nil || hint.Version != valid.Version || len(hint.Fingerprint) != 64 {
		t.Fatal(hint, err)
	}
	for _, edit := range []func(*buildIdentity){
		func(b *buildIdentity) { b.App, b.Channel, b.Version = "clipcatch-test2", "test", "9.9.1-test.1" },
		func(b *buildIdentity) { b.App = "another-app" },
		func(b *buildIdentity) { b.Version = "9.9.1-test.1" },
		func(b *buildIdentity) { b.ShareURL = "https://example.com/untrusted.exe" },
	} {
		bad := valid
		edit(&bad)
		data, _ := json.Marshal(bad)
		if _, err := parseStableUpdateHint(data); err == nil {
			t.Fatal("unsafe hint accepted", bad)
		}
	}
	for _, bad := range [][]byte{[]byte("not JSON"), append(data, []byte(strings.Repeat(" ", 4097))...)} {
		if _, err := parseStableUpdateHint(bad); err == nil {
			t.Fatal("malformed/oversized hint accepted")
		}
	}
}

func TestStartupUpdateCheckAlwaysReleasesCheckingGate(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "offline"}[fail], func(t *testing.T) {
			a := discoveryApp(t)
			a.requireStartupCheck = true
			a.state.AppUpdate.Status = "checking"
			rel := discoveryRelease(t, appVersion)
			a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) {
				if fail {
					return verifiedRelease{}, errors.New("offline fixture")
				}
				return rel, nil
			}
			if !a.newWorkBlockedLocked() {
				t.Fatal("startup gate missing")
			}
			a.checkAppUpdate()
			if a.requireStartupCheck || a.startupUpdateCheckingLocked() || a.newWorkBlockedLocked() || a.updateCheck.running {
				t.Fatal("startup gate not released", a.state.AppUpdate)
			}
			want := "current"
			if fail {
				want = "error"
			}
			if a.state.AppUpdate.Status != want {
				t.Fatal(a.state.AppUpdate)
			}
		})
	}
}

func TestAutomaticDiscoveryQuietUntilReleaseVerification(t *testing.T) {
	a := discoveryApp(t)
	a.latestRelease = discoveryRelease(t, appVersion)
	a.state.Jobs = []*Job{{ID: "active", Status: "downloading", Detail: "working"}}
	hint, next := discoveryHint(t, "9.9.1"), discoveryRelease(t, "9.9.1")
	a.updateHooks.probe = func(context.Context) (updateHint, error) { return hint, nil }
	fetchStarted, releaseFetch, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) {
		close(fetchStarted)
		<-releaseFetch
		return next, nil
	}
	check, _ := a.beginAppUpdateCheck(true, false)
	go func() { a.runAppUpdateCheck(check); close(done) }()
	<-fetchStarted
	a.mu.Lock()
	blocked := a.newWorkBlockedLocked()
	checking, status := a.startupUpdateCheckingLocked(), a.state.AppUpdate.Status
	a.mu.Unlock()
	if blocked || checking || status != "current" {
		t.Error("hint exposed an unverified update", status, blocked, checking)
	}
	close(releaseFetch)
	<-done
	if a.state.AppUpdate.Status != "available" || !a.updateRequiredLocked() || a.latestRelease != next {
		t.Fatal("verified update not shown", a.state.AppUpdate)
	}
	if a.state.Jobs[0].Status != "downloading" || a.state.Jobs[0].Detail != "working" {
		t.Fatal("active job changed")
	}
	if _, err := os.Stat(filepath.Join(a.dataDir, "required-update.json")); err != nil {
		t.Fatal("verified update not persisted", err)
	}
}

func TestAutomaticDiscoveryDeduplicatesEventsAndManualChecks(t *testing.T) {
	a := discoveryApp(t)
	a.latestRelease = discoveryRelease(t, appVersion)
	hint := discoveryHint(t, appVersion)
	var probes, fetches atomic.Int32
	started, releaseProbe := make(chan struct{}), make(chan struct{})
	a.updateHooks.probe = func(context.Context) (updateHint, error) {
		if probes.Add(1) == 1 {
			close(started)
		}
		<-releaseProbe
		return hint, nil
	}
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) { fetches.Add(1); return a.latestRelease, nil }
	check, _ := a.beginAppUpdateCheck(true, false)
	done := make(chan struct{})
	go func() { a.runAppUpdateCheck(check); close(done) }()
	<-started
	var wg sync.WaitGroup
	for n := 0; n < 20; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := discoveryRequest(a, `{"automatic":true}`)
			if w.Code != 200 || !strings.Contains(w.Body.String(), "already_running") {
				t.Error(w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	if w := discoveryRequest(a, "{}"); w.Code != 200 || !strings.Contains(w.Body.String(), "already_running") {
		t.Error(w.Code, w.Body.String())
	}
	close(releaseProbe)
	<-done
	if w := discoveryRequest(a, `{"automatic":true}`); w.Code != 200 || !strings.Contains(w.Body.String(), "skipped") {
		t.Fatal(w.Code, w.Body.String())
	}
	if probes.Load() != 1 || fetches.Load() != 0 {
		t.Fatal("duplicate network checks", probes.Load(), fetches.Load())
	}
	// An explicit manual check can bypass the automatic freshness cooldown.
	manual, reason := a.beginAppUpdateCheck(false, false)
	if manual == nil {
		t.Fatal("manual check throttled", reason)
	}
	a.runAppUpdateCheck(manual)
	if fetches.Load() != 1 {
		t.Fatal("manual release lookup missing")
	}
}

func TestForegroundDiscoveryDoesNotWaitForBackgroundInterval(t *testing.T) {
	a := discoveryApp(t)
	current, next := discoveryRelease(t, appVersion), discoveryRelease(t, "9.9.1")
	hint := discoveryHint(t, next.Version)
	now := a.updateHooks.now()
	started := now
	a.updateHooks.now = func() time.Time { return now }
	probes, fetches := 0, 0
	a.updateHooks.probe = func(context.Context) (updateHint, error) { probes++; return hint, nil }
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) {
		fetches++
		if fetches == 1 {
			return current, nil
		}
		return next, nil
	}
	a.requireStartupCheck = true
	a.checkAppUpdate()
	// Repeated page events immediately after startup still share its result.
	now = started.Add(30 * time.Second)
	if check, reason := a.beginAppUpdateCheck(true, false); check != nil || reason != "throttled" {
		t.Fatal("page event bypassed short freshness guard", reason)
	}
	// Returning to use the app can find a release well before the 15-minute poll.
	now = started.Add(2 * time.Minute)
	runDiscovery(t, a)
	if probes != 1 || fetches != 2 || a.latestRelease != next || a.state.AppUpdate.Status != "available" || !a.updateRequiredLocked() {
		t.Fatal("foreground discovery waited for background cadence", probes, fetches, a.state.AppUpdate)
	}
}

func TestBackgroundDiscoveryWaitsFifteenMinutes(t *testing.T) {
	a := discoveryApp(t)
	current, next := discoveryRelease(t, appVersion), discoveryRelease(t, "9.9.1")
	hint := discoveryHint(t, next.Version)
	started := a.updateHooks.now()
	now := started
	a.updateHooks.now = func() time.Time { return now }
	var probes, fetches atomic.Int32
	probed := make(chan struct{}, 1)
	a.updateHooks.probe = func(context.Context) (updateHint, error) {
		probes.Add(1)
		probed <- struct{}{}
		return hint, nil
	}
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) {
		if fetches.Add(1) == 1 {
			return current, nil
		}
		return next, nil
	}
	ticks, created := make(chan time.Time), make(chan time.Duration, 1)
	var tickerStopped atomic.Bool
	a.updateHooks.newTicker = func(interval time.Duration) (<-chan time.Time, func()) {
		created <- interval
		return ticks, func() { tickerStopped.Store(true) }
	}
	done := make(chan struct{})
	go func() { a.watchAppUpdates(); close(done) }()
	defer func() {
		close(a.workerStop)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("update watcher did not stop")
		}
		if !tickerStopped.Load() {
			t.Error("background ticker was not stopped")
		}
	}()
	var interval time.Duration
	select {
	case interval = <-created:
	case <-time.After(2 * time.Second):
		t.Fatal("background ticker was not created")
	}
	nextTick := started.Add(interval)
	advance := func(elapsed time.Duration) {
		a.mu.Lock()
		now = started.Add(elapsed)
		a.mu.Unlock()
		for !now.Before(nextTick) {
			select {
			case ticks <- nextTick:
			case <-time.After(2 * time.Second):
				t.Fatal("watcher did not consume scheduled tick")
			}
			select {
			case <-probed:
			case <-time.After(2 * time.Second):
				t.Fatal("scheduled background tick did not check update")
			}
			nextTick = nextTick.Add(interval)
		}
	}
	if fetches.Load() != 1 {
		t.Fatal("startup release check missing")
	}
	advance(time.Minute)
	if probes.Load() != 0 {
		t.Fatal("background still checks every minute")
	}
	advance(15*time.Minute - time.Nanosecond)
	if probes.Load() != 0 {
		t.Fatal("background checked before fifteen minutes")
	}
	advance(15 * time.Minute)
	if probes.Load() != 1 {
		t.Fatal("background did not check at fifteen minutes")
	}
}

func TestAutomaticDiscoveryInstallAndStopRejectLateResults(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(map[bool]string{false: "install", true: "stop"}[stop], func(t *testing.T) {
			a := discoveryApp(t)
			known, next := discoveryRelease(t, "9.9.1"), discoveryRelease(t, "9.9.2")
			a.latestRelease = known
			a.state.AppUpdate = UpdateInfo{Status: "available", Latest: known.Version}
			hint := discoveryHint(t, next.Version)
			a.updateHooks.probe = func(context.Context) (updateHint, error) { return hint, nil }
			started, releaseFetch, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) { close(started); <-releaseFetch; return next, nil }
			check, _ := a.beginAppUpdateCheck(true, false)
			go func() { a.runAppUpdateCheck(check); close(done) }()
			<-started
			a.mu.Lock()
			if stop {
				a.stopping = true
			} else {
				a.claimAppUpdateInstallLocked(known)
			}
			a.mu.Unlock()
			if !stop && check.ctx.Err() == nil {
				t.Error("installation did not cancel discovery")
			}
			if w := discoveryRequest(a, `{"automatic":true}`); w.Code != 200 || !strings.Contains(w.Body.String(), "skipped") {
				t.Error(w.Code, w.Body.String())
			}
			if w := discoveryRequest(a, "{}"); w.Code != 409 {
				t.Error("manual check interrupted install/stop", w.Code)
			}
			close(releaseFetch)
			<-done
			if a.latestRelease != known {
				t.Fatal("late release overwrote active install/stop")
			}
			if !stop && a.state.AppUpdate.Status != "downloading" {
				t.Fatal(a.state.AppUpdate)
			}
		})
	}
}

func TestAutomaticDiscoveryOfflineBackoffAndRecovery(t *testing.T) {
	a := discoveryApp(t)
	known := discoveryRelease(t, "9.9.1")
	a.latestRelease = known
	a.state.AppUpdate = UpdateInfo{Status: "error", Latest: known.Version, Detail: "keep installer failure"}
	a.saveRequiredUpdate(known)
	var probes, fetches int
	offline := true
	hint := discoveryHint(t, known.Version)
	a.updateHooks.probe = func(context.Context) (updateHint, error) {
		probes++
		if offline {
			return updateHint{}, errors.New("offline")
		}
		return hint, nil
	}
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) { fetches++; return known, nil }
	for n := 0; n < 6; n++ {
		start := a.updateHooks.now()
		runDiscovery(t, a)
		delay := a.updateCheck.nextAutomatic.Sub(start)
		if delay < time.Minute || delay > 15*time.Minute {
			t.Fatal("unbounded cooldown", delay)
		}
		if check, reason := a.beginAppUpdateCheck(true, false); check != nil || reason != "throttled" {
			t.Fatal("failed probe not throttled", reason)
		}
		advanceDiscovery(a)
	}
	if probes != 6 || fetches != 0 || a.state.AppUpdate.Detail != "keep installer failure" || a.latestRelease != known {
		t.Fatal("probe failure altered verified state or hit API")
	}
	offline = false
	runDiscovery(t, a)
	if a.updateCheck.failures != 0 || a.state.AppUpdate.Status != "error" || a.state.AppUpdate.Detail != "keep installer failure" || fetches != 0 {
		t.Fatal("same release lost prior error after reconnect")
	}
	if _, err := os.Stat(filepath.Join(a.dataDir, "required-update.json")); err != nil {
		t.Fatal("known update cache erased", err)
	}
	advanceDiscovery(a)
	next := discoveryRelease(t, "9.9.2")
	hint = discoveryHint(t, next.Version)
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) { fetches++; return next, nil }
	runDiscovery(t, a)
	if a.state.AppUpdate.Status != "available" || a.latestRelease != next || fetches != 1 {
		t.Fatal("reconnected new release not discovered")
	}
}

func TestAutomaticDiscoveryRetriesAPIAndCDNLagWithoutTrustingHint(t *testing.T) {
	a := discoveryApp(t)
	known, next := discoveryRelease(t, appVersion), discoveryRelease(t, "9.9.2")
	a.latestRelease = known
	hint := discoveryHint(t, next.Version)
	a.updateHooks.probe = func(context.Context) (updateHint, error) { return hint, nil }
	calls := 0
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) {
		calls++
		if calls == 1 {
			return verifiedRelease{}, errors.New("GitHub HTTP 403 fixture")
		}
		if calls == 2 {
			return known, nil
		}
		return next, nil
	}
	for n := 0; n < 2; n++ {
		start := a.updateHooks.now()
		runDiscovery(t, a)
		if a.state.AppUpdate.Status != "current" || a.updateRequiredLocked() || a.latestRelease != known {
			t.Fatal("hint trusted before full validation")
		}
		if delay := a.updateCheck.nextAutomatic.Sub(start); delay < 2*time.Minute || delay > 15*time.Minute {
			t.Fatal("API failure/lag not cooled down", delay)
		}
		if w := discoveryRequest(a, `{"automatic":true}`); !strings.Contains(w.Body.String(), "skipped") {
			t.Fatal("API cooldown bypass", w.Body.String())
		}
		advanceDiscovery(a)
	}
	runDiscovery(t, a)
	if calls != 3 || a.latestRelease != next || a.state.AppUpdate.Status != "available" {
		t.Fatal("lagging hint was permanently skipped", calls, a.state.AppUpdate)
	}
}

func TestAutomaticDiscoveryPreservesVerifiedUpdateOnStaleOrSameRelease(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "same", true: "stale"}[stale], func(t *testing.T) {
			a := discoveryApp(t)
			known := discoveryRelease(t, "9.9.2")
			a.latestRelease = known
			a.state.AppUpdate = UpdateInfo{Status: "error", Latest: known.Version, Detail: "installation checksum failed"}
			a.saveRequiredUpdate(known)
			hint := discoveryHint(t, "9.9.1")
			a.updateCheck.lastHint = "different published identity"
			a.updateHooks.probe = func(context.Context) (updateHint, error) { return hint, nil }
			rel := known
			if stale {
				rel = discoveryRelease(t, appVersion)
			}
			a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) { return rel, nil }
			runDiscovery(t, a)
			if a.latestRelease != known || a.state.AppUpdate.Status != "error" || a.state.AppUpdate.Detail != "installation checksum failed" || !a.updateRequiredLocked() {
				t.Fatal("verified update/error overwritten", a.state.AppUpdate)
			}
			if _, err := os.Stat(filepath.Join(a.dataDir, "required-update.json")); err != nil {
				t.Fatal("required cache erased", err)
			}
		})
	}
}

func TestAutomaticDiscoveryConfirmsWithdrawalWithBothSources(t *testing.T) {
	a := discoveryApp(t)
	known, current := discoveryRelease(t, "9.9.2"), discoveryRelease(t, appVersion)
	a.latestRelease = known
	a.state.AppUpdate = UpdateInfo{Status: "available", Latest: known.Version}
	a.saveRequiredUpdate(known)
	hint := discoveryHint(t, current.Version)
	a.updateHooks.probe = func(context.Context) (updateHint, error) { return hint, nil }
	calls := 0
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) {
		calls++
		if calls == 1 {
			return known, nil
		}
		return current, nil
	}
	// A first, older hint alone cannot retract the already verified update.
	start := a.updateHooks.now()
	runDiscovery(t, a)
	if calls != 1 || a.latestRelease != known || !a.updateRequiredLocked() {
		t.Fatal("old hint retracted update before API agreement")
	}
	if a.updateCheck.nextAutomatic.Sub(start) < 2*time.Minute {
		t.Fatal("reverse CDN/API lag not cooled down")
	}
	advanceDiscovery(a)
	runDiscovery(t, a)
	if calls != 2 || a.latestRelease != current || a.updateRequiredLocked() || a.state.AppUpdate.Status != "current" {
		t.Fatal("confirmed withdrawal left permanent update gate")
	}
	if _, err := os.Stat(filepath.Join(a.dataDir, "required-update.json")); !os.IsNotExist(err) {
		t.Fatal("withdrawn cache retained", err)
	}
}

func TestStartupUpdateKeepsExistingOfficialReleaseAuthority(t *testing.T) {
	a := discoveryApp(t)
	known, current := discoveryRelease(t, "9.9.2"), discoveryRelease(t, appVersion)
	a.latestRelease = known
	a.saveRequiredUpdate(known)
	a.requireStartupCheck = true
	a.state.AppUpdate.Status = "checking"
	probes := 0
	a.updateHooks.probe = func(context.Context) (updateHint, error) {
		probes++
		return updateHint{}, errors.New("startup does not probe")
	}
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) { return current, nil }
	a.checkAppUpdate()
	if probes != 0 || a.latestRelease != current || a.state.AppUpdate.Status != "current" || a.newWorkBlockedLocked() {
		t.Fatal("initial official release check changed", probes, a.state.AppUpdate)
	}
	if _, err := os.Stat(filepath.Join(a.dataDir, "required-update.json")); !os.IsNotExist(err) {
		t.Fatal("startup did not retract old requirement", err)
	}
}

func TestUpdateServerWaitIsHonoredByAutomaticAndManualDiscovery(t *testing.T) {
	for _, mode := range []string{"seconds", "date", "reset"} {
		t.Run(mode, func(t *testing.T) {
			a := discoveryApp(t)
			known := discoveryRelease(t, "9.9.1")
			a.latestRelease = known
			a.state.AppUpdate = UpdateInfo{Status: "available", Latest: known.Version}
			hint := discoveryHint(t, "9.9.2")
			a.updateHooks.probe = func(context.Context) (updateHint, error) { return hint, nil }
			now := a.updateHooks.now()
			deadline := now.Add(time.Hour)
			resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header)}
			switch mode {
			case "seconds":
				resp.Header.Set("Retry-After", "3600")
			case "date":
				resp.Header.Set("Retry-After", deadline.Format(http.TimeFormat))
			case "reset":
				resp.StatusCode = http.StatusForbidden
				resp.Header.Set("Retry-After", "120")
				resp.Header.Set("X-RateLimit-Reset", strconv.FormatInt(deadline.Unix(), 10))
			}
			calls := 0
			a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) {
				calls++
				return verifiedRelease{}, updateHTTPError(resp, "fixture", now)
			}
			runDiscovery(t, a)
			if !a.updateCheck.nextServerCheck.Equal(deadline) {
				t.Fatal("server deadline lost", a.updateCheck.nextServerCheck, deadline)
			}
			// Ordinary backoff expired, but the explicit server wait still applies.
			advanceDiscovery(a)
			if w := discoveryRequest(a, `{"automatic":true}`); w.Code != 200 || !strings.Contains(w.Body.String(), "skipped") {
				t.Fatal(w.Code, w.Body.String())
			}
			if w := discoveryRequest(a, "{}"); w.Code != http.StatusTooManyRequests {
				t.Fatal("manual bypassed server wait", w.Code)
			}
			if calls != 1 || a.state.AppUpdate.Status != "available" || a.latestRelease != known || !a.updateRequiredLocked() {
				t.Fatal("server wait changed already verified update")
			}
			// Retry-After affects discovery, not the existing installer claim.
			a.mu.Lock()
			a.claimAppUpdateInstallLocked(known)
			a.mu.Unlock()
			if a.state.AppUpdate.Status != "downloading" {
				t.Fatal("server wait blocked install")
			}
		})
	}
}

func TestAutomaticDiscoveryRecoversAfterServerWait(t *testing.T) {
	a := discoveryApp(t)
	known, next := discoveryRelease(t, appVersion), discoveryRelease(t, "9.9.1")
	a.latestRelease = known
	hint := discoveryHint(t, next.Version)
	a.updateHooks.probe = func(context.Context) (updateHint, error) { return hint, nil }
	deadline := a.updateHooks.now().Add(time.Hour)
	a.updateCheck.nextServerCheck = deadline
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) { return next, nil }
	a.updateHooks.now = func() time.Time { return deadline.Add(time.Second) }
	runDiscovery(t, a)
	if a.latestRelease != next || a.state.AppUpdate.Status != "available" {
		t.Fatal("server wait never expired")
	}
}

func TestAutomaticDiscoveryRejectsUnverifiedRelease(t *testing.T) {
	a := discoveryApp(t)
	known := discoveryRelease(t, appVersion)
	a.latestRelease = known
	hint := discoveryHint(t, "9.9.1")
	a.updateHooks.probe = func(context.Context) (updateHint, error) { return hint, nil }
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) {
		return verifiedRelease{Version: hint.Version, BinaryURL: "https://example.com/ClipCatch.exe"}, nil
	}
	runDiscovery(t, a)
	if a.latestRelease != known || a.state.AppUpdate.Status != "current" || a.updateRequiredLocked() {
		t.Fatal("unverified release enabled installer")
	}
}

func TestTrialAutomaticDiscoveryNeverUsesStableHint(t *testing.T) {
	oldChannel, oldVersion := appChannel, appVersion
	appChannel, appVersion = "test", "9.9.0-test.1"
	defer func() { appChannel, appVersion = oldChannel, oldVersion }()
	a := discoveryApp(t)
	rel, err := parseTrialRelease(trialReleaseFixture("9.9.0-test.2"))
	if err != nil {
		t.Fatal(err)
	}
	var probes, fetches int
	a.updateHooks.probe = func(context.Context) (updateHint, error) {
		probes++
		return updateHint{}, errors.New("stable forbidden")
	}
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) { fetches++; return rel, nil }
	start := a.updateHooks.now()
	runDiscovery(t, a)
	if probes != 0 || fetches != 1 || a.latestRelease != rel || a.state.AppUpdate.Status != "available" {
		t.Fatal("trial channel crossed", probes, fetches, a.state.AppUpdate)
	}
	if a.updateCheck.nextAutomatic.Sub(start) != 15*time.Minute {
		t.Fatal("trial API cadence changed")
	}
	if check, reason := a.beginAppUpdateCheck(true, false); check != nil || reason != "throttled" {
		t.Fatal("trial event flooded API")
	}
}
