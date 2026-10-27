package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelease196StableIdentityAndUnchangedUpdatePolicy(t *testing.T) {
	if appChannel != "stable" || appDefaultVersion != "1.9.6" || runtimeAppID() != "clipcatch" || runtimePort() != "8765" || runtimeDataName() != "ClipCatch" {
		t.Fatal("mixed release identity")
	}
	b := currentBuildIdentity()
	if err := validateBuildIdentity(b); err != nil {
		t.Fatal(err)
	}
	if b.ShareURL != stableShareURL {
		t.Fatal("share URL changed")
	}
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if !strings.HasSuffix(installedExePath(), filepath.Join("Programs", "ClipCatch", "ClipCatch.exe")) {
		t.Fatal("wrong install path")
	}
	for _, name := range []string{"main.go", "updater.go", "update_policy.go", "trial_runtime.go", "build_identity.go", "browser_bridge.go", "web/lifecycle.js"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "isolatedDevelopment") || strings.Contains(string(data), "CLIPCATCH_DEV196_FIXTURE") {
			t.Fatal("development override in release", name)
		}
	}
	if _, err := os.Stat("development_enabled.go"); !os.IsNotExist(err) {
		t.Fatal("development build override included")
	}
}
