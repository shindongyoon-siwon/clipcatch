//go:build !windows

package main

import "time"

func lockUpdateTarget(string) (func(), error)         { return func() {}, nil }
func updateProcessPeers(string) ([]updatePeer, error) { return nil, nil }
func waitUpdateProcesses(string, time.Duration) error { return nil }
func updatePIDExited(uint32) bool                     { return false }
