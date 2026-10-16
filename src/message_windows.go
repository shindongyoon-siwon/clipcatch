//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

func showInstallError(message string) {
	title, _ := syscall.UTF16PtrFromString("ClipCatch 업데이트 안내")
	body, _ := syscall.UTF16PtrFromString(message)
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(title)), uintptr(0x30))
}
