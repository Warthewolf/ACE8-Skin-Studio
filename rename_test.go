package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The icon moves from the mod's own folder to the game's icon folder under a new skin number: the
// number at the end of its name changes (T_UI_Icon_Skin_su57_101 is T_UI_Icon_Skin_su57 + 102).
func TestRenameIconNumber(t *testing.T) {
	base := gameExports + "/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_101"
	needFile(t, base+".uasset")
	g, err := LoadPackage(base)
	if err != nil {
		t.Fatal(err)
	}
	own := "/Game/SkinStudio/Test_abcde/s1/T_UI_Icon_Skin_su57_101"
	ua, ue, err := g.Retarget(own, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := ParsePackage(ua, ue)
	nn := "/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_603"
	ua, ue, err = p.Rename(nn, map[string]string{"T_UI_Icon_Skin_su57_101": "T_UI_Icon_Skin_su57_603"})
	if err != nil {
		t.Fatal(err)
	}
	q, err := ParsePackage(ua, ue)
	if err != nil {
		t.Fatal(err)
	}
	if q.Name != nn || q.NameText(q.Exports[0].ObjectName) != "T_UI_Icon_Skin_su57_603" {
		t.Fatalf("renamed to %s / %s", q.Name, q.NameText(q.Exports[0].ObjectName))
	}
	if !contains(q.Names, "/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57") || contains(q.Names, "/Game/SkinStudio/Test_abcde/s1/T_UI_Icon_Skin_su57") {
		t.Errorf("names %v", q.Names)
	}
	if !bytes.Equal(q.ExportData(0), g.ExportData(0)) {
		t.Error("the icon's data changed")
	}
	if im, err := DecodeIcon(q); err != nil || im.W != 180 {
		t.Errorf("icon does not decode: %v", err)
	}
	// renaming it back to the game's own name gives the game's file again
	r, _ := ParsePackage(ua, ue)
	ua3, ue3, err := r.Rename("/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_101", map[string]string{"T_UI_Icon_Skin_su57_603": "T_UI_Icon_Skin_su57_101"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ua3, g.UAsset) || !bytes.Equal(ue3, g.UExp) {
		t.Error("renaming back did not give the game's file")
	}
	pyCheck(t, ua, ue, "T_UI_Icon_Skin_su57_603", "/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_603")
}

// The material moves into a numbered material slot of the aircraft's own folder, with its name
// following the game's way (MI_PP0021_su57_603_Decal), and points at the skin's own texture.
func TestRenameMaterialSlot(t *testing.T) {
	base := su57Raw + "/Materials/001/MI_PP0021_su57_001_Decal"
	needFile(t, base+".uasset")
	p, err := LoadPackage(base)
	if err != nil {
		t.Fatal(err)
	}
	oldTex := "/Game/Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_C"
	newTex := "/Game/SkinStudio/Test_abcde/s1/T_PP0021_su57_001_C"
	nn := "/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/603/MI_PP0021_su57_603_Decal"
	ua, ue, err := p.Rename(nn, map[string]string{"MI_PP0021_su57_001_Decal": "MI_PP0021_su57_603_Decal", oldTex: newTex})
	if err != nil {
		t.Fatal(err)
	}
	q, err := ParsePackage(ua, ue)
	if err != nil {
		t.Fatal(err)
	}
	if q.Name != nn {
		t.Fatalf("package %s", q.Name)
	}
	var objs []string
	for _, e := range q.Exports {
		objs = append(objs, q.NameText(e.ObjectName))
	}
	if !contains(objs, "MI_PP0021_su57_603_Decal") {
		t.Fatalf("exports %v", objs)
	}
	var imps []string
	for i := range q.Imports {
		imps = append(imps, q.ImportPath(i))
	}
	if !contains(imps, newTex+".T_PP0021_su57_001_C") || !contains(imps, "/Game/Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_MRX.T_PP0021_su57_001_MRX") {
		t.Fatalf("imports %v", imps)
	}
	for i := range p.Exports {
		if !bytes.Equal(q.ExportData(i), p.ExportData(i)) {
			t.Errorf("export %d data changed", i)
		}
	}
	// a name it does not have is refused
	if _, _, err := p.Rename(nn, map[string]string{"MI_Nothing": "MI_Other"}); err == nil {
		t.Error("renaming a name it does not have should fail")
	}
	pyCheck(t, ua, ue, "MI_PP0021_su57_603_Decal", nn)
}

// pyCheck reads a written package with the independent Python reader of the slot lab.
func pyCheck(t *testing.T, ua, ue []byte, obj, pkgName string) {
	t.Helper()
	lab := "/tmp/claude-0/-home-claude/adb129bf-895e-5b74-bb5f-60ffb1b3819a/scratchpad/slotlab"
	if _, err := os.Stat(lab + "/pkg.py"); err != nil {
		return
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.uasset"), ua, 0o644)
	os.WriteFile(filepath.Join(dir, "x.uexp"), ue, 0o644)
	script := `import sys; sys.path.insert(0, sys.argv[1]); import pkg
P = pkg.Package(sys.argv[2])
print(P.folder); print([e['name'] for e in P.exports]); print([i['name'] for i in P.imports])`
	out, err := exec.Command("python3", "-c", script, lab, filepath.Join(dir, "x.uasset")).CombinedOutput()
	if err != nil {
		t.Fatalf("python reader: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, pkgName+"\n") || !strings.Contains(s, "'"+obj+"'") {
		t.Fatalf("python reader says:\n%s", s)
	}
}

// Skin 001's icon name keeps its number in the text (a leading zero is not split off), so renaming
// it to 603 moves the number out of the text.
func TestRenameIconLeadingZero(t *testing.T) {
	base := gameExports + "/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_001"
	needFile(t, base+".uasset")
	g, err := LoadPackage(base)
	if err != nil {
		t.Fatal(err)
	}
	nn := "/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_603"
	ua, ue, err := g.Rename(nn, map[string]string{"T_UI_Icon_Skin_su57_001": "T_UI_Icon_Skin_su57_603"})
	if err != nil {
		t.Fatal(err)
	}
	q, err := ParsePackage(ua, ue)
	if err != nil {
		t.Fatal(err)
	}
	if q.Name != nn || q.NameText(q.Exports[0].ObjectName) != "T_UI_Icon_Skin_su57_603" || q.Exports[0].ObjectName.Number != 604 {
		t.Fatalf("renamed to %s / %s %+v", q.Name, q.NameText(q.Exports[0].ObjectName), q.Exports[0].ObjectName)
	}
	if !bytes.Equal(q.ExportData(0), g.ExportData(0)) {
		t.Error("the icon's data changed")
	}
	pyCheck(t, ua, ue, "T_UI_Icon_Skin_su57_603", nn)
}
