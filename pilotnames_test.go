package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSpeakerLabelsAndKeys(t *testing.T) {
	for in, want := range map[string]string{
		"LIVE_Tasha": "Tasha", "LIVE_Harrier_A": "Harrier A", "LIVE_Ground _Forces_B": "Ground Forces B", "LIVE_Coster＆AWACS": "Coster & AWACS",
		"ContainerAllyCallsign_Name_LittleBee": "Little Bee", "Joker3": "Joker 3", "Shadow22": "Shadow 22", "Purple Magic 1": "Purple Magic 1",
		"FCU_Ground_Alpha": "FCU Ground Alpha", "LIVE_XO": "XO", "LIVE_TewyUnknown": "Tewy Unknown",
	} {
		if got := speakerLabel(in); got != want {
			t.Errorf("speakerLabel(%q) = %q, want %q", in, got, want)
		}
	}
	same := func(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }
	if k := fieldKeys("name:LIVE_Tasha"); !same(k, []string{"LIVE_Tasha", "ContainerAllyCallsign_Name_Tasha"}) {
		t.Errorf("name keys %v", k)
	}
	if k := fieldKeys("hangar:LIVE_Tasha"); !same(k, []string{"HangarSelectaircraftTitle_Name_Wingman2"}) {
		t.Errorf("Tasha's hangar key %v", k)
	}
	if k := fieldKeys("hangar:LIVE_Kate"); !same(k, []string{"HangarSelectaircraftTitle_Name_Wingman4"}) {
		t.Errorf("Kate's hangar key %v", k)
	}
	if k := fieldKeys("hangar:LIVE_Cruz"); len(k) != 0 {
		t.Errorf("Cruz has no place in the hangar's list: %v", k)
	}
	if k := fieldKeys("name:LIVE_Cruz"); !same(k, []string{"LIVE_Cruz", "ContainerAllyCallsign_Name_Cruz"}) {
		t.Errorf("name keys %v", k)
	}
	if k := fieldKeys("name:LIVE_Ground _Forces_A"); !same(k, []string{"LIVE_Ground _Forces_A"}) {
		t.Errorf("name keys with a space %v", k)
	}
	if k := fieldKeys("callsign:ContainerAllyCallsign_Name_Pinkhead"); !same(k, []string{"ContainerAllyCallsign_Name_Pinkhead"}) {
		t.Errorf("callsign keys %v", k)
	}
	if k := fieldKeys("flight:Joker3"); !same(k, []string{"Callsign_Joker3", "ContainerAllyCallsign_Name_Joker3"}) {
		t.Errorf("flight keys %v", k)
	}
	if !naturalLess("Joker 2", "Joker 10") || naturalLess("Queen 1", "Joker 2") {
		t.Error("naturalLess")
	}
	// what the window may send
	for _, bad := range []map[string]string{{"name:LIVE_X\nEvil=1": "a"}, {"name:A=B": "a"}, {"other:LIVE_X": "a"}, {"name:": "a"}, {"name:LIVE_X": strings.Repeat("x", 41)}, {"hangar:LIVE_Cruz": "a"}} {
		if _, err := cleanNames(bad); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	got, err := cleanNames(map[string]string{"name:LIVE_Tasha": "  Rosa \n  Ivanova ", "name:LIVE_Baxter": "   ", "flight:Purple Magic 1": "Violet 1"})
	if err != nil || len(got) != 2 || got["name:LIVE_Tasha"] != "Rosa Ivanova" {
		t.Fatalf("cleanNames %v %v", got, err)
	}
	ini := namesIni(got)
	// the hangar: the name in capitals, unless the hangar box has its own
	if ini2 := namesIni(map[string]string{"name:LIVE_Tasha": "Rosa Ivanova", "name:LIVE_Baxter": "Ed", "hangar:LIVE_Baxter": "Professor Ed", "name:LIVE_Cruz": "Mia"}); !strings.Contains(ini2, "HangarSelectaircraftTitle_Name_Wingman2=ROSA IVANOVA\r\n") ||
		!strings.Contains(ini2, "HangarSelectaircraftTitle_Name_Wingman1=Professor Ed\r\n") || strings.Count(ini2, "HangarSelectaircraftTitle") != 2 {
		t.Errorf("hangar lines:\n%s", ini2)
	}
	if ini3 := namesIni(map[string]string{"hangar:LIVE_Coster": "NOISE"}); !strings.Contains(ini3, "HangarSelectaircraftTitle_Name_Wingman3=NOISE\r\n") || strings.Contains(ini3, "LIVE_Coster=") {
		t.Errorf("a hangar name alone:\n%s", ini3)
	}
	for _, line := range []string{"[Text]", "LIVE_Tasha=Rosa Ivanova", "ContainerAllyCallsign_Name_Tasha=Rosa Ivanova", "Callsign_Purple Magic 1=Violet 1", "ContainerAllyCallsign_Name_PurpleMagic1=Violet 1"} {
		if !strings.Contains(ini, line+"\r\n") {
			t.Errorf("the names file has no line %q:\n%s", line, ini)
		}
	}
}

func namesEnv(t *testing.T) Settings {
	root := t.TempDir()
	ue := filepath.Join(root, "Win64", "ue4ss", "Mods")
	os.MkdirAll(ue, 0o755)
	os.WriteFile(filepath.Join(ue, "mods.txt"), []byte("x : 1\r\n"), 0o644)
	s := Settings{Workspace: filepath.Join(root, "ws"), UE4SSMods: ue, SaveDir: filepath.Join(root, "Saved", "SaveGames"), Project: "Live"}
	// the game's two lists, in the base folder, when this machine has them
	for _, gp := range []string{speakerTablePath, wingmanTablePath} {
		rel := strings.TrimPrefix(gp, "/Game/")
		for _, ext := range []string{".uasset", ".uexp"} {
			src := filepath.Join(gameExports, filepath.FromSlash(rel)+ext)
			if _, err := os.Stat(src); err == nil {
				copyFile(src, gameFile(s.baseDir(), s.Project, gp)+ext)
			}
		}
	}
	return s
}

func TestPilotNames(t *testing.T) {
	needFile(t, gameExports+"/Datatables/Radio/DT_SpeakerTable.uasset")
	s := namesEnv(t)
	v := pilotNamesView(s)
	if v.NeedLists || v.Problem != "" || len(v.Speakers) < 20 || len(v.Flights) < 5 {
		t.Fatalf("view: lists %v problem %q, %d speakers, %d flights", v.NeedLists, v.Problem, len(v.Speakers), len(v.Flights))
	}
	tasha := v.Speakers[1]
	if tasha.ID != "LIVE_Tasha" || tasha.Side != "wingman" || tasha.Callsign != "ContainerAllyCallsign_Name_Pinkhead" || tasha.Flight != "Joker3" ||
		tasha.Portrait != "T_HUD_SubtitleSpeakerPortrait_00_Tasha00_001" {
		t.Fatalf("Tasha %+v", tasha)
	}
	if v.Speakers[0].ID != "LIVE_Baxter" || v.Flights[0].ID != "Joker2" || v.Speakers[len(v.Speakers)-1].Side != "enemy" {
		t.Fatalf("order: %s first, flight %s first, %s last", v.Speakers[0].ID, v.Flights[0].ID, v.Speakers[len(v.Speakers)-1].Side)
	}

	texts := s.textsDir()
	os.MkdirAll(texts, 0o755)
	// the old test file, and another text file that is read first and gives the same key
	os.WriteFile(filepath.Join(texts, pilotNameTest), []byte("[Text]\r\nLIVE_Tasha=ROSA-1\r\n"), 0o644)
	os.WriteFile(filepath.Join(filepath.Dir(texts), oldFindFile), []byte("Tasha\r\n"), 0o644) // left by 2.3.1
	os.WriteFile(filepath.Join(texts, "AAA_Other.ini"), []byte("[Text]\r\n; LIVE_Baxter=no\r\nContainerAllyCallsign_Name_Pinkhead=Other\r\n"), 0o644)
	os.WriteFile(filepath.Join(texts, "ZZZ_Later.ini"), []byte("[Text]\r\nLIVE_Tasha=Later\r\n"), 0o644)
	v, notes, err := savePilotNames(s, map[string]string{"name:LIVE_Tasha": "Rosa", "callsign:ContainerAllyCallsign_Name_Pinkhead": "Rosebud", "flight:Joker3": "Rose 3", "name:LIVE_Baxter": ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], pilotNameTest) {
		t.Errorf("notes %v", notes)
	}
	if _, err := os.Stat(filepath.Join(texts, pilotNameTest)); err == nil {
		t.Error("the test file is still there")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(texts), oldFindFile)); err == nil {
		t.Error("find.txt is still there")
	}
	if len(v.Clashes) != 1 || v.Clashes[0] != "AAA_Other.ini" {
		t.Errorf("clashes %v", v.Clashes)
	}
	b, err := os.ReadFile(filepath.Join(texts, namesTextFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"LIVE_Tasha=Rosa", "ContainerAllyCallsign_Name_Tasha=Rosa", "HangarSelectaircraftTitle_Name_Wingman2=ROSA", "ContainerAllyCallsign_Name_Pinkhead=Rosebud", "Callsign_Joker3=Rose 3", "ContainerAllyCallsign_Name_Joker3=Rose 3"} {
		if !strings.Contains(string(b), line+"\r\n") {
			t.Errorf("no line %q in\n%s", line, b)
		}
	}
	if strings.Contains(string(b), "LIVE_Baxter") {
		t.Error("an empty name was written")
	}
	if len(v.Names) != 3 || v.Names["flight:Joker3"] != "Rose 3" {
		t.Fatalf("saved names %v", v.Names)
	}
	// SkinSlotText was installed with them
	if st := skinSlotTextStatus(s); !st.Installed {
		t.Fatalf("SkinSlotText not installed: %+v", st)
	}
	// a skin list rebuild does not take the names file away (it only tidies SkinStudio_*.ini)
	if _, err := writeAllTexts(s, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(texts, namesTextFile)); err != nil {
		t.Fatal("writeAllTexts removed the names file")
	}

	// a game: SkinSlotText's log says which texts the game asked for
	logp := filepath.Join(s.ue4ssModsDir(), "SkinSlotText", "Scripts", "SkinSlotText.log")
	time.Sleep(20 * time.Millisecond)
	os.WriteFile(logp, []byte("2026-10-15 20:00:00.000 SkinSlotText 1.2\r\n2026-10-15 20:00:01.000   LIVE_Tasha = Rosa\r\n"+
		"2026-10-15 20:03:00.000 first use: LIVE_Tasha\r\n2026-10-15 20:04:00.000 first use: ContainerAllyCallsign_Name_Joker3\r\n"), 0o644)
	v = pilotNamesView(s)
	if !v.Seen["name:LIVE_Tasha"] || !v.Seen["flight:Joker3"] || v.Seen["callsign:ContainerAllyCallsign_Name_Pinkhead"] || v.LastGame.Before(v.Saved) {
		t.Fatalf("seen %v, last game %v, saved %v", v.Seen, v.LastGame, v.Saved)
	}

	// back to the game's own names
	v, _, err = savePilotNames(s, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(texts, namesTextFile)); err == nil || len(v.Names) != 0 {
		t.Fatal("the names were not taken out")
	}
}

func TestPilotNamesWithoutSetup(t *testing.T) {
	s := Settings{Workspace: t.TempDir(), Project: "Live"}
	v := pilotNamesView(s)
	if !v.NeedLists || v.Problem == "" {
		t.Fatalf("view without UE4SS or lists: %+v", v)
	}
	if _, _, err := savePilotNames(s, map[string]string{"name:LIVE_Tasha": "Rosa"}); err == nil {
		t.Fatal("saved without UE4SS")
	}
}
