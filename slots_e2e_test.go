package main

// Whole new-skin builds, imports and exports against a stand-in for retoc: the test program itself,
// run with FAKE_RETOC set. Its "to-legacy" copies files out of a folder laid out like the game's
// content (made from the real game files on this machine), and its "to-zen" writes a small container
// table naming the files it was given and keeps a copy of them next to it for checking.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestMain(m *testing.M) {
	if os.Getenv("FAKE_RETOC") != "" {
		os.Exit(fakeRetoc(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeRetoc(args []string) int {
	var filters, pos []string
	cmd := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--aes-key" || a == "--version":
			i++
		case a == "--filter":
			i++
			filters = append(filters, args[i])
		case strings.HasPrefix(a, "--"):
		case cmd == "":
			cmd = a
		default:
			pos = append(pos, a)
		}
	}
	f, _ := os.OpenFile(os.Getenv("FAKE_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if f != nil {
		fmt.Fprintln(f, cmd, strings.Join(filters, " "), strings.Join(pos, " "))
		f.Close()
	}
	switch cmd {
	case "to-legacy":
		game, out := os.Getenv("FAKE_GAME"), pos[1]
		n := 0
		filepath.Walk(game, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(game, p)
			full := "../../../" + filepath.ToSlash(rel)
			for _, flt := range filters {
				if strings.Contains(full, flt) {
					copyFile(p, filepath.Join(out, rel))
					n++
					break
				}
			}
			return nil
		})
		fmt.Printf("extracted %d files\n", n)
		return 0
	case "list":
		game := os.Getenv("FAKE_GAME")
		filepath.Walk(game, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				rel, _ := filepath.Rel(game, p)
				fmt.Println("../../../" + filepath.ToSlash(rel))
			}
			return nil
		})
		return 0
	case "to-zen":
		in, utoc := pos[0], pos[1]
		var files []string
		filepath.Walk(in, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && (strings.HasSuffix(p, ".uasset") || strings.HasSuffix(p, ".ubulk")) {
				rel, _ := filepath.Rel(in, p)
				files = append(files, filepath.ToSlash(rel))
			}
			return nil
		})
		h := fnv.New64a()
		h.Write([]byte(filepath.Base(utoc)))
		os.WriteFile(utoc, synthTOC(h.Sum64(), "../../../", files), 0o644)
		os.WriteFile(strings.TrimSuffix(utoc, ".utoc")+".ucas", []byte("ucas of "+strings.Join(files, ",")), 0o644)
		os.WriteFile(strings.TrimSuffix(utoc, ".utoc")+".pak", make([]byte, 347), 0o644)
		// a copy of what was packed, for the test to look at
		copyTree(in, strings.TrimSuffix(utoc, ".utoc")+".input")
		fmt.Printf("packed %d files\n", len(files))
		return 0
	}
	fmt.Println("unknown command", cmd)
	return 1
}

func copyTree(src, dst string) {
	filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(src, p)
			copyFile(p, filepath.Join(dst, rel))
		}
		return nil
	})
}

type slotEnv struct {
	s      Settings
	mods   string
	ue4ss  string
	game   string
	rawDir string // the raw files of the skin the new skins are based on
	log    string
}

func testJob() *Job {
	ctx, cancel := context.WithCancel(context.Background())
	return &Job{ID: "t", ctx: ctx, cancel: cancel, Status: "running"}
}

// newSlotEnv sets up a game folder, mods folder and UE4SS folder, with the real game's skin list,
// unlock list, Su-57 skin 001 material and icon, and a small stand-in for skin 001's colour texture.
func newSlotEnv(t *testing.T) *slotEnv {
	t.Helper()
	needFile(t, gameExports+"/Datatables/Information/DT_Skin.uasset")
	needFile(t, su57Raw+"/Materials/001/MI_PP0021_su57_001_Decal.uasset")
	root := t.TempDir()
	e := &slotEnv{game: filepath.Join(root, "gamefiles"), log: filepath.Join(root, "retoc.log")}
	content := filepath.Join(e.game, "Live", "Content")
	for _, rel := range []string{"Datatables/Information/DT_Skin", "Unlock/DA_Unlock_Skin", "UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_001"} {
		for _, ext := range []string{".uasset", ".uexp"} {
			copyFile(filepath.Join(gameExports, rel+ext), filepath.Join(content, rel+ext))
		}
	}
	for _, n := range []string{"MI_PP0021_su57_001_Decal", "MI_PP0021_su57_001_DAM"} {
		for _, ext := range []string{".uasset", ".uexp"} {
			copyFile(filepath.Join(su57Raw, "Materials", "001", n+ext), filepath.Join(content, "Vehicles/Aircraft/PP/PP0021_su57/Materials/001", n+ext))
		}
	}
	// skin 001's colour texture: the real header with small stand-in pixels (5 squares of 256)
	ue, ub := synthVT(5, 1, 256, 128, 4, "PF_DXT1", 8)
	p, err := LoadPackage(su57Raw + "/Textures/001/T_PP0021_su57_001_C")
	if err != nil {
		t.Fatal(err)
	}
	ua, ue2, err := p.Write(PackageEdit{Exports: [][]byte{ue[:len(ue)-4]}, UExpTail: ue[len(ue)-4:]})
	if err != nil {
		t.Fatal(err)
	}
	e.rawDir = filepath.Join(root, "raw", "Live", "Content", "Vehicles", "Aircraft", "PP", "PP0021_su57", "Textures", "001")
	writePackageFiles(filepath.Join(e.rawDir, "T_PP0021_su57_001_C"), ua, ue2, ub)
	// folders
	e.ue4ss = filepath.Join(root, "Game", "Binaries", "Win64", "ue4ss", "Mods")
	e.mods = filepath.Join(e.ue4ss, "IoStoreLoaderMod")
	os.MkdirAll(filepath.Join(e.mods, "dlls"), 0o755)
	os.WriteFile(filepath.Join(e.ue4ss, "mods.txt"), []byte("BPModLoaderMod : 1\r\n"), 0o644)
	os.WriteFile(filepath.Join(e.ue4ss, "..", "UE4SS.dll"), []byte("x"), 0o644)
	dll := []byte("MZ....")
	for _, c := range utf16.Encode([]rune("1.1.0")) {
		dll = binary.LittleEndian.AppendUint16(dll, c)
	}
	os.WriteFile(filepath.Join(e.mods, "dlls", "main.dll"), append(dll, 0, 0), 0o644)
	paks := filepath.Join(root, "Game", "Content", "Paks")
	os.MkdirAll(paks, 0o755)
	os.WriteFile(filepath.Join(paks, "pakchunk0-Windows.utoc"), []byte("x"), 0o644)
	exe, _ := os.Executable()
	e.s = Settings{PaksDir: paks, AESKey: strings.Repeat("ab", 32), RetocPath: exe, ModsDir: e.mods, Workspace: filepath.Join(root, "workspace")}
	dataDir = root
	defaults(&e.s)
	t.Setenv("FAKE_RETOC", "1")
	t.Setenv("FAKE_GAME", e.game)
	t.Setenv("FAKE_LOG", e.log)
	return e
}

func (e *slotEnv) paint(t *testing.T, c [3]uint8) string {
	dir := filepath.Join(t.TempDir(), "paint-"+hexColour(c)[1:])
	os.MkdirAll(dir, 0o755)
	m := NewImg(1280, 256)
	for i := 0; i < len(m.Pix); i += 4 {
		m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = c[0], c[1], c[2], 255
	}
	SaveImage(filepath.Join(dir, "MySkin_Base_color.png"), m, SaveRGB)
	return dir
}

func readList(t *testing.T, e *slotEnv) *SkinTable {
	t.Helper()
	// the stand-in for retoc keeps a copy of what it packed; read the list from there
	in := filepath.Join(e.s.skinListDir(), "packed", skinListMod+"_P.input")
	st, err := LoadSkinTable(gameFile(in, "Live", skinTablePath), builtinSchema)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestNewSlotBuildImportExport(t *testing.T) {
	e := newSlotEnv(t)
	j := testJob()
	// 1. a new skin with its own colour texture and a recoloured icon
	navy := [3]uint8{20, 30, 60}
	paint := e.paint(t, navy)
	res, err := doBuildSlot(j, e.s, slotReq{planReq: planReq{SkinDir: e.rawDir, PaintDir: paint, Textures: []string{"T_PP0021_su57_001_C"}},
		ModTitle: "Navy Feather", Author: "tester", Name: "Navy Feather", Description: "Dark navy.\nWith feathers.", Unlock: UnlockAlways, Icon: "recolour"})
	if err != nil {
		t.Fatal(err)
	}
	r := res.(slotResult)
	if r.Number != 601 || r.Plane != "su57" {
		t.Fatalf("result %+v", r)
	}
	if len(r.Setup) != 0 {
		t.Errorf("setup problems: %v", r.Setup)
	}
	m, err := loadProject(e.s, r.Mod)
	if err != nil {
		t.Fatal(err)
	}
	sl := m.Slots[0]
	own := slotAssetDir(m.Key, "s1")
	if sl.Material != own+"MI_PP0021_su57_001_Decal" || sl.Icon != own+"T_UI_Icon_Skin_su57_001" {
		t.Fatalf("slot %+v", sl)
	}
	// what the mod's container holds: only the texture, moved into the mod's own folder
	packed := filepath.Join(e.s.projectDir(m.Key), "packed", m.Container+".input")
	if p, err := LoadPackage(gameFile(packed, "Live", own+"T_PP0021_su57_001_C")); err != nil || p.Name != own+"T_PP0021_su57_001_C" {
		t.Fatalf("texture: %v", err)
	}
	if _, err := os.Stat(gameFile(packed, "Live", own+"MI_PP0021_su57_001_Decal") + ".uasset"); err == nil {
		t.Fatal("the material should not be in the mod's container")
	}
	// its material slot: material, damage material and icon under skin number 601, the game's way,
	// packed with the skin list
	listIn := filepath.Join(e.s.skinListDir(), "packed", skinListMod+"_P.input")
	slot := "/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/601/"
	mat, err := LoadPackage(gameFile(listIn, "Live", slot+"MI_PP0021_su57_601_Decal"))
	if err != nil {
		t.Fatal(err)
	}
	if mat.Name != slot+"MI_PP0021_su57_601_Decal" || mat.NameText(mat.Exports[0].ObjectName) != "MI_PP0021_su57_601_Decal" {
		t.Fatalf("material %s / %s", mat.Name, mat.NameText(mat.Exports[0].ObjectName))
	}
	var imps []string
	for i := range mat.Imports {
		imps = append(imps, mat.ImportPath(i))
	}
	if !contains(imps, own+"T_PP0021_su57_001_C.T_PP0021_su57_001_C") || !contains(imps, "/Game/Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_MRX.T_PP0021_su57_001_MRX") {
		t.Fatalf("material imports %v", imps)
	}
	if p, err := LoadPackage(gameFile(listIn, "Live", slot+"MI_PP0021_su57_601_DAM")); err != nil || p.NameText(p.Exports[0].ObjectName) != "MI_PP0021_su57_601_DAM" {
		t.Fatalf("damage material: %v", err)
	}
	icon, err := LoadPackage(gameFile(listIn, "Live", "/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_601"))
	if err != nil || icon.NameText(icon.Exports[0].ObjectName) != "T_UI_Icon_Skin_su57_601" {
		t.Fatalf("icon: %v", err)
	}
	im, _ := DecodeIcon(icon)
	if c := im.Pix[(100*180+90)*4:]; c[0] > 50 || c[2] < 40 {
		t.Errorf("icon not recoloured: %v", c[:3])
	}
	// installed: the mod folder and the skin list
	inst := filepath.Join(e.mods, m.Folder)
	for _, f := range []string{m.Container + ".utoc", m.Container + ".ucas", m.Container + ".pak", manifestName, "icons/s1.png", slotFilesFolder + "/s1"} {
		if _, err := os.Stat(filepath.Join(inst, f)); err != nil {
			t.Errorf("not installed: %s", f)
		}
	}
	if sf, _ := readSlotFolder(filepath.Join(inst, slotFilesFolder)); len(sf) < 4 {
		t.Errorf("slot files installed: %v", sf)
	}
	if _, err := os.Stat(filepath.Join(inst, slotFilesZip)); err == nil {
		t.Error("slotfiles.zip should no longer be written")
	}
	st := readList(t, e)
	row := st.Find(21601001)
	if row == nil || row.Obj.Str("DecalMaterialRef") != slot+"MI_PP0021_su57_601_Decal.MI_PP0021_su57_601_Decal" || row.Obj.Str("SkinNameTextID") != m.textKey("s1", "Name") ||
		row.Obj.Str("MenuIconImageRef") != "/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_601.T_UI_Icon_Skin_su57_601" {
		t.Fatalf("skin list row %+v", row)
	}
	// texts and SkinSlotText
	txt, err := os.ReadFile(filepath.Join(e.ue4ss, "SkinSlotText", "Scripts", "texts", textFilePrefix+m.Key+".ini"))
	if err != nil || !strings.Contains(string(txt), m.textKey("s1", "Name")+"=Navy Feather") || !strings.Contains(string(txt), `=Dark navy.\nWith feathers.`) {
		t.Fatalf("texts: %v\n%s", err, txt)
	}
	if _, err := os.Stat(filepath.Join(e.ue4ss, "SkinSlotText", "Scripts", "SkinSlotText.dll")); err != nil {
		t.Fatal("SkinSlotText not installed")
	}
	// 1b. changing only its name and icon keeps its own colour texture and its number
	if _, err := doBuildSlot(j, e.s, slotReq{planReq: planReq{SkinDir: e.rawDir}, Mod: m.Key, Slot: "s1", Name: "Navy Feather II", Unlock: UnlockAlways, Icon: "base"}); err != nil {
		t.Fatal(err)
	}
	m, _ = loadProject(e.s, m.Key)
	if sl := m.Slots[0]; sl.Material != own+"MI_PP0021_su57_001_Decal" || sl.Icon != "" || sl.Sources["T_PP0021_su57_001_C"] == "" || len(m.Slots) != 1 {
		t.Fatalf("after changing: %+v", sl)
	}
	if _, err := LoadPackage(gameFile(packed, "Live", own+"T_PP0021_su57_001_C")); err != nil {
		t.Fatal("colour texture lost when changing the skin")
	}
	if _, err := os.Stat(gameFile(listIn, "Live", "/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_601") + ".uasset"); err == nil {
		t.Fatal("the old icon should be gone")
	}
	if st := readList(t, e); st.Find(21601001) == nil || st.Find(21601001).Obj.Str("MenuIconImageRef") != "/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_001.T_UI_Icon_Skin_su57_001" {
		t.Fatal("number changed, or the icon is not the original's")
	}
	// 2. a second skin in the same mod, no textures of its own, original icon, unlocked by a mission
	res2, err := doBuildSlot(j, e.s, slotReq{planReq: planReq{SkinDir: e.rawDir}, Mod: m.Key, Name: "Plain copy", Unlock: UnlockMission, Icon: "base"})
	if err != nil {
		t.Fatal(err)
	}
	if res2.(slotResult).Number != 602 {
		t.Fatalf("second skin number %d", res2.(slotResult).Number)
	}
	st = readList(t, e)
	row2 := st.Find(21602001)
	if row2 == nil || row2.Material() != "/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/602/MI_PP0021_su57_602_Decal.MI_PP0021_su57_602_Decal" {
		t.Fatalf("second row %+v", row2)
	}
	// a skin without textures of its own still gets a material slot, using the original's paint
	mat2, err := LoadPackage(gameFile(listIn, "Live", "/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/602/MI_PP0021_su57_602_Decal"))
	if err != nil {
		t.Fatal(err)
	}
	imps = nil
	for i := range mat2.Imports {
		imps = append(imps, mat2.ImportPath(i))
	}
	if !contains(imps, "/Game/Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_C.T_PP0021_su57_001_C") {
		t.Fatalf("second material imports %v", imps)
	}
	// 3. share it
	exp, err := doExportMod(j, e.s, m.Key)
	if err != nil {
		t.Fatal(err)
	}
	zipPath := exp.(map[string]any)["zip"].(string)
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	zr.Close()
	for _, want := range []string{manifestName, "README.txt", "Mods/" + m.Folder + "/" + m.Container + ".utoc", "Mods/" + m.Folder + "_SkinList/" + m.Key + "_SkinList_P.utoc",
		"ue4ss/Mods/SkinSlotText/Scripts/SkinSlotText.dll", "ue4ss/Mods/SkinSlotText/Scripts/texts/" + textFilePrefix + m.Key + ".ini", "icons/s1.png"} {
		if !contains(names, want) {
			t.Errorf("zip lacks %s (has %v)", want, names)
		}
	}
	checkShareable(t, names, m.Folder)
	// 4. someone else imports it while they already have a skin in 601 and 602: it moves
	other := newSlotEnv(t)
	resO, err := doBuildSlot(j, other.s, slotReq{planReq: planReq{SkinDir: other.rawDir}, ModTitle: "Theirs", Name: "Their skin A", Icon: "base"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doBuildSlot(j, other.s, slotReq{planReq: planReq{SkinDir: other.rawDir}, Mod: resO.(slotResult).Mod, Name: "Their skin B", Icon: "base"}); err != nil {
		t.Fatal(err)
	}
	plan, err := planImport(j, other.s, zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Studio || len(plan.Skins) != 2 || plan.Skins[0].Number != 603 || plan.Skins[0].Taken == "" || plan.Update {
		b, _ := json.MarshalIndent(plan, "", " ")
		t.Fatalf("plan %s", b)
	}
	if _, err := doImport(j, other.s, zipPath); err != nil {
		t.Fatal(err)
	}
	if sf, _, _ := modSlotFiles(filepath.Join(other.mods, plan.Folder)); len(sf) < 4 {
		t.Errorf("imported slot files: %v", sf)
	}
	st = readList(t, other)
	for id, want := range map[int64]string{21601001: "Their skin A", 21603001: "Navy Feather", 21604001: "Plain copy"} {
		r := st.Find(id)
		if r == nil {
			t.Fatalf("%d missing", id)
		}
		_ = want
	}
	// the imported navy skin is 603 now: its material slot follows
	if got := st.Find(21603001).Material(); got != "/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/603/MI_PP0021_su57_603_Decal.MI_PP0021_su57_603_Decal" {
		t.Errorf("603 material %s", got)
	}
	otherIn := filepath.Join(other.s.skinListDir(), "packed", skinListMod+"_P.input")
	mat3, err := LoadPackage(gameFile(otherIn, "Live", "/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/603/MI_PP0021_su57_603_Decal"))
	if err != nil {
		t.Fatal(err)
	}
	imps = nil
	for i := range mat3.Imports {
		imps = append(imps, mat3.ImportPath(i))
	}
	if !contains(imps, own+"T_PP0021_su57_001_C.T_PP0021_su57_001_C") {
		t.Errorf("603 should use the imported navy texture: %v", imps)
	}
	// its numbers stay put when the list is rebuilt again
	list, err := RebuildSkinList(j, other.s)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range list.Slots {
		if p.Mod == m.Key && p.Slot == "s1" && p.Number != 603 {
			t.Errorf("navy moved to %d", p.Number)
		}
	}
	// importing the same zip again is an update, not a second copy
	plan2, err := planImport(j, other.s, zipPath)
	if err != nil || !plan2.Update || plan2.Skins[0].Number != 603 {
		t.Fatalf("second import plan %+v %v", plan2, err)
	}
	// 5. removing the first mod frees its numbers; the imported skins keep theirs
	if _, err := removeSlotMod(j, other.s, resO.(slotResult).Mod); err != nil {
		t.Fatal(err)
	}
	st = readList(t, other)
	if st.Find(21601001) != nil || st.Find(21603001) == nil {
		t.Errorf("after removal: 601 %v 603 %v", st.Find(21601001) != nil, st.Find(21603001) != nil)
	}
	// 6. with no skin mods left, the skin list mod goes away
	if _, err := removeSlotMod(j, other.s, m.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(other.mods, skinListMod)); !os.IsNotExist(err) {
		t.Errorf("skin list mod still there")
	}
}

func TestAdoptOtherSkinListMod(t *testing.T) {
	needFile(t, slotTestUtoc[:len(slotTestUtoc)-5]+".ucas")
	e := newSlotEnv(t)
	j := testJob()
	// the earlier experiment mod: its own skin list with skins 601 and 602
	dir := filepath.Join(e.mods, "SkinSlotTest")
	for _, ext := range []string{".utoc", ".ucas", ".pak"} {
		copyFile(strings.TrimSuffix(slotTestUtoc, ".utoc")+ext, filepath.Join(dir, "SkinSlotTest_P"+ext))
	}
	// and a skin made here that wants 601 too
	res, err := doBuildSlot(j, e.s, slotReq{planReq: planReq{SkinDir: e.rawDir}, ModTitle: "Mine", Name: "Mine", Icon: "base", Number: 601})
	if err != nil {
		t.Fatal(err)
	}
	r := res.(slotResult)
	if len(r.List.Adopted) != 1 || len(r.List.Adopted[0].NewSkins) != 2 {
		t.Fatalf("adopted %+v", r.List.Adopted)
	}
	if r.Number != 603 {
		t.Errorf("mine got %d", r.Number)
	}
	st := readList(t, e)
	for _, id := range []int64{21601001, 21602001, 21603001} {
		if st.Find(id) == nil {
			t.Errorf("%d missing", id)
		}
	}
	if !strings.Contains(st.Find(21601001).Obj.Str("SkinNameTextID"), "AircraftSkin_Name_su57_601") {
		t.Errorf("601 should be the experiment's skin")
	}
	lo, _ := os.ReadFile(filepath.Join(e.mods, "loadorder.txt"))
	_ = lo
}

func TestRepackWithNewNameAndIcon(t *testing.T) {
	e := newSlotEnv(t)
	j := testJob()
	paint := e.paint(t, [3]uint8{120, 20, 20})
	res, err := doBuild(j, e.s, planReq{SkinDir: e.rawDir, PaintDir: paint, ModName: "RedOne", Textures: []string{"T_PP0021_su57_001_C"}, Install: true,
		Extras: &skinExtras{Name: "Red One", Description: "All red.", Icon: "recolour"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = res
	ini, err := os.ReadFile(filepath.Join(e.ue4ss, "SkinSlotText", "Scripts", "texts", "SkinStudioRepack_RedOne.ini"))
	if err != nil || !strings.Contains(string(ini), "AircraftSkin_Name_su57_001=Red One") || !strings.Contains(string(ini), "AircraftSkin_Description_su57_001=All red.") {
		t.Fatalf("texts: %v\n%s", err, ini)
	}
	packed := filepath.Join(e.s.modsOut(), "RedOne", "packed", "RedOne_P.input")
	ip, err := LoadPackage(gameFile(packed, "Live", "/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_001"))
	if err != nil {
		t.Fatal("icon not in the mod: ", err)
	}
	im, _ := DecodeIcon(ip)
	if c := im.Pix[(100*180+90)*4:]; c[0] < 90 || c[1] > 50 {
		t.Errorf("icon not red: %v", c[:3])
	}
	// removing the mod on the Mods screen takes its texts along
	if _, err := removeMod(e.s, "RedOne", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(e.ue4ss, "SkinSlotText", "Scripts", "texts", "SkinStudioRepack_RedOne.ini")); !os.IsNotExist(err) {
		t.Errorf("texts left behind")
	}
}

// checkShareable fails when a shared zip holds anything Nexus Mods quarantines (an archive inside
// the archive) or program code other than SkinSlotText. The skins' materials and icons are plain files.
func checkShareable(t *testing.T, names []string, folder string) {
	t.Helper()
	slotFiles := 0
	for _, n := range names {
		switch strings.ToLower(path.Ext(n)) {
		case ".zip", ".7z", ".rar":
			t.Errorf("the shared zip holds an archive: %s", n)
		case ".dll", ".exe", ".lua", ".bat", ".ps1", ".asi":
			if !strings.HasPrefix(n, "ue4ss/Mods/SkinSlotText/Scripts/") {
				t.Errorf("the shared zip holds program code that is not SkinSlotText: %s", n)
			}
		}
		if strings.HasPrefix(n, "Mods/"+folder+"/"+slotFilesFolder+"/") && isSlotFile(strings.TrimPrefix(n, "Mods/"+folder+"/"+slotFilesFolder+"/")) {
			slotFiles++
		}
	}
	if slotFiles < 4 {
		t.Errorf("the shared zip has %d slot files (%v)", slotFiles, names)
	}
}

// A zip shared by 2.0.2 to 2.0.8 has the slot files in a slotfiles.zip and SkinSlotText's DLL in it.
// It still imports, its slot files end up as plain files, and sharing it again gives a clean zip.
func TestImportOldShareZip(t *testing.T) {
	e := newSlotEnv(t)
	j := testJob()
	res, err := doBuildSlot(j, e.s, slotReq{planReq: planReq{SkinDir: e.rawDir, PaintDir: e.paint(t, [3]uint8{20, 40, 120}), Textures: []string{"T_PP0021_su57_001_C"}},
		ModTitle: "Old Share", Name: "Old Share", Icon: "recolour"})
	if err != nil {
		t.Fatal(err)
	}
	key := res.(slotResult).Mod
	exp, err := doExportMod(j, e.s, key)
	if err != nil {
		t.Fatal(err)
	}
	newZip := exp.(map[string]any)["zip"].(string)
	// rewrite it the way 2.0.8 wrote it
	zr, err := zip.OpenReader(newZip)
	if err != nil {
		t.Fatal(err)
	}
	var folder string
	var inner, outer bytes.Buffer
	izw := zip.NewWriter(&inner)
	ozw := zip.NewWriter(&outer)
	for _, f := range zr.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		r.Close()
		if i := strings.Index(f.Name, "/"+slotFilesFolder+"/"); i > 0 && strings.HasPrefix(f.Name, "Mods/") {
			folder = f.Name[len("Mods/"):i]
			w, _ := izw.Create(f.Name[i+len("/"+slotFilesFolder+"/"):])
			w.Write(b)
			continue
		}
		w, _ := ozw.Create(f.Name)
		w.Write(b)
	}
	zr.Close()
	izw.Close()
	w, _ := ozw.Create("Mods/" + folder + "/" + slotFilesZip)
	w.Write(inner.Bytes())
	for n, b := range skinSlotTextFiles() {
		w, _ := ozw.Create("ue4ss/Mods/SkinSlotText/" + n)
		w.Write(b)
	}
	ozw.Close()
	oldZip := filepath.Join(t.TempDir(), "OldShare.zip")
	os.WriteFile(oldZip, outer.Bytes(), 0o644)

	other := newSlotEnv(t)
	plan, err := planImport(j, other.s, oldZip)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SlotZip == "" || plan.SlotDir != "" {
		t.Fatalf("old zip: slot zip %q dir %q", plan.SlotZip, plan.SlotDir)
	}
	if _, err := doImport(j, other.s, oldZip); err != nil {
		t.Fatal(err)
	}
	inst := filepath.Join(other.mods, plan.Folder)
	if _, err := os.Stat(filepath.Join(inst, slotFilesZip)); err == nil {
		t.Error("slotfiles.zip was installed as it was")
	}
	sf, found, err := modSlotFiles(inst)
	if err != nil || !found || len(sf) < 4 {
		t.Fatalf("slot files after import: %v %v %v", sf, found, err)
	}
	st := readList(t, other)
	if got := st.Find(21601001).Material(); got != "/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/601/MI_PP0021_su57_601_Decal.MI_PP0021_su57_601_Decal" {
		t.Errorf("material %s", got)
	}
	// shared again from here: plain files, no DLL
	exp2, err := doExportMod(j, other.s, key)
	if err != nil {
		t.Fatal(err)
	}
	zr2, err := zip.OpenReader(exp2.(map[string]any)["zip"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr2.File {
		names = append(names, f.Name)
	}
	zr2.Close()
	checkShareable(t, names, plan.Folder)
}

// A mod installed by 2.0.2 to 2.0.8 keeps its slot files in slotfiles.zip: they are read from it,
// a slotfiles folder wins over it, and installing the mod again replaces the zip with the folder.
func TestModSlotFilesOldZip(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range []string{"s1/MI_X_Decal.uasset", "s1/MI_X_Decal.uexp", "../evil.uasset", "s1/notes.txt"} {
		w, _ := zw.Create(n)
		w.Write([]byte("zip:" + n))
	}
	zw.Close()
	os.WriteFile(filepath.Join(dir, slotFilesZip), buf.Bytes(), 0o644)
	files, found, err := modSlotFiles(dir)
	if err != nil || !found || len(files) != 2 || string(files["s1/MI_X_Decal.uexp"]) != "zip:s1/MI_X_Decal.uexp" {
		t.Fatalf("from the zip: %v %v %v", files, found, err)
	}
	proj := filepath.Join(t.TempDir(), "proj")
	os.MkdirAll(filepath.Join(proj, "s1"), 0o755)
	os.WriteFile(filepath.Join(proj, "s1", "MI_Y_Decal.uasset"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(proj, "s1", "MI_Y_Decal.uexp"), []byte("b"), 0o644)
	if err := installSlotFiles(proj, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, slotFilesZip)); !os.IsNotExist(err) {
		t.Error("the old zip should be gone")
	}
	files, found, _ = modSlotFiles(dir)
	if !found || len(files) != 2 || string(files["s1/MI_Y_Decal.uexp"]) != "b" {
		t.Fatalf("from the folder: %v", files)
	}
	// nothing left to install: the folder goes too
	os.RemoveAll(proj)
	if err := installSlotFiles(proj, dir); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := modSlotFiles(dir); found {
		t.Error("an empty install should leave no slot files")
	}
}
