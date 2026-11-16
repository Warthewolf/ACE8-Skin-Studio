package main

import (
	"os"
	"path/filepath"
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
