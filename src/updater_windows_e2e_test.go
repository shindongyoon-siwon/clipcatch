//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This exists only in the Go test executable, never the distributed app.
func TestMain(m *testing.M) {
	if os.Getenv("CLIPCATCH_E2E_UPDATER") == "1" {
		appChannel = "test"
		appVersion = "1.9.1-test.5"
		if os.Getenv("CLIPCATCH_E2E_STABLE") == "1" {
			appChannel = "stable"
			appVersion = "1.9.2"
		}
		self, _ := os.Executable()
		if !strings.EqualFold(filepath.Clean(self), filepath.Clean(installedExePath())) {
			applyClipCatchUpdate([]string{installedExePath(), appVersion})
			os.Exit(0)
		}
		data, _ := os.ReadFile(self)
		if strings.HasSuffix(string(data), "TEST2_OLD_EXECUTABLE") {
			appVersion = "1.9.1-test.3"
			if os.Getenv("CLIPCATCH_E2E_STABLE") == "1" {
				appVersion = "1.9.1"
			}
		} else if os.Getenv("CLIPCATCH_E2E_BAD_NEW") == "1" {
			appVersion = "0.0.0-test.0"
			if os.Getenv("CLIPCATCH_E2E_STABLE") == "1" {
				appVersion = "0.0.0"
			}
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{"app": runtimeAppID(), "version": appVersion})
		})
		mux.HandleFunc("/fixture-stop", func(w http.ResponseWriter, r *http.Request) {
			go func() { time.Sleep(100 * time.Millisecond); os.Exit(0) }()
		})
		s := http.Server{Addr: "127.0.0.1:" + runtimePort(), Handler: mux, ReadHeaderTimeout: time.Second}
		_ = s.ListenAndServe()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestWindowsTEST2UpdateAndRollback(t *testing.T) {
	for _, bad := range []bool{false, true} {
		t.Run(map[bool]string{false: "healthy", true: "rollback"}[bad], func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "Programs", "ClipCatch-Test2", trialAssetName)
			os.MkdirAll(filepath.Dir(target), 0700)
			self, _ := os.Executable()
			data, e := os.ReadFile(self)
			if e != nil {
				t.Fatal(e)
			}
			original := append(append([]byte{}, data...), []byte("TEST2_OLD_EXECUTABLE")...)
			if e = os.WriteFile(target, original, 0700); e != nil {
				t.Fatal(e)
			}
			stable := filepath.Join(root, "Programs", "ClipCatch", "ClipCatch.exe")
			os.MkdirAll(filepath.Dir(stable), 0700)
			os.WriteFile(stable, []byte("DO NOT TOUCH PRODUCTION"), 0600)
			dd := filepath.Join(root, "ClipCatch-Test2")
			os.MkdirAll(dd, 0700)
			history := filepath.Join(dd, "history.json")
			os.WriteFile(history, []byte("KEEP HISTORY"), 0600)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, self)
			cmd.Env = append(os.Environ(), "CLIPCATCH_E2E_UPDATER=1", "LOCALAPPDATA="+root)
			if bad {
				cmd.Env = append(cmd.Env, "CLIPCATCH_E2E_BAD_NEW=1")
			}
			hideProcessWindow(cmd)
			if out, e := cmd.CombinedOutput(); e != nil {
				t.Fatalf("helper failed %v %s", e, out)
			}
			// The rollback launch is asynchronous. Wait for the restored
			// program to serve health before trying to stop it during cleanup.
			// This also verifies rollback restarts the old program, not only
			// that its executable bytes were restored.
			defer stopUpdateFixture(t)
			expectedVersion := "1.9.1-test.5"
			if bad {
				expectedVersion = "1.9.1-test.3"
			}
			if e := waitUpdateFixture(expectedVersion, 10*time.Second); e != nil {
				t.Fatal(e)
			}

			raw, e := os.ReadFile(filepath.Join(dd, "update-receipt.json"))
			if e != nil {
				t.Fatal(e)
			}
			var r UpdateReceipt
			if e = json.Unmarshal(raw, &r); e != nil {
				t.Fatal(e)
			}
			want := "verified"
			if bad {
				want = "rolled_back"
			}
			if r.Status != want {
				t.Fatalf("receipt %s want %s", r.Status, want)
			}
			out, e := os.ReadFile(target)
			if e != nil {
				t.Fatal(e)
			}
			if bad && !strings.HasSuffix(string(out), "TEST2_OLD_EXECUTABLE") {
				t.Fatal("old binary not restored")
			}
			if !bad && len(out) != len(data) {
				t.Fatal("new binary not copied")
			}
			saved, _ := os.ReadFile(stable)
			if string(saved) != "DO NOT TOUCH PRODUCTION" {
				t.Fatal("production changed")
			}
			saved, _ = os.ReadFile(history)
			if string(saved) != "KEEP HISTORY" {
				t.Fatal("history changed")
			}
		})
	}
}

func waitUpdateFixture(version string, timeout time.Duration) error {
	client := http.Client{Timeout: 300 * time.Millisecond}
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		resp, e := client.Get("http://127.0.0.1:18766/api/health")
		if e == nil {
			var health map[string]string
			e = json.NewDecoder(resp.Body).Decode(&health)
			resp.Body.Close()
			if e == nil && health["app"] == "clipcatch-test2" && health["version"] == version {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("test update executable did not start with version %s", version)
}
func stopUpdateFixture(t *testing.T) {
	t.Helper()
	client := http.Client{Timeout: 300 * time.Millisecond}
	end := time.Now().Add(10 * time.Second)
	requested := false
	for time.Now().Before(end) {
		resp, e := client.Get("http://127.0.0.1:18766/fixture-stop")
		if e == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			requested = true
		} else if requested {
			// The socket closes slightly before Windows releases the image.
			time.Sleep(500 * time.Millisecond)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Error("test-only update fixture did not shut down")
}

func TestWindowsStableUpdateAndRollback(t *testing.T) {
	for _, bad := range []bool{false, true} {
		t.Run(fmt.Sprint(bad), func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "Programs", "ClipCatch", "ClipCatch.exe")
			os.MkdirAll(filepath.Dir(target), 0700)
			self, _ := os.Executable()
			data, e := os.ReadFile(self)
			if e != nil {
				t.Fatal(e)
			}
			original := append(append([]byte{}, data...), []byte("TEST2_OLD_EXECUTABLE")...)
			os.WriteFile(target, original, 0700)
			trial := filepath.Join(root, "Programs", "ClipCatch-Test2", trialAssetName)
			os.MkdirAll(filepath.Dir(trial), 0700)
			os.WriteFile(trial, []byte("KEEP TEST2"), 0600)
			dd := filepath.Join(root, "ClipCatch")
			os.MkdirAll(dd, 0700)
			os.WriteFile(filepath.Join(dd, "history.json"), []byte("KEEP HISTORY"), 0600)
			ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, self)
			cmd.Env = append(os.Environ(), "CLIPCATCH_E2E_UPDATER=1", "CLIPCATCH_E2E_STABLE=1", "LOCALAPPDATA="+root)
			if bad {
				cmd.Env = append(cmd.Env, "CLIPCATCH_E2E_BAD_NEW=1")
			}
			hideProcessWindow(cmd)
			if out, e := cmd.CombinedOutput(); e != nil {
				t.Fatalf("%v %s", e, out)
			}
			defer func() {
				client := http.Client{Timeout: time.Second}
				for i := 0; i < 40; i++ {
					resp, e := client.Get("http://127.0.0.1:8765/fixture-stop")
					if e != nil {
						time.Sleep(500 * time.Millisecond)
						return
					}
					resp.Body.Close()
					time.Sleep(250 * time.Millisecond)
				}
				t.Error("fixture did not stop")
			}()
			version := "1.9.2"
			status := "verified"
			if bad {
				version = "1.9.1"
				status = "rolled_back"
			}
			client := http.Client{Timeout: time.Second}
			healthy := false
			for i := 0; i < 40; i++ {
				resp, e := client.Get("http://127.0.0.1:8765/api/health")
				if e == nil {
					var h map[string]string
					json.NewDecoder(resp.Body).Decode(&h)
					resp.Body.Close()
					if h["app"] == "clipcatch" && h["version"] == version {
						healthy = true
						break
					}
				}
				time.Sleep(250 * time.Millisecond)
			}
			if !healthy {
				t.Fatal("stable updater health mismatch")
			}
			raw, e := os.ReadFile(filepath.Join(dd, "update-receipt.json"))
			if e != nil {
				t.Fatal(e)
			}
			var receipt UpdateReceipt
			json.Unmarshal(raw, &receipt)
			if receipt.Status != status {
				t.Fatal(receipt)
			}
			kept, _ := os.ReadFile(trial)
			if string(kept) != "KEEP TEST2" {
				t.Fatal("stable update modified trial")
			}
			kept, _ = os.ReadFile(filepath.Join(dd, "history.json"))
			if string(kept) != "KEEP HISTORY" {
				t.Fatal("history changed")
			}
		})
	}
}
