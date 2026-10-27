//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestUpdateWindowsExclusiveLockAndLaunchGate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	target := installedExePath()
	os.MkdirAll(filepath.Dir(target), 0700)
	release, e := lockUpdateTarget(target)
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	if _, e = lockUpdateTarget(target); !errors.Is(e, errUpdateBusy) {
		t.Fatal("duplicate lock accepted", e)
	}
	t.Setenv(updateRestartEnv, "forged")
	if !updateLaunchGate() {
		t.Fatal("launch did not stop during transaction")
	}
	release()
	release()
	if updateLaunchGate() {
		t.Fatal("stale file prevents next launch")
	}
}
func TestUpdateWindowsLockedOldBackupDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ClipCatch.exe")
	source := filepath.Join(dir, "new.exe")
	updateTestFile(t, target, "current")
	updateTestFile(t, source, "new")
	updateTestFile(t, target+".backup", "locked old backup")
	path, _ := syscall.UTF16PtrFromString(target + ".backup")
	h, e := syscall.CreateFile(path, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer syscall.CloseHandle(h)
	if e = os.Rename(target, target+".backup"); e == nil {
		t.Fatal("old failure was not reproduced")
	}
	staged, e := stageUpdateFile(source, target)
	if e != nil {
		t.Fatal(e)
	}
	swap, e := startUpdateSwap(staged, target, "native-lock")
	if e != nil {
		t.Fatal(e)
	}
	swap.keepPrevious()
	updateTestContents(t, target, "new")
	updateTestContents(t, target+".backup", "locked old backup")
	updateTestContents(t, swap.Backup, "current")
	t.Log("Reproduced old destination-lock failure; new transaction succeeds without deleting that backup")
}

// A test-only process with a deliberately late exit. Not included in the app.
func TestUpdateNativeChild(t *testing.T) {
	if os.Getenv("CLIPCATCH_UPDATE_TEST_CHILD") != "1" {
		t.Skip("native fixture only")
	}
	if os.Getenv("CLIPCATCH_UPDATE_TEST_LOCK") != "" {
		release, e := lockUpdateTarget(os.Getenv("CLIPCATCH_UPDATE_TEST_LOCK"))
		status := "owned"
		if errors.Is(e, errUpdateBusy) {
			status = "busy"
		} else if e != nil {
			status = e.Error()
		}
		os.WriteFile(os.Getenv("CLIPCATCH_UPDATE_TEST_READY"), []byte(status), 0600)
		if release != nil {
			release()
		}
		os.Exit(0)
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		os.Exit(2)
	}
	server := &http.Server{ReadHeaderTimeout: time.Second}
	mux := http.NewServeMux()
	server.Handler = mux
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"app": runtimeAppID(), "channel": appChannel, "version": appVersion})
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(ClientState{}) })
	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
		go func() { time.Sleep(30 * time.Millisecond); server.Close() }()
	})
	os.WriteFile(os.Getenv("CLIPCATCH_UPDATE_TEST_READY"), []byte("http://"+listener.Addr().String()), 0600)
	server.Serve(listener)
	time.Sleep(700 * time.Millisecond)
	os.Exit(0)
}
func startUpdateNativeFixture(t *testing.T, dir string, extra ...string) (*exec.Cmd, string, string) {
	t.Helper()
	self, _ := os.Executable()
	data, e := os.ReadFile(self)
	if e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(dir, 0700)
	target := filepath.Join(dir, "ClipCatch.exe")
	if e = os.WriteFile(target, data, 0700); e != nil {
		t.Fatal(e)
	}
	ready := filepath.Join(dir, "ready")
	cmd := exec.Command(target, "-test.run=^TestUpdateNativeChild$")
	cmd.Env = append(os.Environ(), "CLIPCATCH_UPDATE_TEST_CHILD=1", "CLIPCATCH_UPDATE_TEST_READY="+ready)
	cmd.Env = append(cmd.Env, extra...)
	hideProcessWindow(cmd)
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait(); time.Sleep(100 * time.Millisecond) })
	end := time.Now().Add(10 * time.Second)
	for time.Now().Before(end) {
		b, e := os.ReadFile(ready)
		if e == nil && len(b) > 0 {
			return cmd, target, string(b)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("fixture did not become ready")
	return nil, "", ""
}
func TestUpdateWindowsProcessPortsAndActualExit(t *testing.T) {
	a, target, base := startUpdateNativeFixture(t, filepath.Join(t.TempDir(), "exact"))
	b, other, _ := startUpdateNativeFixture(t, filepath.Join(t.TempDir(), "unrelated"))
	peers, e := updateProcessPeers(target)
	if e != nil || len(peers) != 1 || peers[0].Base != base || peers[0].PID != uint32(a.Process.Pid) {
		t.Fatal("dynamic port/identity mismatch", peers, e)
	}
	if !strings.HasPrefix(base, "http://127.0.0.1:") {
		t.Fatal(base)
	}
	if e = waitUpdateProcesses(target, 100*time.Millisecond); e == nil {
		t.Fatal("still running process treated as exited")
	}
	if e = stopUpdatePeers(peers); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	if e = waitUpdateProcesses(target, 5*time.Second); e != nil {
		t.Fatal(e)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Fatal("socket shutdown confused with process exit")
	}
	if e = a.Wait(); e != nil {
		t.Fatal(e)
	}
	otherPeers, e := updateProcessPeers(other)
	if e != nil || len(otherPeers) != 1 || otherPeers[0].PID != uint32(b.Process.Pid) {
		t.Fatal("unrelated same-name process was affected", e)
	}
	if e = stopUpdatePeers(otherPeers); e != nil {
		t.Fatal(e)
	}
	if e = waitUpdateProcesses(other, 5*time.Second); e != nil {
		t.Fatal(e)
	}
	b.Wait()
}
func TestUpdateWindowsLockIsCrossProcess(t *testing.T) {
	target := filepath.Join(t.TempDir(), "ClipCatch.exe")
	release, e := lockUpdateTarget(target)
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	c, _, status := startUpdateNativeFixture(t, filepath.Join(t.TempDir(), "competitor"), "CLIPCATCH_UPDATE_TEST_LOCK="+target)
	if e = c.Wait(); e != nil {
		t.Fatal(e)
	}
	if status != "busy" {
		t.Fatal("second process acquired lock", status)
	}
}
func TestUpdateWindowsLockedTargetPreservesOriginal(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ClipCatch.exe")
	source := filepath.Join(dir, "new.exe")
	updateTestFile(t, target, "working original")
	updateTestFile(t, source, "new")
	path, _ := syscall.UTF16PtrFromString(target)
	h, e := syscall.CreateFile(path, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer syscall.CloseHandle(h)
	if _, e = startUpdateSwap(source, target, "blocked"); e == nil {
		t.Fatal("locked destination accepted")
	}
	updateTestContents(t, target, "working original")
	updateTestContents(t, source, "new")
}
