package main

// SkinSlotText: the small UE4SS mod that gives the game the names and descriptions of new skins.
// It is built into this program and installed into UE4SS's Mods folder when it is missing or older.

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed embed/SkinSlotText
var skinSlotTextFS embed.FS

type sstStatus struct {
	Dir       string `json:"dir"`       // UE4SS's Mods folder ("" when not found)
	Installed bool   `json:"installed"` // SkinSlotText is there
	Current   bool   `json:"current"`   // and it is the version built into this program
	Disabled  bool   `json:"disabled"`  // mods.txt switches it off
	Problem   string `json:"problem,omitempty"`
}

func skinSlotTextStatus(s Settings) sstStatus {
	st := sstStatus{Dir: s.ue4ssModsDir()}
	if st.Dir == "" {
		st.Problem = "UE4SS's Mods folder was not found, so the names and descriptions of new skins cannot be installed. Set it in Settings (it is the folder that holds mods.txt)."
		return st
	}
	dll := filepath.Join(st.Dir, "SkinSlotText", "Scripts", "SkinSlotText.dll")
	have, err := os.ReadFile(dll)
	st.Installed = err == nil
	want, _ := skinSlotTextFS.ReadFile("embed/SkinSlotText/Scripts/SkinSlotText.dll")
	st.Current = st.Installed && bytes.Equal(have, want)
	if b, err := os.ReadFile(filepath.Join(st.Dir, "mods.txt")); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			k, v, ok := strings.Cut(l, ":")
			if ok && strings.EqualFold(strings.TrimSpace(k), "SkinSlotText") && strings.TrimSpace(v) == "0" {
				st.Disabled = true
			}
		}
	}
	switch {
	case !st.Installed:
		st.Problem = "SkinSlotText (the UE4SS mod that shows the names and descriptions of new skins) is not installed yet; it is installed with the first new skin."
	case st.Disabled:
		st.Problem = "SkinSlotText is switched off in UE4SS's mods.txt (SkinSlotText : 0), so new skins show their text keys instead of their names. Change the 0 to 1."
	}
	return st
}

// installSkinSlotText copies the built-in SkinSlotText into UE4SS's Mods folder. The texts folder
// is left alone.
func installSkinSlotText(s Settings) error {
	dir := s.ue4ssModsDir()
	if dir == "" {
		return fmt.Errorf("UE4SS's Mods folder was not found; set it in Settings")
	}
	dest := filepath.Join(dir, "SkinSlotText")
	err := fs.WalkDir(skinSlotTextFS, "embed/SkinSlotText", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, "embed/SkinSlotText"), "/")
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := skinSlotTextFS.ReadFile(p)
		if err != nil {
			return err
		}
		if old, err := os.ReadFile(target); err == nil && bytes.Equal(old, b) {
			return nil
		}
		if err := os.WriteFile(target, b, 0o644); err != nil {
			return fmt.Errorf("could not write %s (%v). If the game is running, close it and try again", filepath.Base(target), err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Join(dest, "Scripts", "texts"), 0o755)
	return nil
}

// skinSlotTextFiles lists the built-in files (for putting into a shared zip): path inside the mod -> bytes.
func skinSlotTextFiles() map[string][]byte {
	out := map[string][]byte{}
	fs.WalkDir(skinSlotTextFS, "embed/SkinSlotText", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := skinSlotTextFS.ReadFile(p)
			out[strings.TrimPrefix(p, "embed/SkinSlotText/")] = b
		}
		return nil
	})
	return out
}
