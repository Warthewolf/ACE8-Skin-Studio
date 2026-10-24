package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A skin made by 2.0.0 or 2.0.1 kept its material and icon in its mod's container and the
// workspace, with no slotfiles.zip. Rebuilding the skin list gives it a numbered material slot too.
func TestOldLayoutGetsMaterialSlot(t *testing.T) {
	e := newSlotEnv(t)
	j := testJob()
	paint := e.paint(t, [3]uint8{20, 30, 60})
	res, err := doBuildSlot(j, e.s, slotReq{planReq: planReq{SkinDir: e.rawDir, PaintDir: paint, Textures: []string{"T_PP0021_su57_001_C"}},
		ModTitle: "Old Navy", Name: "Old Navy", Unlock: UnlockAlways, Icon: "recolour"})
	if err != nil {
		t.Fatal(err)
	}
	r := res.(slotResult)
	m, _ := loadProject(e.s, r.Mod)
	// turn it into the old layout: no slot files in the mod, the files next to the textures in the workspace
	os.RemoveAll(filepath.Join(e.mods, m.Folder, slotFilesFolder))
	os.Remove(filepath.Join(e.mods, m.Folder, slotFilesZip))
	sdir := e.s.slotFilesDir(m.Key, "s1")
	es, _ := os.ReadDir(sdir)
	for _, f := range es {
		os.Rename(filepath.Join(sdir, f.Name()), filepath.Join(slotInputDir(e.s, m, "s1"), f.Name()))
	}
	os.RemoveAll(filepath.Join(e.s.projectDir(m.Key), "slotfiles"))
	st := loadListState(e.s)
	st.Layout = 0
	saveListState(e.s, st)
	if ov := slotsOverviewNow(e.s); ov.Stale == "" {
		t.Error("an old skin list should be marked for rebuilding")
	}
	if _, err := RebuildSkinList(j, e.s); err != nil {
		t.Fatal(err)
	}
	row := readList(t, e).Find(21601001)
	if row == nil || row.Material() != "/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/601/MI_PP0021_su57_601_Decal.MI_PP0021_su57_601_Decal" ||
		row.Obj.Str("MenuIconImageRef") != "/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_601.T_UI_Icon_Skin_su57_601" {
		t.Fatalf("row %+v", row)
	}
	if ov := slotsOverviewNow(e.s); ov.Stale != "" {
		t.Errorf("still stale: %s", ov.Stale)
	}
	// a skin from another computer in the old layout (no workspace copy, no slotfiles.zip) keeps
	// the paths in its mod
	os.RemoveAll(e.s.projectDir(m.Key))
	if _, err := RebuildSkinList(j, e.s); err != nil {
		t.Fatal(err)
	}
	row = readList(t, e).Find(21601001)
	own := slotAssetDir(m.Key, "s1")
	if row == nil || row.Material() != own+"MI_PP0021_su57_001_Decal.MI_PP0021_su57_001_Decal" {
		t.Fatalf("old-layout row %+v", row)
	}
}
