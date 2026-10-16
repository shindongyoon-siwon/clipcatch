//go:build windows

package main

import (
	"errors"
	"syscall"
	"unsafe"
)

type dpapiBlob struct {
	Size uint32
	Data *byte
}

var crypt32 = syscall.NewLazyDLL("crypt32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var dpapiProtect = crypt32.NewProc("CryptProtectData")
var dpapiUnprotect = crypt32.NewProc("CryptUnprotectData")
var dpapiLocalFree = kernel32.NewProc("LocalFree")

const cryptProtectUIForbidden = uintptr(1)

func dpapiCall(proc *syscall.LazyProc, plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return nil, errors.New("비어 있는 인증 정보")
	}
	inp := dpapiBlob{Size: uint32(len(plain)), Data: &plain[0]}
	var out dpapiBlob
	rv, _, sysErr := proc.Call(uintptr(unsafe.Pointer(&inp)), 0, 0, 0, 0, cryptProtectUIForbidden, uintptr(unsafe.Pointer(&out)))
	if rv == 0 {
		return nil, sysErr
	}
	if out.Data == nil || out.Size == 0 {
		return nil, errors.New("암호화 결과가 비어 있어")
	}
	defer dpapiLocalFree.Call(uintptr(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...), nil
}
func protectRedditData(plain []byte) ([]byte, error)    { return dpapiCall(dpapiProtect, plain) }
func unprotectRedditData(cipher []byte) ([]byte, error) { return dpapiCall(dpapiUnprotect, cipher) }
