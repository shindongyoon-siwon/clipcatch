package main

import (
	"context"
	"time"
)

// A successful shell request is distinct from an observed foreground window.
type folderFocusResult struct {
	Requested  bool   `json:"requested"`
	Foreground bool   `json:"foreground"`
	Restored   bool   `json:"restored"`
	Reused     bool   `json:"reused"`
	Reason     string `json:"focus_reason,omitempty"`
}
type folderFocusOps struct {
	Find        func() uintptr // only an exact-path, genuine Explorer window
	Open        func() error
	Foreground  func() uintptr
	InputStamp  func() uint32
	Minimized   func(uintptr) bool
	Restore     func(uintptr)
	Activate    func(uintptr)
	StillTarget func(uintptr) bool
	Sleep       func(time.Duration)
}

// Bounded, user-initiated activation. Never change system focus settings,
// synthesize input, attach input queues, or keep a window always-on-top.
func requestFolderFocus(ctx context.Context, o folderFocusOps, attempts int) (folderFocusResult, error) {
	r := folderFocusResult{}
	if e := ctx.Err(); e != nil {
		return r, e
	}
	initial, input := o.Foreground(), o.InputStamp()
	target := o.Find()
	r.Reused = target != 0
	if target == 0 {
		if err := o.Open(); err != nil {
			return r, err
		}
	}
	r.Requested = true
	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			r.Reason = "cancelled"
			return r, nil
		}
		if target == 0 {
			target = o.Find()
		}
		if target != 0 && o.StillTarget(target) {
			if o.Foreground() == target {
				r.Foreground = true
				return r, nil
			}
			// A click in another app, or further typing, withdraws this focus request.
			if (o.Foreground() != 0 && o.Foreground() != initial) || o.InputStamp() != input {
				r.Reason = "user_switched"
				return r, nil
			}
			if o.Minimized(target) {
				o.Restore(target)
				o.Sleep(80 * time.Millisecond)
				r.Restored = !o.Minimized(target)
			}
			if ctx.Err() != nil {
				r.Reason = "cancelled"
				return r, nil
			}
			if (o.Foreground() != 0 && o.Foreground() != initial && o.Foreground() != target) || o.InputStamp() != input {
				r.Reason = "user_switched"
				return r, nil
			}
			if !o.StillTarget(target) {
				r.Reason = "window_changed"
				return r, nil
			}
			o.Activate(target)
			for j := 0; j < 4; j++ {
				if o.Foreground() == target && o.StillTarget(target) {
					r.Foreground = true
					return r, nil
				}
				if ctx.Err() != nil {
					r.Reason = "cancelled"
					return r, nil
				}
				if o.Foreground() != 0 && o.Foreground() != initial && o.Foreground() != target {
					break
				}
				o.Sleep(40 * time.Millisecond)
			}
			r.Reason = "focus_denied"
			return r, nil
		}
		target = 0
		if o.InputStamp() != input {
			r.Reason = "user_switched"
			return r, nil
		}
		o.Sleep(100 * time.Millisecond)
	}
	r.Reason = "window_not_found"
	return r, nil
}
