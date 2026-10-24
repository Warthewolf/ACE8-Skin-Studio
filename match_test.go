package main

import (
	"fmt"
	"strings"
	"testing"
)

func tileNames(dir, stem string, n int, ext string) []string {
	var out []string
	for u := 1001; u < 1001+n; u++ {
		out = append(out, fmt.Sprintf("%s/%s.%d%s", dir, stem, u, ext))
	}
	return out
}

func TestSplitTile(t *testing.T) {
	for name, want := range map[string][2]any{
		"T_PP0018_fa36_0GR_C.1001.png":             {"T_PP0018_fa36_0GR_C", 1001},
		"MI_PP0021_su57_001_Decal_Base_color_1003": {"MI_PP0021_su57_001_Decal_Base_color", 1003},
		"1002_skin.tga":                            {"skin", 1002},
		"skin.png":                                 {"skin", 0},
		"PP1021_thing.png":                         {"PP1021_thing", 0},
		"a.1001.final.png":                         {"a.final", 1001},
	} {
		stem, udim := splitTile(name)
		if stem != want[0].(string) || udim != want[1].(int) {
			t.Errorf("%s: got %q %d, want %v", name, stem, udim, want)
		}
	}
}

// The files from a tester's report: colour tiles named after another skin's colour texture,
// finish tiles split into channels, and a different skin being rebuilt.
func TestGameStyleNames(t *testing.T) {
	uexp, _ := synthVT(6, 1, 64, 32, 4, "PF_DXT1", 8)
	vt, err := ParseVTex(uexp)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	files = append(files, tileNames("D:/split", "T_PP0018_fa36_0GR_C", 6, ".png")...)
	files = append(files, tileNames("D:/split", "T_PP0018_fa36_001_MRX_Metallic", 6, ".png")...)
	files = append(files, tileNames("D:/split", "T_PP0018_fa36_001_MRX_Roughness", 6, ".png")...)
	files = append(files, tileNames("D:/split", "T_PP0018_fa36_001_NO_AO", 6, ".png")...)
	files = append(files, tileNames("D:/split", "T_PP0018_fa36_001_NO_Normal_DirectX", 6, ".png")...)
	names := []string{"T_PP0018_fa36_002_C", "T_PP0018_fa36_002_MRX"}
	sets := FindPainted(files, names, nil)

	rec := PlanRecipe(KindColour, "T_PP0018_fa36_002_C", vt, sets)
	if !rec.Ready || !strings.Contains(rec.Uses[0], "T_PP0018_fa36_0GR_C") || !strings.Contains(rec.Uses[0], "6 tiles") {
		t.Fatalf("colour tiles named after another skin were not accepted: %+v", rec)
	}
	if rec := PlanRecipe(KindFinish, "T_PP0018_fa36_002_MRX", vt, sets); !rec.Ready || !strings.HasPrefix(rec.Uses[0], "metallic: 6 tiles") {
		t.Fatalf("finish channels not accepted: %+v", rec)
	}
	if rec := PlanRecipe(KindNormal, "T_PP0018_fa36_002_NO", vt, sets); !rec.Ready || !strings.HasPrefix(rec.Uses[0], "normal map: 6 tiles") {
		t.Fatalf("normal channels not accepted: %+v", rec)
	}

	// Painter's own names still win over a game-style name when both are there.
	both := append(append([]string(nil), files...), tileNames("D:/split", "MySkin_Base_color", 6, ".png")...)
	if rec := PlanRecipe(KindColour, "T_PP0018_fa36_002_C", vt, FindPainted(both, names, nil)); !rec.Ready || !strings.HasPrefix(rec.Uses[0], "base colour: 6 tiles") {
		t.Fatalf("base colour files should win: %+v", rec)
	}
	// A picture named exactly after the texture wins over everything.
	exact := append(append([]string(nil), both...), tileNames("D:/split", "T_PP0018_fa36_002_C", 6, ".png")...)
	if rec := PlanRecipe(KindColour, "T_PP0018_fa36_002_C", vt, FindPainted(exact, names, nil)); !rec.Ready || !strings.HasPrefix(rec.Uses[0], "ready-made picture: 6 tiles") {
		t.Fatalf("exactly named files should win: %+v", rec)
	}

	// Complete finish and normal pictures named after another skin work too.
	whole := append(tileNames("D:/x", "T_PP0018_fa36_001_MRX", 6, ".tga"), tileNames("D:/x", "T_PP0018_fa36_001_NO", 6, ".tga")...)
	ws := FindPainted(whole, names, nil)
	for kind, tex := range map[Kind]string{KindFinish: "T_PP0018_fa36_002_MRX", KindNormal: "T_PP0018_fa36_002_NO"} {
		if rec := PlanRecipe(kind, tex, vt, ws); !rec.Ready || !strings.HasPrefix(rec.Uses[0], "complete picture: 6 tiles") {
			t.Fatalf("%s: complete picture not accepted: %+v", tex, rec)
		}
	}

	// Too few tiles of a game-style picture are reported as such.
	if rec := PlanRecipe(KindColour, "T_PP0018_fa36_002_C", vt, FindPainted(files[:4], names, nil)); rec.Ready || !strings.Contains(rec.Missing, "only 4 of 6") {
		t.Fatalf("short set: %+v", rec)
	}
}

func TestClashingSets(t *testing.T) {
	uexp, _ := synthVT(5, 1, 64, 32, 4, "PF_DXT1", 8)
	vt, err := ParseVTex(uexp)
	if err != nil {
		t.Fatal(err)
	}
	// two skins' colour tiles in one folder must not be mixed together
	files := append(tileNames("D:/s", "T_X_001_C", 5, ".png"), tileNames("D:/s", "T_X_0GR_C", 5, ".png")...)
	rec := PlanRecipe(KindColour, "T_X_002_C", vt, FindPainted(files, []string{"T_X_002_C"}, nil))
	if rec.Ready || !strings.Contains(rec.Missing, "more than one set") || !strings.Contains(rec.Missing, "T_X_001_C") || !strings.Contains(rec.Missing, "T_X_0GR_C") {
		t.Fatalf("clash not reported: %+v", rec)
	}
	if _, _, err := Compose(KindColour, "T_X_002_C", vt, nil, FindPainted(files, []string{"T_X_002_C"}, nil)); err == nil {
		t.Fatal("a clash must stop the build")
	}
	// ... unless one of them is named exactly after the texture being rebuilt
	if rec := PlanRecipe(KindColour, "T_X_001_C", vt, FindPainted(files, []string{"T_X_001_C"}, nil)); !rec.Ready || !strings.HasPrefix(rec.Uses[0], "ready-made picture") {
		t.Fatalf("exact name should settle it: %+v", rec)
	}
	// same for Painter-style names
	two := append(tileNames("D:/s", "A_Base_color", 5, ".png"), tileNames("D:/s", "B_Base_color", 5, ".png")...)
	if rec := PlanRecipe(KindColour, "T_X_002_C", vt, FindPainted(two, nil, nil)); rec.Ready || !strings.Contains(rec.Missing, "more than one set") {
		t.Fatalf("clash not reported: %+v", rec)
	}
	// groups that hold different tiles are simply joined
	joined := append(tileNames("D:/s", "A_Base_color", 3, ".png"), "D:/s/B_Base_color.1004.png", "D:/s/B_Base_color.1005.png")
	if rec := PlanRecipe(KindColour, "T_X_002_C", vt, FindPainted(joined, nil, nil)); !rec.Ready {
		t.Fatalf("separate tiles should join: %+v", rec)
	}
	// the same tiles saved as both PNG and TGA are one set, not a clash
	twice := append(tileNames("D:/s", "A_Base_color", 5, ".png"), tileNames("D:/s", "A_Base_color", 5, ".tga")...)
	if rec := PlanRecipe(KindColour, "T_X_002_C", vt, FindPainted(twice, nil, nil)); !rec.Ready {
		t.Fatalf("png and tga copies should not clash: %+v", rec)
	}
	// tiles still win over one wide picture of the same channel
	s := FindPainted(append(tileNames("D:/s", "A_Base_color", 5, ".png"), "D:/s/Wide_Base_color.png"), nil, nil)[ChBase]
	if s.Stitched != "" || len(s.Tiles) != 5 || len(s.Clash) != 0 {
		t.Fatalf("tiles should win over a wide picture: %+v", s)
	}
}
