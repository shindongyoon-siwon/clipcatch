package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// clipcatch is the public distribution repository. No access token is needed to check releases.
const (
	clipCatchReleaseRepo = "shindongyoon-siwon/clipcatch"
	clipCatchReleaseAPI  = "https://api.github.com/repos/" + clipCatchReleaseRepo + "/releases/latest"
)

var releaseTagRE = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

type UpdateInfo struct {
	Status  string  `json:"status"` // checking, unavailable, current, available, downloading, installing, error
	Latest  string  `json:"latest,omitempty"`
	Detail  string  `json:"detail,omitempty"`
	Percent float64 `json:"percent"`
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
		Digest             string `json:"digest"`
	} `json:"assets"`
}

type verifiedRelease struct {
	Version   string
	BinaryURL string
	SumsURL   string
	Digest    string
}

func parseSemver(v string) ([3]uint64, error) {
	var out [3]uint64
	m := releaseTagRE.FindStringSubmatch("v" + strings.TrimPrefix(v, "v"))
	if m == nil {
		return out, errors.New("잘못된 버전 형식")
	}
	for i := 0; i < 3; i++ {
		n, e := strconv.ParseUint(m[i+1], 10, 32)
		if e != nil {
			return out, e
		}
		out[i] = n
	}
	return out, nil
}
func compareSemver(a, b string) int {
	aa, e := parseSemver(a)
	if e != nil {
		return 0
	}
	bb, e := parseSemver(b)
	if e != nil {
		return 0
	}
	for i := 0; i < 3; i++ {
		if aa[i] < bb[i] {
			return -1
		}
		if aa[i] > bb[i] {
			return 1
		}
	}
	return 0
}

func parseGithubRelease(in githubRelease) (verifiedRelease, error) {
	var out verifiedRelease
	if in.Draft || in.Prerelease {
		return out, errors.New("정식 릴리스가 아닙니다")
	}
	if !releaseTagRE.MatchString(in.TagName) {
		return out, errors.New("릴리스 태그가 유효하지 않습니다")
	}
	out.Version = strings.TrimPrefix(in.TagName, "v")
	expectedBase := "https://github.com/" + clipCatchReleaseRepo + "/releases/download/" + in.TagName + "/"
	for _, a := range in.Assets {
		switch a.Name {
		case "ClipCatch.exe":
			if a.BrowserDownloadURL != expectedBase+"ClipCatch.exe" {
				return out, errors.New("실행파일 주소가 예상한 배포 저장소가 아닙니다")
			}
			if a.Size < 1*1024*1024 || a.Size > 100*1024*1024 {
				return out, errors.New("실행파일 용량이 비정상적입니다")
			}
			out.BinaryURL = a.BrowserDownloadURL
			if strings.HasPrefix(a.Digest, "sha256:") && len(a.Digest) == 71 && shaRE.MatchString(strings.TrimPrefix(a.Digest, "sha256:")) {
				out.Digest = strings.ToLower(strings.TrimPrefix(a.Digest, "sha256:"))
			}
		case "SHA256SUMS":
			if a.BrowserDownloadURL != expectedBase+"SHA256SUMS" {
				return out, errors.New("검증 파일 주소가 예상한 배포 저장소가 아닙니다")
			}
			out.SumsURL = a.BrowserDownloadURL
		}
	}
	if out.BinaryURL == "" || (out.Digest == "" && out.SumsURL == "") {
		return out, errors.New("ClipCatch.exe 또는 SHA256 검증 정보가 없습니다")
	}
	return out, nil
}

func fetchLatestRelease() (verifiedRelease, error) {
	if isTrialBuild() {
		return fetchTrialReleases()
	}
	var out verifiedRelease
	req, err := http.NewRequest(http.MethodGet, clipCatchReleaseAPI, nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ClipCatch/"+appVersion)
	client := http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return out, errors.New("릴리스가 아직 게시되지 않았습니다")
	}
	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("GitHub 응답 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return out, err
	}
	var rel githubRelease
	if err := json.Unmarshal(data, &rel); err != nil {
		return out, err
	}
	return parseGithubRelease(rel)
}

func (a *App) setUpdate(info UpdateInfo) {
	a.mu.Lock()
	a.state.AppUpdate = info
	a.mu.Unlock()
}
func (a *App) checkAppUpdate() {
	a.setUpdate(UpdateInfo{Status: "checking", Detail: "새로운 버전 확인 중..."})
	rel, err := fetchLatestRelease()
	if err != nil {
		msg := err.Error()
		if errors.Is(err, errNoTrialRelease) {
			a.setUpdate(UpdateInfo{Status: "unavailable", Detail: errNoTrialRelease.Error()})
		} else if strings.Contains(msg, "아직 게시되지") {
			a.setUpdate(UpdateInfo{Status: "unavailable", Detail: "GitHub 배포 준비 중"})
		} else {
			a.setUpdate(UpdateInfo{Status: "error", Detail: "업데이트 확인 실패: " + msg})
		}
		return
	}
	a.mu.Lock()
	a.latestRelease = rel
	a.mu.Unlock()
	if compareRuntimeVersions(rel.Version, appVersion) <= 0 {
		a.setUpdate(UpdateInfo{Status: "current", Latest: rel.Version, Detail: "최신 버전을 사용하고 있어요."})
	} else {
		a.setUpdate(UpdateInfo{Status: "available", Latest: rel.Version, Detail: "새 버전이 있어요. 파일을 다시 받을 필요 없이 설치할 수 있어요."})
	}
}

func (a *App) checkAppUpdateHandler(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	switch a.state.AppUpdate.Status {
	case "downloading", "installing":
		a.mu.Unlock()
		writeError(w, http.StatusConflict, "업데이트 설치가 끝난 후 다시 확인해 주세요.")
		return
	case "checking":
		a.mu.Unlock()
		// The automatic startup check may already be running. Treat a click as
		// a request to SHOW the in-progress status, not an error or another fetch.
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true, "already_running": true})
		return
	}
	// Claim the operation before starting the goroutine to prevent duplicate checks.
	a.state.AppUpdate = UpdateInfo{Status: "checking", Detail: "새로운 버전 확인 중..."}
	a.mu.Unlock()
	go a.checkAppUpdate()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) installAppUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "windows" {
		writeError(w, 400, "Windows에서만 설치할 수 있습니다.")
		return
	}
	a.mu.Lock()
	if (a.state.AppUpdate.Status != "available" && a.state.AppUpdate.Status != "error") || !a.updateRequiredLocked() {
		a.mu.Unlock()
		writeError(w, 409, "먼저 프로그램 업데이트를 확인해 주세요.")
		return
	}
	if a.premiereSelecting {
		a.mu.Unlock()
		writeError(w, 409, "\uD30C\uC77C \uC120\uD0DD\uC744 \uB9C8\uCE5C \uD6C4 \uC5C5\uB370\uC774\uD2B8\uD574 \uC8FC\uC138\uC694.")
		return
	}
	if isTrialBuild() {
		if err := validateTrialUpdate(a.latestRelease); err != nil {
			a.mu.Unlock()
			writeError(w, 409, err.Error())
			return
		}
	}
	for _, job := range a.state.Jobs {
		if job.Status == "queued" || job.Status == "downloading" {
			a.mu.Unlock()
			writeError(w, 409, "진행 중인 영상 다운로드가 끝난 후 업데이트해 주세요.")
			return
		}
	}
	exe, err := os.Executable()
	target := installedExePath()
	if err != nil || !strings.EqualFold(filepath.Clean(exe), filepath.Clean(target)) {
		a.mu.Unlock()
		writeError(w, 409, "설치 폴더의 ClipCatch를 실행해 주세요.")
		return
	}
	if a.state.Phase != "ready" {
		a.mu.Unlock()
		writeError(w, 409, "\uB3C4\uAD6C \uC900\uBE44 \uD6C4 \uB2E4\uC2DC \uC2DC\uB3C4\uD574 \uC8FC\uC138\uC694.")
		return
	}
	release := a.latestRelease
	a.state.AppUpdate = UpdateInfo{Status: "downloading", Latest: release.Version, Detail: "새 버전 다운로드 중..."}
	a.mu.Unlock()
	go a.downloadAndInstall(release, target)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func releaseChecksum(rel verifiedRelease) (string, error) {
	digest := rel.Digest
	if rel.SumsURL != "" {
		txt, err := fetchSmallText(rel.SumsURL)
		if err != nil {
			return "", fmt.Errorf("검증 파일을 가져오지 못했습니다: %w", err)
		}
		var parsed string
		if trialVersionRE.MatchString(rel.Version) {
			parsed, err = readNamedChecksum(txt, trialAssetName)
		} else {
			parsed, err = readAppChecksum(txt)
		}
		if err != nil {
			return "", err
		}
		if digest != "" && !strings.EqualFold(parsed, digest) {
			return "", errors.New("GitHub 릴리스 검증값이 서로 일치하지 않습니다")
		}
		digest = parsed
	}
	if digest == "" {
		return "", errors.New("검증값이 없습니다")
	}
	return digest, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// A freshly downloaded executable may repair an old per-user installation.
// This ONLY accepts the currently published GitHub binary, never an arbitrary EXE.
func verifyManualInstaller(source string, rel verifiedRelease, digest string) error {
	if compareRuntimeVersions(rel.Version, appVersion) != 0 {
		return fmt.Errorf("현재 최신 배포판은 v%s입니다. 최신 파일을 다운로드해 주세요", rel.Version)
	}
	actual, err := sha256File(source)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, digest) {
		return errors.New("실행파일의 검증값이 공식 배포본과 다릅니다. GitHub에서 다시 다운로드해 주세요")
	}
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	header := make([]byte, 2)
	_, err = io.ReadFull(f, header)
	_ = f.Close()
	if err != nil || !bytes.Equal(header, []byte{'M', 'Z'}) {
		return errors.New("Windows 실행파일 형식이 올바르지 않습니다")
	}
	return nil
}

func installedBinaryMatches(source, target string) bool {
	a, err := sha256File(source)
	if err != nil {
		return false
	}
	b, err := sha256File(target)
	return err == nil && strings.EqualFold(a, b)
}

// Avoid killing a downloader in the middle of a job. The old app's stop handler
// exits gracefully; the helper waits for the Windows image lock to be released.
func stopExistingInstanceForUpgrade() error {
	return stopExistingInstanceForUpgradeAt("http://127.0.0.1:" + runtimePort())
}

func stopExistingInstanceForUpgradeAt(base string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(base + "/api/health")
	if err != nil {
		return nil
	} // likely not running
	var health struct {
		App string `json:"app"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&health)
	_ = resp.Body.Close()
	if health.App != runtimeAppID() {
		return errors.New("다른 프로그램이 ClipCatch 포트를 사용하고 있습니다. 해당 프로그램을 종료한 후 다시 실행해 주세요")
	}
	resp, err = client.Get(base + "/api/state")
	if err != nil {
		return fmt.Errorf("기존 프로그램의 상태를 확인할 수 없습니다: %w", err)
	}
	var state ClientState
	parseErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&state)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || parseErr != nil {
		return errors.New("기존 프로그램 상태를 확인할 수 없습니다. ClipCatch를 종료한 후 다시 실행해 주세요")
	}
	for _, job := range state.Jobs {
		if job != nil && (job.Status == "queued" || job.Status == "downloading") {
			return errors.New("진행 중인 영상 다운로드가 끝난 후 업데이트해 주세요")
		}
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/stop", strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", base)
	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("기존 프로그램을 종료하지 못했습니다: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("기존 프로그램을 종료하지 못했습니다. 직접 종료 후 다시 시도해 주세요")
	}
	return nil
}

func launchManualRepair(source, target string) error {
	rel, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("공식 업데이트 정보를 확인할 수 없습니다: %w", err)
	}
	digest, err := releaseChecksum(rel)
	if err != nil {
		return err
	}
	if err = verifyManualInstaller(source, rel, digest); err != nil {
		return err
	}
	if err = stopExistingInstanceForUpgrade(); err != nil {
		return err
	}
	cmd := hiddenCommand(source, "--clipcatch-apply-update", target, appVersion)
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("복구 업데이트를 시작할 수 없습니다: %w", err)
	}
	return cmd.Process.Release()
}

func (a *App) downloadAndInstall(rel verifiedRelease, target string) {
	if isTrialBuild() {
		if e := validateTrialUpdate(rel); e != nil {
			a.setUpdate(UpdateInfo{Status: "error", Detail: e.Error()})
			return
		}
	}
	digest, err := releaseChecksum(rel)
	if err != nil {
		a.setUpdate(UpdateInfo{Status: "error", Detail: "업데이트 검증 실패: " + err.Error()})
		return
	}
	newFile := filepath.Join(a.dataDir, "updates", "new-"+runtimeUpdateAssetName())
	if err := os.MkdirAll(filepath.Dir(newFile), 0700); err != nil {
		a.setUpdate(UpdateInfo{Status: "error", Detail: err.Error()})
		return
	}
	_ = os.Remove(newFile)
	err = downloadVerified(rel.BinaryURL, newFile, digest, func(now, total int64) {
		a.setUpdate(UpdateInfo{Status: "downloading", Latest: rel.Version, Detail: "새 버전 다운로드 중...", Percent: percent(now, total)})
	})
	if err != nil {
		a.setUpdate(UpdateInfo{Status: "error", Detail: "새 버전 다운로드 실패: " + err.Error()})
		return
	}
	// Reject an unexpected file even if checksum text was mistakenly published.
	f, err := os.Open(newFile)
	if err != nil {
		a.setUpdate(UpdateInfo{Status: "error", Detail: err.Error()})
		return
	}
	magic := make([]byte, 2)
	_, err = io.ReadFull(f, magic)
	_ = f.Close()
	if err != nil || !bytes.Equal(magic, []byte{'M', 'Z'}) {
		_ = os.Remove(newFile)
		a.setUpdate(UpdateInfo{Status: "error", Detail: "다운로드된 파일이 Windows 실행파일이 아닙니다."})
		return
	}
	cmd := hiddenCommand(newFile, "--clipcatch-apply-update", target, rel.Version)
	if err := cmd.Start(); err != nil {
		a.setUpdate(UpdateInfo{Status: "error", Detail: "업데이트 설치 프로세스를 시작하지 못했습니다: " + err.Error()})
		return
	}
	_ = cmd.Process.Release()
	a.setUpdate(UpdateInfo{Status: "installing", Latest: rel.Version, Detail: "설치 중입니다. 잠시 후 자동으로 재실행됩니다.", Percent: 100})
	time.Sleep(400 * time.Millisecond)
	if a.server != nil {
		_ = a.server.Close()
	}
	os.Exit(0) // Windows frees the running EXE; the verified new EXE handles replacement.
}

func readAppChecksum(txt string) (string, error) {
	for _, line := range strings.Split(txt, "\n") {
		cols := strings.Fields(line)
		if len(cols) >= 2 && strings.TrimPrefix(cols[len(cols)-1], "*") == "ClipCatch.exe" && len(cols[0]) == 64 && shaRE.MatchString(cols[0]) {
			return strings.ToLower(cols[0]), nil
		}
	}
	return "", errors.New("SHA256SUMS에 ClipCatch.exe 검증값이 없습니다")
}

// The updater runs from the VERIFIED downloaded executable, not the in-use target.
func applyClipCatchUpdate(args []string) {
	if runtime.GOOS != "windows" || len(args) != 2 {
		return
	}
	target := installedExePath()
	if !strings.EqualFold(filepath.Clean(args[0]), filepath.Clean(target)) {
		return
	}
	if compareRuntimeVersions(args[1], appVersion) != 0 {
		return
	}
	self, err := os.Executable()
	if err != nil {
		return
	}
	if strings.EqualFold(filepath.Clean(self), filepath.Clean(target)) {
		return
	}
	backup := target + ".backup"
	_ = os.Remove(backup)
	renamed := false
	for i := 0; i < 100; i++ {
		if err := os.Rename(target, backup); err == nil {
			renamed = true
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !renamed {
		logUpdate("Could not move in-use target: " + target)
		showInstallError("업데이트 중 파일을 교체하지 못했어. ClipCatch를 완전히 종료한 후 다시 시도해 줘.")
		return
	}
	rollback := func() {
		// On Windows, the newly started program may take time to release its image file.
		for i := 0; i < 40; i++ {
			_ = os.Remove(target)
			if os.Rename(backup, target) == nil {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		logUpdate("Rollback could not restore backup; file remains at " + backup)
	}
	// Copy instead of renaming a running updater executable.
	in, err := os.Open(self)
	if err != nil {
		rollback()
		return
	}
	temp := target + ".incoming"
	_ = os.Remove(temp)
	out, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		_ = in.Close()
		rollback()
		return
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	_ = in.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(temp)
		rollback()
		return
	}
	if err := os.Rename(temp, target); err != nil {
		rollback()
		return
	}
	cmd := hiddenCommand(target)
	if err := cmd.Start(); err != nil {
		logUpdate("Failed to launch new executable: " + err.Error())
		rollback()
		return
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	// Verify the restarted application becomes healthy; otherwise restore the backup.
	healthy := false
	client := &http.Client{Timeout: 800 * time.Millisecond}
	for i := 0; i < 40; i++ {
		time.Sleep(350 * time.Millisecond)
		resp, err := client.Get("http://127.0.0.1:" + runtimePort() + "/api/health")
		if err != nil {
			continue
		}
		var body struct {
			App     string `json:"app"`
			Version string `json:"version"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&body)
		_ = resp.Body.Close()
		if body.App == runtimeAppID() && body.Version == appVersion {
			healthy = true
			break
		}
	}
	if healthy {
		// Retain the previous working executable until the next update.
		if err := writeUpdateReceipt(clipCatchDataDir(), "verified", appVersion, "새 버전 실행과 버전 확인까지 완료했어."); err != nil {
			logUpdate("Cannot save update receipt: " + err.Error())
		}
		logUpdate("Updated to " + appVersion)
	} else {
		logUpdate("New version did not become healthy; rolling back. pid=" + strconv.Itoa(pid))
		if runtime.GOOS == "windows" {
			_ = hiddenCommand("taskkill.exe", "/F", "/PID", strconv.Itoa(pid)).Run()
		}
		time.Sleep(500 * time.Millisecond)
		rollback()
		_ = writeUpdateReceipt(clipCatchDataDir(), "rolled_back", appVersion, "새 버전 실행 확인에 실패해서 기존 버전으로 복구했어.")
		old := hiddenCommand(target)
		_ = old.Start()
		if old.Process != nil {
			_ = old.Process.Release()
		}
	}
}

func logUpdate(msg string) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	if x := os.Getenv("LOCALAPPDATA"); x != "" {
		dir = x
	}
	dir = filepath.Join(dir, runtimeDataName())
	_ = os.MkdirAll(dir, 0700)
	f, err := os.OpenFile(filepath.Join(dir, "updater.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		_, _ = fmt.Fprintln(f, time.Now().Format(time.RFC3339), msg)
		_ = f.Close()
	}
}

func installedExePath() string {
	if isTrialBuild() {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "ClipCatch-Test2", "ClipCatch-Test2.exe")
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "ClipCatch", "ClipCatch.exe")
}

// One-time migration to a stable per-user location: subsequent updates do not need ZIP extraction.
func installAndRelaunch() bool {
	if runtime.GOOS != "windows" || os.Getenv("LOCALAPPDATA") == "" || os.Getenv("CLIPCATCH_PORTABLE") == "1" {
		return false
	}
	source, err := os.Executable()
	if err != nil {
		return false
	}
	target := installedExePath()
	if strings.EqualFold(filepath.Clean(source), filepath.Clean(target)) {
		return false
	}
	if isTrialBuild() {
		if err := stopExistingInstanceForUpgrade(); err != nil {
			showInstallError(err.Error())
			return true
		}
		time.Sleep(450 * time.Millisecond)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		showInstallError("ClipCatch 설치 폴더를 만들 수 없습니다: " + err.Error())
		return true
	}
	if _, err := os.Stat(target); err == nil {
		if !installedBinaryMatches(source, target) {
			// Launching a newly downloaded official binary MUST upgrade, not reopen an old copy.
			if err := launchManualRepair(source, target); err != nil {
				logUpdate("Manual update failed: " + err.Error())
				showInstallError("업데이트를 완료하지 못했어.\n" + err.Error() + "\n\nClipCatch의 기존 파일과 다운로드 기록은 그대로 보관돼 있어.")
			}
			return true
		}
	} else if os.IsNotExist(err) {
		tmp := target + ".new"
		in, openErr := os.Open(source)
		if openErr != nil {
			showInstallError("실행파일을 열 수 없습니다: " + openErr.Error())
			return true
		}
		out, createErr := os.OpenFile(tmp, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0700)
		if createErr != nil {
			_ = in.Close()
			showInstallError("설치 파일을 만들 수 없습니다: " + createErr.Error())
			return true
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		_ = in.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(tmp)
			showInstallError("실행파일 복사에 실패했습니다")
			return true
		}
		if err := os.Rename(tmp, target); err != nil {
			_ = os.Remove(tmp)
			showInstallError("실행파일 설치에 실패했습니다: " + err.Error())
			return true
		}
	} else {
		showInstallError("기존 설치 파일을 확인할 수 없습니다: " + err.Error())
		return true
	}
	makeClipCatchShortcuts(target)
	cmd := hiddenCommand(target)
	if err := cmd.Start(); err != nil {
		showInstallError("ClipCatch를 시작할 수 없습니다: " + err.Error())
		return true
	}
	_ = cmd.Process.Release()
	return true
}

func makeClipCatchShortcuts(target string) {
	// PowerShell is invoked internally; the user never needs to run terminal commands.
	name := "ClipCatch"
	if isTrialBuild() {
		name = "ClipCatch TEST2"
	}
	paths := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		desktop := filepath.Join(home, "Desktop")
		if stat, err := os.Stat(desktop); err == nil && stat.IsDir() {
			paths = append(paths, filepath.Join(desktop, name+".lnk"))
		}
	}
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		start := filepath.Join(appdata, "Microsoft", "Windows", "Start Menu", "Programs")
		_ = os.MkdirAll(start, 0755)
		paths = append(paths, filepath.Join(start, name+".lnk"))
	}
	script := `$ErrorActionPreference='Stop';$s=(New-Object -ComObject WScript.Shell).CreateShortcut($env:CLIPCATCH_SHORTCUT_PATH);$s.TargetPath=$env:CLIPCATCH_SHORTCUT_TARGET;$s.WorkingDirectory=(Split-Path $env:CLIPCATCH_SHORTCUT_TARGET);$s.IconLocation=$env:CLIPCATCH_SHORTCUT_TARGET+',0';$s.Save()`
	for _, p := range paths {
		cmd := hiddenCommand("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.Env = append(os.Environ(), "CLIPCATCH_SHORTCUT_PATH="+p, "CLIPCATCH_SHORTCUT_TARGET="+target)
		if err := cmd.Run(); err != nil {
			logUpdate("Cannot create shortcut: " + err.Error())
		}
	}
}
