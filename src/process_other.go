//go:build !windows

package main

import "os/exec"

func hideProcessWindow(cmd *exec.Cmd) {}

func configureProcessCancellation(cmd *exec.Cmd) {}
