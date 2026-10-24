package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gameExports = "/mnt/user-data/uploads/AceComba8EXPORTS/Live/Content"
const su57Raw = "/mnt/user-data/uploads/PP0021_su57"

func needFile(t *testing.T, p string) {
	t.Helper()
	if _, err := os.Stat(p); err != nil {
		t.Skip("game file not on this machine: " + p)
	}
}

func realPackages(t *testing.T) []string {
	var out []string
	for _, root := range []string{gameExports, su57Raw} {
		filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err == nil && strings.HasSuffix(p, ".uasset") {
				if _, e := os.Stat(strings.TrimSuffix(p, ".uasset") + ".uexp"); e == nil {
					out = append(out, strings.TrimSuffix(p, ".uasset"))
				}
			}
			return nil
		})
	}
	if len(out) == 0 {
		t.Skip("no game packages on this machine")
	}
	return out
}

func TestPackageRoundTrip(t *testing.T) {
	n := 0
	for _, base := range realPackages(t) {
		p, err := LoadPackage(base)
		if err != nil {
			t.Logf("%s: %v (skipped)", filepath.Base(base), err)
			continue
		}
		ua, ue, err := p.Write(PackageEdit{})
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		if !bytes.Equal(ua, p.UAsset) || !bytes.Equal(ue, p.UExp) {
			t.Errorf("%s: writing it unchanged gave different bytes", base)
		}
		// the hashes this program works out match the game's
		for i, nm := range p.Names {
			if p.nameHashes[i] != [4]byte{} && nameHashes(nm) != p.nameHashes[i] {
				t.Errorf("%s: hash of %q differs", filepath.Base(base), nm)
				break
			}
		}
		n++
	}
	if n < 5 {
		t.Fatalf("only %d packages could be read", n)
	}
	t.Logf("%d packages round-trip", n)
}

func TestFNameSplit(t *testing.T) {
	cases := []struct {
		in   string
		base string
		num  int32
	}{
		{"X_602", "X", 603}, {"X_001", "X_001", 0}, {"X_0", "X", 1}, {"X", "X", 0}, {"X_", "X_", 0},
		{"T_UI_Icon_Skin_su57_101", "T_UI_Icon_Skin_su57", 102}, {"MI_PP0021_su57_001_Decal", "MI_PP0021_su57_001_Decal", 0},
		{"_5", "_5", 0}, {"A_12345678901", "A_12345678901", 0},
	}
	for _, c := range cases {
		b, n := splitFName(c.in)
		if b != c.base || n != c.num {
			t.Errorf("%s: got %s,%d", c.in, b, n)
		}
		if joinFName(b, n) != c.in {
			t.Errorf("%s does not join back", c.in)
		}
	}
}

func TestRetargetMaterial(t *testing.T) {
	base := su57Raw + "/Materials/001/MI_PP0021_su57_001_Decal"
	needFile(t, base+".uasset")
	p, err := LoadPackage(base)
	if err != nil {
		t.Fatal(err)
	}
	newName := "/Game/SkinStudio/Test_ab12/s1/MI_PP0021_su57_001_Decal"
	oldTex := "/Game/Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_C"
	newTex := "/Game/SkinStudio/Test_ab12/s1/T_PP0021_su57_001_C"
	ua, ue, err := p.Retarget(newName, map[string]string{oldTex: newTex})
	if err != nil {
		t.Fatal(err)
	}
	q, err := ParsePackage(ua, ue)
	if err != nil {
		t.Fatal(err)
	}
	if q.Name != newName {
		t.Errorf("package name %s", q.Name)
	}
	var imports []string
	for i := range q.Imports {
		imports = append(imports, q.ImportPath(i))
	}
	all := strings.Join(imports, "\n")
	if !strings.Contains(all, newTex+".T_PP0021_su57_001_C") {
		t.Errorf("colour texture not moved:\n%s", all)
	}
	if !strings.Contains(all, "/Game/Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_MRX.T_PP0021_su57_001_MRX") {
		t.Errorf("finish texture should stay:\n%s", all)
	}
	if !bytes.Equal(q.ExportData(0), p.ExportData(0)) {
		t.Errorf("export data changed")
	}
	// moving it back gives the original bytes
	ua2, ue2, err := q.Retarget(p.Name, map[string]string{newTex: oldTex})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ua2, p.UAsset) || !bytes.Equal(ue2, p.UExp) {
		t.Errorf("moving back did not restore the package")
	}
}

func TestRetargetSplitIcon(t *testing.T) {
	base := gameExports + "/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_101"
	needFile(t, base+".uasset")
	p, err := LoadPackage(base)
	if err != nil {
		t.Fatal(err)
	}
	nn := "/Game/SkinStudio/Test_ab12/s2/T_UI_Icon_Skin_su57_101"
	ua, ue, err := p.Retarget(nn, nil)
	if err != nil {
		t.Fatal(err)
	}
	q, err := ParsePackage(ua, ue)
	if err != nil {
		t.Fatal(err)
	}
	if q.Name != nn || !contains(q.Names, "/Game/SkinStudio/Test_ab12/s2/T_UI_Icon_Skin_su57") {
		t.Errorf("not moved: %s %v", q.Name, q.Names)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
