package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestActivityCooldownSurvivesRestartAndManualCheckStillWorks(t *testing.T) {
	a := discoveryApp(t)
	now := time.Now()
	a.updateHooks.now = func() time.Time { return now }
	rel := discoveryRelease(t, appVersion)
	calls := 0
	a.updateHooks.fetch = func(context.Context) (verifiedRelease, error) { calls++; return rel, nil }
	a.requireStartupCheck = true
	a.watchAppUpdates()
	if calls != 1 {
		t.Fatal(calls)
	}
	b := newApp(a.dataDir, t.TempDir())
	b.updateHooks = a.updateHooks
	b.watchAppUpdates()
	if calls != 1 || b.requireStartupCheck || b.state.AppUpdate.Status != "current" {
		t.Fatal("restart ignored freshness", calls, b.state.AppUpdate)
	}
	for _, elapsed := range []time.Duration{time.Minute, 59 * time.Minute} {
		now = time.Now().Add(elapsed)
		if c, _ := b.beginAppUpdateCheck(true, false); c != nil {
			t.Fatal("too early", elapsed)
		}
	}
	c, reason := b.beginAppUpdateCheck(false, false)
	if c == nil {
		t.Fatal("manual check lost", reason)
	}
	b.runAppUpdateCheck(c)
	if calls != 2 {
		t.Fatal("manual check missing", calls)
	}
}
func TestActivityCacheDoesNotTrustOtherBuildOrInstaller(t *testing.T) {
	for _, change := range []string{"version", "url", "future", "malformed"} {
		t.Run(change, func(t *testing.T) {
			a := discoveryApp(t)
			now := time.Now()
			c := activityUpdateCache{Current: appVersion, Checked: now, Next: now.Add(time.Hour), Release: discoveryRelease(t, appVersion), Info: UpdateInfo{Status: "current"}}
			switch change {
			case "version":
				c.Current = "0.0.1"
			case "url":
				c.Release.BinaryURL = "https://example.com/ClipCatch.exe"
			case "future":
				c.Checked = now.Add(24 * time.Hour)
			}
			data, _ := json.Marshal(c)
			if change == "malformed" {
				data = []byte(strings.Repeat("x", 65000))
			}
			os.WriteFile(filepath.Join(a.dataDir, "activity-update.json"), data, 0600)
			b := newApp(a.dataDir, t.TempDir())
			if !b.updateCheck.nextAutomatic.IsZero() || b.state.AppUpdate.Status != "checking" {
				t.Fatal("unsafe cache used", change)
			}
		})
	}
}
