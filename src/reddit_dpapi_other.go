//go:build !windows

package main

import "errors"

func protectRedditData([]byte) ([]byte, error) {
	return nil, errors.New("Windows에서만 사용할 수 있어")
}
func unprotectRedditData([]byte) ([]byte, error) {
	return nil, errors.New("Windows에서만 사용할 수 있어")
}
