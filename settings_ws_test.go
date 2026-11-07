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

func TestWriteWhole(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "f")
	os.WriteFile(p, []byte("old"), 0o644)
	if err := writeWhole(p, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "new" || fileExists(p+".part") {
		t.Fatalf("written: %q", b)
	}
	// it cannot be put in place (a folder is there): nothing is left behind
	dir := filepath.Join(d, "dir")
	os.MkdirAll(filepath.Join(dir, "x"), 0o755)
	if err := writeWhole(dir, []byte("x"), 0o644); err == nil || fileExists(dir+".part") {
		t.Fatalf("over a folder: %v", err)
	}
}

// A settings.json with a mistake in it is kept, not written over, and the player is told.
func TestBrokenSettingsKept(t *testing.T) {
	old, oldSet := dataDir, settings
	dataDir = t.TempDir()
	t.Cleanup(func() { dataDir, settings, settingsProblem = old, oldSet, "" })
	p := filepath.Join(dataDir, "settings.json")
	broken := []byte(`{"aesKey":"0x` + strings.Repeat("1", 64) + `","paksDir":"D:\Games\ACE COMBAT 8\Game\Content\Paks"}`)
	os.WriteFile(p, broken, 0o644)
	loadSettings()
	s := getSettings()
	if s.AESKey != "" || settingsNote() == "" {
		t.Fatalf("broken file: key %q, note %q", s.AESKey, settingsNote())
	}
	kept, _ := filepath.Glob(filepath.Join(dataDir, "settings.broken*.json"))
	if len(kept) != 1 {
		t.Fatalf("copies kept: %v", kept)
	}
	if b, _ := os.ReadFile(kept[0]); string(b) != string(broken) {
		t.Fatal("the copy is not the file")
	}
	c := s.checks()
	if c["aes"].OK || !strings.Contains(c["aes"].Note, kept[0]) || !strings.Contains(c["settings"].Note, kept[0]) {
		t.Fatalf("not told: %q", c["aes"].Note)
	}
	// started again before anything was saved: the same file is not copied again
	loadSettings()
	if again, _ := filepath.Glob(filepath.Join(dataDir, "settings.broken*.json")); len(again) != 1 {
		t.Fatalf("copies kept: %v", again)
	}
	// saving writes a new settings.json in one piece; the copy stays
	s.Theme = "light"
	if err := saveSettings(s); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(kept[0]); string(b) != string(broken) {
		t.Fatal("the copy was changed")
	}
	if fileExists(p + ".part") {
		t.Fatal("the new file was left beside settings.json")
	}
	loadSettings()
	if getSettings().Theme != "light" || settingsNote() != "" {
		t.Fatalf("saved settings not read back: %q %q", getSettings().Theme, settingsNote())
	}
	// an empty file (a save cut short by an older version) starts fresh and is said to be empty
	os.WriteFile(p, nil, 0o644)
	loadSettings()
	if !strings.Contains(settingsNote(), "empty") {
		t.Fatalf("empty file: %q", settingsNote())
	}
	// one value of the wrong kind: the rest is still read, and a copy is kept
	key := "0x" + strings.Repeat("2", 64)
	os.WriteFile(p, []byte(`{"aesKey":"`+key+`","theme":5}`), 0o644)
	loadSettings()
	if getSettings().AESKey != key || !strings.Contains(settingsNote(), "A setting") {
		t.Fatalf("wrong kind of value: key %q, note %q", getSettings().AESKey, settingsNote())
	}
	if kept, _ := filepath.Glob(filepath.Join(dataDir, "settings.broken*.json")); len(kept) < 1 {
		t.Fatal("no copy kept")
	}
}
