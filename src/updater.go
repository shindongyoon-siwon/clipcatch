package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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
		if strings.Contains(msg, "아직 게시되지") {
			a.setUpdate(UpdateInfo{Status: "unavailable", Detail: "GitHub 배포 준비 중"})
		} else {
			a.setUpdate(UpdateInfo{Status: "error", Detail: "업데이트 확인 실패: " + msg})
		}
		return
	}
	a.mu.Lock()
	a.latestRelease = rel
	a.mu.Unlock()
	if compareSemver(rel.Version, appVersion) <= 0 {
		a.setUpdate(UpdateInfo{Status: "current", Latest: rel.Version, Detail: "최신 버전을 사용하고 있어요."})
	} else {
		a.setUpdate(UpdateInfo{Status: "available", Latest: rel.Version, Detail: "새 버전이 있어요. 파일을 다시 받을 필요 없이 설치할 수 있어요."})
	}
}

func (a *App) checkAppUpdateHandler(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	busy := a.state.AppUpdate.Status == "downloading" || a.state.AppUpdate.Status == "installing" || a.state.AppUpdate.Status == "checking"
	a.mu.Unlock()
	if busy {
		writeError(w, http.StatusConflict, "이미 업데이트를 확인하거나 설치 중입니다.")
		return
	}
	go a.checkAppUpdate()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) installAppUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "windows" {
		writeError(w, 400, "Windows에서만 설치할 수 있습니다.")
		return
	}
	a.mu.Lock()
	if a.state.AppUpdate.Status != "available" || a.latestRelease.BinaryURL == "" {
		a.mu.Unlock()
		writeError(w, 409, "먼저 프로그램 업데이트를 확인해 주세요.")
		return
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
	release := a.latestRelease
	a.state.AppUpdate = UpdateInfo{Status: "downloading", Latest: release.Version, Detail: "새 버전 다운로드 중..."}
	a.mu.Unlock()
	go a.downloadAndInstall(release, target)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) downloadAndInstall(rel verifiedRelease, target string) {
	digest := rel.Digest
	if rel.SumsURL != "" {
		txt, err := fetchSmallText(rel.SumsURL)
		if err != nil {
			a.setUpdate(UpdateInfo{Status: "error", Detail: "검증 파일을 가져오지 못했습니다: " + err.Error()})
			return
		}
		parsed, err := readAppChecksum(txt)
		if err != nil {
			a.setUpdate(UpdateInfo{Status: "error", Detail: err.Error()})
			return
		}
		if digest != "" && !strings.EqualFold(parsed, digest) {
			a.setUpdate(UpdateInfo{Status: "error", Detail: "GitHub 릴리스 검증값이 서로 일치하지 않습니다."})
			return
		}
		digest = parsed
	}
	if digest == "" {
		a.setUpdate(UpdateInfo{Status: "error", Detail: "검증값이 없습니다."})
		return
	}
	newFile := filepath.Join(a.dataDir, "updates", "ClipCatch-new.exe")
	if err := os.MkdirAll(filepath.Dir(newFile), 0700); err != nil {
		a.setUpdate(UpdateInfo{Status: "error", Detail: err.Error()})
		return
	}
	_ = os.Remove(newFile)
	err := downloadVerified(rel.BinaryURL, newFile, digest, func(now, total int64) {
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
	cmd := exec.Command(newFile, "--clipcatch-apply-update", target, rel.Version)
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
	if compareSemver(args[1], appVersion) != 0 {
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
	cmd := exec.Command(target)
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
		resp, err := client.Get("http://127.0.0.1:8765/api/health")
		if err != nil {
			continue
		}
		var body struct {
			App     string `json:"app"`
			Version string `json:"version"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&body)
		_ = resp.Body.Close()
		if body.App == "clipcatch" && body.Version == appVersion {
			healthy = true
			break
		}
	}
	if healthy {
		_ = os.Remove(backup)
		logUpdate("Updated to " + appVersion)
	} else {
		logUpdate("New version did not become healthy; rolling back. pid=" + strconv.Itoa(pid))
		if runtime.GOOS == "windows" {
			_ = exec.Command("taskkill.exe", "/F", "/PID", strconv.Itoa(pid)).Run()
		}
		time.Sleep(500 * time.Millisecond)
		rollback()
		old := exec.Command(target)
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
	dir = filepath.Join(dir, "ClipCatch")
	_ = os.MkdirAll(dir, 0700)
	f, err := os.OpenFile(filepath.Join(dir, "updater.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		_, _ = fmt.Fprintln(f, time.Now().Format(time.RFC3339), msg)
		_ = f.Close()
	}
}

func installedExePath() string {
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
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return false
	}
	// Never overwrite an existing installed build with an old copy from a ZIP.
	if _, err := os.Stat(target); os.IsNotExist(err) {
		tmp := target + ".new"
		in, err := os.Open(source)
		if err != nil {
			return false
		}
		out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0700)
		if err != nil {
			_ = in.Close()
			return false
		}
		_, cErr := io.Copy(out, in)
		closeErr := out.Close()
		_ = in.Close()
		if cErr != nil || closeErr != nil {
			_ = os.Remove(tmp)
			return false
		}
		if err := os.Rename(tmp, target); err != nil {
			_ = os.Remove(tmp)
			return false
		}
	}
	if _, err := os.Stat(target); err != nil {
		return false
	}
	makeClipCatchShortcuts(target)
	cmd := exec.Command(target)
	if err := cmd.Start(); err != nil {
		return false
	}
	_ = cmd.Process.Release()
	return true
}

func makeClipCatchShortcuts(target string) {
	// PowerShell is invoked internally; the user never needs to run terminal commands.
	paths := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		desktop := filepath.Join(home, "Desktop")
		if stat, err := os.Stat(desktop); err == nil && stat.IsDir() {
			paths = append(paths, filepath.Join(desktop, "ClipCatch.lnk"))
		}
	}
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		start := filepath.Join(appdata, "Microsoft", "Windows", "Start Menu", "Programs")
		_ = os.MkdirAll(start, 0755)
		paths = append(paths, filepath.Join(start, "ClipCatch.lnk"))
	}
	script := `$ErrorActionPreference='Stop';$s=(New-Object -ComObject WScript.Shell).CreateShortcut($env:CLIPCATCH_SHORTCUT_PATH);$s.TargetPath=$env:CLIPCATCH_SHORTCUT_TARGET;$s.WorkingDirectory=(Split-Path $env:CLIPCATCH_SHORTCUT_TARGET);$s.IconLocation=$env:CLIPCATCH_SHORTCUT_TARGET+',0';$s.Save()`
	for _, p := range paths {
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.Env = append(os.Environ(), "CLIPCATCH_SHORTCUT_PATH="+p, "CLIPCATCH_SHORTCUT_TARGET="+target)
		if err := cmd.Run(); err != nil {
			logUpdate("Cannot create shortcut: " + err.Error())
		}
	}
}
