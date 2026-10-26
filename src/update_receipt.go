package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// The updater writes a receipt only AFTER the relaunched version passes
// its local health/version check. The UI reads it even if the old tab remains.
type UpdateReceipt struct {
	Status  string `json:"status"` // verified | rolled_back
	Version string `json:"version"`
	Detail  string `json:"detail"`
	At      string `json:"at"`
}

func clipCatchDataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	if runtime.GOOS == "windows" && os.Getenv("LOCALAPPDATA") != "" {
		base = os.Getenv("LOCALAPPDATA")
	}
	return filepath.Join(base, runtimeDataName())
}

func writeUpdateReceipt(dir, status, version, detail string) error {
	if status != "verified" && status != "rolled_back" {
		return os.ErrInvalid
	}
	if !validReceiptVersion(version) {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(UpdateReceipt{Status: status, Version: version, Detail: detail, At: time.Now().Format(time.RFC3339)})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "update-receipt.json"), data, 0600)
}

func readUpdateReceipt(dir string) *UpdateReceipt {
	data, err := os.ReadFile(filepath.Join(dir, "update-receipt.json"))
	if err != nil || len(data) > 8192 {
		return nil
	}
	var receipt UpdateReceipt
	if json.Unmarshal(data, &receipt) != nil {
		return nil
	}
	if receipt.Status != "verified" && receipt.Status != "rolled_back" {
		return nil
	}
	if !validReceiptVersion(receipt.Version) {
		return nil
	}
	// Stale receipts should not keep displaying indefinitely.
	at, err := time.Parse(time.RFC3339, receipt.At)
	if err != nil || time.Since(at) > 7*24*time.Hour || time.Since(at) < -5*time.Minute {
		return nil
	}
	return &receipt
}

func validReceiptVersion(v string) bool {
	if isTrialBuild() {
		_, e := parseTrialVersion(v)
		return e == nil
	}
	_, e := parseSemver(v)
	return e == nil
}
