package main

import (
	"context"
	"os/exec"
	"time"
)

func hiddenCommand(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	hideProcessWindow(c)
	return c
}
func hiddenCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, name, args...)
	hideProcessWindow(c)
	c.WaitDelay = 3 * time.Second
	configureProcessCancellation(c)
	return c
}
