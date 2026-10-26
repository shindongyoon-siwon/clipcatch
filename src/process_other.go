//go:build !windows

package main

import "os/exec"

func hideProcessWindow(cmd *exec.Cmd) {}
