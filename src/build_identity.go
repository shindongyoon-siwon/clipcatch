package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const stableShareURL = "https://github.com/" + clipCatchReleaseRepo + "/releases/latest/download/ClipCatch.exe"

type buildIdentity struct {
	App      string `json:"app"`
	Channel  string `json:"channel"`
	Version  string `json:"version"`
	ShareURL string `json:"share_url"`
}

func currentBuildIdentity() buildIdentity {
	return buildIdentity{runtimeAppID(), appChannel, appVersion, stableShareURL}
}
func validateBuildIdentity(b buildIdentity) error {
	if b.Channel == "stable" && b.App == "clipcatch" {
		_, e := parseSemver(b.Version)
		return e
	}
	if b.Channel == "test" && b.App == "clipcatch-test2" {
		_, e := parseTrialVersion(b.Version)
		return e
	}
	return errors.New("ClipCatch build channel and version do not match")
}

// Called only after SHA256 verification of an official release artifact. Go
// -trimpath builds do not consistently retain ldflags, so read the explicit
// side-effect-free --build-info result instead of guessing from the file name.
func fileBuildIdentity(path string) (buildIdentity, error) {
	var identity buildIdentity
	dir, e := os.MkdirTemp("", "clipcatch-identity-")
	if e != nil {
		return identity, e
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "identity.json")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := hiddenCommandContext(ctx, path, "--build-info", output)
	if e = cmd.Run(); e != nil {
		return identity, e
	}
	data, e := os.ReadFile(output)
	if e != nil || len(data) > 4096 {
		return identity, errors.New("build identity missing")
	}
	if e = json.Unmarshal(data, &identity); e != nil {
		return identity, e
	}
	return identity, validateBuildIdentity(identity)
}
func verifyInstallerIdentity(path, version string) error {
	b, e := fileBuildIdentity(path)
	if e != nil {
		return e
	}
	if b.Channel != appChannel || b.App != runtimeAppID() || b.Version != version {
		return errors.New("installer identity mismatch; refusing a cross-channel update")
	}
	return nil
}
func buildLabel() string {
	if isTrialBuild() {
		return "TEST2 \ud14c\uc2a4\ud2b8\ud310"
	}
	return "\uc6b4\uc601\ud310"
}

// Also support a file output for Windows GUI builds which have no console.
func writeBuildInfo(path string) error {
	data, e := json.MarshalIndent(currentBuildIdentity(), "", "  ")
	if e != nil {
		return e
	}
	if path == "" {
		_, e = fmt.Fprintln(os.Stdout, string(data))
		return e
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}
