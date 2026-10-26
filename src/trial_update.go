package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TEST2 only. Never use /releases/latest or select a production asset.
const trialReleasePrefix = "test2-v"
const trialAssetName = "ClipCatch-Test2.exe"
const trialReleaseAPI = "https://api.github.com/repos/" + clipCatchReleaseRepo + "/releases?per_page=50"

var trialVersionRE = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)-test\.([0-9]+)$`)
var errNoTrialRelease = errors.New("\uc0c8 \ud14c\uc2a4\ud2b8 \uc5c5\ub370\uc774\ud2b8\uac00 \uc544\uc9c1 \uacf5\uac1c\ub418\uc9c0 \uc54a\uc558\uc2b5\ub2c8\ub2e4.")

func parseTrialVersion(v string) ([4]uint64, error) {
	var result [4]uint64
	m := trialVersionRE.FindStringSubmatch(v)
	if m == nil {
		return result, errors.New("invalid TEST2 version")
	}
	for i := 0; i < 4; i++ {
		n, e := strconv.ParseUint(m[i+1], 10, 32)
		if e != nil {
			return result, e
		}
		result[i] = n
	}
	return result, nil
}
func compareTrialVersions(a, b string) (int, error) {
	aa, e := parseTrialVersion(a)
	if e != nil {
		return 0, e
	}
	bb, e := parseTrialVersion(b)
	if e != nil {
		return 0, e
	}
	for i := range aa {
		if aa[i] < bb[i] {
			return -1, nil
		}
		if aa[i] > bb[i] {
			return 1, nil
		}
	}
	return 0, nil
}
func compareRuntimeVersions(a, b string) int {
	if !isTrialBuild() {
		return compareSemver(a, b)
	}
	c, e := compareTrialVersions(a, b)
	if e != nil {
		return -1
	}
	return c
}
func runtimeUpdateAssetName() string {
	if isTrialBuild() {
		return trialAssetName
	}
	return "ClipCatch.exe"
}
func parseTrialRelease(in githubRelease) (verifiedRelease, error) {
	var out verifiedRelease
	if in.Draft || !in.Prerelease || !strings.HasPrefix(in.TagName, trialReleasePrefix) {
		return out, errors.New("not a published TEST2 prerelease")
	}
	version := strings.TrimPrefix(in.TagName, trialReleasePrefix)
	if _, e := parseTrialVersion(version); e != nil {
		return out, e
	}
	base := "https://github.com/" + clipCatchReleaseRepo + "/releases/download/" + in.TagName + "/"
	seen := map[string]bool{}
	out.Version = version
	for _, a := range in.Assets {
		if a.Name != trialAssetName && a.Name != "SHA256SUMS" {
			continue
		}
		if seen[a.Name] {
			return verifiedRelease{}, errors.New("duplicate update asset")
		}
		seen[a.Name] = true
		if a.BrowserDownloadURL != base+a.Name {
			return verifiedRelease{}, errors.New("untrusted test asset URL")
		}
		if a.Name == trialAssetName {
			if a.Size < 1024*1024 || a.Size > 100*1024*1024 {
				return verifiedRelease{}, errors.New("invalid test asset size")
			}
			out.BinaryURL = a.BrowserDownloadURL
			if a.Digest != "" {
				if !strings.HasPrefix(a.Digest, "sha256:") || !shaRE.MatchString(strings.TrimPrefix(a.Digest, "sha256:")) {
					return verifiedRelease{}, errors.New("invalid test digest")
				}
				out.Digest = strings.ToLower(strings.TrimPrefix(a.Digest, "sha256:"))
			}
		} else {
			out.SumsURL = a.BrowserDownloadURL
		}
	}
	if out.BinaryURL == "" || out.SumsURL == "" {
		return verifiedRelease{}, errors.New("test executable/checksum missing")
	}
	return out, nil
}
func validateTrialUpdate(rel verifiedRelease) error {
	if _, e := parseTrialVersion(rel.Version); e != nil {
		return e
	}
	base := "https://github.com/" + clipCatchReleaseRepo + "/releases/download/" + trialReleasePrefix + rel.Version + "/"
	if rel.BinaryURL != base+trialAssetName || rel.SumsURL != base+"SHA256SUMS" {
		return errors.New("TEST2 \uc804\uc6a9 \uc5c5\ub370\uc774\ud2b8\ub9cc \uc124\uce58\ud560 \uc218 \uc788\uc2b5\ub2c8\ub2e4.")
	}
	return nil
}
func selectLatestTrial(releases []githubRelease) (verifiedRelease, error) {
	var best verifiedRelease
	for _, r := range releases {
		rel, e := parseTrialRelease(r)
		if e != nil {
			continue
		}
		if best.Version == "" {
			best = rel
			continue
		}
		if c, e := compareTrialVersions(rel.Version, best.Version); e == nil && c > 0 {
			best = rel
		}
	}
	if best.Version == "" {
		return best, errNoTrialRelease
	}
	return best, nil
}
func fetchTrialReleases() (verifiedRelease, error) {
	var out verifiedRelease
	req, e := http.NewRequest(http.MethodGet, trialReleaseAPI, nil)
	if e != nil {
		return out, e
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ClipCatch-Test2/"+appVersion)
	client := http.Client{Timeout: 15 * time.Second}
	resp, e := client.Do(req)
	if e != nil {
		return out, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("GitHub HTTP %d", resp.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(resp.Body, (2<<20)+1))
	if e != nil {
		return out, e
	}
	if len(data) > 2<<20 {
		return out, errors.New("update response too large")
	}
	var releases []githubRelease
	if e = json.Unmarshal(data, &releases); e != nil {
		return out, e
	}
	return selectLatestTrial(releases)
}
func readNamedChecksum(txt, name string) (string, error) {
	result := ""
	for _, line := range strings.Split(txt, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if result != "" || !shaRE.MatchString(fields[0]) {
			return "", errors.New("invalid or duplicate checksum")
		}
		result = strings.ToLower(fields[0])
	}
	if result == "" {
		return "", errors.New("TEST2 \uac80\uc99d\uac12\uc744 \ud655\uc778\ud560 \uc218 \uc5c6\uc2b5\ub2c8\ub2e4.")
	}
	return result, nil
}
func updateInProgress(s string) bool { return s == "downloading" || s == "installing" }
