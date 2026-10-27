//go:build windows

package main

import (
 "fmt"
 "os/exec"
 "path/filepath"
 "runtime"
 "syscall"
 "unsafe"
)

var folderWindowsDirectory = syscall.NewLazyDLL("kernel32.dll").NewProc("GetWindowsDirectoryW")

func folderExplorerCommand(path string) (*exec.Cmd, error) {
 full, err := checkedFolderPath(path)
 if err != nil { return nil, err }
 // Use the OS directory, not PATH or a user-writable executable beside the app.
 buf := make([]uint16, 32768)
 n, _, callErr := folderWindowsDirectory.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
 runtime.KeepAlive(buf)
 if n == 0 || n >= uintptr(len(buf)) {
  return nil, fmt.Errorf("GetWindowsDirectoryW failed: %v", callErr)
 }
 explorer := filepath.Join(syscall.UTF16ToString(buf[:n]), "explorer.exe")
 // One Unicode argument. No shell command string, hidden startup flags,
 // app helper, COM scan, focus loop, or permanent topmost window.
 return exec.Command(explorer, full), nil
}

func openDirectoryNative(path string) error {
 cmd, err := folderExplorerCommand(path)
 if err != nil { return err }
 if err = cmd.Start(); err != nil { return fmt.Errorf("Explorer start: %w", err) }
 // Explorer can forward to an existing process or remain running. Its exit
 // code and lifetime are not a folder-open receipt; only reap our handle.
 go func() { _ = cmd.Wait() }()
 return nil
}
