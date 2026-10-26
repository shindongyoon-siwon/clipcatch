package main

import (
	"os"
	"path/filepath"
)

// Set only when building the standalone trial executable. Stable defaults stay unchanged.
var appChannel = "stable"

func isTrialBuild() bool { return appChannel == "test" }
func runtimeAppID() string {
	if isTrialBuild() {
		return "clipcatch-test2"
	}
	return "clipcatch"
}
func runtimeDataName() string {
	if isTrialBuild() {
		return "ClipCatch-Test2"
	}
	return appName
}
func runtimePort() string {
	if isTrialBuild() {
		return "18766"
	}
	return "8765"
}
func runtimeSaveDir(home string) string {
	name, key := "ClipCatch", "CLIPCATCH_SAVEDIR"
	if isTrialBuild() {
		name, key = "ClipCatch-Test2", "CLIPCATCH_TEST2_SAVEDIR"
	}
	if override := os.Getenv(key); override != "" {
		return override
	}
	return filepath.Join(home, "Downloads", name)
}
func selectedOutputMode(requested, fallback string) (string, bool) {
	if requested == "" {
		requested = fallback
	}
	if requested != originalMode && requested != premiereMode {
		return "", false
	}
	return requested, true
}
