//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// ttyPipe gives a pseudo-terminal: w goes to the program as its error output, which it takes for a
// terminal, and r reads what it writes there. retoc writes why a file could not be converted only
// when its output is a terminal (see runToolTTY). Reading r ends with an error once the program and
// every other holder of w have closed it.
func ttyPipe() (r, w *os.File, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := m.Fd()
	var unlock int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); e != 0 {
		m.Close()
		return nil, nil, e
	}
	var n uint32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); e != 0 {
		m.Close()
		return nil, nil, e
	}
	// a wide terminal, so progress lines are not cut short
	ws := struct{ Row, Col, X, Y uint16 }{40, 200, 0, 0}
	syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws)))
	s, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		return nil, nil, err
	}
	return m, s, nil
}
