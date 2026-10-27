package main

import (
	"context"
	"testing"
	"time"
)

func TestFolderFocusWaitsForShellRegistrationWithoutBlamingUser(t *testing.T) {
	f := focusFixture{front: 11, allow: true, valid: true}
	o := f.ops()
	calls := 0
	o.Find = func() uintptr {
		calls++
		if calls < 4 {
			return 0
		}
		return 42
	}
	o.Open = func() error { f.opens++; f.target = 42; f.front = 42; return nil }
	o.Sleep = func(time.Duration) {}
	r, e := requestFolderFocus(context.Background(), o, 5)
	if e != nil || !r.Foreground || r.Reused || f.opens != 1 {
		t.Fatal(r, e, calls)
	}
}
