//go:build windows

package main

import (
	"context"
	"testing"
)

func TestChildCommandsHaveNoConsole(t *testing.T) {
	for _, c := range []bool{false, true} {
		cmd := hiddenCommand("cmd.exe", "/C", "exit 0")
		if c {
			cmd = hiddenCommandContext(context.Background(), "cmd.exe", "/C", "exit 0")
		}
		if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&0x08000000 == 0 {
			t.Fatal("visible child console")
		}
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
	}
}
