package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const emblemTemplateTex = "/Emblem/tga/AC8/T_emblem_00_AC8_010_001"

// the game's mappings file on this machine (made with UE4SS)
const testUsmap = "/mnt/user-data/uploads/DOWNLOADS/Live-5.4.3-0+UE5-e3ba1016.usmap"

// addEmblemGame puts the game's emblem lists and the template emblem's pictures into the test game.
func addEmblemGame(t *testing.T, e *slotEnv) {
	t.Helper()
	needFile(t, gameExports+"/Datatables/Information/DT_Emblem.uasset")
	needFile(t, gameExports+emblemTemplateTex+".uasset")
	content := filepath.Join(e.game, "Live", "Content")
	var rels []string
	for _, rel := range []string{"Datatables/Information/DT_Emblem", "Unlock/DA_Unlock_Emblem", strings.TrimPrefix(emblemTemplateTex, "/")} {
		rels = append(rels, rel)
	}
	for _, suf := range []string{"_s", "_m", "_mm", "_lll"} {
		rels = append(rels, "Emblem/png/AC8/T_emblem_00_AC8_010_001"+suf)
	}
	for _, rel := range rels {
		for _, ext := range []string{".uasset", ".uexp", ".ubulk"} {
			if _, err := os.Stat(filepath.Join(gameExports, rel+ext)); err == nil {
				copyFile(filepath.Join(gameExports, rel+ext), filepath.Join(content, rel+ext))
			}
		}
	}
}

// emblemPicture is a red disc on a see-through background.
func emblemPicture(t *testing.T) string {
	m := NewImg(300, 200)
	for y := 0; y < 200; y++ {
		for x := 0; x < 300; x++ {
			dx, dy := x-150, y-100
			if dx*dx+dy*dy < 80*80 {
				o := (y*300 + x) * 4
				m.Pix[o], m.Pix[o+1], m.Pix[o+2], m.Pix[o+3] = 220, 30, 30, 255
			}
		}
	}
	p := filepath.Join(t.TempDir(), "owl.png")
	if err := SaveImage(p, m, SaveRGBA); err != nil {
		t.Fatal(err)
	}
	return p
}

func readEmblemList(t *testing.T, e *slotEnv) (*SkinTable, *UnlockTable) {
	t.Helper()
	in := filepath.Join(e.s.skinListDir(), "packed", skinListMod+"_P.input")
	et, err := loadEmblemTable(gameFile(in, "Live", emblemTablePath), builtinSchemaOrUsmap(t))
	if err != nil {
		t.Fatal(err)
	}
	ut, err := loadEmblemUnlockTable(gameFile(in, "Live", emblemUnlockPath), builtinSchemaOrUsmap(t))
	if err != nil {
		t.Fatal(err)
	}
	return et, ut
}

// builtinSchemaOrUsmap is the layout the emblem list is read with: the built-in one when it knows
// the emblem rows, otherwise the mappings file on this machine.
func builtinSchemaOrUsmap(t *testing.T) *Schema {
	if _, ok := builtinSchema.Structs[emblemRowStruct]; ok {
		return builtinSchema
	}
	sc, err := cachedUsmap(testUsmap)
	if err != nil {
		t.Skip("no layout for the emblem list")
	}
	return sc
}

func TestEmblemsEndToEnd(t *testing.T) {
	e := newSlotEnv(t)
	addEmblemGame(t, e)
	if _, ok := builtinSchema.Structs[emblemRowStruct]; !ok {
		needFile(t, testUsmap)
		e.s.Usmap = testUsmap
	}
	j := testJob()
	pic := emblemPicture(t)
	res, err := doBuildEmblem(j, e.s, emblemReq{ModTitle: "Night Owls", Author: "tester", Name: "Night Owl", Description: "A red moon.\nOver the sea.", Picture: pic})
	if err != nil {
		t.Fatal(err)
	}
	r := res.(emblemResult)
	if r.Number != 601 || r.Emblem != "e1" {
		t.Fatalf("result %+v", r)
	}
	m, err := loadProject(e.s, r.Mod)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Emblems) != 1 || len(m.Slots) != 0 || m.Emblems[0].IconFile != "icons/e1.png" {
		t.Fatalf("manifest %+v", m)
	}
	// the mod's container: its five pictures, renamed into the game's emblem folders
	packed := filepath.Join(e.s.projectDir(m.Key), "packed", m.Container+".input")
	for _, ep := range emblemPictures {
		path := emblemPicturePath(m.Key, "e1", ep.suffix)
		tx, p, ub, err := LoadTex2D(gameFile(packed, "Live", path))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if p.Name != path || p.NameText(p.Exports[0].ObjectName) != packageBaseName(path) {
			t.Fatalf("%s is named %s / %s", path, p.Name, p.NameText(p.Exports[0].ObjectName))
		}
		im, err := tx.DecodeMip(p.UExp, ub, 0)
		if err != nil {
			t.Fatal(err)
		}
		mid := im.Pix[(im.H/2*im.W+im.W/2)*4:]
		corner := im.Pix[(2*im.W+2)*4:]
		if mid[0] < 180 || mid[1] > 70 || mid[3] < 240 || corner[3] > 10 {
			t.Errorf("%s: middle %v, corner %v", path, mid[:4], corner[:4])
		}
	}
	// the emblem list and its unlock rule, in the skin list mod
	et, ut := readEmblemList(t, e)
	row := findEmblem(et, emblemIDOf(601))
	if row == nil {
		t.Fatal("no emblem row 601")
	}
	o := row.Obj
	if o.Str("EmblemNameTextID") != m.textKey("e1", "Name") || o.Str("EmblemStringID") != emblemStem(m.Key, "e1") ||
		o.Str("EmbFilePathIngame") != emblemPicturePath(m.Key, "e1", "")+"."+emblemStem(m.Key, "e1") ||
		o.Str("MenuIconImageRef") != emblemPicturePath(m.Key, "e1", "_mm")+"."+emblemStem(m.Key, "e1")+"_mm" || o.Int("SortNumber") != 106010 {
		t.Fatalf("emblem row %+v", o)
	}
	if len(et.Rows) != 286 {
		t.Errorf("%d emblem rows", len(et.Rows))
	}
	var unlock *Obj
	for _, x := range ut.Entries() {
		if unlockedBy(x, "UnlockEmblem") == emblemIDOf(601) {
			unlock = x
		}
	}
	if unlock == nil || unlock.Int("ID") != 1500213 {
		t.Fatalf("unlock entry %+v", unlock)
	}
	// names through SkinSlotText
	txt, err := os.ReadFile(filepath.Join(e.ue4ss, "SkinSlotText", "Scripts", "texts", textFilePrefix+m.Key+".ini"))
	if err != nil || !strings.Contains(string(txt), m.textKey("e1", "Name")+"=Night Owl") || !strings.Contains(string(txt), `=A red moon.\nOver the sea.`) {
		t.Fatalf("texts: %v\n%s", err, txt)
	}
	// installed with its icon picture
	inst := filepath.Join(e.mods, m.Folder)
	for _, f := range []string{m.Container + ".utoc", manifestName, "icons/e1.png"} {
		if _, err := os.Stat(filepath.Join(inst, f)); err != nil {
			t.Errorf("not installed: %s", f)
		}
	}
	// 2. a second emblem gets the next number; a new skin can go in the same mod
	res2, err := doBuildEmblem(j, e.s, emblemReq{Mod: m.Key, Name: "Day Owl", Picture: pic})
	if err != nil {
		t.Fatal(err)
	}
	if res2.(emblemResult).Number != 602 || res2.(emblemResult).Emblem != "e2" {
		t.Fatalf("second %+v", res2)
	}
	if _, err := doBuildSlot(j, e.s, slotReq{planReq: planReq{SkinDir: e.rawDir}, Mod: m.Key, Name: "Owl skin", Icon: "base"}); err != nil {
		t.Fatal(err)
	}
	// 3. renaming changes only the text; changing it without a picture keeps its pictures and number
	if _, err := editSlotTexts(j, e.s, slotEditReq{Mod: m.Key, Slot: "e1", Name: "Night Owl II"}); err != nil {
		t.Fatal(err)
	}
	txt, _ = os.ReadFile(filepath.Join(e.ue4ss, "SkinSlotText", "Scripts", "texts", textFilePrefix+m.Key+".ini"))
	if !strings.Contains(string(txt), m.textKey("e1", "Name")+"=Night Owl II") {
		t.Fatalf("renamed: %s", txt)
	}
	if _, err := doBuildEmblem(j, e.s, emblemReq{Mod: m.Key, Emblem: "e2", Name: "Day Owl", Description: "Now with a text"}); err != nil {
		t.Fatal(err)
	}
	et, _ = readEmblemList(t, e)
	if findEmblem(et, emblemIDOf(602)) == nil {
		t.Fatal("e2 lost its number")
	}
	if _, err := os.Stat(gameFile(filepath.Join(e.s.projectDir(m.Key), "packed", m.Container+".input"), "Live", emblemPicturePath(m.Key, "e2", "_lll")) + ".uasset"); err != nil {
		t.Fatal("e2 lost its pictures")
	}
	// 4. the overview shows both
	ov := slotsOverviewNow(e.s)
	if len(ov.Mods) != 1 || len(ov.Mods[0].Emblems) != 2 || ov.Mods[0].Emblems[0].Number != 601 || ov.Stale != "" {
		t.Fatalf("overview %+v", ov)
	}
	// 5. sharing: the stand-alone list in the zip has the emblem list with this mod's emblems
	ex, err := doExportMod(j, e.s, m.Key)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(ex.(map[string]any)["zip"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	readme := ""
	for _, f := range zr.File {
		names = append(names, f.Name)
		if f.Name == "README.txt" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			readme = string(b)
		}
	}
	zr.Close()
	all := strings.Join(names, "\n")
	if !strings.Contains(all, "icons/e1.png") || !strings.Contains(all, "icons/e2.png") || !strings.Contains(readme, "1 new skin in a slot of its own and 2 new emblems") ||
		!strings.Contains(readme, "Night Owl II (emblem)") {
		t.Fatalf("zip:\n%s\n%s", all, readme)
	}
	// 6. removing an emblem takes it out of the list; its number is not given to the next one
	if _, err := removeSlot(j, e.s, m.Key, "e1"); err != nil {
		t.Fatal(err)
	}
	et, _ = readEmblemList(t, e)
	if findEmblem(et, emblemIDOf(601)) != nil || findEmblem(et, emblemIDOf(602)) == nil {
		t.Fatal("removing e1")
	}
	m, _ = loadProject(e.s, m.Key)
	if m.emblem("e1") != nil {
		t.Fatal("e1 still in the mod")
	}
	if _, err := os.Stat(gameFile(filepath.Join(e.s.projectDir(m.Key), "input"), "Live", emblemPicturePath(m.Key, "e1", "")) + ".uasset"); err == nil {
		t.Fatal("e1's pictures are still in the mod")
	}
	st := loadListState(e.s)
	if len(st.EmblemRetired) < 2 {
		t.Fatalf("retired emblem ids %v", st.EmblemRetired)
	}
}

func TestEmblemSquareBleeds(t *testing.T) {
	m := NewImg(8, 4)
	for x := 0; x < 4; x++ {
		o := (1*8 + x) * 4
		m.Pix[o], m.Pix[o+1], m.Pix[o+2], m.Pix[o+3] = 200, 100, 50, 255
	}
	sq := emblemSquare(m)
	if sq.W != 8 || sq.H != 8 {
		t.Fatalf("%dx%d", sq.W, sq.H)
	}
	// a see-through corner far away takes the picture's colour, not black
	c := sq.Pix[(7*8+7)*4:]
	if c[0] != 200 || c[1] != 100 || c[2] != 50 || c[3] != 0 {
		t.Errorf("corner %v", c[:4])
	}
}
