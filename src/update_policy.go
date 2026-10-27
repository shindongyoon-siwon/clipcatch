package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	stableUpdateHintURL = "https://github.com/" + clipCatchReleaseRepo + "/releases/latest/download/build-info.json"
	// Stable builds only check on user activity; this is a cooldown, not a timer.
	automaticUpdateCooldown = time.Hour
	trialUpdateInterval     = 15 * time.Minute
)

// The public build identity is only a change hint. It never supplies an
// installer URL or replaces the existing release/checksum/identity checks.
type updateHint struct {
	Version     string
	Fingerprint string
}

type updateCheckHooks struct {
	now       func() time.Time
	probe     func(context.Context) (updateHint, error)
	fetch     func(context.Context) (verifiedRelease, error)
	newTicker func(time.Duration) (<-chan time.Time, func())
}

type updateCheckState struct {
	running         bool
	generation      uint64
	cancel          context.CancelFunc
	nextAutomatic   time.Time
	nextServerCheck time.Time
	failures        int
	lastHint        string
}

type appUpdateCheck struct {
	ctx        context.Context
	cancel     context.CancelFunc
	generation uint64
	automatic  bool
	startup    bool
	trial      bool
	started    time.Time
	known      verifiedRelease
	lastHint   string
	hooks      updateCheckHooks
}

type updateRetryError struct {
	err     error
	retryAt time.Time
}

func (e *updateRetryError) Error() string { return e.err.Error() }
func (e *updateRetryError) Unwrap() error { return e.err }

func updateHTTPError(resp *http.Response, label string, now time.Time) error {
	err := fmt.Errorf("%s HTTP %d", label, resp.StatusCode)
	var retryAt time.Time
	if seconds, parseErr := strconv.ParseUint(resp.Header.Get("Retry-After"), 10, 32); parseErr == nil {
		retryAt = now.Add(time.Duration(seconds) * time.Second)
	} else if date, parseErr := http.ParseTime(resp.Header.Get("Retry-After")); parseErr == nil {
		retryAt = date
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		if seconds, parseErr := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); parseErr == nil {
			if reset := time.Unix(seconds, 0); reset.After(retryAt) {
				retryAt = reset
			}
		}
	}
	if retryAt.After(now) {
		return &updateRetryError{err: err, retryAt: retryAt}
	}
	return err
}

func parseStableUpdateHint(data []byte) (updateHint, error) {
	var identity buildIdentity
	if len(data) > 4096 || json.Unmarshal(data, &identity) != nil {
		return updateHint{}, errors.New("업데이트 안내 형식이 올바르지 않습니다")
	}
	if identity.App != "clipcatch" || identity.Channel != "stable" || validateBuildIdentity(identity) != nil || identity.ShareURL != stableShareURL {
		return updateHint{}, errors.New("정식 ClipCatch 업데이트 안내가 아닙니다")
	}
	hash := sha256.Sum256(data)
	return updateHint{Version: identity.Version, Fingerprint: hex.EncodeToString(hash[:])}, nil
}

func fetchStableUpdateHint(ctx context.Context) (updateHint, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stableUpdateHintURL, nil)
	if err != nil {
		return updateHint{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ClipCatch/"+appVersion)
	client := http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return updateHint{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return updateHint{}, updateHTTPError(resp, "업데이트 안내", time.Now())
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil {
		return updateHint{}, err
	}
	return parseStableUpdateHint(data)
}

func (a *App) updateCheckHooksLocked() updateCheckHooks {
	hooks := a.updateHooks
	if hooks.now == nil {
		hooks.now = time.Now
	}
	if hooks.probe == nil {
		hooks.probe = fetchStableUpdateHint
	}
	if hooks.fetch == nil {
		hooks.fetch = fetchLatestReleaseContext
	}
	return hooks
}

// All user activity events and tabs share one claim. The first startup check owns the
// initial checking state; subsequent automatic checks leave the UI untouched.
func (a *App) beginAppUpdateCheck(automatic, initial bool) (*appUpdateCheck, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopping || updateInProgress(a.state.AppUpdate.Status) {
		return nil, "busy"
	}
	if a.updateCheck.running || (!initial && a.state.AppUpdate.Status == "checking") {
		return nil, "running"
	}
	hooks := a.updateCheckHooksLocked()
	now := hooks.now()
	if now.Before(a.updateCheck.nextServerCheck) {
		return nil, "server_cooldown"
	}
	if automatic && now.Before(a.updateCheck.nextAutomatic) {
		if initial {
			a.requireStartupCheck = false
		}
		return nil, "throttled"
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.updateCheck.generation++
	a.updateCheck.running = true
	a.updateCheck.cancel = cancel
	check := &appUpdateCheck{ctx: ctx, cancel: cancel, generation: a.updateCheck.generation, automatic: automatic, startup: initial || a.requireStartupCheck, trial: isTrialBuild(), started: now, known: a.latestRelease, lastHint: a.updateCheck.lastHint, hooks: hooks}
	if !automatic || check.startup {
		a.state.AppUpdate = UpdateInfo{Status: "checking", Latest: a.latestRelease.Version, Notes: a.latestRelease.Notes, ReleaseURL: a.latestRelease.ReleaseURL, Detail: "프로그램 새 버전을 확인하는 중"}
	}
	return check, ""
}

func (a *App) runAppUpdateCheck(check *appUpdateCheck) {
	defer check.cancel()
	var hint updateHint
	var err error
	full := true
	if check.automatic && !check.startup && !check.trial {
		hint, err = check.hooks.probe(check.ctx)
		if err != nil {
			a.finishAppUpdateCheck(check, hint, verifiedRelease{}, false, err)
			return
		}
		full = !releaseCacheValid(check.known) || compareSemver(hint.Version, check.known.Version) != 0 || (check.lastHint != "" && hint.Fingerprint != check.lastHint)
	}
	var rel verifiedRelease
	if full {
		rel, err = check.hooks.fetch(check.ctx)
		if err == nil && !releaseCacheValid(rel) {
			err = errors.New("공식 업데이트 검증 정보가 없습니다")
		}
	}
	a.finishAppUpdateCheck(check, hint, rel, full, err)
}

func updateCheckDelay(trial bool, failures int, full bool) time.Duration {
	if trial {
		return trialUpdateInterval
	}
	// Failed checks also wait for the next user action after one hour. Explicit
	// manual checks still work, except when GitHub supplied a Retry-After limit.
	return automaticUpdateCooldown
}

func (a *App) finishAppUpdateCheck(check *appUpdateCheck, hint updateHint, rel verifiedRelease, full bool, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.updateCheck.generation != check.generation {
		return
	}
	a.updateCheck.running, a.updateCheck.cancel = false, nil
	defer a.saveActivityUpdateCacheLocked()
	if a.stopping || updateInProgress(a.state.AppUpdate.Status) {
		return
	}
	if check.startup {
		a.requireStartupCheck = false
	}
	// A CDN hint can arrive before the API's release response catches up.
	// Keep retrying at a bounded rate rather than marking that hint complete.
	lagging := err == nil && full && hint.Version != "" && compareSemver(hint.Version, rel.Version) != 0
	if err != nil || lagging {
		a.updateCheck.failures++
	} else {
		a.updateCheck.failures = 0
	}
	base := check.started
	if a.updateCheck.failures > 0 {
		base = check.hooks.now()
	}
	a.updateCheck.nextAutomatic = base.Add(updateCheckDelay(check.trial, a.updateCheck.failures, full))
	if err != nil {
		var retryErr *updateRetryError
		if errors.As(err, &retryErr) && retryErr.retryAt.After(a.updateCheck.nextServerCheck) {
			a.updateCheck.nextServerCheck = retryErr.retryAt
		}
		if !check.automatic || check.startup {
			status, detail := "error", "업데이트 확인 실패: "+err.Error()
			if !a.updateRequiredLocked() {
				if errors.Is(err, errNoTrialRelease) {
					status, detail = "unavailable", errNoTrialRelease.Error()
				} else if strings.Contains(err.Error(), "아직 게시되지") {
					status, detail = "unavailable", "GitHub 배포 준비 중"
				}
			}
			a.state.AppUpdate = UpdateInfo{Status: status, Detail: detail, Latest: a.latestRelease.Version, Notes: a.latestRelease.Notes, ReleaseURL: a.latestRelease.ReleaseURL}
		}
		return
	}
	if hint.Fingerprint != "" {
		a.updateCheck.lastHint = hint.Fingerprint
	}
	if !full {
		return
	}
	// A stale automatic API response must not retract a verified update or
	// erase the reason an attempted installation failed.
	confirmedWithdrawal := hint.Version != "" && compareSemver(hint.Version, rel.Version) == 0
	if check.automatic && !check.startup && !check.trial && a.updateRequiredLocked() && compareRuntimeVersions(rel.Version, a.latestRelease.Version) < 0 && !confirmedWithdrawal {
		return
	}
	preserve := check.automatic && rel.Version == a.latestRelease.Version && (a.state.AppUpdate.Status == "available" || a.state.AppUpdate.Status == "error")
	a.latestRelease = rel
	a.saveRequiredUpdate(rel)
	if preserve {
		a.state.AppUpdate.Latest, a.state.AppUpdate.Notes, a.state.AppUpdate.ReleaseURL = rel.Version, rel.Notes, rel.ReleaseURL
		return
	}
	info := UpdateInfo{Status: "current", Latest: rel.Version, Notes: rel.Notes, ReleaseURL: rel.ReleaseURL, Detail: "최신 버전입니다"}
	if compareRuntimeVersions(rel.Version, appVersion) > 0 {
		info.Status, info.Detail = "available", "새 버전 "+rel.Version+"을 설치할 수 있어요"
	}
	a.state.AppUpdate = info
}

// The caller has already checked every installation prerequisite while holding
// a.mu. A late discovery response can no longer overwrite installation state.
func (a *App) claimAppUpdateInstallLocked(rel verifiedRelease) {
	if a.updateCheck.cancel != nil {
		a.updateCheck.cancel()
	}
	a.updateCheck.generation++
	a.updateCheck.running, a.updateCheck.cancel = false, nil
	a.state.AppUpdate = UpdateInfo{Status: "downloading", Latest: rel.Version, Notes: rel.Notes, ReleaseURL: rel.ReleaseURL, Detail: "업데이트 다운로드 중"}
}

func safeReleaseNotes(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	r := []rune(strings.TrimSpace(s))
	if len(r) > 12000 {
		r = r[:12000]
	}
	return string(r)
}
func releaseCacheValid(r verifiedRelease) bool {
	var tag, asset string
	if isTrialBuild() {
		if validateTrialUpdate(r) != nil {
			return false
		}
		tag = trialReleasePrefix + r.Version
		asset = trialAssetName
	} else {
		if _, e := parseSemver(r.Version); e != nil {
			return false
		}
		tag = "v" + r.Version
		asset = "ClipCatch.exe"
	}
	base := "https://github.com/" + clipCatchReleaseRepo + "/releases/download/" + tag + "/"
	return r.BinaryURL == base+asset && (r.SumsURL == base+"SHA256SUMS" || r.SumsURL == "") && ((len(r.Digest) == 64 && shaRE.MatchString(r.Digest)) || r.SumsURL != "")
}
func (a *App) loadRequiredUpdate() {
	p := filepath.Join(a.dataDir, "required-update.json")
	b, e := os.ReadFile(p)
	if e != nil || len(b) > 64000 {
		return
	}
	var r verifiedRelease
	if json.Unmarshal(b, &r) != nil || !releaseCacheValid(r) {
		return
	}
	if compareRuntimeVersions(r.Version, appVersion) <= 0 {
		_ = os.Remove(p)
		return
	}
	r.Notes = safeReleaseNotes(r.Notes)
	tag := "v" + r.Version
	if isTrialBuild() {
		tag = trialReleasePrefix + r.Version
	}
	r.ReleaseURL = "https://github.com/" + clipCatchReleaseRepo + "/releases/tag/" + tag
	a.latestRelease = r
}
func (a *App) saveRequiredUpdate(r verifiedRelease) {
	p := filepath.Join(a.dataDir, "required-update.json")
	if compareRuntimeVersions(r.Version, appVersion) <= 0 {
		_ = os.Remove(p)
		return
	}
	if !releaseCacheValid(r) {
		return
	}
	b, e := json.Marshal(r)
	if e != nil {
		return
	}
	if os.MkdirAll(a.dataDir, 0700) != nil {
		return
	}
	f, e := os.CreateTemp(a.dataDir, "update-note-*.json")
	if e != nil {
		return
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return
	}
	if f.Close() != nil {
		return
	}
	_ = os.Rename(name, p)
}
func (a *App) watchAppUpdates() {
	a.checkAppUpdate()
	// No stable background ticker, sleeping update goroutine or idle requests.
	if !isTrialBuild() {
		return
	}
	interval := trialUpdateInterval
	a.mu.Lock()
	newTicker := a.updateHooks.newTicker
	a.mu.Unlock()
	if newTicker == nil {
		newTicker = func(interval time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(interval)
			return t.C, t.Stop
		}
	}
	ticks, stop := newTicker(interval)
	defer stop()
	for {
		select {
		case <-a.workerStop:
			return
		case <-ticks:
			if check, _ := a.beginAppUpdateCheck(true, false); check != nil {
				a.runAppUpdateCheck(check)
			}
		}
	}
}

// All paths are fixed to the same release channel; never promote TEST2 implicitly.
func downloadCurrentInstaller(rel verifiedRelease, digest string) (string, error) {
	if !releaseCacheValid(rel) {
		return "", errors.New("untrusted installer URL")
	}
	dir := filepath.Join(clipCatchDataDir(), "updates")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	f, e := os.CreateTemp(dir, "manual-*-"+runtimeUpdateAssetName())
	if e != nil {
		return "", e
	}
	name := f.Name()
	f.Close()
	_ = os.Remove(name)
	if e = downloadVerified(rel.BinaryURL, name, digest, func(int64, int64) {}); e != nil {
		os.Remove(name)
		return "", e
	}
	if e = verifyInstallerIdentity(name, rel.Version); e != nil {
		os.Remove(name)
		return "", e
	}
	return name, nil
}
