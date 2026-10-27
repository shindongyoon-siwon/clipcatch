package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

type focusFixture struct {
	target, front                uintptr
	stamp                        uint32
	mini, allow, valid           bool
	opens, restores, activations int
	onFind, onSleep              func()
}

func (f *focusFixture) ops() folderFocusOps {
	return folderFocusOps{
		Find: func() uintptr {
			if f.onFind != nil {
				f.onFind()
			}
			return f.target
		}, Open: func() error { f.opens++; f.target = 42; f.valid = true; return nil },
		Foreground: func() uintptr { return f.front }, InputStamp: func() uint32 { return f.stamp }, Minimized: func(uintptr) bool { return f.mini },
		Restore: func(uintptr) { f.restores++; f.mini = false }, Activate: func(h uintptr) {
			f.activations++
			if f.allow {
				f.front = h
			}
		},
		StillTarget: func(h uintptr) bool { return f.valid && h == f.target }, Sleep: func(time.Duration) {
			if f.onSleep != nil {
				f.onSleep()
			}
		},
	}
}
func TestFolderFocusRestoresOnlyExactExistingWindow(t *testing.T) {
	f := focusFixture{target: 42, front: 11, mini: true, allow: true, valid: true}
	r, e := requestFolderFocus(context.Background(), f.ops(), 3)
	if e != nil || !r.Foreground || !r.Restored || !r.Reused || f.opens != 0 || f.restores != 1 || f.activations != 1 {
		t.Fatal(r, e, f)
	}
}
func TestFolderFocusNewWindowAndPolicyDenial(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		f := focusFixture{front: 11, allow: allowed}
		r, e := requestFolderFocus(context.Background(), f.ops(), 3)
		if e != nil || !r.Requested || r.Foreground != allowed || r.Reused || f.opens != 1 || f.activations != 1 {
			t.Fatal(r, e, f)
		}
		if !allowed && r.Reason != "focus_denied" {
			t.Fatal("false success", r)
		}
	}
}
func TestFolderFocusDoesNotStealAfterUserSwitchOrTyping(t *testing.T) {
	for _, typing := range []bool{false, true} {
		f := focusFixture{target: 42, front: 11, allow: true, valid: true}
		f.onFind = func() {
			if typing {
				f.stamp++
			} else {
				f.front = 99
			}
		}
		r, e := requestFolderFocus(context.Background(), f.ops(), 3)
		if e != nil || r.Foreground || r.Reason != "user_switched" || f.restores != 0 || f.activations != 0 {
			t.Fatal(r, e, f)
		}
	}
}
func TestFolderFocusRechecksAfterAsyncRestore(t *testing.T) {
	f := focusFixture{target: 42, front: 11, mini: true, allow: true, valid: true}
	f.onSleep = func() { f.front = 99 }
	r, e := requestFolderFocus(context.Background(), f.ops(), 3)
	if e != nil || r.Foreground || f.activations != 0 || r.Reason != "user_switched" {
		t.Fatal(r, e, f)
	}
}
func TestFolderFocusNoWindowNoFalseSuccessAndCancelledRequest(t *testing.T) {
	f := focusFixture{front: 11}
	ops := f.ops()
	ops.Find = func() uintptr { return 0 }
	r, e := requestFolderFocus(context.Background(), ops, 3)
	if e != nil || r.Foreground || r.Reason != "window_not_found" || f.activations != 0 {
		t.Fatal(r, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f = focusFixture{front: 11}
	if _, e = requestFolderFocus(ctx, f.ops(), 3); e == nil || f.opens != 0 {
		t.Fatal("cancelled request opened a folder")
	}
	f = focusFixture{front: 11}
	ops = f.ops()
	ops.Open = func() error { return errors.New("shell rejected") }
	r, e = requestFolderFocus(context.Background(), ops, 3)
	if e == nil || r.Requested {
		t.Fatal("shell error hidden", r, e)
	}
}
func TestFolderFocusAPIReportsObservedResult(t *testing.T) {
	for _, front := range []bool{true, false} {
		w := httptest.NewRecorder()
		serveFocusedSaveFolder(w, httptest.NewRequest("POST", "/api/open-folder", nil), t.TempDir(), func(context.Context, string) (folderFocusResult, error) {
			return folderFocusResult{Requested: true, Foreground: front, Reused: true}, nil
		})
		var b map[string]any
		json.Unmarshal(w.Body.Bytes(), &b)
		if w.Code != 200 || b["foreground"] != front || b["reused"] != true {
			t.Fatal(w.Code, b)
		}
	}
}
