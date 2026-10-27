//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFolderWindowsDirectVisibleExactArgument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "\uD55C\uAE00 folder & (1)")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	cmd, err := folderExplorerCommand(path)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(cmd.Path) || !strings.EqualFold(filepath.Base(cmd.Path), "explorer.exe") || len(cmd.Args) != 2 || cmd.Args[1] != path {
		t.Fatal(cmd)
	}
	if cmd.SysProcAttr != nil || cmd.Cancel != nil {
		t.Fatal("Explorer must not use hidden/cancellable tool startup")
	}
	for _, bad := range []string{"", "bad\x00path", filepath.Join(path, "missing")} {
		if _, err := folderExplorerCommand(bad); err == nil {
			t.Fatal("accepted", bad)
		}
	}
}
