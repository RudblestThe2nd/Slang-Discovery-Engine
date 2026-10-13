//go:build !windows

package main

import "errors"

func seal(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	return "", errors.New("secret storage requires Windows DPAPI")
}
func reveal(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	return "", errors.New("secret storage requires Windows DPAPI")
}
