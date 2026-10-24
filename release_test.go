package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- choosing between sets

func TestChoosingASet(t *testing.T) {
	uexp, _ := synthVT(5, 1, 64, 32, 4, "PF_DXT1", 8)
	vt, err := ParseVTex(uexp)
	if err != nil {
		t.Fatal(err)
	}
	files := append(tileNames("D:/s", "T_X_001_C", 5, ".png"), tileNames("D:/s", "T_X_0GR_C", 3, ".png")...)
	names := []string{"T_X_002_C"}
	rec := PlanRecipe(KindColour, "T_X_002_C", vt, FindPainted(files, names, nil))
	if rec.Ready || len(rec.Choices) != 1 || rec.Choices[0].Key != wholeKey(KindColour) || len(rec.Choices[0].Options) != 2 || rec.Choices[0].Picked != "" {
		t.Fatalf("an unsettled clash should offer a choice: %+v", rec)
	}
	if o := rec.Choices[0].Options; o[0].Name != "T_X_001_C" || o[0].Tiles != 5 || o[1].Name != "T_X_0GR_C" || o[1].Tiles != 3 {
		t.Fatalf("options wrong: %+v", o)
	}
	pick := map[string]string{wholeKey(KindColour): "t_x_001_c"} // case does not matter
	sets := FindPainted(files, names, pick)
	rec = PlanRecipe(KindColour, "T_X_002_C", vt, sets)
	if !rec.Ready || rec.Choices[0].Picked != "T_X_001_C" || len(rec.Sources) != 1 || len(rec.Sources[0].Files) != 5 {
		t.Fatalf("a pick should settle it: %+v", rec)
	}
	if filepath.Base(rec.Sources[0].Files[4]) != "T_X_001_C.1005.png" || rec.Sources[0].Name != "T_X_001_C" || rec.Sources[0].Cols != 5 {
		t.Fatalf("source list wrong: %+v", rec.Sources[0])
	}
	// the files are named after skin 001 while 002 is rebuilt: worth a note
	if len(rec.Notes) != 1 || !strings.HasPrefix(rec.Notes[0], "These pictures are named after T_X_001_C, not after") {
		t.Fatalf("expected a note about the other skin: %+v", rec.Notes)
	}
	// picking the short set is reported as short, not silently accepted
	rec = PlanRecipe(KindColour, "T_X_002_C", vt, FindPainted(files, names, map[string]string{wholeKey(KindColour): "T_X_0GR_C"}))
	if rec.Ready || !strings.Contains(rec.Missing, "only 3 of 5") {
		t.Fatalf("short pick: %+v", rec)
	}
	// a pick that names nothing leaves the question open
	rec = PlanRecipe(KindColour, "T_X_002_C", vt, FindPainted(files, names, map[string]string{wholeKey(KindColour): "gone"}))
	if rec.Ready || rec.Choices[0].Picked != "" {
		t.Fatalf("stale pick: %+v", rec)
	}
}

func TestSourcesAndNotes(t *testing.T) {
	uexp, _ := synthVT(6, 1, 64, 32, 4, "PF_DXT1", 8)
	vt, _ := ParseVTex(uexp)
	var files []string
	files = append(files, tileNames("D:/s", "T_PP0018_fa36_001_MRX_Metallic", 6, ".png")...)
	files = append(files, tileNames("D:/s", "T_PP0018_fa36_001_MRX_Roughness", 6, ".png")...)
	rec := PlanRecipe(KindFinish, "T_PP0018_fa36_002_MRX", vt, FindPainted(files, []string{"T_PP0018_fa36_002_MRX"}, nil))
	if !rec.Ready || len(rec.Sources) != 3 || rec.Sources[0].Label != "metallic" || rec.Sources[1].Label != "roughness" || !rec.Sources[2].Kept || rec.Sources[2].Label != "blue mask" {
		t.Fatalf("sources: %+v", rec.Sources)
	}
	if len(rec.Notes) != 1 || !strings.HasPrefix(rec.Notes[0], "The metallic and roughness pictures are named after T_PP0018_fa36_001_MRX, not after the texture being rebuilt (T_PP0018_fa36_002_MRX)") {
		t.Fatalf("notes: %+v", rec.Notes)
	}
	// same skin, and Painter-style names: nothing to remark on
	same := append(tileNames("D:/s", "T_PP0018_fa36_002_MRX_Metallic", 6, ".png"), tileNames("D:/s", "MySkin_Roughness", 6, ".png")...)
	if rec := PlanRecipe(KindFinish, "T_PP0018_fa36_002_MRX", vt, FindPainted(same, nil, nil)); !rec.Ready || len(rec.Notes) != 0 {
		t.Fatalf("unexpected notes: %+v", rec)
	}
	for stem, want := range map[string]string{"T_PP0018_fa36_001_MRX_Metallic": "T_PP0018_fa36_001_MRX", "T_PP0018_fa36_0GR_C": "T_PP0018_fa36_0GR_C",
		"t_x_101_no_Normal_DirectX": "t_x_101_NO", "MySkin_Base_color": "", "T_PP0021_su57_DirtMask": ""} {
		if got := namedAfter(stem); got != want {
			t.Errorf("namedAfter(%s) = %q, want %q", stem, got, want)
		}
	}
}

func TestSkinFolderName(t *testing.T) {
	for name, want := range map[string]string{"T_PP0018_fa36_001_C.png": "T_PP0018_fa36_001", "T_PP0018_fa36_001_MRX.tga": "T_PP0018_fa36_001",
		"T_PP0018_fa36_001_NO.png": "T_PP0018_fa36_001", "T_X_101_C2.png": "T_X_101", "my skin.png": "my skin", "a:b?.png": "a_b", "...png": "tiles"} {
		if got := skinFolderName(name); got != want {
			t.Errorf("skinFolderName(%s) = %q, want %q", name, got, want)
		}
	}
}

// ---- failure advice

func TestExplainToolFailure(t *testing.T) {
	cases := []struct {
		line    string
		packing bool
		want    string
	}{
		{"Error: missing encryption key", false, "does not open the game's files"},
		{"Error: container is encrypted but no AES key for FGuid supplied", false, "does not open the game's files"},
		{"error: invalid value for '--aes-key': invalid AES key", false, "does not accept the AES key"},
		{"Error: Oodle decompression failed: expected 65536 output bytes, got 0", false, "AES key is probably wrong"},
		{"thread 'main' panicked at src/lib.rs", false, "AES key is probably wrong"},
		{"Error: invalid EIoStoreTocVersion value", false, "newest retoc"},
		{"Error: failed to download oodle", false, "Oodle library"},
		{"Error: There is not enough space on the disk. (os error 112)", false, "disk is full"},
		{"Error: The process cannot access the file because it is being used by another process. (os error 32)", true, "in use"},
		{"Error: Access is denied. (os error 5)", true, "refused access"},
		{"Error: The system cannot find the path specified. (os error 3)", false, "Paks folder"},
		{"Error: something new", false, "If this happens for every aircraft"},
		{"Error: something new", true, "could not pack the mod"},
	}
	for _, c := range cases {
		if got := explainToolFailure([]string{"INFO reading", c.line}, c.packing); !strings.Contains(got, c.want) {
			t.Errorf("%q (packing %v): got %q, want it to mention %q", c.line, c.packing, got, c.want)
		}
	}
}

// ---- installed mods

// synthTOC builds a small unencrypted container table like the ones retoc writes.
func synthTOC(containerID uint64, mount string, files []string) []byte {
	fstr := func(s string) []byte {
		b := make([]byte, 4, 5+len(s))
		binary.LittleEndian.PutUint32(b, uint32(len(s)+1))
		return append(append(b, s...), 0)
	}
	u32 := func(v uint32) []byte { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); return b }
	const none = 0xFFFFFFFF
	var names []string
	nameIx := map[string]uint32{}
	intern := func(s string) uint32 {
		if i, ok := nameIx[s]; ok {
			return i
		}
		nameIx[s] = uint32(len(names))
		names = append(names, s)
		return nameIx[s]
	}
	type dir struct{ name, child, sibling, file uint32 }
	type file struct{ name, next, user uint32 }
	dirs := []dir{{none, none, none, none}}
	var fileEntries []file
	var ids [][12]byte
	for _, f := range files {
		parts := strings.Split(f, "/")
		at := uint32(0)
		for _, p := range parts[:len(parts)-1] {
			found := uint32(none)
			for c := dirs[at].child; c != none; c = dirs[c].sibling {
				if names[dirs[c].name] == p {
					found = c
				}
			}
			if found == none {
				dirs = append(dirs, dir{intern(p), none, dirs[at].child, none})
				found = uint32(len(dirs) - 1)
				dirs[at].child = found
			}
			at = found
		}
		// the id is made from the path without its ending, so .uasset and .ubulk share the first 8 bytes
		var id [12]byte
		stem := strings.TrimSuffix(f, filepath.Ext(f))
		h := uint64(14695981039346656037)
		for _, c := range []byte(strings.ToLower(stem)) {
			h = (h ^ uint64(c)) * 1099511628211
		}
		binary.LittleEndian.PutUint64(id[:], h)
		id[11] = 1
		if strings.HasSuffix(f, ".ubulk") {
			id[11] = 2
		}
		fileEntries = append(fileEntries, file{intern(parts[len(parts)-1]), dirs[at].file, uint32(len(ids))})
		dirs[at].file = uint32(len(fileEntries) - 1)
		ids = append(ids, id)
	}
	var header [12]byte // the container's own header chunk
	binary.LittleEndian.PutUint64(header[:], containerID)
	header[11] = 6
	ids = append(ids, header)

	var d []byte
	d = append(d, fstr(mount)...)
	d = append(d, u32(uint32(len(dirs)))...)
	for _, e := range dirs {
		d = append(append(append(append(d, u32(e.name)...), u32(e.child)...), u32(e.sibling)...), u32(e.file)...)
	}
	d = append(d, u32(uint32(len(fileEntries)))...)
	for _, e := range fileEntries {
		d = append(append(append(d, u32(e.name)...), u32(e.next)...), u32(e.user)...)
	}
	d = append(d, u32(uint32(len(names)))...)
	for _, n := range names {
		d = append(d, fstr(n)...)
	}
	const blocks = 3
	b := make([]byte, 144)
	copy(b, tocMagic)
	b[16] = 6
	binary.LittleEndian.PutUint32(b[20:], 144)
	binary.LittleEndian.PutUint32(b[24:], uint32(len(ids)))
	binary.LittleEndian.PutUint32(b[28:], blocks)
	binary.LittleEndian.PutUint32(b[32:], 12)
	binary.LittleEndian.PutUint32(b[40:], 32)
	binary.LittleEndian.PutUint32(b[48:], uint32(len(d)))
	binary.LittleEndian.PutUint64(b[56:], containerID)
	b[80] = 8 // indexed
	for _, id := range ids {
		b = append(b, id[:]...)
	}
	b = append(b, make([]byte, len(ids)*10+blocks*12)...)
	b = append(b, d...)
	return append(b, make([]byte, len(ids)*33)...)
}

func writeMod(t *testing.T, dir, base string, containerID uint64, files []string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, base+".utoc"), synthTOC(containerID, "../../../", files), 0o644)
	os.WriteFile(filepath.Join(dir, base+".ucas"), make([]byte, 5000), 0o644)
	os.WriteFile(filepath.Join(dir, base+".pak"), make([]byte, 347), 0o644)
}

func TestInstalledMods(t *testing.T) {
	root := t.TempDir()
	mods := filepath.Join(root, "Mods")
	tex := "Live/Content/Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_C"
	writeMod(t, filepath.Join(mods, "Su57Navy"), "Su57Navy_P", 1, []string{tex + ".uasset", tex + ".ubulk"})
	writeMod(t, filepath.Join(mods, "Su57Desert"), "Su57Desert_P", 2, []string{tex + ".uasset", tex + ".ubulk",
		"Live/Content/Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_MRX.uasset"})
	writeMod(t, filepath.Join(mods, "Sky"), "Sky", 3, []string{"Live/Content/Cloudly/CSP_Night.uasset"})
	writeMod(t, mods, "Loose_P", 4, []string{"Live/Content/Other/Thing.uasset"}) // lying directly in the mods folder
	os.MkdirAll(filepath.Join(mods, "dlls"), 0o755)                              // not a mod
	os.WriteFile(filepath.Join(mods, "dlls", "main.dll"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(mods, "Tool", "packed"), 0o755) // a container one level deeper is not a mod either
	os.WriteFile(filepath.Join(mods, "Tool", "packed", "x.utoc"), synthTOC(9, "../../../", nil), 0o644)
	os.WriteFile(filepath.Join(mods, "Broken.utoc"), []byte("not a container"), 0o644)

	found := scanMods(mods)
	by := map[string]InstalledMod{}
	for _, m := range found {
		by[m.Name] = m
	}
	if len(found) != 5 {
		t.Fatalf("found %d mods, want 5: %+v", len(found), by)
	}
	navy := by["Su57Navy"]
	want := "Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_C"
	if len(navy.Replaces) != 1 || navy.Replaces[0] != want || len(navy.Files) != 3 || navy.Size != 5000+347+int64(len(synthTOC(1, "../../../", []string{tex + ".uasset", tex + ".ubulk"}))) {
		t.Fatalf("Su57Navy read wrongly: %+v", navy)
	}
	if len(navy.Conflicts) != 1 || navy.Conflicts[0].With != "Su57Desert" || len(navy.Conflicts[0].Files) != 1 || navy.Conflicts[0].Files[0] != want || navy.Conflicts[0].Same {
		t.Fatalf("conflict not found: %+v", navy.Conflicts)
	}
	if d := by["Su57Desert"]; len(d.Replaces) != 2 || len(d.Conflicts) != 1 || d.Conflicts[0].With != "Su57Navy" {
		t.Fatalf("Su57Desert: %+v", d)
	}
	if s := by["Sky"]; len(s.Conflicts) != 0 || len(s.Replaces) != 1 || s.Replaces[0] != "Cloudly/CSP_Night" {
		t.Fatalf("Sky: %+v", s)
	}
	if l := by["Loose_P"]; !l.Loose || len(l.Files) != 3 || l.Dir != mods {
		t.Fatalf("loose mod: %+v", l)
	}
	if b := by["Broken"]; b.Note == "" || len(b.Replaces) != 0 {
		t.Fatalf("broken container should carry a note: %+v", b)
	}

	// removing moves the files into the workspace and takes the conflict away
	s := Settings{ModsDir: mods, Workspace: filepath.Join(root, "ws")}
	dest, err := removeMod(s, "Su57Desert", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "Su57Desert_P.ucas")); err != nil || !strings.HasPrefix(dest, s.removedModsDir()) {
		t.Fatalf("removed mod not kept: %v %s", err, dest)
	}
	if _, err := os.Stat(filepath.Join(mods, "Su57Desert")); err == nil {
		t.Fatal("mod folder still in the mods folder")
	}
	dest, err = removeMod(s, "Loose_P", true)
	if err != nil {
		t.Fatal(err)
	}
	if es, _ := os.ReadDir(dest); len(es) != 3 {
		t.Fatalf("loose mod: moved %d files, want 3", len(es))
	}
	for _, m := range scanMods(mods) {
		if m.Name == "Su57Navy" && len(m.Conflicts) != 0 {
			t.Fatalf("conflict should be gone: %+v", m.Conflicts)
		}
		if m.Name == "Loose_P" || m.Name == "Su57Desert" {
			t.Fatalf("%s is still listed", m.Name)
		}
	}
	if _, err := removeMod(s, "dlls", false); err == nil {
		t.Fatal("a folder that is not a mod must not be removable")
	}
	if _, err := removeMod(s, "../ws", false); err == nil {
		t.Fatal("names outside the mods folder must be refused")
	}
	if _, err := os.Stat(filepath.Join(mods, "dlls", "main.dll")); err != nil {
		t.Fatal("unrelated folder was touched")
	}
	// two mods that are the same container inside
	writeMod(t, filepath.Join(mods, "CopyA"), "Same_P", 77, []string{"Live/Content/A.uasset"})
	writeMod(t, filepath.Join(mods, "CopyB"), "Same_P", 77, []string{"Live/Content/B.uasset"})
	for _, m := range scanMods(mods) {
		if m.Name == "CopyA" && (len(m.Conflicts) != 1 || !m.Conflicts[0].Same) {
			t.Fatalf("same container not flagged: %+v", m.Conflicts)
		}
	}
	// one of the game's own containers lying in the folder is not read as a mod
	big := append(synthTOC(8, "../../../", []string{"Live/Content/Big.uasset"}), make([]byte, maxModTOC)...)
	os.WriteFile(filepath.Join(mods, "pakchunk0-Windows.utoc"), big, 0o644)
	for _, m := range scanMods(mods) {
		if m.Name == "pakchunk0-Windows" && (m.Note == "" || len(m.Replaces) != 0 || len(m.Conflicts) != 0) {
			t.Fatalf("a huge container should be skipped with a note: %+v", m)
		}
	}
	if scanMods(filepath.Join(root, "nowhere")) != nil || scanMods("") != nil {
		t.Fatal("a missing folder should give nil")
	}
}

// Real containers written by retoc, when they are on this machine.
func TestRealContainers(t *testing.T) {
	base := "/mnt/user-data/uploads/ue4ss/Mods/IoStoreLoaderMod"
	b, err := os.ReadFile(filepath.Join(base, "Su57Navy", "Su57Navy_P.utoc"))
	if err != nil {
		t.Skip("no real containers here")
	}
	info, err := parseTOC(b)
	if err != nil || len(info.ChunkIDs) != 3 || len(info.Names) != 2 {
		t.Fatalf("real container: %v %+v", err, info)
	}
	if gameFileLabel(info.Names[0]) != "Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_C" {
		t.Fatalf("name read wrongly: %q", info.Names[0])
	}
	if mods := scanMods(base); len(mods) < 3 {
		t.Fatalf("expected the real mods to be listed, got %d", len(mods))
	}
}

func TestDamagedContainers(t *testing.T) {
	good := synthTOC(5, "../../../", []string{"Live/Content/A/B.uasset", "Live/Content/A/C.uasset"})
	for cut := 0; cut < len(good); cut += 7 {
		parseTOC(good[:cut]) // must not panic
	}
	for i := 144; i < len(good); i += 3 {
		bad := append([]byte(nil), good...)
		bad[i] ^= 0xFF
		parseTOC(bad) // must not panic or hang
	}
}

// ---- the report

func TestReportHidesTheKey(t *testing.T) {
	key := strings.Repeat("AB12", 16)
	sc := scrubber{key: key, home: `C:\Users\Someone`}
	in := "retoc --aes-key 0x" + key + " list\nlower " + strings.ToLower(key) + "\nother " + strings.Repeat("0f", 32) + "\npath c:\\users\\someone\\Desktop\\x and C:/Users/Someone/y"
	out := sc.clean(in)
	if strings.Contains(strings.ToLower(out), "ab12ab12") || strings.Contains(out, strings.Repeat("0f", 32)) {
		t.Fatalf("key left in: %s", out)
	}
	if strings.Contains(strings.ToLower(out), "someone") || !strings.Contains(out, `<home>\Desktop\x`) || !strings.Contains(out, "<home>/y") {
		t.Fatalf("user name left in: %s", out)
	}
	dataDir = t.TempDir()
	settings = Settings{AESKey: "0x" + key, PaksDir: filepath.Join(dataDir, "Paks")}
	defaults(&settings)
	rep := buildReport()
	if strings.Contains(strings.ToLower(rep), "ab12ab12") || !strings.Contains(rep, "AES key: set, never included") || !strings.Contains(rep, appVersion) {
		t.Fatalf("report wrong:\n%s", rep)
	}
}

// ---- one wide picture instead of tiles

func TestWidePicture(t *testing.T) {
	uexp, ubulk := synthVT(5, 1, 64, 32, 4, "PF_DXT1", 8)
	vt, err := ParseVTex(uexp)
	if err != nil {
		t.Fatal(err)
	}
	if err := vt.ResolveChunks(uexp, ubulk); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	pic := smoothPicture(vt.Width, vt.Height, 5, 1, 3)
	save := func(sub, name string, im *Img) string {
		p := filepath.Join(dir, sub, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := SaveImage(p, im, SaveRGB); err != nil {
			t.Fatal(err)
		}
		return p
	}
	names := []string{"T_X_002_C"}
	// the whole skin as one picture, named the game's way after another skin
	full := save("a", "T_X_0GR_C.png", pic)
	sets := FindPainted([]string{full}, names, nil)
	rec := PlanRecipe(KindColour, "T_X_002_C", vt, sets)
	if !rec.Ready || len(rec.Sources) != 1 || !rec.Sources[0].Wide || rec.Sources[0].Cols != 5 || rec.Sources[0].Rows != 1 || rec.Sources[0].Files[0] != full {
		t.Fatalf("one wide picture should be accepted: %+v", rec)
	}
	if len(rec.Notes) != 1 || !strings.HasPrefix(rec.Notes[0], "This picture is named after T_X_0GR_C, not after the texture being rebuilt (T_X_002_C). That is fine if it is the picture you mean to use.") {
		t.Fatalf("note for a single wide picture: %+v", rec.Notes)
	}
	whole, _, err := Compose(KindColour, "T_X_002_C", vt, ubulk, sets)
	if err != nil || meanAbsDiff(whole, pic) != 0 {
		t.Fatalf("wide picture did not come through unchanged: %v", err)
	}
	// the same pixels as tiles give exactly the same picture
	outs, err := WriteTiles(pic, 5, 1, KindColour, false, filepath.Join(dir, "t"), "T_X_0GR_C", ".png", nil)
	if err != nil || len(outs) != 5 {
		t.Fatal(err)
	}
	var tilePaths []string
	for _, o := range outs {
		tilePaths = append(tilePaths, o.Path)
	}
	fromTiles, _, err := Compose(KindColour, "T_X_002_C", vt, ubulk, FindPainted(tilePaths, names, nil))
	if err != nil || meanAbsDiff(fromTiles, whole) != 0 {
		t.Fatalf("tiles and wide picture differ: %v", err)
	}
	// Painter-style name, half size: accepted, with a note that it will be resized
	half := save("b", "MySkin_Base_color.tga", resize(pic, vt.Width/2, vt.Height/2))
	rec = PlanRecipe(KindColour, "T_X_002_C", vt, FindPainted([]string{half}, names, nil))
	if !rec.Ready || len(rec.Notes) != 1 || !strings.Contains(rec.Notes[0], "will be resized to 320 x 64") {
		t.Fatalf("half-size wide picture: %+v", rec)
	}
	// one square without a tile number is the wrong shape, and that is said before building
	square := save("c", "MySkin_Base_color.png", resize(pic, 64, 64))
	rec = PlanRecipe(KindColour, "T_X_002_C", vt, FindPainted([]string{square}, names, nil))
	if rec.Ready || !strings.Contains(rec.Missing, "64 x 64, which is not the shape of this texture (320 x 64)") || !strings.Contains(rec.Missing, "all 5 tiles") {
		t.Fatalf("wrong shape should be refused at once: %+v", rec)
	}
	// two wide pictures of the same channel: a choice, like with tiles
	other := save("a", "T_X_101_C.png", pic)
	rec = PlanRecipe(KindColour, "T_X_002_C", vt, FindPainted([]string{full, other}, names, nil))
	if rec.Ready || len(rec.Choices) != 1 || rec.Choices[0].Options[0].Tiles != 0 {
		t.Fatalf("two wide pictures should offer a choice: %+v", rec)
	}
	rec = PlanRecipe(KindColour, "T_X_002_C", vt, FindPainted([]string{full, other}, names, map[string]string{wholeKey(KindColour): "T_X_101_C"}))
	if !rec.Ready || rec.Sources[0].Files[0] != other {
		t.Fatalf("choosing a wide picture: %+v", rec)
	}
	// finish: metallic and roughness as two wide pictures
	m := save("d", "MySkin_Metallic.png", pic)
	r := save("d", "MySkin_Roughness.png", pic)
	rec = PlanRecipe(KindFinish, "T_X_002_MRX", vt, FindPainted([]string{m, r}, nil, nil))
	if !rec.Ready || !rec.Sources[0].Wide || !rec.Sources[1].Wide || !rec.Sources[2].Kept {
		t.Fatalf("wide finish pictures: %+v", rec)
	}
}
