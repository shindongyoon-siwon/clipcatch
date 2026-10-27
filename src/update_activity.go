package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Stored in the existing per-channel app data directory, never in the source
// tree. This avoids repeating checks when the user closes/reopens the program.
type activityUpdateCache struct {
	Current    string          `json:"current"`
	Checked    time.Time       `json:"checked"`
	Next       time.Time       `json:"next"`
	ServerNext time.Time       `json:"server_next"`
	Release    verifiedRelease `json:"release"`
	Hint       string          `json:"hint"`
	Info       UpdateInfo      `json:"info"`
}

func (a *App) nextActivityCheckLocked() time.Time {
	next := a.updateCheck.nextAutomatic
	if a.updateCheck.nextServerCheck.After(next) {
		next = a.updateCheck.nextServerCheck
	}
	return next
}
func (a *App) requestActivityUpdateCheck() {
	if check, _ := a.beginAppUpdateCheck(true, false); check != nil {
		go a.runAppUpdateCheck(check)
	}
}
func (a *App) loadActivityUpdateCache() {
	if isTrialBuild() {
		return
	}
	data, err := os.ReadFile(filepath.Join(a.dataDir, "activity-update.json"))
	var c activityUpdateCache
	now := time.Now()
	if err != nil || len(data) > 64000 || json.Unmarshal(data, &c) != nil || c.Current != appVersion || c.Checked.After(now.Add(time.Minute)) || now.Sub(c.Checked) > 24*time.Hour || c.Next.After(c.Checked.Add(automaticUpdateCooldown+time.Minute)) {
		return
	}
	if c.Release.Version != "" && !releaseCacheValid(c.Release) {
		return
	}
	switch c.Info.Status {
	case "current", "available", "error", "unavailable":
	default:
		return
	}
	// Do not retract a newer verified release loaded from the existing cache.
	if c.Release.Version != "" && (a.latestRelease.Version == "" || compareRuntimeVersions(c.Release.Version, a.latestRelease.Version) >= 0) {
		c.Release.Notes = safeReleaseNotes(c.Release.Notes)
		c.Release.ReleaseURL = "https://github.com/" + clipCatchReleaseRepo + "/releases/tag/v" + c.Release.Version
		a.latestRelease = c.Release
	}
	a.updateCheck.nextAutomatic, a.updateCheck.nextServerCheck = c.Next, c.ServerNext
	a.updateCheck.lastHint = c.Hint
	if now.Before(a.nextActivityCheckLocked()) {
		a.state.AppUpdate = c.Info
		a.state.AppUpdate.Notes, a.state.AppUpdate.ReleaseURL = a.latestRelease.Notes, a.latestRelease.ReleaseURL
		if a.updateRequiredLocked() && c.Info.Status != "error" {
			a.state.AppUpdate.Status = "available"
		}
		a.state.AppUpdate.Latest = a.latestRelease.Version
	}
}
func (a *App) saveActivityUpdateCacheLocked() {
	if isTrialBuild() || a.stopping || updateInProgress(a.state.AppUpdate.Status) {
		return
	}
	now := a.updateCheckHooksLocked().now()
	c := activityUpdateCache{appVersion, now, a.updateCheck.nextAutomatic, a.updateCheck.nextServerCheck, a.latestRelease, a.updateCheck.lastHint, a.state.AppUpdate}
	data, err := json.Marshal(c)
	if err != nil || os.MkdirAll(a.dataDir, 0700) != nil {
		return
	}
	f, err := os.CreateTemp(a.dataDir, "activity-update-*.tmp")
	if err != nil {
		return
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return
	}
	if f.Close() != nil {
		return
	}
	_ = os.Rename(name, filepath.Join(a.dataDir, "activity-update.json"))
}
