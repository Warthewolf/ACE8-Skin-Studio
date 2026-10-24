package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every skin of the game can be the start of a new skin: every skin material and every skin icon
// can be moved into a numbered slot.
func TestSweepMaterialSlots(t *testing.T) {
	st, _ := loadGameTables(t)
	failed, done := 0, 0
	for _, r := range st.Rows {
		mat := packagePathOf(r.Material())
		icon := packagePathOf(r.Obj.Str("MenuIconImageRef"))
		for _, p := range []string{mat, icon} {
			if p == "" || !strings.HasPrefix(p, "/Game/") {
				continue
			}
			f := ""
			folder := packageBaseName(parentPath(p))
			for _, c := range []string{filepath.Join(gameExports, strings.TrimPrefix(p, "/Game/")),
				filepath.Join(su57Raw, "Materials", folder, packageBaseName(p)), filepath.Join("/mnt/user-data/uploads/Materials", folder, packageBaseName(p))} {
				if _, err := os.Stat(c + ".uasset"); err == nil && strings.Contains(p, "su57") {
					f = c
					break
				}
			}
			if f == "" {
				continue
			}
			g, err := LoadPackage(f)
			if err != nil {
				t.Errorf("%s: %v", p, err)
				continue
			}
			obj := packageBaseName(p)
			own := "/Game/SkinStudio/Sweep_abcde/s1/" + obj
			ua, ue, err := g.Retarget(own, nil)
			if err != nil {
				t.Errorf("%s: retarget: %v", p, err)
				continue
			}
			q, _ := ParsePackage(ua, ue)
			sl := SlotDef{Key: "s1", Template: r.Obj.Int("SkinID"), Material: own}
			if p == icon {
				sl = SlotDef{Key: "s1", Template: r.Obj.Int("SkinID"), Icon: own}
			}
			dir := t.TempDir()
			im := InstalledSlotMod{Dir: dir, Manifest: &ModManifest{Key: "Sweep_abcde"}}
			writeSlotFiles(t, dir, map[string]*Package{obj: q})
			s := Settings{Project: "Live"}
			gotMat, gotIcon, notes, err := numberedSlotFiles(s, filepath.Join(dir, "out"), &r, im, sl, 777)
			done++
			if err != nil || len(notes) > 0 || (p == mat && !strings.Contains(gotMat, "/777/")) || (p == icon && !strings.HasSuffix(gotIcon, "_777")) {
				failed++
				t.Errorf("%s: %v %v %s %s", p, err, notes, gotMat, gotIcon)
			}
		}
	}
	t.Logf("%d files moved into numbered slots, %d failed", done, failed)
}

func writeSlotFiles(t *testing.T, dir string, pkgs map[string]*Package) {
	src := filepath.Join(dir, "src")
	for name, p := range pkgs {
		os.MkdirAll(filepath.Join(src, "s1"), 0o755)
		os.WriteFile(filepath.Join(src, "s1", name+".uasset"), p.UAsset, 0o644)
		os.WriteFile(filepath.Join(src, "s1", name+".uexp"), p.UExp, 0o644)
	}
	if err := installSlotFiles(src, dir); err != nil {
		t.Fatal(err)
	}
}
