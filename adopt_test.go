package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// Small skin and unlock lists made in memory, laid out like the game's (for tests that do not need
// the game's own files).

func synthSkinRow(id int64, mat, nameKey string) SkinRow {
	sc := builtinSchema
	o := &Obj{Struct: skinRowStruct}
	o.Set(sc, "SkinID", int32(id))
	o.Set(sc, "PlaneStringID", "su57")
	o.Set(sc, "DecalMaterialRef", softPathTo(mat))
	o.Set(sc, "SkinCategory", "Campaign")
	o.Set(sc, "SkinNameTextID", nameKey)
	o.Set(sc, "MenuIconImageRef", softPathTo(fmt.Sprintf("/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_%03d", slotOf(id))))
	o.Set(sc, "SortNumber", int32(3170000+slotOf(id)))
	o.Set(sc, "DLCID", "Live")
	return SkinRow{Key: skinRowKey(id), Obj: o}
}

func gameMaterial(n int) string {
	return fmt.Sprintf("/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/%03d/MI_PP0021_su57_%03d_Decal", n, n)
}

func synthEntry(id, skin int64) *Obj {
	sc := builtinSchema
	c := &Obj{Struct: "LiveConditionEntry"}
	c.Set(sc, "ConditionType", "CompleteLevel")
	c.Set(sc, "Parameters", "MissionId:AllBase,Mode:Campaign")
	a := &Obj{Struct: "LiveActionEntry"}
	a.Set(sc, "ActionType", "UnlockSkin")
	a.Set(sc, "Parameters", fmt.Sprintf("Id:%d,Mode:Campaign", skin))
	e := &Obj{Struct: "LiveCAConfigurationEntry"}
	e.Set(sc, "ID", uint32(id))
	e.Set(sc, "ConditionEntryList", []any{c})
	e.Set(sc, "ActionEntryList", []any{a})
	return e
}

func synthUnlockTable(entries ...*Obj) *UnlockTable {
	sc := builtinSchema
	o := &Obj{Struct: unlockStruct}
	o.Set(sc, "MinId", uint32(1400000))
	o.Set(sc, "MaxId", uint32(1409999))
	arr := []any{}
	for _, e := range entries {
		arr = append(arr, e)
	}
	o.Set(sc, "ConfigurationEntryList", arr)
	return &UnlockTable{sc: sc, Obj: o, action: "UnlockSkin", cutoff: oldUnlockStart}
}

// synthGameLists is a game with Su-57 skins 001 and 002, whose last unlock rule is 1400588.
func synthGameLists() (*SkinTable, *UnlockTable) {
	st := &SkinTable{sc: builtinSchema, Rows: []SkinRow{
		synthSkinRow(21001001, gameMaterial(1), "AircraftSkin_Name_su57_001"),
		synthSkinRow(21002001, gameMaterial(2), "AircraftSkin_Name_su57_002"),
	}}
	return st, synthUnlockTable(synthEntry(1400001, 21001001), synthEntry(1400588, 21002001))
}

// otherModLists is another tool's copy of both lists: the game's, plus rows and rules of its own.
func otherModLists(rows []SkinRow, rules ...*Obj) adoptedLists {
	st, ut := synthGameLists()
	st.Rows = append(st.Rows, rows...)
	ut.Append(rules...)
	return adoptedLists{st, ut}
}

func ruleFor(ut *UnlockTable, skin int64) []int64 {
	var ids []int64
	for _, e := range ut.Entries() {
		if unlockedSkin(e) == skin {
			ids = append(ids, e.Int("ID"))
		}
	}
	return ids
}

func TestSynthListsWriteBack(t *testing.T) {
	st, ut := synthGameLists()
	if !sameObj(builtinSchema, st.Rows[0].Obj, st.Rows[0].Obj.Clone()) || sameObj(builtinSchema, st.Rows[0].Obj, st.Rows[1].Obj) {
		t.Fatal("rows cannot be compared")
	}
	if ut.lastLowID() != 1400588 {
		t.Fatalf("last id %d", ut.lastLowID())
	}
	if _, err := ut.NewUnlockEntry(1400689, 21601001); err != nil {
		t.Fatal(err)
	}
}

// Two mods made with other tools both add Su-57 skin 601 with rule 1400589 (the id after the
// game's). Both skins are kept: the second gets a free number and an unlock id of its own. A change
// one of them makes to a game skin is not undone by the other carrying the game's row as it is.
func TestAdoptTwoModsWithTheSameSkin(t *testing.T) {
	st, ut := synthGameLists()
	aaa := otherModLists([]SkinRow{synthSkinRow(21601001, "/Game/AAA/MI_AAA_Decal", "AAA_Name")}, synthEntry(1400589, 21601001))
	changed := synthSkinRow(21001001, "/Game/AAA/MI_Game001_Decal", "AircraftSkin_Name_su57_001")
	aaa.st.Rows[0] = changed // AAA also repaints the game's skin 001
	bbb := otherModLists([]SkinRow{synthSkinRow(21601001, "/Game/BBB/MI_BBB_Decal", "BBB_Name")}, synthEntry(1400589, 21601001))
	adopted := []AdoptedMod{{Folder: "AAA", NewSkins: []AdoptedSkin{}, read: true}, {Folder: "BBB", NewSkins: []AdoptedSkin{}, read: true}}
	adoptLists(testJob(), st, ut, nil, adopted, []adoptedLists{aaa, bbb}, SkinListState{})

	if r := st.Find(21601001); r == nil || r.Material() != "/Game/AAA/MI_AAA_Decal.MI_AAA_Decal" {
		t.Fatalf("601 should be AAA's: %+v", r)
	}
	if r := st.Find(21602001); r == nil || r.Material() != "/Game/BBB/MI_BBB_Decal.MI_BBB_Decal" {
		t.Fatalf("BBB's skin should be 602: %+v", r)
	}
	if r := st.Find(21001001); r == nil || r.Material() != "/Game/AAA/MI_Game001_Decal.MI_Game001_Decal" {
		t.Fatalf("AAA's change to skin 001 was undone: %+v", r)
	}
	if got := ruleFor(ut, 21601001); len(got) != 1 || got[0] != 1400589 {
		t.Fatalf("601's rules %v", got)
	}
	if got := ruleFor(ut, 21602001); len(got) != 1 || got[0] == 1400589 || got[0] == 0 {
		t.Fatalf("602's rules %v", got)
	}
	a, b := adopted[0], adopted[1]
	if len(a.NewSkins) != 1 || a.Changed != 1 || len(b.NewSkins) != 1 || b.Changed != 0 || b.NewSkins[0].Number != 602 || !b.NewSkins[0].Moved {
		t.Fatalf("AAA %+v\nBBB %+v", a, b)
	}
	if n := len(st.Rows); n != 4 {
		t.Fatalf("%d rows", n)
	}
}

// A skin from another mod's list keeps the number it was given, while this program's skins come and
// go and other mods arrive.
func TestAdoptedSkinKeepsItsNumber(t *testing.T) {
	mine := fakeSlotMod("Mine_abcde", UnlockMission)
	xxx := func() adoptedLists {
		return otherModLists([]SkinRow{synthSkinRow(21601001, "/Game/XXX/MI_X_Decal", "X_Name")}, synthEntry(1400589, 21601001))
	}
	build := func(old SkinListState, mods []InstalledSlotMod, adopted []AdoptedMod, lists []adoptedLists) (SkinListState, *SkinTable, []SlotAssign) {
		t.Helper()
		st, ut := synthGameLists()
		adoptLists(testJob(), st, ut, mods, adopted, lists, old)
		_, assign, _ := assignNumbers(st, ut, mods, old)
		var names []string
		for _, a := range adopted {
			names = append(names, a.Folder)
		}
		placed := map[string]bool{}
		for _, a := range assign {
			placed[a.Mod+"/"+a.Slot] = true
		}
		all := assign
		for _, a := range old.Assign {
			if !placed[a.Mod+"/"+a.Slot] {
				all = append(all, a)
			}
		}
		return SkinListState{Assign: all, Mods: builtKeys(mods), Adopted: names, AdoptedSkins: adoptedRecords(old, adopted)}, st, assign
	}
	newAdopted := func(names ...string) []AdoptedMod {
		var out []AdoptedMod
		for _, n := range names {
			out = append(out, AdoptedMod{Folder: n, NewSkins: []AdoptedSkin{}, read: true})
		}
		return out
	}
	// 1. our skin has 601; XXX, which wants 601 too, gets 602
	old := SkinListState{Assign: []SlotAssign{{Mod: "Mine_abcde", Slot: "s1", Aircraft: 21, Number: 601, UnlockID: 1400689}}, Mods: []string{"Mine_abcde/s1"}}
	ad := newAdopted("XXX")
	state, st, _ := build(old, []InstalledSlotMod{mine}, ad, []adoptedLists{xxx()})
	if st.Find(21602001) == nil || ad[0].NewSkins[0].Number != 602 {
		t.Fatalf("1: %+v", ad[0].NewSkins)
	}
	// 2. our mod is taken out, and AAA (whose folder sorts first) arrives wanting 602: XXX keeps 602
	aaa := otherModLists([]SkinRow{synthSkinRow(21602001, "/Game/AAA/MI_A_Decal", "A_Name")})
	ad = newAdopted("AAA", "XXX")
	state, st, _ = build(state, nil, ad, []adoptedLists{aaa, xxx()})
	if r := st.Find(21602001); r == nil || r.Material() != "/Game/XXX/MI_X_Decal.MI_X_Decal" {
		t.Fatalf("2: XXX should still be 602: %+v %+v", ad[0].NewSkins, ad[1].NewSkins)
	}
	if ad[0].NewSkins[0].Number != 601 {
		t.Fatalf("2: AAA got %d", ad[0].NewSkins[0].Number)
	}
	// 3. our mod comes back: AAA had 601 while it was out, so our skin gets a free number, and an
	// unlock id of its own for it; XXX and AAA keep theirs
	ad = newAdopted("AAA", "XXX")
	_, st, assign := build(state, []InstalledSlotMod{mine}, ad, []adoptedLists{aaa, xxx()})
	if ad[0].NewSkins[0].Number != 601 || ad[1].NewSkins[0].Number != 602 {
		t.Fatalf("3: AAA %+v XXX %+v", ad[0].NewSkins, ad[1].NewSkins)
	}
	if len(assign) != 1 || assign[0].Number != 603 || assign[0].UnlockID == 1400689 || assign[0].UnlockID == 0 {
		t.Fatalf("3: ours %+v", assign)
	}
}

// A big mod whose files sort before the skin list (more than the window lists) is still seen to
// carry a skin list.
func TestBigModWithSkinListIsFound(t *testing.T) {
	mods := filepath.Join(t.TempDir(), "Mods")
	var files []string
	for i := 0; i < maxReplaceList+20; i++ {
		files = append(files, fmt.Sprintf("Live/Content/Characters/Pilot/T_Pilot_%03d.uasset", i))
	}
	files = append(files, "Live/Content/Datatables/Information/DT_Skin.uasset")
	writeMod(t, filepath.Join(mods, "BigPack"), "BigPack_P", 11, files)
	found := scanMods(mods)
	if len(found) != 1 || len(found[0].Replaces) != maxReplaceList {
		t.Fatalf("found %+v", found)
	}
	for _, r := range found[0].Replaces {
		if strings.HasPrefix(r, "Datatables/") {
			t.Fatal("the test needs the skin list to be cut from the window's list")
		}
	}
	if k := modKind(found[0]); k != "tables" {
		t.Errorf("kind %q", k)
	}
	if tm := findTableMods(Settings{ModsDir: mods}, nil); len(tm) != 1 || tm[0].Folder != "BigPack" {
		t.Errorf("table mods %+v", tm)
	}
}

// A skin from another mod's list that was moved off a number keeps its files at that number's
// paths, so a skin of ours with that number does not write its own files there.
func TestPathsInList(t *testing.T) {
	st, _ := synthGameLists()
	moved := synthSkinRow(21602001, gameMaterial(601), "X_Name")
	moved.Obj.Set(builtinSchema, "MenuIconImageRef", softPathTo("/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_601"))
	st.Rows = append(st.Rows, moved)
	in := pathsInList(st)
	for p, want := range map[string]bool{
		gameMaterial(601): true,
		"/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/601/":                       true, // where its damage material is too
		"/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_601":                          true,
		"/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/603/":                       false,
		"/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_603":                          false,
		"/Game/Vehicles/Aircraft/PP/PP0021_su57/Materials/601/MI_PP0021_su57_601_DAM": false,
	} {
		if in[p] != want {
			t.Errorf("%s: %v", p, in[p])
		}
	}
}
