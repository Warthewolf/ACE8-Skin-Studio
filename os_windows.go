//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func findEdge() string {
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
		if base := os.Getenv(env); base != "" {
			p := filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe")
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// openURL shows the program's window: an Edge "app" window when possible, otherwise the default browser.
func openURL(url string, appWindow bool) error {
	if appWindow {
		if edge := findEdge(); edge != "" {
			cmd := exec.Command(edge, "--app="+url, "--window-size=1280,900")
			if err := cmd.Start(); err == nil {
				go cmd.Wait()
				return nil
			}
		}
	}
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	hideWindow(cmd)
	return cmd.Start()
}

// revealPath opens a folder in Explorer, or selects a file inside its folder.
func revealPath(p string) error {
	st, err := os.Stat(p)
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	if st.IsDir() {
		cmd = exec.Command("explorer.exe", p)
	} else {
		cmd = exec.Command("explorer.exe", "/select,", p)
	}
	return cmd.Start() // Explorer returns a non-zero exit code even when it works
}

const canPick = true

// The game's folders are found in Settings on Windows; these are for Proton (see os_other.go).
func platformSaveGamesDir() string { return "" }
func platformPaksDir() string      { return "" }

const exeName = "retoc.exe"

// findRunningGame names the game's program when it is running ("" when not).
func findRunningGame() string {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(snap)
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		if name := syscall.UTF16ToString(e.ExeFile[:]); isGameExe(name) {
			return name
		}
	}
	return ""
}

var (
	procCreateNamedPipeW = syscall.NewLazyDLL("kernel32.dll").NewProc("CreateNamedPipeW")
	ttyPipeSeq           uint32
)

// ttyPipe gives a pipe that console programs take for a terminal: its name marks it as an MSYS
// pseudo-terminal, which the console library retoc uses checks for. retoc prints why it could not
// convert a file through its progress display, which it leaves out when its output is not a
// terminal; written to this pipe, it is kept. w goes to the program; r is read here.
func ttyPipe() (r, w *os.File, err error) {
	seq := atomic.AddUint32(&ttyPipeSeq, 1)
	name := fmt.Sprintf(`\\.\pipe\msys-acestudio%d-%d-pty0-to-master`, os.Getpid(), seq)
	n16, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, nil, err
	}
	const pipeAccessInbound, pipeTypeByte = 0x1, 0x0
	h, _, e := procCreateNamedPipeW.Call(uintptr(unsafe.Pointer(n16)), pipeAccessInbound, pipeTypeByte, 1, 1<<16, 1<<16, 0, 0)
	if syscall.Handle(h) == syscall.InvalidHandle {
		return nil, nil, e
	}
	c, err := syscall.CreateFile(n16, syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		syscall.CloseHandle(syscall.Handle(h))
		return nil, nil, err
	}
	return os.NewFile(h, name), os.NewFile(uintptr(c), name), nil
}

// Oodle for retoc (see oodle.go).
const (
	oodleName         = "oo2core_9_win64.dll"
	oodleGameName     = "oo2core_9_win64.dll"
	oodlePlatformPath = "win/redist/oo2core_9_win64.dll"
	oodleSHA256       = "6f5d41a7892ea6b2db420f2458dad2f84a63901c9a93ce9497337b16c195f457"
)
