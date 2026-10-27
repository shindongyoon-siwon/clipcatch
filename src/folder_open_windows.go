//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const folderShowNormal = 1 // SW_SHOWNORMAL, not hidden-tool startup flags.
var folderShellExecute = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")
var folderCoInit = syscall.NewLazyDLL("ole32.dll").NewProc("CoInitializeEx")
var folderCoUninit = syscall.NewLazyDLL("ole32.dll").NewProc("CoUninitialize")

func folderWindowsArgs(path string) ([]uint16, []uint16, error) {
	verb, _ := syscall.UTF16FromString("open")
	file, err := syscall.UTF16FromString(path)
	return verb, file, err
}
func folderShellResult(result uintptr) error {
	if result > 32 {
		return nil
	}
	return fmt.Errorf("Windows folder open failed (ShellExecuteW code %d)", result)
}
func openDirectoryNative(path string) error {
	verb, file, err := folderWindowsArgs(path)
	if err != nil {
		return err
	}
	for _, p := range []*syscall.LazyProc{folderShellExecute, folderCoInit, folderCoUninit} {
		if err = p.Find(); err != nil {
			return err
		}
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := folderCoInit.Call(0, 0x2|0x4) // STA and COINIT_DISABLE_OLE1DDE.
	if uint32(hr) > 1 {
		return fmt.Errorf("Windows folder connection failed (HRESULT %08x)", uint32(hr))
	}
	defer folderCoUninit.Call()
	result, _, _ := folderShellExecute.Call(0, uintptr(unsafe.Pointer(&verb[0])), uintptr(unsafe.Pointer(&file[0])), 0, 0, folderShowNormal)
	runtime.KeepAlive(verb)
	runtime.KeepAlive(file)
	return folderShellResult(result)
}
