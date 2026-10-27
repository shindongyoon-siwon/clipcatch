//go:build windows

package main

import (
	"syscall"
	"testing"
	"unsafe"
)

func TestFolderFocusWindowsABIAndExactPath(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 && (unsafe.Sizeof(folderVariant{}) != 24 || unsafe.Alignof(folderVariant{}) != 8 || unsafe.Sizeof(folderDispParams{}) != 24) {
		t.Fatal("VARIANT/DISPPARAMS ABI mismatch")
	}
	if !folderExactPath(`C:\Users\Test\Downloads\ClipCatch`, `c:\users\test\downloads\ClipCatch\`) || folderExactPath(`C:\Downloads\ClipCatch`, `C:\Downloads\ClipCatch Other`) || folderExactPath("", "") {
		t.Fatal("folder path mismatch")
	}
	for _, p := range []*syscall.LazyProc{folderCoCreate, folderVariantClear, folderStringLen, folderSetForeground, folderShowAsync, folderGetForeground, folderGetClass, folderLastInput, folderIsVisible} {
		if e := p.Find(); e != nil {
			t.Fatal(p.Name, e)
		}
	}
	if folderExplorerClass(0) {
		t.Fatal("invalid HWND accepted")
	}
}
