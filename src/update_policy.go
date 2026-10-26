package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

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
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-a.workerStop:
			return
		case <-t.C:
			a.mu.Lock()
			busy := updateInProgress(a.state.AppUpdate.Status) || a.state.AppUpdate.Status == "checking" || a.stopping
			if !busy {
				a.state.AppUpdate.Status = "checking"
			}
			a.mu.Unlock()
			if !busy {
				a.checkAppUpdate()
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
