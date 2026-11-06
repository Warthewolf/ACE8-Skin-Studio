package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func elfHeader(machine byte) []byte {
	h := make([]byte, 64)
	copy(h, "\x7fELF")
	h[4], h[5] = 2, 1 // 64-bit, little-endian
	h[18] = machine
	return h
}

func TestProgramKind(t *testing.T) {
	for _, c := range []struct {
		head []byte
		want string
	}{
		{[]byte("MZ\x90\x00"), "windows"},
		{elfHeader(0x3E), "linux"},
		{elfHeader(0xB7), "linux-arm"},
		{elfHeader(0x03), "linux-other"},
		{[]byte("#!/bin/sh\n"), "script"},
		{[]byte{0xCF, 0xFA, 0xED, 0xFE}, "mac"},
		{[]byte("PK\x03\x04"), ""},
	} {
		if got := programKind(c.head); got != c.want {
			t.Errorf("%q: %q, want %q", c.head[:4], got, c.want)
		}
	}
}

// The ways a retoc cannot run on Linux are each explained, and a retoc that is not marked as a
// program is marked as one.
func TestRetocProblemLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	dir := t.TempDir()
	write := func(name string, b []byte, mode os.FileMode) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := retocProblem(write("win", []byte("MZ\x90\x00 rest"), 0o755)); !strings.Contains(p, "Windows version") {
		t.Errorf("Windows retoc: %q", p)
	}
	if runtime.GOARCH == "amd64" {
		if p := retocProblem(write("arm", elfHeader(0xB7), 0o755)); !strings.Contains(p, "ARM") {
			t.Errorf("ARM retoc: %q", p)
		}
	}
	if p := retocProblem(write("zip", []byte("PK\x03\x04 a zip"), 0o644)); !strings.Contains(p, "is not the retoc program") {
		t.Errorf("a zip: %q", p)
	}
	// too old a Linux: the program stops at once with glibc's message
	old := write("old", []byte("#!/bin/sh\necho \"retoc: /lib/x86_64-linux-gnu/libc.so.6: version \\`GLIBC_2.34' not found (required by retoc)\" >&2\nexit 1\n"), 0o644)
	p := retocProblem(old)
	if !strings.Contains(p, "glibc 2.34") || !strings.Contains(p, retocBuildFromSource) {
		t.Errorf("old glibc: %q", p)
	}
	if st, _ := os.Stat(old); st.Mode()&0o100 == 0 {
		t.Error("retoc was not marked as a program")
	}
	// a missing library
	lib := write("lib", []byte("#!/bin/sh\necho 'retoc: error while loading shared libraries: libssl.so.3: cannot open shared object file' >&2\nexit 127\n"), 0o755)
	if p := retocProblem(lib); !strings.Contains(p, "libssl.so.3") {
		t.Errorf("missing library: %q", p)
	}
	// a retoc that runs
	good := write("good", []byte("#!/bin/sh\necho usage\n"), 0o755)
	if p := retocProblem(good); p != "" {
		t.Errorf("working retoc: %q", p)
	}
	// the answer is kept until the file changes
	calls := 0
	retocProbe = func(string) string { calls++; return "" }
	t.Cleanup(func() { retocProbe = probeRetoc })
	again := write("again", []byte("#!/bin/sh\n"), 0o755)
	retocProblem(again)
	retocProblem(again)
	if calls != 1 {
		t.Errorf("probed %d times", calls)
	}
	if p := retocProblem(filepath.Join(dir, "gone")); !strings.Contains(p, "not at") {
		t.Errorf("missing file: %q", p)
	}
}

func TestExplainLinuxFailures(t *testing.T) {
	tail := []string{"retoc: /lib/x86_64-linux-gnu/libc.so.6: version `GLIBC_2.35' not found (required by retoc)"}
	if got := explainToolFailure(tail, false); !strings.Contains(got, "glibc 2.35") {
		t.Errorf("glibc: %q", got)
	}
	if got := explainToolFailure([]string{"bash: ./retoc: cannot execute binary file: Exec format error"}, false); !strings.Contains(got, "not a program for this computer") {
		t.Errorf("exec format: %q", got)
	}
}

// retoc unpacked from its Linux download sits in a folder of its own; it is found there.
func TestFindRetocInReleaseFolder(t *testing.T) {
	old := dataDir
	dataDir = t.TempDir()
	t.Cleanup(func() { dataDir = old })
	sub := filepath.Join(dataDir, "retoc_cli-x86_64-unknown-linux-gnu")
	os.MkdirAll(sub, 0o755)
	want := filepath.Join(sub, exeName)
	os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", "")
	if got := (Settings{}).findRetoc(); got != want {
		t.Fatalf("found %q, want %q", got, want)
	}
	// a folder chosen in Settings is looked into the same way
	other := t.TempDir()
	sub2 := filepath.Join(other, "retoc-x86_64-unknown-linux-gnu")
	os.MkdirAll(sub2, 0o755)
	os.WriteFile(filepath.Join(sub2, exeName), []byte("#!/bin/sh\n"), 0o755)
	if got := (Settings{RetocPath: other}).findRetoc(); got != filepath.Join(sub2, exeName) {
		t.Fatalf("chosen folder: found %q", got)
	}
}

// Oodle goes next to the file retoc really is, which is where retoc looks for it, not next to a
// symbolic link to it.
func TestOodleNextToRealRetoc(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links")
	}
	root := t.TempDir()
	real := filepath.Join(root, "real", exeName)
	link := filepath.Join(root, "bin", exeName)
	os.MkdirAll(filepath.Dir(real), 0o755)
	os.MkdirAll(filepath.Dir(link), 0o755)
	os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755)
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	os.WriteFile(filepath.Join(filepath.Dir(real), oodleName), make([]byte, 200_000), 0o644)
	oldURL := oodleBaseURL
	oodleBaseURL = "http://127.0.0.1:1/" // nothing may be downloaded
	t.Cleanup(func() { oodleBaseURL = oldURL })
	if err := ensureOodle(testJob(), link, ""); err != nil {
		t.Fatalf("Oodle next to the real retoc was not used: %v", err)
	}
}

// On Linux retoc's error output is a terminal in the second try, so it prints its reasons.
func TestTTYPipeIsATerminal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	if _, err := os.Stat("/dev/ptmx"); err != nil {
		t.Skip("no /dev/ptmx")
	}
	var lines []string
	err := runToolTTY(testJob(), "/bin/sh", []string{"-c", "if [ -t 2 ]; then echo terminal >&2; else echo pipe >&2; fi; echo out"}, func(l string) { lines = append(lines, l) })
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(lines, "|")
	if !strings.Contains(got, "terminal") || !strings.Contains(got, "out") {
		t.Fatalf("lines %q", got)
	}
}

// A Windows retoc.exe still chosen in Settings on Linux gives way to a Linux retoc next to the
// program; with no other retoc, it is the one found, so Settings can say what is wrong with it.
func TestFindRetocPrefersOneForThisComputer(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux x86-64 only")
	}
	old := dataDir
	dataDir = t.TempDir()
	t.Cleanup(func() { dataDir = old })
	t.Setenv("PATH", "")
	win := filepath.Join(dataDir, "retoc.exe")
	os.WriteFile(win, []byte("MZ\x90\x00 windows"), 0o644)
	s := Settings{RetocPath: win}
	if got := s.findRetoc(); got != win {
		t.Fatalf("only retoc.exe: found %q", got)
	}
	if c := s.checks()["retoc"]; c.OK || !strings.Contains(c.Note, "Windows version") {
		t.Fatalf("Settings check: %+v", c)
	}
	linux := filepath.Join(dataDir, "retoc")
	os.WriteFile(linux, elfHeader(0x3E), 0o755)
	if got := s.findRetoc(); got != linux {
		t.Fatalf("with a Linux retoc too: found %q", got)
	}
}
