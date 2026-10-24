package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func loadGameTables(t *testing.T) (*SkinTable, *UnlockTable) {
	t.Helper()
	needFile(t, gameExports+"/Datatables/Information/DT_Skin.uasset")
	st, err := LoadSkinTable(gameExports+"/Datatables/Information/DT_Skin", builtinSchema)
	if err != nil {
		t.Fatal(err)
	}
	ut, err := LoadUnlockTable(gameExports+"/Unlock/DA_Unlock_Skin", builtinSchema)
	if err != nil {
		t.Fatal(err)
	}
	return st, ut
}

func TestReadTables(t *testing.T) {
	st, ut := loadGameTables(t)
	if len(st.Rows) < 800 {
		t.Fatalf("%d rows", len(st.Rows))
	}
	r := st.Find(21001001)
	if r == nil || r.Plane() != "su57" || !strings.Contains(r.Material(), "MI_PP0021_su57_001_Decal") {
		t.Fatalf("su57 001 row: %+v", r)
	}
	if ut.MinID() != 1400000 || ut.MaxID() != 1409999 || len(ut.Entries()) < 500 {
		t.Fatalf("unlock list: %d %d %d", ut.MinID(), ut.MaxID(), len(ut.Entries()))
	}
	if st.SortBase(21) != 3170000 {
		t.Errorf("sort base %d", st.SortBase(21))
	}
}

func TestAddSlotRows(t *testing.T) {
	st, ut := loadGameTables(t)
	tmpl := st.Find(21001001)
	row := SkinRow{Key: skinRowKey(skinIDOf(21, 601)), Obj: tmpl.Obj.Clone()}
	sc := builtinSchema
	row.Obj.Set(sc, "SkinID", int32(skinIDOf(21, 601)))
	row.Obj.Set(sc, "DecalMaterialRef", softPathTo("/Game/SkinStudio/Test_ab12/s1/MI_PP0021_su57_001_Decal"))
	row.Obj.Set(sc, "MenuIconImageRef", softPathTo("/Game/SkinStudio/Test_ab12/s1/T_UI_Icon_Skin_su57_101"))
	row.Obj.Set(sc, "SkinNameTextID", "SS_Test_ab12_s1_Name")
	row.Obj.Set(sc, "SortNumber", int32(3170900))
	st.Insert(21, row)
	e, err := ut.NewUnlockEntry(1400589, skinIDOf(21, 601))
	if err != nil {
		t.Fatal(err)
	}
	ut.Append(e)
	dir := t.TempDir()
	for _, x := range []struct {
		rel string
		w   func() ([]byte, []byte, error)
	}{{"Datatables/Information/DT_Skin", st.Write}, {"Unlock/DA_Unlock_Skin", ut.Write}} {
		ua, ue, err := x.w()
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, x.rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p+".uasset", ua, 0o644)
		os.WriteFile(p+".uexp", ue, 0o644)
	}
	st2, err := LoadSkinTable(filepath.Join(dir, "Datatables/Information/DT_Skin"), sc)
	if err != nil {
		t.Fatal(err)
	}
	r := st2.Find(skinIDOf(21, 601))
	if r == nil || r.Obj.Str("DecalMaterialRef") != "/Game/SkinStudio/Test_ab12/s1/MI_PP0021_su57_001_Decal.MI_PP0021_su57_001_Decal" ||
		r.Obj.Str("MenuIconImageRef") != "/Game/SkinStudio/Test_ab12/s1/T_UI_Icon_Skin_su57_101.T_UI_Icon_Skin_su57_101" || r.Key != "I_EM_21601001" {
		t.Fatalf("row read back: %+v", r)
	}
	if len(st2.Rows) != len(st.Rows) {
		t.Fatal("row count")
	}
	ut2, err := LoadUnlockTable(filepath.Join(dir, "Unlock/DA_Unlock_Skin"), sc)
	if err != nil {
		t.Fatal(err)
	}
	last := ut2.Entries()[len(ut2.Entries())-1]
	if last.Int("ID") != 1400589 || unlockedSkin(last) != 21601001 {
		t.Fatalf("entry read back: %d %d", last.Int("ID"), unlockedSkin(last))
	}
	// an independent reader (the Python slot lab) agrees
	lab := "/tmp/claude-0/-home-claude/adb129bf-895e-5b74-bb5f-60ffb1b3819a/scratchpad/slotlab"
	if _, err := os.Stat(lab + "/dtskin.py"); err == nil {
		script := `import sys; sys.path.insert(0, sys.argv[1]); import dtskin, dump, pkg, json
dtskin.ROW='LiveSkinDataTable'
t = dtskin.load(sys.argv[2] + '/Datatables/Information/DT_Skin', dump.M())
r = [x for k, x, a, z in t['rows'] if x['SkinID'] == 21601001][0]
print(dump.clean(r)['DecalMaterialRef'], dump.clean(r)['MenuIconImageRef'], len(t['rows']))
A = pkg.Package(sys.argv[2] + '/Unlock/DA_Unlock_Skin.uasset')
o, rr = dump.read_export(A)
e = o['ConfigurationEntryList'][-1]
print(e['ID'], e['ConditionEntryList'][0]['ConditionType'], e['ActionEntryList'][0]['Parameters'])`
		out, err := exec.Command("python3", "-c", script, lab, dir).CombinedOutput()
		if err != nil {
			t.Fatalf("python reader: %v\n%s", err, out)
		}
		s := string(out)
		if !strings.Contains(s, "/Game/SkinStudio/Test_ab12/s1/MI_PP0021_su57_001_Decal:MI_PP0021_su57_001_Decal") ||
			!strings.Contains(s, "/Game/SkinStudio/Test_ab12/s1/T_UI_Icon_Skin_su57_101:T_UI_Icon_Skin_su57_101") ||
			!strings.Contains(s, "1400589 CompleteLevel Id:21601001,Mode:Campaign") {
			t.Fatalf("python reader says:\n%s", s)
		}
		t.Logf("python reader: %s", strings.TrimSpace(s))
	}
}

func TestUsmapMatchesBuiltin(t *testing.T) {
	p := "/mnt/user-data/uploads/DOWNLOADS/Live-5.4.3-0+UE5-e3ba1016.usmap"
	needFile(t, p)
	sc, err := LoadUsmapSchema(p)
	if err != nil {
		t.Fatal(err)
	}
	for name, slots := range builtinSchema.Structs {
		other := sc.Structs[name]
		if len(other) != len(slots) {
			t.Fatalf("%s: %d slots vs %d", name, len(other), len(slots))
		}
		for i := range slots {
			if (slots[i] == nil) != (other[i] == nil) || slots[i] != nil && (slots[i].Name != other[i].Name || slots[i].Type.Kind != other[i].Type.Kind) {
				t.Fatalf("%s slot %d differs", name, i)
			}
		}
	}
	if _, err := LoadSkinTable(gameExports+"/Datatables/Information/DT_Skin", sc); err != nil {
		t.Fatal(err)
	}
}

func fakeSlotMod(key string, unlock string) InstalledSlotMod {
	return InstalledSlotMod{Dir: "/x/" + key, Manifest: &ModManifest{Key: key, Title: key, Slots: []SlotDef{{Key: "s1", Aircraft: 21, Plane: "su57", Template: 21001001, Name: key, Unlock: unlock}}}}
}

func TestUnlockIDsNeverReused(t *testing.T) {
	st, ut := loadGameTables(t)
	last := ut.lastLowID()
	if last != 1400588 {
		t.Fatalf("the game's last unlock id is %d", last)
	}
	a := fakeSlotMod("Alpha_abcde", UnlockAlways)
	placed, assign, refs := assignNumbers(st, ut, []InstalledSlotMod{a}, SkinListState{})
	// the first new skin gets an id after the game's, with room for a game update's
	if len(assign) != 1 || assign[0].UnlockID != 1400689 || assign[0].UnlockID2 != 0 {
		t.Fatalf("assign %+v", assign)
	}
	if err := addSlotRows(builtinSchema, st, ut, placed, refs); err != nil {
		t.Fatal(err)
	}
	// one rule, the game's own rule for skins 001 and 002, last in the list
	var rules []string
	for _, e := range ut.Entries() {
		if unlockedSkin(e) == placed[0].SkinID {
			cv, _ := e.Get("ConditionEntryList")
			c := cv.([]any)[0].(*Obj)
			rules = append(rules, c.Str("ConditionType")+" "+c.Str("Parameters"))
		}
	}
	if strings.Join(rules, ",") != "CompleteLevel MissionId:AllBase,Mode:Campaign" {
		t.Fatalf("rules %v", rules)
	}
	if es := ut.Entries(); es[len(es)-1].Int("ID") != 1400689 {
		t.Fatal("the new rule is not last")
	}
	// Alpha is removed and Bravo arrives: Bravo must not get Alpha's id
	st2, ut2 := loadGameTables(t)
	b := fakeSlotMod("Bravo_abcde", UnlockMission)
	old := SkinListState{Assign: assign, Retired: []int64{assign[0].UnlockID}}
	_, assign2, _ := assignNumbers(st2, ut2, []InstalledSlotMod{b}, old)
	if assign2[0].UnlockID != 1400690 {
		t.Fatalf("reused or skipped: %+v after %+v", assign2, assign)
	}
	// and Alpha keeps its id when it comes back
	st3, ut3 := loadGameTables(t)
	_, assign3, _ := assignNumbers(st3, ut3, []InstalledSlotMod{a, b}, SkinListState{Assign: append(assign, assign2...)})
	for _, x := range assign3 {
		if x.Mod == "Alpha_abcde" && (x.UnlockID != assign[0].UnlockID || x.Number != assign[0].Number) {
			t.Fatalf("alpha changed: %+v", x)
		}
	}
	// a skin made with 2.0.0 to 2.0.2 (ids 1401000 and 1401001) gets a new id after the game's; with
	// the SkinSlotTest experiment installed its ids count as the last ones used
	st4, ut4 := loadGameTables(t)
	older := SkinListState{Assign: []SlotAssign{{Mod: "Alpha_abcde", Slot: "s1", Aircraft: 21, Number: 603, UnlockID: 1401000, UnlockID2: 1401001}},
		Retired: []int64{1401000, 1401001}, Foreign: experimentIDs}
	_, assign4, _ := assignNumbers(st4, ut4, []InstalledSlotMod{a}, older)
	if assign4[0].UnlockID != 1400689 || assign4[0].Number != 603 {
		t.Fatalf("old skin: %+v", assign4)
	}
}

// A skin whose mod was taken out and comes back does not take back a number another skin has had
// since; that skin keeps it, whatever order the mods' folders are in.
func TestReturningSkinDoesNotTakeNumber(t *testing.T) {
	st, ut := loadGameTables(t)
	arctic := fakeSlotMod("Arctic_abcde", UnlockMission) // folder sorts first
	crimson := fakeSlotMod("Crimson_abcde", UnlockMission)
	old := SkinListState{
		Assign: []SlotAssign{
			{Mod: "Arctic_abcde", Slot: "s1", Aircraft: 21, Number: 602, UnlockID: 1400690},  // parked: its mod was taken out
			{Mod: "Crimson_abcde", Slot: "s1", Aircraft: 21, Number: 602, UnlockID: 1400691}, // got 602 after that
		},
		Mods: []string{"Crimson_abcde/s1"},
	}
	for _, mods := range [][]InstalledSlotMod{{arctic, crimson}, {crimson, arctic}} {
		_, assign, _ := assignNumbers(st, ut, mods, old)
		got := map[string]int{}
		for _, a := range assign {
			got[a.Mod] = a.Number
		}
		if got["Crimson_abcde"] != 602 || got["Arctic_abcde"] != 601 && got["Arctic_abcde"] != 603 {
			t.Fatalf("numbers %v", got)
		}
	}
}
