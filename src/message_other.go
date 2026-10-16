//go:build !windows

package main

import "log"

func showInstallError(message string) { log.Println(message) }
