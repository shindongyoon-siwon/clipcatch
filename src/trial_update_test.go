package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func trialReleaseFixture(version string) githubRelease {
	base := "https://github.com/" + clipCatchReleaseRepo + "/releases/download/" + trialReleasePrefix + version + "/"
	body, _ := json.Marshal(map[string]any{"tag_name": trialReleasePrefix + version, "prerelease": true, "draft": false, "assets": []map[string]any{
		{"name": trialAssetName, "size": 55000000, "browser_download_url": base + trialAssetName},
		{"name": "SHA256SUMS", "size": 90, "browser_download_url": base + "SHA256SUMS"},
	}})
	var r githubRelease
	_ = json.Unmarshal(body, &r)
	return r
}
func TestTrialReleaseChannelAndVersions(t *testing.T) {
	r := trialReleaseFixture("1.9.1-test.5")
	if _, e := parseTrialRelease(r); e != nil {
		t.Fatal(e)
	}
	if _, e := parseGithubRelease(r); e == nil {
		t.Fatal("production accepted test release")
	}
	for _, edit := range []func(*githubRelease){
		func(r *githubRelease) { r.Prerelease = false }, func(r *githubRelease) { r.Draft = true },
		func(r *githubRelease) { r.TagName = "v1.9.1" }, func(r *githubRelease) { r.TagName = "test2-v1.9.1-test.bad" },
		func(r *githubRelease) { r.Assets[0].BrowserDownloadURL = "https://attacker.test/tool.exe" },
		func(r *githubRelease) { r.Assets[0].Name = "ClipCatch.exe" }, func(r *githubRelease) { r.Assets[0].Size = 200 },
		func(r *githubRelease) { r.Assets = r.Assets[:1] }, func(r *githubRelease) { r.Assets = append(r.Assets, r.Assets[0]) },
	} {
		r := trialReleaseFixture("1.9.1-test.5")
		edit(&r)
		if _, e := parseTrialRelease(r); e == nil {
			t.Fatal("unsafe release accepted", r)
		}
	}
	best, e := selectLatestTrial([]githubRelease{trialReleaseFixture("1.9.1-test.2"), trialReleaseFixture("1.9.1-test.10"), trialReleaseFixture("1.9.1-test.5"), {TagName: "v99.0.0"}})
	if e != nil || best.Version != "1.9.1-test.10" {
		t.Fatal("not numeric comparison", best, e)
	}
	if _, e := selectLatestTrial(nil); !errors.Is(e, errNoTrialRelease) {
		t.Fatal(e)
	}
	for _, v := range []string{"1.9.1", "1.9.1-test.-1", "../1.9.1-test.1", "99999999999999.0.0-test.4"} {
		if _, e := parseTrialVersion(v); e == nil {
			t.Fatal(v)
		}
	}
}
func TestTrialNamedChecksumAndTamper(t *testing.T) {
	digest := strings.Repeat("a", 64)
	if got, e := readNamedChecksum(digest+"  "+trialAssetName, trialAssetName); e != nil || got != digest {
		t.Fatal(e)
	}
	if _, e := readNamedChecksum(digest+"  ClipCatch.exe", trialAssetName); e == nil {
		t.Fatal("stable checksum accepted")
	}
	if _, e := readNamedChecksum(strings.Repeat(digest+"  "+trialAssetName+"\n", 2), trialAssetName); e == nil {
		t.Fatal("duplicate checksum accepted")
	}
	oldChannel, oldVersion := appChannel, appVersion
	appChannel = "test"
	appVersion = "1.9.1-test.5"
	defer func() { appChannel, appVersion = oldChannel, oldVersion }()
	p := filepath.Join(t.TempDir(), trialAssetName)
	b := []byte("MZ sample for hash validation only")
	os.WriteFile(p, b, 0600)
	sum := sha256.Sum256(b)
	rel, _ := parseTrialRelease(trialReleaseFixture(appVersion))
	if e := validateTrialUpdate(rel); e != nil {
		t.Fatal(e)
	}
	if e := verifyManualInstaller(p, rel, hex.EncodeToString(sum[:])); e != nil {
		t.Fatal(e)
	}
	if e := verifyManualInstaller(p, rel, digest); e == nil {
		t.Fatal("changed executable accepted")
	}
	rel.Version = "1.9.1-test.6"
	if e := verifyManualInstaller(p, rel, hex.EncodeToString(sum[:])); e == nil {
		t.Fatal("wrong version accepted")
	}
	if e := validateTrialUpdate(verifiedRelease{Version: "1.9.0"}); e == nil {
		t.Fatal("stable release accepted by TEST2")
	}
}
func TestUpdateLockPreventsNewDownloadsAndBrowserJobs(t *testing.T) {
	a := newApp(t.TempDir(), t.TempDir())
	a.state.Phase = "ready"
	token := browserTestPair(t, a)
	a.state.AppUpdate.Status = "installing"
	w := browserRequest(a, "/bridge/page", testExtensionOrigin, token, `{"page_url":"https://www.youtube.com/watch?v=abcdefghijk","output_mode":"premiere"}`)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = browserRequest(a, "/bridge/download", testExtensionOrigin, token, `{"page_url":"https://www.reddit.com/comments/abc123/","media_url":"https://v.redd.it/abcde12345/DASHPlaylist.mpd"}`)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	a.downloadHandler(w, httptest.NewRequest("POST", "/api/download", strings.NewReader(`{"urls":"https://www.youtube.com/watch?v=abcdefghijk"}`)))
	if w.Code != 409 || len(a.state.Jobs) != 0 {
		t.Fatal("new job during update", w.Code)
	}
}

func TestTrialPortableStopCannotTouchProduction(t *testing.T) {
	old := appChannel
	appChannel = "test"
	defer func() { appChannel = old }()
	stopped := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			w.Write([]byte(`{"app":"clipcatch","version":"1.9.0"}`))
			return
		}
		if r.URL.Path == "/api/stop" {
			stopped = true
		}
	}))
	defer server.Close()
	if e := stopExistingInstanceForUpgradeAt(server.URL); e == nil {
		t.Fatal("accepted production server")
	}
	if stopped {
		t.Fatal("stopped production app")
	}
}

func TestTrialReceiptPersistsAndRejectsStable(t *testing.T) {
	old := appChannel
	appChannel = "test"
	defer func() { appChannel = old }()
	dir := t.TempDir()
	if e := writeUpdateReceipt(dir, "verified", "1.9.1-test.5", "checked"); e != nil {
		t.Fatal(e)
	}
	if r := readUpdateReceipt(dir); r == nil || r.Version != "1.9.1-test.5" {
		t.Fatal("lost trial receipt")
	}
	if e := writeUpdateReceipt(dir, "verified", "1.9.0", "wrong channel"); e == nil {
		t.Fatal("stable receipt accepted")
	}
	appChannel = "stable"
	if readUpdateReceipt(dir) != nil {
		t.Fatal("test receipt accepted by stable")
	}
}
