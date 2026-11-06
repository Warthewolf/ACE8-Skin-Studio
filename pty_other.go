//go:build !windows && !linux

package main

import (
	"errors"
	"os"
)

// ttyPipe is made on Windows (os_windows.go) and Linux (pty_linux.go) only.
func ttyPipe() (r, w *os.File, err error) {
	return nil, nil, errors.New("not available on this system")
}
