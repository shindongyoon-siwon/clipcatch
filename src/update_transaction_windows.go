//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var queryUpdateImage = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")
var updateTCPTable = syscall.NewLazyDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

// Windows sharing mode 0 is an OS-owned cross-process/cross-session lock.
// A crashed updater releases its HANDLE automatically; the small lock file may
// remain and is reused. We never use mere file existence as a lock.
func lockUpdateTarget(target string) (func(), error) {
	p, e := syscall.UTF16PtrFromString(target + ".update-lock")
	if e != nil {
		return nil, e
	}
	h, e := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		if errors.Is(e, syscall.Errno(32)) || errors.Is(e, syscall.Errno(33)) {
			return nil, errUpdateBusy
		}
		return nil, e
	}
	var once sync.Once
	return func() { once.Do(func() { syscall.CloseHandle(h) }) }, nil
}

type updateProcess struct {
	PID    uint32
	Handle syscall.Handle
}

func closeUpdateProcesses(ps []updateProcess) {
	for _, p := range ps {
		syscall.CloseHandle(p.Handle)
	}
}
func exactUpdateProcesses(target string) ([]updateProcess, error) {
	snapshot, e := syscall.CreateToolhelp32Snapshot(2, 0)
	if e != nil {
		return nil, e
	}
	defer syscall.CloseHandle(snapshot)
	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	ps := []updateProcess{}
	for e = syscall.Process32First(snapshot, &entry); e == nil; e = syscall.Process32Next(snapshot, &entry) {
		if entry.ProcessID == uint32(os.Getpid()) {
			continue
		}
		// Query all accessible processes: aliases such as downloaded copies must not
		// lead to a name-only kill. Only the exact installed image path is retained.
		h, err := syscall.OpenProcess(0x1000|0x00100000, false, entry.ProcessID)
		if err != nil {
			if strings.EqualFold(syscall.UTF16ToString(entry.ExeFile[:]), filepath.Base(target)) && !errors.Is(err, syscall.Errno(87)) {
				closeUpdateProcesses(ps)
				return nil, fmt.Errorf("cannot inspect PID %d: %w", entry.ProcessID, err)
			}
			continue
		}
		path := make([]uint16, 32768)
		n := uint32(len(path))
		ok, _, callErr := queryUpdateImage.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&path[0])), uintptr(unsafe.Pointer(&n)))
		if ok != 0 && sameUpdatePath(syscall.UTF16ToString(path[:n]), target) {
			ps = append(ps, updateProcess{PID: entry.ProcessID, Handle: h})
		} else {
			syscall.CloseHandle(h)
			if ok == 0 && strings.EqualFold(syscall.UTF16ToString(entry.ExeFile[:]), filepath.Base(target)) && !errors.Is(callErr, syscall.Errno(87)) {
				closeUpdateProcesses(ps)
				return nil, fmt.Errorf("cannot read image path for PID %d: %v", entry.ProcessID, callErr)
			}
		}
	}
	if e != nil && !errors.Is(e, syscall.ERROR_NO_MORE_FILES) {
		closeUpdateProcesses(ps)
		return nil, e
	}
	return ps, nil
}
func updateProcessPeers(target string) ([]updatePeer, error) {
	ps, e := exactUpdateProcesses(target)
	if e != nil {
		return nil, e
	}
	defer closeUpdateProcesses(ps)
	if len(ps) == 0 {
		return nil, nil
	}
	owned := map[uint32]bool{}
	for _, p := range ps {
		owned[p.PID] = true
	}
	size := uint32(0)
	r, _, _ := updateTCPTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, 2, 3, 0) // IPv4 owner-PID listeners.
	if r != 122 && r != 0 {
		return nil, syscall.Errno(r)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if size < 4 || size > 16<<20 {
			return nil, errors.New("invalid TCP table size")
		}
		buf := make([]byte, size)
		r, _, _ = updateTCPTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, 2, 3, 0)
		if r == 122 {
			continue
		}
		if r != 0 {
			return nil, syscall.Errno(r)
		}
		count := int(binary.LittleEndian.Uint32(buf[:4]))
		if count > (len(buf)-4)/24 {
			return nil, errors.New("truncated TCP table")
		}
		peers := []updatePeer{}
		for i := 0; i < count; i++ {
			row := buf[4+i*24 : 4+(i+1)*24]
			pid := binary.LittleEndian.Uint32(row[20:24])
			if !owned[pid] {
				continue
			}
			if !net.IP(row[4:8]).Equal(net.IPv4(127, 0, 0, 1)) {
				continue
			}
			port := binary.BigEndian.Uint16(row[8:10])
			peers = append(peers, updatePeer{PID: pid, Base: fmt.Sprintf("http://127.0.0.1:%d", port)})
		}
		return peers, nil
	}
	return nil, errors.New("TCP table changed repeatedly")
}
func waitUpdateProcesses(target string, timeout time.Duration) error {
	end := time.Now().Add(timeout)
	for {
		ps, e := exactUpdateProcesses(target)
		if e != nil {
			return e
		}
		if len(ps) == 0 {
			return nil
		}
		for _, p := range ps {
			remaining := time.Until(end)
			if remaining <= 0 {
				closeUpdateProcesses(ps)
				return fmt.Errorf("installed process PID %d did not exit; no forced termination", p.PID)
			}
			ms := remaining / time.Millisecond
			if ms > 250 {
				ms = 250
			}
			_, e = syscall.WaitForSingleObject(p.Handle, uint32(ms))
			if e != nil {
				closeUpdateProcesses(ps)
				return e
			}
		}
		closeUpdateProcesses(ps)
	}
}
func updatePIDExited(pid uint32) bool {
	if pid == 0 {
		return false
	}
	h, e := syscall.OpenProcess(0x00100000, false, pid)
	if e != nil {
		return errors.Is(e, syscall.Errno(87))
	}
	defer syscall.CloseHandle(h)
	result, e := syscall.WaitForSingleObject(h, 0)
	return e == nil && result == syscall.WAIT_OBJECT_0
}
