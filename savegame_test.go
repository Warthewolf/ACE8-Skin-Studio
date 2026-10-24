package main

// The save tools, on saves made here with the same layout as the game's (no real save is needed).

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ------------------------------------------------------------------ making saves

func gvStr(s string) []byte {
	b := binary.LittleEndian.AppendUint32(nil, uint32(len(s)+1))
	return append(append(b, s...), 0)
}

func gvTypeBytes(t gvType) []byte {
	b := gvStr(t.Name)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(t.Sub)))
	for _, s := range t.Sub {
		b = append(b, gvTypeBytes(s)...)
	}
	return b
}

func gvTag(name string, t gvType, flags byte, val []byte) []byte {
	b := append(gvStr(name), gvTypeBytes(t)...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(val)))
	b = append(b, flags)
	return append(b, val...)
}

func gvListBytes(props ...[]byte) []byte {
	return append(bytes.Join(props, nil), gvStr("None")...)
}

var (
	tInt   = gvType{Name: "IntProperty"}
	tU32   = gvType{Name: "UInt32Property"}
	tBool  = gvType{Name: "BoolProperty"}
	tByte  = gvType{Name: "ByteProperty", Sub: []gvType{{Name: "None"}}}
	tLive  = []gvType{{Name: "/Script/Live"}}
	tArrIn = func(in gvType) gvType { return gvType{Name: "ArrayProperty", Sub: []gvType{in}} }
	tEnum  = func(n string) gvType {
		return gvType{Name: "EnumProperty", Sub: []gvType{{Name: n, Sub: tLive}, {Name: "ByteProperty"}}}
	}
	tStruct = func(n string) gvType { return gvType{Name: "StructProperty", Sub: []gvType{{Name: n, Sub: tLive}}} }
)

func i32s(v ...int32) []byte {
	b := binary.LittleEndian.AppendUint32(nil, uint32(len(v)))
	for _, x := range v {
		b = binary.LittleEndian.AppendUint32(b, uint32(x))
	}
	return b
}

func i32b(v int32) []byte { return binary.LittleEndian.AppendUint32(nil, uint32(v)) }

func structArray(name string, elems ...[]byte) []byte {
	b := binary.LittleEndian.AppendUint32(nil, uint32(len(elems)))
	return append(b, bytes.Join(elems, nil)...)
}

func unlockRule(id uint32, done bool, action string) []byte {
	var f byte
	if done {
		f = 0x10
	}
	return gvListBytes(
		gvTag("ID", tU32, 0, i32b(int32(id))),
		gvTag("bIsActivated", tBool, f, nil),
		gvTag("Category", tEnum("EMissionCategory"), 0, gvStr("EMissionCategory::Campaign")),
		gvTag("ActionType", tEnum("ELiveCAActionType"), 0, gvStr("ELiveCAActionType::"+action)))
}

func pilotData(skin int32, emblems ...int32) []byte {
	return gvListBytes(
		gvTag("PilotId", tEnum("ELivePilotID"), 0, gvStr("ELivePilotID::Player")),
		gvTag("SkinID", tInt, 0, i32b(skin)),
		gvTag("EmblemFlags", tByte, 0, []byte{0}),
		gvTag("EmblemIDs", tArrIn(tInt), 0, i32s(emblems...)))
}

// testSave makes a campaign save like the game's: the given unlocked skins and emblems, two unlock
// rules the game has done, one aircraft whose player and wingman wear the given skins and
// emblems, and an aircraft set.
func testSave(skins, emblems []int32, wear [2]int32, wearEmblem int32) []byte {
	common := gvListBytes(
		gvTag("LastAgreedEULAVersion", tInt, 0, i32b(1)),
		gvTag("UnlockData", tArrIn(tStruct("LiveUnlockSaveData")), 0, structArray("",
			unlockRule(1400001, true, "UnlockSkin"), unlockRule(1500001, true, "UnlockEmblem"), unlockRule(1300001, false, "UnlockMedal"))),
		gvTag("TotalPlayTimeSec", gvType{Name: "UInt64Property"}, 0, make([]byte, 8)),
		gvTag(saveSkinList, tArrIn(tU32), 0, i32s(skins...)),
		gvTag(saveNewSkinList, tArrIn(tU32), 0, i32s(skins...)),
		gvTag(saveEmblemList, tArrIn(tU32), 0, i32s(emblems...)),
		gvTag(saveNewEmblemList, tArrIn(tU32), 0, i32s()),
		gvTag("OwnedAircrafts", gvType{Name: "MapProperty", Sub: []gvType{tU32, {Name: "ByteProperty"}}}, 0, append(i32s(0, 1), 0x15, 0, 0, 0, 1)))
	record := gvListBytes(
		gvTag("PlaneID", tInt, 0, i32b(21010)),
		gvTag("PilotData", tArrIn(tStruct("LiveAircraftPilotSaveData")), 0, structArray("",
			pilotData(wear[0], wearEmblem, 0, 0, 0, 0, 0), pilotData(wear[1], 0, 0, 0, 0, 0, 0))))
	set := gvListBytes(
		gvTag("SlotName", gvType{Name: "StrProperty"}, 0, gvStr("SLOT 01")),
		gvTag("PlaneID", tInt, 0, i32b(21010)),
		gvTag("EmblemIDs", tArrIn(tInt), 0, i32s(wearEmblem, 0, 0, 0, 0, 0)),
		gvTag("SkinID", tInt, 0, i32b(wear[0])),
		gvTag("When", gvType{Name: "StructProperty", Sub: []gvType{{Name: "DateTime", Sub: []gvType{{Name: "/Script/CoreUObject"}}}}}, 0, make([]byte, 8)))
	campaign := gvListBytes(
		gvTag("AircraftTypeRecords", tArrIn(tStruct("LiveAircraftTypeSaveData")), 0, structArray("", record)),
		gvTag("AircraftSets", tArrIn(tStruct("LiveAircraftSetSaveData")), 0, structArray("", set)),
		gvTag("Flags", gvType{Name: "SetProperty", Sub: []gvType{tEnum("ELiveMiscFlagID")}}, 0, i32s(0, 0)))
	return testSaveFile("/Script/Live.LiveCampaignSaveGame", append(common, campaign...))
}

func testSaveFile(class string, packed []byte) []byte {
	b := []byte("GVAS")
	for _, v := range []int32{3, 522, 1012} {
		b = binary.LittleEndian.AppendUint32(b, uint32(v))
	}
	b = append(b, 5, 0, 4, 0, 3, 0)
	b = binary.LittleEndian.AppendUint32(b, 0)
	b = append(b, gvStr("++UE5+Release-5.4")...)
	b = append(b, i32b(3)...)
	b = append(b, i32b(1)...)
	b = append(b, make([]byte, 16)...)
	b = append(b, i32b(7)...)
	b = append(b, gvStr(class)...)
	b = append(b, 0)
	pd := append(i32b(int32(len(packed))), packed...)
	b = append(b, gvListBytes(
		gvTag("SavedVersion", tInt, 0, i32b(38)),
		gvTag("PackedData", tArrIn(gvType{Name: "ByteProperty"}), 0, pd),
		gvTag("Checksum", tU32, 0, i32b(int32(crc32.Update(saveChecksumSeed, crc32.IEEETable, packed)))))...)
	return append(b, 0, 0, 0, 0)
}

func readIDs(t *testing.T, f *saveFile, list string) map[int64]bool {
	t.Helper()
	v, _, err := saveIDs(f, list)
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64]bool{}
	for _, id := range v {
		out[int64(id)] = true
	}
	return out
}

func wornIDs(f *saveFile) (skins, emblems []int64) {
	f.walk(func(p *gvProp) {
		switch p.Name {
		case "SkinID":
			v, _ := p.int32Val()
			skins = append(skins, int64(v))
		case "EmblemIDs":
			v, _ := p.uint32s()
			emblems = append(emblems, int64(int32(v[0])))
		}
	})
	return
}

// ------------------------------------------------------------------ the format

func TestSaveReadsAndWritesBack(t *testing.T) {
	raw := testSave([]int32{21001001, 21002001}, []int32{1003}, [2]int32{21001001, -1}, 0)
	f, err := parseSave(raw)
	if err != nil {
		t.Fatal(err)
	}
	if f.Class != "/Script/Live.LiveCampaignSaveGame" || len(f.Packed) != 2 {
		t.Fatalf("class %q, %d lists", f.Class, len(f.Packed))
	}
	if !bytes.Equal(f.bytes(), raw) {
		t.Fatal("a save read and written again is not the same")
	}
	rules := f.prop(saveUnlockRules)
	if len(rules.Elems) != 3 || !rules.Elems[0].find("bIsActivated").boolVal() || rules.Elems[2].find("bIsActivated").boolVal() ||
		rules.Elems[1].find("ActionType").text() != actionUnlockEmblem {
		t.Fatalf("unlock rules read wrong: %d", len(rules.Elems))
	}
	if p := f.prop("AircraftSets"); len(p.Elems) != 1 || p.Elems[0].find("When").Val == nil {
		t.Fatal("aircraft sets read wrong (a DateTime stays as bytes)")
	}
	// a damaged save is refused
	bad := append([]byte(nil), raw...)
	bad[len(bad)-200] ^= 0x40
	if _, err := parseSave(bad); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("damaged save: %v", err)
	}
	if _, err := parseSave([]byte("PNG....")); err == nil {
		t.Fatal("not a save, but read")
	}
	if _, err := parseSave(raw[:len(raw)/2]); err == nil {
		t.Fatal("half a save, but read")
	}
	// a change: the sizes and the checksum follow
	_, p, _ := saveIDs(f, saveSkinList)
	p.setUint32s([]uint32{1, 2, 3, 4, 5})
	g, err := parseSave(f.bytes())
	if err != nil {
		t.Fatal(err)
	}
	if v := readIDs(t, g, saveSkinList); len(v) != 5 || !v[5] {
		t.Fatalf("changed list %v", v)
	}
	if len(g.bytes()) != len(raw)+12 {
		t.Fatalf("size %d, want %d", len(g.bytes()), len(raw)+12)
	}
}

func TestUnlockAndVanillaSave(t *testing.T) {
	newSkin, oldNew := skinIDOf(21, 601), skinIDOf(21, 602)
	newEmblem := emblemIDOf(601)
	raw := testSave([]int32{21001001, 21002001, int32(oldNew)}, []int32{1003}, [2]int32{int32(oldNew), int32(newSkin)}, int32(newEmblem))
	f, err := parseSave(raw)
	if err != nil {
		t.Fatal(err)
	}
	items := []saveItem{
		{Kind: "skin", Name: "Owl", ModTitle: "Night", ID: newSkin, UnlockID: 1400601},
		{Kind: "skin", Name: "Old owl", ModTitle: "Night", ID: oldNew, UnlockID: 1400602},
		{Kind: "emblem", Name: "Moon", ModTitle: "Night", ID: newEmblem, UnlockID: 1500213},
	}
	lines, err := unlockItems(f, items)
	if err != nil || len(lines) != 2 {
		t.Fatalf("unlock: %v %v", lines, err)
	}
	g, err := parseSave(f.bytes())
	if err != nil {
		t.Fatal(err)
	}
	if s := readIDs(t, g, saveSkinList); !s[newSkin] || !s[oldNew] || len(s) != 4 {
		t.Fatalf("skins %v", s)
	}
	if s := readIDs(t, g, saveNewSkinList); !s[newSkin] || len(s) != 4 {
		t.Fatalf("new skins %v", s)
	}
	if s := readIDs(t, g, saveEmblemList); !s[newEmblem] || !s[1003] {
		t.Fatalf("emblems %v", s)
	}
	if s := readIDs(t, g, saveNewEmblemList); !s[newEmblem] || len(s) != 1 {
		t.Fatalf("new emblems %v", s)
	}
	rules := g.prop(saveUnlockRules)
	if len(rules.Elems) != 6 {
		t.Fatalf("%d unlock rules", len(rules.Elems))
	}
	last := rules.Elems[5]
	if id, _ := last.find("ID").int32Val(); id != 1500213 || last.find("ActionType").text() != actionUnlockEmblem || !last.find("bIsActivated").boolVal() {
		t.Fatalf("emblem rule %d %s", id, last.find("ActionType").text())
	}
	// nothing more to do the second time
	if lines, _ := unlockItems(g, items); len(lines) != 0 {
		t.Fatalf("unlocked twice: %v", lines)
	}

	// the game's own lists: skin 001 of the Su-57 is the game's, and so (after an update) is 603
	game := gameIDs{Skins: map[int64]bool{21001001: true, 21002001: true, skinIDOf(21, 603): true}, SkinRules: map[int64]bool{1400001: true},
		Emblems: map[int64]bool{1003: true}, EmblemRules: map[int64]bool{1500001: true}}
	if isModSkin(skinIDOf(21, 603), game.Skins) || !isModSkin(newSkin, game.Skins) || isModSkin(21002001, nil) ||
		!isModEmblem(newEmblem, nil) || isModEmblem(emblemIDOf(450), nil) || isModEmblem(1220013003, nil) {
		t.Fatal("new ids told apart wrong")
	}
	lines, err = vanillaSave(g, game, map[int64]bool{1400602: true})
	if err != nil || len(lines) == 0 {
		t.Fatalf("vanilla: %v %v", lines, err)
	}
	h, err := parseSave(g.bytes())
	if err != nil {
		t.Fatal(err)
	}
	if s := readIDs(t, h, saveSkinList); len(s) != 2 || !s[21001001] || !s[21002001] {
		t.Fatalf("skins left %v", s)
	}
	if s := readIDs(t, h, saveNewSkinList); len(s) != 2 {
		t.Fatalf("new skins left %v", s)
	}
	if s := readIDs(t, h, saveEmblemList); len(s) != 1 || !s[1003] {
		t.Fatalf("emblems left %v", s)
	}
	skins, emblems := wornIDs(h)
	// the player's aircraft and the set go back to skin 001 (unlocked), the wingman's too
	for _, id := range skins {
		if id != 21001001 {
			t.Fatalf("worn skins %v", skins)
		}
	}
	for _, id := range emblems {
		if id != 0 {
			t.Fatalf("worn emblems %v", emblems)
		}
	}
	rules = h.prop(saveUnlockRules)
	var left []int32
	for _, e := range rules.Elems {
		id, _ := e.find("ID").int32Val()
		left = append(left, id)
	}
	if len(left) != 3 || left[0] != 1400001 || left[1] != 1500001 || left[2] != 1300001 {
		t.Fatalf("unlock rules left %v", left)
	}
	if lines, _ := vanillaSave(h, game, nil); len(lines) != 0 {
		t.Fatalf("second time: %v", lines)
	}
	// without the game's lists, and with skin 001 not unlocked: a skin goes to -1
	raw = testSave([]int32{21002001, int32(newSkin)}, nil, [2]int32{int32(newSkin), 0}, 0)
	f, _ = parseSave(raw)
	if _, err := vanillaSave(f, gameIDs{}, nil); err != nil {
		t.Fatal(err)
	}
	if skins, _ := wornIDs(f); skins[0] != -1 || skins[1] != 0 || skins[2] != -1 {
		t.Fatalf("worn skins %v", skins)
	}
}

// ------------------------------------------------------------------ the tools

func saveEnv(t *testing.T) Settings {
	root := t.TempDir()
	s := Settings{SaveDir: filepath.Join(root, "Saved", "SaveGames"), Workspace: filepath.Join(root, "ws"), ModsDir: filepath.Join(root, "mods")}
	os.MkdirAll(s.SaveDir, 0o755)
	gameRunning = func() string { return "" }
	t.Cleanup(func() { gameRunning = findRunningGame })
	return s
}

func TestSaveBackupsAndRestore(t *testing.T) {
	s := saveEnv(t)
	j := testJob()
	if _, err := makeSaveBackup(s, "x"); err == nil {
		t.Fatal("backed up a save that is not there")
	}
	if _, err := doRestoreBackup(j, s, "nope", false); err == nil {
		t.Fatal("restored a backup that is not there")
	}
	camp := testSave([]int32{21001001}, nil, [2]int32{0, 0}, 0)
	system := testSaveFile("/Script/Live.LiveSystemSaveGame", gvListBytes(gvTag("Version", tInt, 0, i32b(0))))
	os.WriteFile(filepath.Join(s.SaveDir, campaignSave), camp, 0o644)
	os.WriteFile(filepath.Join(s.SaveDir, systemSave), system, 0o644)
	os.WriteFile(filepath.Join(s.SaveDir, "steam_autocloud.vdf"), []byte("x"), 0o644)
	b1, err := makeSaveBackup(s, "made by hand")
	if err != nil {
		t.Fatal(err)
	}
	b2, err := makeSaveBackup(s, "again")
	if err != nil || b2.ID == b1.ID || len(b1.Files) != 2 {
		t.Fatalf("second backup %+v %v", b2, err)
	}
	// one made when the game started
	auto := filepath.Join(s.saveBackupsDir(), autoBackupFolder)
	os.MkdirAll(auto, 0o755)
	old := testSave([]int32{21001001, 21002001}, nil, [2]int32{0, 0}, 0)
	os.WriteFile(filepath.Join(auto, "03 Campaign.sav"), old, 0o644)
	os.WriteFile(filepath.Join(auto, "03 System.sav"), system, 0o644)
	os.WriteFile(filepath.Join(auto, "03.txt"), []byte("2026-01-02 10:11:12\n"), 0o644)
	os.WriteFile(filepath.Join(auto, "04 Campaign.sav"), []byte("broken"), 0o644)
	list := listSaveBackups(s)
	if len(list) != 4 {
		t.Fatalf("backups %+v", list)
	}
	var a3 *saveBackup
	for i := range list {
		if list[i].ID == "auto/03" {
			a3 = &list[i]
		}
	}
	if a3 == nil || !a3.Auto || a3.When.Year() != 2026 || a3.When.Month() != time.January || len(a3.Files) != 2 || list[len(list)-1].ID != "auto/03" {
		t.Fatalf("auto backup %+v", a3)
	}
	for _, bad := range []string{"../x", "auto/../../x", "a/b", "", ".."} {
		if _, err := s.backupPath(bad, campaignSave); err == nil {
			t.Errorf("backup id %q allowed", bad)
		}
	}
	// the game running stops it
	gameRunning = func() string { return "AceCombat8.exe" }
	if _, err := doRestoreBackup(j, s, "auto/03", false); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("restored with the game running: %v", err)
	}
	gameRunning = func() string { return "" }
	if _, err := doRestoreBackup(j, s, "auto/04", false); err == nil || !strings.Contains(err.Error(), "damaged") {
		t.Fatalf("restored a broken backup: %v", err)
	}
	res, err := doRestoreBackup(j, s, "auto/03", false)
	if err != nil || !res.Changed || !res.Cloud || res.Backup.ID == "" {
		t.Fatalf("restore %+v %v", res, err)
	}
	if b, _ := os.ReadFile(filepath.Join(s.SaveDir, campaignSave)); !bytes.Equal(b, old) {
		t.Fatal("the backup was not put back")
	}
	// the save as it was before is in the new backup
	p, _ := s.backupPath(res.Backup.ID, campaignSave)
	if b, _ := os.ReadFile(p); !bytes.Equal(b, camp) {
		t.Fatal("the save before the restore was not backed up")
	}
	if _, err := os.Stat(filepath.Join(s.SaveDir, campaignSave+".skinstudio-new")); err == nil {
		t.Fatal("the temporary file was left")
	}
	// a system save is not a campaign save
	os.WriteFile(filepath.Join(auto, "05 Campaign.sav"), system, 0o644)
	if _, err := doRestoreBackup(j, s, "auto/05", false); err == nil {
		t.Fatal("a system save was put back as the campaign")
	}
	for _, x := range isGameExeCases {
		if isGameExe(x.name) != x.want {
			t.Errorf("isGameExe(%q)", x.name)
		}
	}
}

var isGameExeCases = []struct {
	name string
	want bool
}{{"AceCombat8.exe", true}, {"acecombat8-win64-shipping.exe", true}, {"AceCombat7.exe", false}, {"explorer.exe", false}}

func TestSaveBackupMod(t *testing.T) {
	s := saveEnv(t)
	ue := filepath.Join(t.TempDir(), "ue4ss", "Mods")
	os.MkdirAll(ue, 0o755)
	os.WriteFile(filepath.Join(ue, "mods.txt"), []byte("x : 1\r\n"), 0o644)
	if st := saveBackupModStatus(s); st.Problem == "" {
		t.Fatal("no UE4SS, but no problem")
	}
	s.UE4SSMods = ue
	keepSaveBackupMod(s)
	st := saveBackupModStatus(s)
	if !st.Installed || !st.Current {
		t.Fatalf("not installed: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(s.saveBackupsDir(), autoBackupFolder)); err != nil {
		t.Fatal("the folder for its backups was not made")
	}
	lua, _ := os.ReadFile(filepath.Join(ue, saveBackupMod, "Scripts", "main.lua"))
	if !bytes.Contains(lua, []byte("SkinStudioSaveBackups\\\\auto")) || !bytes.Contains(lua, []byte("KEEP = 20")) {
		t.Fatal("main.lua is not the backup script")
	}
	// a save folder that is not the usual one is written down for it
	if b, err := os.ReadFile(filepath.Join(ue, saveBackupMod, "Scripts", "paths.txt")); err != nil || !strings.HasPrefix(string(b), s.SaveDir+"\r\n") {
		t.Fatalf("paths.txt %q %v", b, err)
	}
	if err := removeSaveBackupMod(s); err != nil {
		t.Fatal(err)
	}
	s.SaveBackupOff = true
	keepSaveBackupMod(s)
	if st := saveBackupModStatus(s); st.Installed {
		t.Fatal("put back after it was taken out")
	}
}

// The whole thing with a real skin list: build a skin and an emblem, unlock them in the save,
// then prepare the save for the game without mods.
func TestSaveToolsEndToEnd(t *testing.T) {
	e := newSlotEnv(t)
	addEmblemGame(t, e)
	if _, ok := builtinSchema.Structs[emblemRowStruct]; !ok {
		needFile(t, testUsmap)
		e.s.Usmap = testUsmap
	}
	save := saveEnv(t)
	e.s.SaveDir = save.SaveDir
	j := testJob()
	if _, err := doUnlockNow(j, e.s); err == nil {
		t.Fatal("unlocked with nothing built")
	}
	res, err := doBuildEmblem(j, e.s, emblemReq{ModTitle: "Night Owls", Name: "Night Owl", Picture: emblemPicture(t)})
	if err != nil {
		t.Fatal(err)
	}
	mod := res.(emblemResult).Mod
	if _, err := doBuildSlot(j, e.s, slotReq{planReq: planReq{SkinDir: e.rawDir}, Mod: mod, Name: "Owl skin", Icon: "base"}); err != nil {
		t.Fatal(err)
	}
	items := installedSaveItems(e.s)
	if len(items) != 2 {
		t.Fatalf("items %+v", items)
	}
	var skin, emblem saveItem
	for _, it := range items {
		if it.Kind == "skin" {
			skin = it
		} else {
			emblem = it
		}
	}
	if skin.ID != skinIDOf(21, 601) || skin.Name != "Owl skin" || skin.UnlockID == 0 || emblem.ID != emblemIDOf(601) || emblem.UnlockID != 1500213 || emblem.ModTitle != "Night Owls" {
		t.Fatalf("skin %+v emblem %+v", skin, emblem)
	}
	if _, err := doUnlockNow(j, e.s); err == nil || !strings.Contains(err.Error(), "Campaign.sav") {
		t.Fatalf("unlocked without a save: %v", err)
	}
	camp := testSave([]int32{21001001}, []int32{1003}, [2]int32{21001001, 0}, 0)
	path := filepath.Join(e.s.SaveDir, campaignSave)
	os.WriteFile(path, camp, 0o644)
	st := saveStatus(e.s)
	if !st.Found || st.Problem != "" || len(st.Items) != 2 || st.Items[0].Unlocked || st.Items[1].Unlocked {
		t.Fatalf("status before %+v", st)
	}
	gameRunning = func() string { return "AceCombat8.exe" }
	if _, err := doUnlockNow(j, e.s); err == nil {
		t.Fatal("changed the save with the game running")
	}
	gameRunning = func() string { return "" }
	ch, err := doUnlockNow(j, e.s)
	if err != nil || !ch.Changed || len(ch.Lines) != 2 || ch.Backup.ID == "" {
		t.Fatalf("unlock %+v %v", ch, err)
	}
	st = saveStatus(e.s)
	if !st.Items[0].Unlocked || !st.Items[1].Unlocked || len(st.Backups) != 1 {
		t.Fatalf("status after %+v", st)
	}
	again, err := doUnlockNow(j, e.s)
	if err != nil || again.Changed {
		t.Fatalf("second unlock %+v %v", again, err)
	}
	// wear them, as the game would after picking them in the hangar
	b, _ := os.ReadFile(path)
	f, _ := parseSave(b)
	f.walk(func(p *gvProp) {
		if p.Name == "SkinID" {
			p.setInt32(int32(skin.ID))
		}
		if p.Name == "EmblemIDs" {
			p.setUint32s([]uint32{uint32(emblem.ID), 0, 0, 0, 0, 0})
		}
	})
	os.WriteFile(path, f.bytes(), 0o644)
	if st := saveStatus(e.s); st.Worn != 6 {
		t.Fatalf("worn %d", st.Worn)
	}
	ch, err = doPrepareVanilla(j, e.s)
	if err != nil || !ch.Changed {
		t.Fatalf("vanilla %+v %v", ch, err)
	}
	b, _ = os.ReadFile(path)
	f, err = parseSave(b)
	if err != nil {
		t.Fatal(err)
	}
	if s := readIDs(t, f, saveSkinList); s[skin.ID] || !s[21001001] {
		t.Fatalf("skins %v", s)
	}
	if s := readIDs(t, f, saveEmblemList); s[emblem.ID] || !s[1003] {
		t.Fatalf("emblems %v", s)
	}
	skins, emblems := wornIDs(f)
	for i := range skins {
		if skins[i] != 21001001 {
			t.Fatalf("worn skins %v", skins)
		}
	}
	for i := range emblems {
		if emblems[i] != 0 {
			t.Fatalf("worn emblems %v", emblems)
		}
	}
	for _, r := range f.prop(saveUnlockRules).Elems {
		if id, _ := r.find("ID").int32Val(); int64(id) == skin.UnlockID || int64(id) == emblem.UnlockID {
			t.Fatalf("rule %d left", id)
		}
	}
	if st := saveStatus(e.s); st.Worn != 0 || st.Items[0].Unlocked || len(st.Backups) != 2 {
		t.Fatalf("status after vanilla %+v", st)
	}
	// and back: the backup made before it puts the unlocked save back
	list := listSaveBackups(e.s)
	if _, err := doRestoreBackup(j, e.s, list[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if st := saveStatus(e.s); st.Worn != 6 || !st.Items[0].Unlocked {
		t.Fatalf("status after restore %+v", st)
	}
	// SkinSlotText's install put the game-start backup in too
	if st := saveBackupModStatus(e.s); !st.Current {
		t.Fatalf("game-start backup not installed: %+v", st)
	}
}
