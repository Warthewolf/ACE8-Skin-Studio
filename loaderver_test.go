package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func utf16Bytes(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return append(b, 0, 0)
}

// The mod loader is found with its version written in any of the usual ways, in the usual folder or
// in a UE4SS mod named after it; a DLL that says no version still counts as found.
func TestLoaderVersion(t *testing.T) {
	root := t.TempDir()
	ue := filepath.Join(root, "ue4ss", "Mods")
	mods := filepath.Join(ue, "IoStoreLoaderMod")
	os.MkdirAll(filepath.Join(mods, "dlls"), 0o755)
	os.WriteFile(filepath.Join(ue, "mods.txt"), []byte("IoStoreLoaderMod : 1\n"), 0o644)
	s := Settings{ModsDir: mods}
	dll := filepath.Join(mods, "dlls", "main.dll")
	if v := loaderVersion(s); v != "" {
		t.Fatalf("no loader: %q", v)
	}
	for _, c := range []struct {
		data []byte
		want string
	}{
		{utf16Bytes("1.3.1"), "1.3.1"},
		{utf16Bytes("1.3.1.0"), "1.3.1"},        // Windows version resources use four parts
		{[]byte("xx\x00v1.2.0\x00yy"), "1.2.0"}, // written as plain text
		{[]byte("MZ no version here"), loaderFound},
	} {
		os.WriteFile(dll, c.data, 0o644)
		if v := loaderVersion(s); v != c.want {
			t.Errorf("%q: %q, want %q", c.data, v, c.want)
		}
	}
	// a loader in a UE4SS mod of its own name, with the mods folder elsewhere
	os.Remove(dll)
	other := filepath.Join(ue, "AC8ModLoader", "dlls")
	os.MkdirAll(other, 0o755)
	os.WriteFile(filepath.Join(other, "main.dll"), utf16Bytes("1.4.0"), 0o644)
	if v := loaderVersion(s); v != "1.4.0" {
		t.Fatalf("loader in its own UE4SS mod: %q", v)
	}
}

// UE4SS's own Mods folder chosen as the mods folder is caught in Settings, which names the loader's
// folder inside it; the loader is still found there.
func TestModsFolderIsUE4SSFolder(t *testing.T) {
	ue := filepath.Join(t.TempDir(), "ue4ss", "Mods")
	loader := filepath.Join(ue, "IoStoreLoaderMod", "dlls")
	os.MkdirAll(loader, 0o755)
	os.WriteFile(filepath.Join(ue, "mods.txt"), []byte("IoStoreLoaderMod : 1\n"), 0o644)
	os.WriteFile(filepath.Join(loader, "main.dll"), utf16Bytes("1.3.1"), 0o644)
	s := Settings{ModsDir: ue}
	c := s.checks()["mods"]
	if c.OK || !strings.Contains(c.Note, filepath.Join(ue, "IoStoreLoaderMod")) {
		t.Fatalf("check: %+v", c)
	}
	if v := loaderVersion(s); v != "1.3.1" {
		t.Fatalf("loader: %q", v)
	}
	if c := (Settings{ModsDir: filepath.Join(ue, "IoStoreLoaderMod")}).checks()["mods"]; !c.OK {
		t.Fatalf("the right folder: %+v", c)
	}
}

// An installed SkinSlotText whose script is older than this program's is not current, and is brought
// up to date by refreshSkinSlotText; one that is not installed stays not installed.
func TestRefreshSkinSlotText(t *testing.T) {
	ue := filepath.Join(t.TempDir(), "ue4ss", "Mods")
	os.MkdirAll(ue, 0o755)
	os.WriteFile(filepath.Join(ue, "mods.txt"), []byte(""), 0o644)
	s := Settings{UE4SSMods: ue}
	refreshSkinSlotText(s)
	if _, err := os.Stat(filepath.Join(ue, "SkinSlotText")); err == nil {
		t.Fatal("installed where it was not")
	}
	if err := installSkinSlotText(s); err != nil {
		t.Fatal(err)
	}
	if st := skinSlotTextStatus(s); !st.Installed || !st.Current {
		t.Fatalf("fresh install: %+v", st)
	}
	lua := filepath.Join(ue, "SkinSlotText", "Scripts", "main.lua")
	os.WriteFile(lua, []byte("-- SkinSlotText 1.2 (old)\\n"), 0o644)
	if st := skinSlotTextStatus(s); st.Current {
		t.Fatal("an old script counts as current")
	}
	refreshSkinSlotText(s)
	if b, _ := os.ReadFile(lua); !strings.Contains(string(b), "1.2.1") {
		t.Fatalf("not updated: %q", b[:40])
	}
}
