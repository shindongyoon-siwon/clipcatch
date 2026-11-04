package main

import (
	"os"
	"path/filepath"
)

// Set only when building the standalone trial executable. Stable defaults stay unchanged.
var appChannel = "stable"

func isSectionBuild() bool { return appChannel == "section-test" }
func isTrialBuild() bool   { return appChannel == "test" || isSectionBuild() }
func runtimeAppID() string {
	if isSectionBuild() {
		return "clipcatch-section-test"
	}
	if isTrialBuild() {
		return "clipcatch-test2"
	}
	return "clipcatch"
}
func runtimeDataName() string {
	if isSectionBuild() {
		return "ClipCatch-SectionTest"
	}
	if isTrialBuild() {
		return "ClipCatch-Test2"
	}
	return appName
}
func runtimePort() string {
	if isSectionBuild() {
		return "18767"
	}
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
	if isSectionBuild() {
		name, key = "ClipCatch-SectionTest", "CLIPCATCH_SECTION_SAVEDIR"
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
