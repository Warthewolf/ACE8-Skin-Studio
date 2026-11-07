package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// folderFiles lists a folder's entries with each file's contents ("<dir>" for a folder).
func folderFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		if e.IsDir() {
			out[e.Name()] = "<dir>"
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(b)
	}
	return out
}

func sameFiles(t *testing.T, what string, got, want map[string]string) {
	t.Helper()
	var g, w []string
	for k, v := range got {
		g = append(g, k+"="+v)
	}
	for k, v := range want {
		w = append(w, k+"="+v)
	}
	sort.Strings(g)
	sort.Strings(w)
	if strings.Join(g, " ") != strings.Join(w, " ") {
		t.Fatalf("%s:\n got  %v\n want %v", what, g, w)
	}
}

// Installing a mod replaces all of its files or none: a failure part-way leaves the old mod as it
// was, never a new .utoc next to an old .ucas.
func TestInstallFilesAllOrNothing(t *testing.T) {
	root := t.TempDir()
	src, dest := filepath.Join(root, "packed"), filepath.Join(root, "Mods", "MySu57")
	os.MkdirAll(src, 0o755)
	os.MkdirAll(dest, 0o755)
	old := map[string]string{}
	var files []string
	for _, ext := range []string{".utoc", ".ucas", ".pak"} {
		n := "MySu57_P" + ext
		os.WriteFile(filepath.Join(src, n), []byte("NEW"+ext), 0o644)
		os.WriteFile(filepath.Join(dest, n), []byte("OLD"+ext), 0o644)
		old[n] = "OLD" + ext
		files = append(files, filepath.Join(src, n))
	}
	// a new file that cannot be read (the copy fails after the .utoc)
	missing := []string{files[0], filepath.Join(src, "gone.ucas"), files[2]}
	if err := installFiles(dest, missing); err == nil || !strings.Contains(err.Error(), "Nothing there was changed") {
		t.Fatalf("expected an error, got %v", err)
	}
	sameFiles(t, "after a failed copy", folderFiles(t, dest), old)

	// an old file that cannot be moved aside (as when the game has it open)
	blocker := filepath.Join(dest, "MySu57_P.ucas.old")
	os.MkdirAll(filepath.Join(blocker, "x"), 0o755)
	if err := installFiles(dest, files); err == nil {
		t.Fatal("expected an error")
	}
	want := map[string]string{"MySu57_P.ucas.old": "<dir>"}
	for k, v := range old {
		want[k] = v
	}
	sameFiles(t, "after a failed replace", folderFiles(t, dest), want)
	os.RemoveAll(blocker)

	if err := installFiles(dest, files); err != nil {
		t.Fatal(err)
	}
	sameFiles(t, "after installing", folderFiles(t, dest), map[string]string{"MySu57_P.utoc": "NEW.utoc", "MySu57_P.ucas": "NEW.ucas", "MySu57_P.pak": "NEW.pak"})

	// a first install that fails leaves nothing behind
	fresh := filepath.Join(root, "Mods", "Fresh")
	if err := installFiles(fresh, missing); err == nil {
		t.Fatal("expected an error")
	}
	sameFiles(t, "a failed first install", folderFiles(t, fresh), map[string]string{})
}

// A built mod is not put into a folder that holds something else: a skin mod made here, the
// merged skin list, or another mod. An earlier build of the same mod is replaced.
func TestInstallBuiltModOwnFolderOnly(t *testing.T) {
	root := t.TempDir()
	s := Settings{ModsDir: filepath.Join(root, "Mods")}
	src := filepath.Join(root, "packed")
	os.MkdirAll(src, 0o755)
	build := func(mod string) []string {
		var files []string
		for _, ext := range []string{".utoc", ".ucas", ".pak"} {
			p := filepath.Join(src, mod+"_P"+ext)
			os.WriteFile(p, []byte("NEW"), 0o644)
			files = append(files, p)
		}
		return files
	}
	put := func(folder string, names ...string) map[string]string {
		dir := filepath.Join(s.ModsDir, folder)
		os.MkdirAll(dir, 0o755)
		for _, n := range names {
			os.WriteFile(filepath.Join(dir, n), []byte("THEIRS"), 0o644)
		}
		return folderFiles(t, dir)
	}
	for folder, names := range map[string][]string{
		"Su57_Red":    {manifestName, "k3x9_P.utoc", "k3x9_P.ucas", "k3x9_P.pak"},
		skinListMod:   {skinListMod + "_P.utoc", skinListMod + "_P.ucas", skinListMod + "_P.pak"},
		"CoolPaint":   {"CoolPaint_v2_P.utoc", "CoolPaint_v2_P.ucas", "CoolPaint_v2_P.pak"},
		"OnlyANote":   {manifestName},
		"EmptyFolder": nil,
	} {
		before := put(folder, names...)
		err := installBuiltMod(testJob(), s, folder, build(folder), map[string]any{})
		if folder == "EmptyFolder" {
			if err != nil {
				t.Errorf("%s: %v", folder, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "Give this mod another name") {
			t.Errorf("%s: expected a refusal, got %v", folder, err)
		}
		sameFiles(t, folder, folderFiles(t, filepath.Join(s.ModsDir, folder)), before)
	}
	// the same folder name in other letter case is the merged skin list too
	if err := installProblem(s, strings.ToLower(skinListMod)); err == nil {
		t.Error("the skin list's folder in lower case was allowed")
	}
	// an earlier build of the same mod is replaced
	put("MySu57", "MySu57_P.utoc", "MySu57_P.ucas", "MySu57_P.pak")
	if err := installBuiltMod(testJob(), s, "MySu57", build("MySu57"), map[string]any{}); err != nil {
		t.Fatal(err)
	}
	sameFiles(t, "rebuilt", folderFiles(t, filepath.Join(s.ModsDir, "MySu57")), map[string]string{"MySu57_P.utoc": "NEW", "MySu57_P.ucas": "NEW", "MySu57_P.pak": "NEW"})
}

// A Repack's new name and description are written only once the mod is ready, and an installed
// build without them takes away what an earlier build left.
func TestRepackTextsFollowTheBuild(t *testing.T) {
	s := Settings{UE4SSMods: t.TempDir()}
	path := replaceTextFile(s, "MySu57")
	if path == "" {
		t.Fatal("no texts folder")
	}
	if err := writeReplaceTexts(testJob(), s, "MySu57", "[Text]\r\nAircraftSkin_Name_su57_001=Red Baron\r\n", false); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err != nil || !strings.Contains(string(b), "Red Baron") {
		t.Fatalf("texts not written: %v %q", err, b)
	}
	// built again without them but not installed: the installed build keeps its texts
	writeReplaceTexts(testJob(), s, "MySu57", "", false)
	if !fileExists(path) {
		t.Fatal("texts taken out although the new build was not installed")
	}
	// built again without them and installed: the game's own name comes back
	if err := writeReplaceTexts(testJob(), s, "MySu57", "", true); err != nil {
		t.Fatal(err)
	}
	if fileExists(path) {
		t.Fatal("the earlier build's texts are still there")
	}
}

// A skin whose row in the skin list names no icon has no icon to replace; it is not fetched
// (an empty name would make retoc take the whole game out).
func TestSkinIconMustBeNamed(t *testing.T) {
	if _, err := skinIcon(SkinRow{Obj: &Obj{}}, "Su-57", 1); err == nil {
		t.Error("a row without an icon was accepted")
	}
	zero := SkinRow{Obj: &Obj{Props: []Prop{{Name: "MenuIconImageRef", Zero: true}}}}
	if _, err := skinIcon(zero, "Su-57", 1); err == nil {
		t.Error("a row with an empty icon was accepted")
	}
	pkg := "/Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_001"
	row := SkinRow{Obj: &Obj{Props: []Prop{{Name: "MenuIconImageRef", Val: SoftPath{Package: pkg, Asset: "T_UI_Icon_Skin_su57_001"}}}}}
	if got, err := skinIcon(row, "Su-57", 1); err != nil || got != pkg {
		t.Errorf("got %q, %v", got, err)
	}
}
