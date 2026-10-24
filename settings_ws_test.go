package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceProblem(t *testing.T) {
	d := t.TempDir()
	if p := workspaceProblem(filepath.Join(d, "ws")); p != "" {
		t.Fatalf("usable folder reported: %s", p)
	}
	f := filepath.Join(d, "file")
	os.WriteFile(f, []byte("x"), 0o644)
	if p := workspaceProblem(f); !strings.Contains(p, "a file") {
		t.Fatalf("file not reported: %q", p)
	}
	if p := workspaceProblem(filepath.Join(f, "sub")); p == "" {
		t.Fatal("folder under a file reported usable")
	}
	if workspaceProblem("") == "" {
		t.Fatal("empty reported usable")
	}
}
