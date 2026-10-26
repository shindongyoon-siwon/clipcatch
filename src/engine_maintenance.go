package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	ytDlpNightlyURL = "https://github.com/yt-dlp/yt-dlp-nightly-builds/releases/latest/download/yt-dlp.exe"
	ytDlpNightlySUM = "https://github.com/yt-dlp/yt-dlp-nightly-builds/releases/latest/download/SHA2-256SUMS"
)

type EngineMetadata struct {
	Current string `json:"current"`
	Backup  string `json:"backup,omitempty"`
}

type EngineInfo struct {
	Channel     string `json:"channel"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	CanRollback bool   `json:"can_rollback"`
	CheckedAt   string `json:"checked_at,omitempty"`
}

func normalizeChannel(channel string) string {
	if channel == "nightly" {
		return "nightly"
	}
	return "stable"
}

func (a *App) loadEngineMetadata() {
	meta := EngineMetadata{Current: "stable"}
	if b, err := os.ReadFile(filepath.Join(a.dataDir, "engine-state.json")); err == nil {
		var saved EngineMetadata
		if json.Unmarshal(b, &saved) == nil && (saved.Current == "stable" || saved.Current == "nightly") {
			meta.Current = saved.Current
			if saved.Backup == "stable" || saved.Backup == "nightly" {
				meta.Backup = saved.Backup
			}
		}
	}
	a.engineMeta = meta
	a.state.Engine = EngineInfo{Channel: meta.Current, Status: "idle", Message: "다운로드 엔진 확인 대기 중", CanRollback: nonempty(filepath.Join(a.toolDir, "yt-dlp-backup.exe"))}
}

func (a *App) persistEngineMetadata(meta EngineMetadata) error {
	if err := os.MkdirAll(a.dataDir, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	tmp := filepath.Join(a.dataDir, "engine-state.json.part")
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	// Do not remove the old metadata before the new file is ready.
	target := filepath.Join(a.dataDir, "engine-state.json")
	if err = os.Rename(tmp, target); err != nil {
		return err
	}
	return nil
}

func engineURL(channel string) (string, string) {
	if channel == "nightly" {
		return ytDlpNightlyURL, ytDlpNightlySUM
	}
	return ytDlpURL, "https://github.com/yt-dlp/yt-dlp/releases/latest/download/SHA2-256SUMS"
}

func fetchEngineHash(channel string) (string, error) {
	if channel != "stable" && channel != "nightly" {
		return "", errors.New("지원하지 않는 엔진 채널")
	}
	_, sumURL := engineURL(channel)
	s, err := fetchSmallText(sumURL)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(s, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		if strings.TrimPrefix(strings.TrimPrefix(parts[len(parts)-1], "*"), "./") == "yt-dlp.exe" && shaRE.MatchString(parts[0]) && len(parts[0]) == 64 {
			return strings.ToLower(parts[0]), nil
		}
	}
	return "", errors.New("공식 yt-dlp SHA256 목록에 Windows 실행파일이 없습니다")
}

func (a *App) engineCheckOnStartup() {
	if runtime.GOOS != "windows" {
		return
	}
	a.mu.Lock()
	if a.state.Phase != "ready" {
		a.mu.Unlock()
		return
	}
	channel := a.engineMeta.Current
	path := a.ytDlp
	a.state.Engine.Status = "checking"
	a.state.Engine.Message = "영상 엔진 최신 여부 확인 중..."
	a.mu.Unlock()
	want, err := fetchEngineHash(channel)
	if err != nil {
		a.mu.Lock()
		a.state.Engine.Status = "offline"
		a.state.Engine.Message = "최신 엔진 확인 실패 · 기존 엔진으로 다운로드는 계속 가능"
		a.state.Engine.CheckedAt = time.Now().Format(time.RFC3339)
		a.mu.Unlock()
		return
	}
	have, hashErr := sha256File(path)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.engineMeta.Current != channel || a.state.Phase != "ready" || a.state.Engine.Status != "checking" {
		return
	}
	a.state.Engine.CheckedAt = time.Now().Format(time.RFC3339)
	if hashErr != nil {
		a.state.Engine.Status = "missing"
		a.state.Engine.Message = "엔진 파일 확인 실패 · 문제 해결에서 복구해 줘"
	} else if strings.EqualFold(have, want) {
		a.state.Engine.Status = "current"
		a.state.Engine.Message = "영상 엔진 최신 상태"
	} else {
		a.state.Engine.Status = "available"
		a.state.Engine.Message = "새로운 영상 엔진 사용 가능 · 필요하면 업데이트해 줘"
	}
}

// Engine replacement is a separate operation from ClipCatch program updates.
// Only official stable/nightly yt-dlp assets with matching SHA256 are accepted.
func (a *App) beginEngineOperation(w http.ResponseWriter, channel, kind string) bool {
	a.mu.Lock()
	if a.state.Phase != "ready" {
		a.mu.Unlock()
		writeError(w, 409, "다운로드 준비 또는 엔진 업데이트가 끝난 다음 시도해 줘.")
		return false
	}
	if len(a.activeRuns) > 0 {
		a.mu.Unlock()
		writeError(w, 409, "진행 중인 작업이 종료된 후 다시 시도해 줘.")
		return false
	}
	for _, j := range a.state.Jobs {
		if j.Status == "queued" || j.Status == "downloading" {
			a.mu.Unlock()
			writeError(w, 409, "진행 중인 다운로드가 끝난 뒤 엔진을 변경해 줘.")
			return false
		}
	}
	if runtime.GOOS != "windows" {
		writeError(w, 400, "Windows 실행파일에서만 사용 가능해.")
		return false
	}
	a.state.Phase = "preparing"
	a.state.SetupPct = 0
	a.state.SetupError = ""
	if kind == "rollback" {
		if !nonempty(filepath.Join(a.toolDir, "yt-dlp-backup.exe")) {
			a.state.Phase = "ready"
			a.mu.Unlock()
			writeError(w, 409, "되돌릴 이전 엔진이 없어.")
			return false
		}
		a.state.SetupText = "이전 다운로드 엔진 복구 중..."
	} else if channel == "nightly" {
		a.state.SetupText = "공식 시험판 엔진 확인 중..."
	} else {
		a.state.SetupText = "안정판 엔진 최신 버전 확인 중..."
	}
	a.state.Engine.Status = "working"
	a.state.Engine.Message = a.state.SetupText
	a.mu.Unlock()
	writeJSON(w, 200, map[string]bool{"ok": true})
	return true
}

func (a *App) repairEngineHandler(w http.ResponseWriter, r *http.Request) {
	if !a.beginEngineOperation(w, "nightly", "update") {
		return
	}
	go a.replaceEngine("nightly")
}
func (a *App) rollbackEngineHandler(w http.ResponseWriter, r *http.Request) {
	if !a.beginEngineOperation(w, "", "rollback") {
		return
	}
	go a.rollbackEngine()
}

// Use rename-based replacement so failure does not discard the working binary.
// Keep just one old engine for a one-click rollback.
// Interrupted engine swaps may leave the old engine at the backup path.
// Restore a usable binary on the next startup without any network access.
func recoverInterruptedEngineFiles(dir string) error {
	target := filepath.Join(dir, "yt-dlp.exe")
	backup := filepath.Join(dir, "yt-dlp-backup.exe")
	previous := backup + ".previous"
	stage := target + ".rollback-stage"
	if nonempty(target) {
		if !nonempty(backup) && nonempty(previous) {
			return os.Rename(previous, backup)
		}
		if nonempty(backup) && nonempty(previous) {
			_ = os.Remove(previous)
		}
		return nil
	}
	if nonempty(backup) {
		if err := os.Rename(backup, target); err != nil {
			return err
		}
		if nonempty(previous) {
			_ = os.Rename(previous, backup)
		}
		return nil
	}
	if nonempty(stage) {
		return os.Rename(stage, target)
	}
	if nonempty(previous) {
		return os.Rename(previous, target)
	}
	return nil
}

func replaceEngineFiles(target, next, backup string) error {
	if !nonempty(next) {
		return errors.New("검증된 새 엔진 파일이 없어")
	}
	oldBackup := backup + ".previous"
	_ = os.Remove(oldBackup)
	hadBackup := nonempty(backup)
	if hadBackup {
		if err := os.Rename(backup, oldBackup); err != nil {
			return fmt.Errorf("이전 백업 보관 실패: %w", err)
		}
	}
	hadTarget := nonempty(target)
	if hadTarget {
		if err := os.Rename(target, backup); err != nil {
			if hadBackup {
				_ = os.Rename(oldBackup, backup)
			}
			return fmt.Errorf("현재 엔진 백업 실패: %w", err)
		}
	}
	if err := os.Rename(next, target); err != nil {
		if hadTarget {
			_ = os.Rename(backup, target)
		}
		if hadBackup {
			_ = os.Rename(oldBackup, backup)
		}
		return fmt.Errorf("새 엔진 교체 실패: %w", err)
	}
	_ = os.Remove(oldBackup)
	return nil
}

func rollbackEngineFiles(target, backup string) error {
	if !nonempty(backup) {
		return errors.New("이전 버전 백업이 없어")
	}
	stage := target + ".rollback-stage"
	_ = os.Remove(stage)
	if err := os.Rename(target, stage); err != nil {
		return err
	}
	if err := os.Rename(backup, target); err != nil {
		_ = os.Rename(stage, target)
		return err
	}
	if err := os.Rename(stage, backup); err != nil {
		// Restore a usable target and keep the original backup if possible.
		_ = os.Rename(target, stage+".restored")
		_ = os.Rename(stage, target)
		_ = os.Rename(stage+".restored", backup)
		return err
	}
	return nil
}

func (a *App) replaceEngine(channel string) {
	url, _ := engineURL(channel)
	checksum, err := fetchEngineHash(channel)
	if err != nil {
		a.engineOperationFailed("엔진 검증 정보 확인 실패", err)
		return
	}
	if err = os.MkdirAll(a.toolDir, 0700); err != nil {
		a.engineOperationFailed("엔진 폴더 준비 실패", err)
		return
	}
	dest := filepath.Join(a.toolDir, "yt-dlp.exe")
	next := filepath.Join(a.toolDir, "yt-dlp-next.exe")
	backup := filepath.Join(a.toolDir, "yt-dlp-backup.exe")
	_ = os.Remove(next)
	if err = downloadVerified(url, next, checksum, func(now, total int64) {
		label := "안정판"
		if channel == "nightly" {
			label = "시험판"
		}
		a.setSetup("preparing", label+" 엔진 다운로드 중...", "", percent(now, total))
	}); err != nil {
		a.engineOperationFailed("엔진 다운로드 실패", err)
		return
	}
	// Only expected Windows executable data is accepted; checksum alone proves
	// it matches the asset, not its format.
	f, err := os.Open(next)
	if err != nil {
		a.engineOperationFailed("엔진 실행파일 확인 실패", err)
		return
	}
	hdr := make([]byte, 2)
	n, readErr := f.Read(hdr)
	_ = f.Close()
	if readErr != nil || n != 2 || hdr[0] != 'M' || hdr[1] != 'Z' {
		_ = os.Remove(next)
		a.engineOperationFailed("엔진 파일 형식 오류", errors.New("다운로드한 파일이 Windows 실행파일이 아니야"))
		return
	}
	a.mu.Lock()
	meta := a.engineMeta
	a.mu.Unlock()
	if err = replaceEngineFiles(dest, next, backup); err != nil {
		a.engineOperationFailed("엔진 교체 실패", err)
		return
	}
	old := meta.Current
	meta.Current = channel
	if nonempty(backup) {
		meta.Backup = normalizeChannel(old)
	} else {
		meta.Backup = ""
	}
	if err = a.persistEngineMetadata(meta); err != nil {
		// File change succeeded; never misleadingly claim it failed or roll back
		// without verifying. Recover state as accurately as possible.
		a.mu.Lock()
		a.engineMeta = meta
		a.ytDlp = dest
		a.mu.Unlock()
		a.engineOperationFailed("엔진 변경은 됐지만 설정 저장 실패", err)
		return
	}
	a.mu.Lock()
	a.engineMeta = meta
	a.ytDlp = dest
	a.state.Engine = EngineInfo{Channel: channel, Status: "current", Message: "엔진 설치 완료 · 문제가 생기면 이전 버전으로 되돌릴 수 있어", CanRollback: nonempty(backup), CheckedAt: time.Now().Format(time.RFC3339)}
	a.mu.Unlock()
	if channel == "nightly" {
		a.setSetup("ready", "시험판 엔진 복구 완료", "", 100)
	} else {
		a.setSetup("ready", "yt-dlp 업데이트 완료", "", 100)
	}
}
func (a *App) rollbackEngine() {
	dest := filepath.Join(a.toolDir, "yt-dlp.exe")
	backup := filepath.Join(a.toolDir, "yt-dlp-backup.exe")
	a.mu.Lock()
	meta := a.engineMeta
	a.mu.Unlock()
	if err := rollbackEngineFiles(dest, backup); err != nil {
		a.engineOperationFailed("이전 엔진 복구 실패", err)
		return
	}
	meta.Current, meta.Backup = normalizeChannel(meta.Backup), normalizeChannel(meta.Current)
	if err := a.persistEngineMetadata(meta); err != nil {
		a.mu.Lock()
		a.engineMeta = meta
		a.ytDlp = dest
		a.mu.Unlock()
		a.engineOperationFailed("복구 완료, 설정 저장 실패", err)
		return
	}
	a.mu.Lock()
	a.engineMeta = meta
	a.ytDlp = dest
	a.state.Engine = EngineInfo{Channel: meta.Current, Status: "current", Message: "이전 엔진으로 되돌리기 완료", CanRollback: true, CheckedAt: time.Now().Format(time.RFC3339)}
	a.mu.Unlock()
	a.setSetup("ready", "엔진 되돌리기 완료", "", 100)
}
func (a *App) engineOperationFailed(prefix string, err error) {
	a.mu.Lock()
	a.state.Engine.Status = "error"
	a.state.Engine.Message = prefix + " · 문제 해결에서 다시 시도해 줘"
	a.state.Engine.CanRollback = nonempty(filepath.Join(a.toolDir, "yt-dlp-backup.exe"))
	a.mu.Unlock()
	a.setSetup("ready", "업데이트 실패", prefix+": "+err.Error(), 0)
}
