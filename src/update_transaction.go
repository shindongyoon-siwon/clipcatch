package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var errUpdateBusy = errors.New("another verified update owns the installation")

const updateRestartEnv = "CLIPCATCH_UPDATE_RESTART"

type updatePeer struct {
	PID  uint32
	Base string
}
type updateRestart struct {
	Token  string
	Digest string
	At     time.Time
}
type updateSwap struct{ Target, Backup, NewHash string }
type updateFailure struct{ Code, Stage, Version, Detail, At string }

func newUpdateNonce() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func sameUpdatePath(a, b string) bool {
	aa, e := filepath.Abs(a)
	if e != nil {
		return false
	}
	bb, e := filepath.Abs(b)
	if e != nil {
		return false
	}
	if s, e := filepath.EvalSymlinks(aa); e == nil {
		aa = s
	}
	if s, e := filepath.EvalSymlinks(bb); e == nil {
		bb = s
	}
	return strings.EqualFold(filepath.Clean(aa), filepath.Clean(bb))
}

// Copy and verify BEFORE moving the working installation. A short write or disk
// error cannot turn an intact installation into a missing executable.
func stageUpdateFile(source, target string) (string, error) {
	expected, e := sha256File(source)
	if e != nil {
		return "", e
	}
	in, e := os.Open(source)
	if e != nil {
		return "", e
	}
	defer in.Close()
	f, e := os.CreateTemp(filepath.Dir(target), ".clipcatch-incoming-*.exe")
	if e != nil {
		return "", e
	}
	name := f.Name()
	good := false
	defer func() {
		f.Close()
		if !good {
			os.Remove(name)
		}
	}()
	if _, e = io.Copy(f, in); e != nil {
		return "", e
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	got, e := sha256File(name)
	if e != nil {
		return "", e
	}
	if got != expected {
		return "", errors.New("staged executable checksum mismatch")
	}
	good = true
	return name, nil
}
func renameUpdateFile(from, to string, timeout time.Duration) error {
	end := time.Now().Add(timeout)
	var last error
	for {
		last = os.Rename(from, to)
		if last == nil {
			return nil
		}
		if errors.Is(last, os.ErrNotExist) || !time.Now().Before(end) {
			return last
		}
		time.Sleep(100 * time.Millisecond)
	}
}
func startUpdateSwap(staged, target, nonce string) (*updateSwap, error) {
	sum, e := sha256File(staged)
	if e != nil {
		return nil, e
	}
	s := &updateSwap{Target: target, Backup: target + ".backup-" + nonce, NewHash: sum}
	if _, e = os.Lstat(s.Backup); !os.IsNotExist(e) {
		return nil, errors.New("unique backup path already exists or is inaccessible")
	}
	if e = renameUpdateFile(target, s.Backup, 3*time.Second); e != nil {
		return nil, fmt.Errorf("backup rename: %w", e)
	}
	if e = renameUpdateFile(staged, target, 3*time.Second); e != nil {
		restore := renameUpdateFile(s.Backup, target, 3*time.Second)
		if restore != nil {
			return nil, fmt.Errorf("install rename: %v; restore failed: %v; preserved backup: %s", e, restore, s.Backup)
		}
		return nil, fmt.Errorf("install rename failed; original restored: %w", e)
	}
	return s, nil
}
func (s *updateSwap) rollback() error {
	if sum, e := sha256File(s.Target); e == nil {
		if sum != s.NewHash {
			return errors.New("target changed unexpectedly; backup preserved instead of overwriting")
		}
		// Never delete a file another process may still be using.
		failed := s.Backup + ".failed"
		if e = renameUpdateFile(s.Target, failed, 3*time.Second); e != nil {
			return e
		}
		if e = renameUpdateFile(s.Backup, s.Target, 3*time.Second); e != nil {
			return fmt.Errorf("restore failed: %w; backup %s", e, s.Backup)
		}
		_ = os.Remove(failed)
		return nil
	} else if !os.IsNotExist(e) {
		return e
	}
	return renameUpdateFile(s.Backup, s.Target, 3*time.Second)
}
func (s *updateSwap) keepPrevious() {
	// A locked old .backup must NOT block installation. Rotation happens only
	// AFTER the new process is healthy. Failure leaves both backups untouched.
	legacy := s.Target + ".backup"
	if e := os.Remove(legacy); e != nil && !os.IsNotExist(e) {
		logUpdate("Previous backup retained at " + s.Backup + ": " + e.Error())
		return
	}
	if e := os.Rename(s.Backup, legacy); e != nil {
		logUpdate("Previous backup retained at " + s.Backup + ": " + e.Error())
	}
}
func readUpdateJSON(client *http.Client, url string, out any) error {
	r, e := client.Get(url)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("HTTP %d from %s", r.StatusCode, url)
	}
	return json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(out)
}
func checkUpdatePeer(client *http.Client, p updatePeer) error {
	var h struct{ App, Channel string }
	if e := readUpdateJSON(client, p.Base+"/api/health", &h); e != nil {
		return e
	}
	if h.App != runtimeAppID() || (h.Channel != "" && h.Channel != appChannel) {
		return errors.New("process identity/channel mismatch")
	}
	var s ClientState
	if e := readUpdateJSON(client, p.Base+"/api/state", &s); e != nil {
		return e
	}
	if s.ActiveTasks > 0 {
		return errors.New("active processing; save work and retry after it finishes")
	}
	for _, j := range s.Jobs {
		if j != nil && (j.Status == "queued" || j.Status == "downloading") {
			return errors.New("queued or active work; update postponed")
		}
	}
	return nil
}
func stopUpdatePeers(peers []updatePeer) error {
	client := &http.Client{Timeout: 2 * time.Second}
	// Inspect ALL owned endpoints before stopping any of them; no force flag.
	for _, p := range peers {
		if e := checkUpdatePeer(client, p); e != nil {
			if updatePIDExited(p.PID) {
				continue
			}
			return fmt.Errorf("PID %d: %w", p.PID, e)
		}
	}
	for _, p := range peers {
		req, e := http.NewRequest("POST", p.Base+"/api/stop", strings.NewReader("{}"))
		if e != nil {
			return e
		}
		req.Header.Set("Origin", p.Base)
		req.Header.Set("Content-Type", "application/json")
		r, e := client.Do(req)
		if e != nil {
			if updatePIDExited(p.PID) {
				continue
			}
			return e
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			return fmt.Errorf("graceful shutdown refused by PID %d: HTTP %d", p.PID, r.StatusCode)
		}
	}
	return nil
}
func updateLaunchGate() bool {
	target := installedExePath()
	token := os.Getenv(updateRestartEnv)
	os.Unsetenv(updateRestartEnv)
	if token != "" {
		self, _ := os.Executable()
		var m updateRestart
		raw, e := os.ReadFile(target + ".restart.json")
		if e == nil && len(raw) < 4096 && json.Unmarshal(raw, &m) == nil && len(token) == 32 && len(m.Token) == 32 && time.Since(m.At) >= 0 && time.Since(m.At) < 2*time.Minute && sameUpdatePath(self, target) && subtle.ConstantTimeCompare([]byte(token), []byte(m.Token)) == 1 {
			if sum, e := sha256File(self); e == nil && sum == m.Digest {
				return false
			}
		}
	}
	if _, e := os.Stat(filepath.Dir(target)); os.IsNotExist(e) {
		return false
	}
	unlock, e := lockUpdateTarget(target)
	if e != nil {
		logUpdate("Launch deferred while update lock is held: " + e.Error())
		if !errors.Is(e, errUpdateBusy) {
			showInstallError("CC-UPD-LOCK: " + e.Error())
		}
		return true
	}
	unlock()
	return false
}
func launchUpdatedTarget(target, token, digest string) (*exec.Cmd, error) {
	m, _ := json.Marshal(updateRestart{Token: token, Digest: digest, At: time.Now()})
	if e := os.WriteFile(target+".restart.json", m, 0600); e != nil {
		return nil, e
	}
	cmd := hiddenCommand(target)
	cmd.Env = append(os.Environ(), updateRestartEnv+"="+token)
	if e := cmd.Start(); e != nil {
		return nil, e
	}
	return cmd, nil
}
func verifyUpdateHealth(pid uint32, timeout time.Duration) bool {
	client := &http.Client{Timeout: 600 * time.Millisecond}
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		var h struct{ App, Version, Channel string }
		if e := readUpdateJSON(client, "http://127.0.0.1:"+runtimePort()+"/api/health", &h); e == nil && h.App == runtimeAppID() && h.Version == appVersion && (h.Channel == "" || h.Channel == appChannel) {
			// Bind health to the process actually launched, not a stale listener.
			peers, e := updateProcessPeers(installedExePath())
			if e == nil {
				for _, p := range peers {
					if p.PID == pid && p.Base == "http://127.0.0.1:"+runtimePort() {
						return true
					}
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}
func performSafeUpdate(source, target string) error {
	unlock, e := lockUpdateTarget(target)
	if e != nil {
		return e
	}
	defer unlock()
	if installedBinaryMatches(source, target) {
		// A competing installer already finished. No second swap or downgrade.
		unlock()
		cmd := hiddenCommand(target)
		if e = cmd.Start(); e != nil {
			return e
		}
		return cmd.Process.Release()
	}
	staged, e := stageUpdateFile(source, target)
	if e != nil {
		return fmt.Errorf("CC-UPD-STAGE: %w", e)
	}
	defer os.Remove(staged)
	nonce, e := newUpdateNonce()
	if e != nil {
		return e
	}
	peers, e := updateProcessPeers(target)
	if e != nil {
		return fmt.Errorf("CC-UPD-PROCESS: %w", e)
	}
	if e = stopUpdatePeers(peers); e != nil {
		return fmt.Errorf("CC-UPD-BUSY: %w", e)
	}
	if e = waitUpdateProcesses(target, 20*time.Second); e != nil {
		return fmt.Errorf("CC-UPD-EXIT: %w", e)
	}
	swap, e := startUpdateSwap(staged, target, nonce)
	if e != nil {
		return fmt.Errorf("CC-UPD-REPLACE: %w", e)
	}
	defer os.Remove(target + ".restart.json")
	child, e := launchUpdatedTarget(target, nonce, swap.NewHash)
	if e == nil && verifyUpdateHealth(uint32(child.Process.Pid), 18*time.Second) {
		child.Process.Release()
		swap.keepPrevious()
		makeClipCatchShortcuts(target)
		if e = writeUpdateReceipt(clipCatchDataDir(), "verified", appVersion, "새 버전 실행과 버전 확인까지 완료했어."); e != nil {
			logUpdate("Receipt: " + e.Error())
		}
		os.Remove(filepath.Join(clipCatchDataDir(), "update-failure.json"))
		logUpdate("Verified update completed: " + appVersion)
		return nil
	}
	if child != nil {
		// Handle-based termination: only the new process we started, never /IM.
		_ = child.Process.Kill()
		_ = child.Wait()
	}
	if restore := swap.rollback(); restore != nil {
		return fmt.Errorf("CC-UPD-RESTORE: %w", restore)
	}
	os.Remove(target + ".restart.json")
	unlock()
	old := hiddenCommand(target)
	if err := old.Start(); err == nil {
		old.Process.Release()
	} else {
		logUpdate("Restored file but relaunch failed: " + err.Error())
	}
	_ = writeUpdateReceipt(clipCatchDataDir(), "rolled_back", appVersion, "새 버전 실행 확인에 실패해 이전 파일로 복구했어.")
	logUpdate(fmt.Sprintf("New process failed verification; original restored: %v", e))
	return nil
}
func reportSafeUpdateFailure(e error) {
	if errors.Is(e, errUpdateBusy) {
		logUpdate("Duplicate updater ignored; existing transaction continues")
		return
	}
	detail := e.Error()
	code := "CC-UPD-UNKNOWN"
	if i := strings.Index(detail, ":"); i > 0 && strings.HasPrefix(detail, "CC-UPD-") {
		code = detail[:i]
	}
	logUpdate(detail)
	dir := clipCatchDataDir()
	_ = os.MkdirAll(dir, 0700)
	b, _ := json.MarshalIndent(updateFailure{Code: code, Stage: "update", Version: appVersion, Detail: detail, At: time.Now().Format(time.RFC3339)}, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "update-failure.json"), b, 0600)
	// The transaction lock has already been released before a modal is shown.
	showInstallError("업데이트를 완료하지 못했어. [" + code + "]\n원본과 저장된 영상은 삭제하지 않았어.\n같은 실행을 반복하지 말고 이 코드와 아래 기록을 공유해 줘.\n" + filepath.Join(dir, "updater.log") + "\n\n" + detail)
}
