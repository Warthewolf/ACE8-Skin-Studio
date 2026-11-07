package main

// Checking retoc before it is run, so a retoc that cannot run on this computer is explained in
// plain words instead of failing with "Extracted 0" or a message about the AES key.
//
// What goes wrong, mostly on Linux:
//   - the Windows retoc.exe (or retoc for ARM) chosen on Linux, or the Linux retoc on Windows;
//   - the file is not marked as a program (unpacked from a .zip, or copied from a Windows drive);
//   - this Linux is older than the one retoc was built for: retoc's Linux release is built on
//     Ubuntu 22.04 and needs glibc 2.34 or newer, so on Ubuntu 20.04, Debian 11, Mint 20 and the
//     like it stops at once with "version `GLIBC_2.34' not found";
//   - the file is on a drive that does not allow programs to run (mounted noexec).

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

const retocReleases = "https://github.com/trumank/retoc/releases"

// retocLinuxAsset is the file to download from retoc's releases for this computer.
func retocLinuxAsset() string {
	if runtime.GOARCH == "arm64" {
		return "retoc_cli-aarch64-unknown-linux-gnu.tar.xz"
	}
	return "retoc_cli-x86_64-unknown-linux-gnu.tar.xz"
}

// retocBuildFromSource is how to get a retoc made for this very Linux.
const retocBuildFromSource = "cargo install --git https://github.com/trumank/retoc retoc_cli"

type retocCheckKey struct {
	path string
	size int64
	mod  time.Time
}

var (
	retocChecked   = map[retocCheckKey]string{}
	retocCheckedMu sync.Mutex
	// retocProbe runs retoc once to see that it starts (tests replace it).
	retocProbe = probeRetoc
)

// retocProblem says in plain words why the retoc at path cannot run here ("" when it looks fine).
// On Linux a retoc that is not marked as a program is marked as one. The answer is kept until the
// file changes, so this is cheap to ask often.
func retocProblem(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Sprintf("retoc is not at %s any more. Choose it again in Settings.", path)
	}
	key := retocCheckKey{path, st.Size(), st.ModTime()}
	retocCheckedMu.Lock()
	p, ok := retocChecked[key]
	retocCheckedMu.Unlock()
	if ok {
		return p
	}
	p = checkRetocFile(path, st)
	retocCheckedMu.Lock()
	retocChecked[key] = p
	retocCheckedMu.Unlock()
	if p != "" {
		logf("retoc check: %s", p)
	}
	return p
}

func checkRetocFile(path string, st os.FileInfo) string {
	head := make([]byte, 64)
	f, err := os.Open(path)
	if err != nil {
		return fmt.Sprintf("retoc (%s) could not be opened (%v).", path, err)
	}
	n, _ := f.Read(head)
	f.Close()
	head = head[:n]
	kind := programKind(head)
	if runtime.GOOS == "windows" {
		if kind == "linux" || kind == "linux-arm" {
			return "The retoc chosen in Settings is the Linux version. Windows needs retoc.exe, from " + retocReleases + " (retoc_cli-x86_64-pc-windows-msvc.zip)."
		}
		return ""
	}
	switch kind {
	case "windows":
		return "The retoc chosen is the Windows version (retoc.exe), which does not run on Linux. Download " + retocLinuxAsset() + " from " + retocReleases + ", unpack it, and put the file retoc next to this program (or choose it in Settings)."
	case "linux-arm":
		if runtime.GOARCH != "arm64" {
			return "The retoc chosen is made for ARM computers, not this one. Download " + retocLinuxAsset() + " from " + retocReleases + " instead."
		}
	case "linux":
		if runtime.GOARCH == "arm64" {
			return "The retoc chosen is made for x86-64 computers, not this one. Download " + retocLinuxAsset() + " from " + retocReleases + " instead."
		}
	case "mac":
		return "The retoc chosen is the macOS version. Download " + retocLinuxAsset() + " from " + retocReleases + " instead."
	case "linux-other":
		return "The retoc chosen is made for another kind of computer. Download " + retocLinuxAsset() + " from " + retocReleases + " instead."
	case "script":
	default:
		return fmt.Sprintf("%s is not the retoc program. Download %s from %s, unpack it, and choose the file retoc in it.", path, retocLinuxAsset(), retocReleases)
	}
	// marked as a program?
	if st.Mode()&0o111 == 0 {
		if err := os.Chmod(path, st.Mode()|0o755); err != nil {
			return fmt.Sprintf("retoc is not marked as a program, and it could not be marked (%v). In a terminal run: chmod +x \"%s\"", err, path)
		}
		logf("marked %s as a program", path)
	}
	return retocProbe(path)
}

// retocSuitsHere tells from a file's first bytes whether it can be a retoc for this computer.
func retocSuitsHere(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	head := make([]byte, 64)
	n, _ := f.Read(head)
	f.Close()
	switch programKind(head[:n]) {
	case "windows":
		return runtime.GOOS == "windows"
	case "linux":
		return runtime.GOOS == "linux" && runtime.GOARCH == "amd64"
	case "linux-arm":
		return runtime.GOOS == "linux" && runtime.GOARCH == "arm64"
	case "mac":
		return runtime.GOOS == "darwin"
	case "script":
		return runtime.GOOS != "windows"
	}
	return false
}

// programKind tells what a file is from its first bytes: windows, linux, linux-arm, mac, script or "".
func programKind(head []byte) string {
	switch {
	case bytes.HasPrefix(head, []byte("MZ")):
		return "windows"
	case bytes.HasPrefix(head, []byte("#!")):
		return "script"
	case bytes.HasPrefix(head, []byte{0xCF, 0xFA, 0xED, 0xFE}), bytes.HasPrefix(head, []byte{0xCA, 0xFE, 0xBA, 0xBE}):
		return "mac"
	case bytes.HasPrefix(head, []byte("\x7fELF")) && len(head) >= 20:
		var m uint16
		if head[5] == 2 { // big-endian
			m = binary.BigEndian.Uint16(head[18:])
		} else {
			m = binary.LittleEndian.Uint16(head[18:])
		}
		switch m {
		case 0xB7: // AArch64
			return "linux-arm"
		case 0x3E: // x86-64
			return "linux"
		}
		return "linux-other"
	}
	return ""
}

// probeRetoc runs "retoc --help" once and explains the ways retoc cannot start on this Linux.
func probeRetoc(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--help")
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err == nil || ctx.Err() != nil {
		return "" // it ran (or is merely slow); the real task says more if something is wrong
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return retocStartProblem(path, err)
	}
	if p := retocLinuxProblem(string(out)); p != "" {
		return p
	}
	return ""
}

var reGlibc = regexp.MustCompile("GLIBC_([0-9.]+)'? not found")

// retocLinuxProblem explains output that means retoc could not start on this Linux ("" for other
// output).
func retocLinuxProblem(out string) string {
	if m := reGlibc.FindStringSubmatch(out); m != nil {
		have := ""
		if v := glibcVersion(); v != "" {
			have = " (this one has " + v + ")"
		}
		return "This retoc is made for a newer Linux than this one: it needs glibc " + m[1] + " or newer" + have +
			". The ready-made retoc is built on Ubuntu 22.04, so it runs on Ubuntu 22.04, Debian 12, Fedora 36, Linux Mint 21 and newer. On an older Linux, make a retoc for it with Rust (rustup.rs): " +
			retocBuildFromSource + ", then choose ~/.cargo/bin/retoc in Settings."
	}
	if i := strings.Index(out, "error while loading shared libraries:"); i >= 0 {
		line := strings.TrimSpace(strings.SplitN(out[i:], "\n", 2)[0])
		return "retoc could not start because a library it needs is missing on this computer (" + line + "). Install it with your system's package manager, or make a retoc for this Linux with Rust: " + retocBuildFromSource + "."
	}
	return ""
}

// glibcVersion is this computer's glibc version, read from ldd ("" when unknown).
func glibcVersion() string {
	out, err := exec.Command("ldd", "--version").CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	if m := regexp.MustCompile(`(?i)(?:glibc|gnu libc)[^0-9]*?(\d+\.\d+)`).FindStringSubmatch(string(out)); m != nil {
		return "glibc " + m[1]
	}
	if m := regexp.MustCompile(`\b(2\.\d+)\b`).FindStringSubmatch(strings.SplitN(string(out), "\n", 2)[0]); m != nil {
		return "glibc " + m[1]
	}
	return ""
}

// retocStartProblem explains why retoc could not be started at all.
func retocStartProblem(path string, err error) string {
	switch {
	case errors.Is(err, syscall.ENOEXEC) || strings.Contains(err.Error(), "exec format error"):
		if runtime.GOOS == "windows" {
			return "retoc.exe could not be started: it is not a Windows program. Download retoc_cli-x86_64-pc-windows-msvc.zip from " + retocReleases + "."
		}
		return "retoc could not be started: it is not a program for this computer. Download " + retocLinuxAsset() + " from " + retocReleases + " and choose the file retoc in it."
	case errors.Is(err, os.ErrPermission):
		if runtime.GOOS == "windows" {
			return "Windows refused to start retoc.exe (" + err.Error() + "). Check that your antivirus is not blocking it."
		}
		return "Linux refused to start retoc (" + err.Error() + "). If it is on a drive that does not allow programs (a USB stick, an SD card, or a drive mounted with noexec), move the ACE8SkinStudio folder into your home folder. Otherwise run: chmod +x \"" + path + "\""
	case errors.Is(err, os.ErrNotExist):
		return "retoc could not be started: " + path + " is not there any more. Choose it again in Settings."
	}
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("%s could not be started (%v). Check that the file is still there; antivirus programs sometimes remove or block it.", exeBase(path), err)
	}
	return fmt.Sprintf("retoc could not be started (%v).", err)
}

// retocCandidates are the places retoc is looked for in a folder: the folder itself, and the
// folders its downloads unpack to (retoc, retoc_cli-x86_64-unknown-linux-gnu, ...).
func retocCandidates(dir string) []string {
	out := []string{filepath.Join(dir, exeName), filepath.Join(dir, "retoc", exeName)}
	for _, pattern := range []string{"retoc_cli-*", "retoc-*"} {
		if ms, _ := filepath.Glob(filepath.Join(dir, pattern, exeName)); len(ms) > 0 {
			out = append(out, ms...)
		}
	}
	return out
}

// retocMissingNote is the Settings note when no retoc is found.
func retocMissingNote() string {
	if runtime.GOOS != "windows" {
		if _, err := os.Stat(filepath.Join(dataDir, "retoc.exe")); err == nil {
			return "retoc.exe next to this program is the Windows version, which does not run on Linux. The Linux package of Skin Studio comes with retoc for Linux: get it again, or download " + retocLinuxAsset() + " from " + retocReleases + ", unpack it, and put the file retoc next to this program."
		}
		return "retoc was not found. It comes with Skin Studio, next to the program: get the Linux package again, or download " + retocLinuxAsset() + " from " + retocReleases + ", unpack it, and put the file retoc next to this program (or choose it as the retoc program in Settings)."
	}
	return "retoc was not found. It comes with Skin Studio, next to the program: get the package again (antivirus programs sometimes remove it), or download it from " + retocReleases + " and choose " + exeName + " as the retoc program in Settings."
}
