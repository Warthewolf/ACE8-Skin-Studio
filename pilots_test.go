package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var pilotListRows = []string{
	"../../../Live/Content/Character/PI/PI003/Texture/Acce/T_PI003_Helmet01_01_C.uasset",
	"../../../Live/Content/Character/PI/PI003/Texture/Acce/T_PI003_Helmet01_01_C.ubulk",
	"../../../Live/Content/Character/PI/PI003/Texture/Acce/T_PI003_Helmet01_01_MRO.uasset",
	"../../../Live/Content/Character/PI/PI003/Texture/Acce/T_PI003_Helmet01_01_N.uasset",
	"../../../Live/Content/Character/PI/PI003/Texture/Acce/T_PI003_Helmet01_02_C.uasset",
	"../../../Live/Content/Character/PI/PI003/Material/Acce/MI_PI003_Helmet01.uasset",
	"../../../Live/Content/Character/PI/PI003/Material/Acce/MI_PI003_Helmet02.uasset",
	"../../../Live/Content/Character/PI/PI003/Mesh/Acce/SK_PI003_Acce01.uasset",
	"../../../Live/Content/Character/PI/PI402/Texture/T_PI402_Helmet01_01_C.uasset",
	"../../../Live/Content/Character/PI/PI002/Texture/Acce/T_PI002_Helmet02_01_Noise.uasset",
	"../../../Live/Content/UI/Texture/HUD/SubtitleSpeakerPortrait/T_HUD_SubtitleSpeakerPortrait_00_Tasha00_001.uasset",
	"../../../Live/Content/UI/Texture/HUD/SubtitleSpeakerPortrait/T_HUD_SubtitleSpeakerPortrait_00_Tasha00_000.uasset",
	"../../../Live/Content/UI/Texture/HUD/SubtitleSpeakerPortrait/T_HUD_SubtitleSpeakerPortrait_00_Jackpot1_00_000.uasset",
	"../../../Live/Content/UI/Texture/HUD/SubtitleSpeakerPortrait/T_HUD_SubtitleSpeakerPortrait_ComWinBase_000.uasset",
}

func TestBuildPilotIndex(t *testing.T) {
	idx, mats := buildPilotIndex(pilotListRows)
	if len(idx.Pilots) != 2 || idx.Pilots[0].ID != "PI003" || idx.Pilots[1].ID != "PI402" {
		t.Fatalf("pilots: %+v", idx.Pilots)
	}
	p := idx.Pilots[0]
	if p.Name != "Tasha" || len(p.Sheets) != 2 || p.Sheets[0].Key != "Helmet01_01" || p.Sheets[1].Key != "Helmet01_02" {
		t.Fatalf("PI003: %+v", p)
	}
	sh := p.Sheets[0]
	if sh.Stem != "T_PI003_Helmet01_01" || sh.Helmet != 1 || sh.Part != 1 || len(sh.Textures) != 3 ||
		sh.Textures["N"] != "/Game/Character/PI/PI003/Texture/Acce/T_PI003_Helmet01_01_N" {
		t.Errorf("sheet: %+v", sh)
	}
	if len(p.Sheets[1].Textures) != 1 || idx.Pilots[1].Sheets[0].Textures["C"] != "/Game/Character/PI/PI402/Texture/T_PI402_Helmet01_01_C" {
		t.Errorf("other sheets: %+v %+v", p.Sheets[1], idx.Pilots[1].Sheets)
	}
	if len(mats["PI003"]) != 2 {
		t.Errorf("materials: %v", mats)
	}
	// portraits: Tasha's first (she is a known pilot), the frame pieces left out
	if len(idx.Portraits) != 3 || idx.Portraits[0].Label != "Tasha" || idx.Portraits[1].Label != "Tasha 2" || idx.Portraits[2].Label != "Jackpot1" {
		t.Fatalf("portraits: %+v", idx.Portraits)
	}
	if idx.Portraits[0].Pilot != "PI003" || idx.Portraits[0].Text == nil || idx.Portraits[0].Text.Callsign != "PINKY" || idx.Portraits[2].Pilot != "" {
		t.Errorf("portrait details: %+v", idx.Portraits)
	}
	if strings.Join(p.Portraits, ",") != "T_HUD_SubtitleSpeakerPortrait_00_Tasha00_000,T_HUD_SubtitleSpeakerPortrait_00_Tasha00_001" {
		t.Errorf("Tasha's portraits: %v", p.Portraits)
	}
}

func testPilot() *Pilot {
	return &Pilot{ID: "PI003", Name: "Tasha", Sheets: []PilotSheet{
		{Key: "Helmet01_01", Stem: "T_PI003_Helmet01_01", Textures: map[string]string{"C": "c1", "MRO": "m1", "N": "n1"}, Materials: []string{"MI_PI003_Helmet01", "MI_PI003_Helmet03"}},
		{Key: "Helmet01_02", Stem: "T_PI003_Helmet01_02", Textures: map[string]string{"C": "c2", "MRO": "m2", "N": "n2"}, Materials: []string{"MI_PI003_Helmet02"}},
	}}
}

func TestHelmetNames(t *testing.T) {
	p := testPilot()
	for name, want := range map[string][2]string{
		"T_PI003_Helmet01_01_BaseColor.png":                {hcBase, "Helmet01_01"},
		"T_PI003_Helmet01_02_Metallic.png":                 {hcMetallic, "Helmet01_02"},
		"T_PI003_Helmet01_02_AO.tga":                       {hcAO, "Helmet01_02"},
		"T_PI003_Helmet01_01_Normal_DirectX.png":           {hcNormal, "Helmet01_01"},
		"SK_PI003_Acce01_MI_PI003_Helmet02_Roughness.png":  {hcRoughness, "Helmet01_02"},
		"SK_PI003_Acce01_MI_PI003_Helmet03_Base_color.png": {hcBase, "Helmet01_01"},
		"T_PI003_Helmet01_02_MRO.png":                      {hcMRO, "Helmet01_02"},
		"T_PI003_Helmet01_01_C.png":                        {hcBase, "Helmet01_01"},
		"T_PI003_Helmet01_01_N.png":                        {hcNormal, "Helmet01_01"},
		"Helmet_Base_color.png":                            {hcBase, ""},
		"T_PI003_Helmet01_01_Height.png":                   {"", "Helmet01_01"},
	} {
		ch, _, _ := helmetChannel(name)
		if ch != want[0] {
			t.Errorf("%s: channel %q, want %q", name, ch, want[0])
		}
		if k := sheetFromName(name, p); k != want[1] {
			t.Errorf("%s: part %q, want %q", name, k, want[1])
		}
	}
	// a helmet with one part takes every picture
	one := &Pilot{ID: "PI006", Sheets: p.Sheets[:1]}
	if k := sheetFromName("Helmet_Base_color.png", one); k != "Helmet01_01" {
		t.Errorf("one part: %q", k)
	}
}

func TestPlanPilot(t *testing.T) {
	p := testPilot()
	pl := planPilot(p, []pilotFile{
		{Path: "/x/T_PI003_Helmet01_01_BaseColor.png"},
		{Path: "/x/T_PI003_Helmet01_01_Normal.png"},
		{Path: "/x/T_PI003_Helmet01_01_Normal_DirectX.png"}, // wins over the plain normal
		{Path: "/x/Helmet_Base_color.png", Sheet: "Helmet01_02"},
		{Path: "/x/Helmet_Roughness.png"}, // no part
		{Path: "/x/notes.txt"},
		{Path: "/x/T_PI003_Helmet01_01_Emissive.png"},
	})
	if len(pl.Problems) != 0 || len(pl.Unplaced) != 1 || len(pl.Ignored) != 1 || pl.pictures() != 3 {
		t.Fatalf("%+v", pl)
	}
	if pl.Sheets[0].Uses[hcNormal].Path != "/x/T_PI003_Helmet01_01_Normal_DirectX.png" || pl.Sheets[1].Uses[hcBase].Path != "/x/Helmet_Base_color.png" {
		t.Errorf("%+v", pl.Sheets)
	}
	// two base colour pictures for one part
	pl = planPilot(p, []pilotFile{{Path: "/a/T_PI003_Helmet01_01_BaseColor.png"}, {Path: "/b/T_PI003_Helmet01_01_C.png"}})
	if len(pl.Problems) != 1 || !strings.Contains(pl.Problems[0], "more than one base colour") {
		t.Errorf("clash: %+v", pl.Problems)
	}
	// a part without a normal map in the game
	p.Sheets[1].Textures = map[string]string{"C": "c2"}
	pl = planPilot(p, []pilotFile{{Path: "/a/T_PI003_Helmet01_02_Normal_OpenGL.png"}})
	if len(pl.Problems) != 1 || !pl.Sheets[1].Uses[hcNormal].OpenGL {
		t.Errorf("missing texture: %+v", pl)
	}
}

func TestOTFFromUExp(t *testing.T) {
	f := gameExports + "/UI/Font/0708_AcesSleipnirText-Regular.uexp"
	needFile(t, f)
	b, _ := os.ReadFile(f)
	otf, err := otfFromUExp(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(otf[:4]) != "OTTO" || len(otf) < 10000 || len(otf) > len(b) {
		t.Fatalf("%d bytes", len(otf))
	}
	n := int(binary.BigEndian.Uint16(otf[4:]))
	tags := ""
	for i := 0; i < n; i++ {
		tags += string(otf[12+16*i:16+16*i]) + " "
	}
	if !strings.Contains(tags, "CFF ") || !strings.Contains(tags, "name") || !strings.Contains(tags, "cmap") {
		t.Errorf("tables: %s", tags)
	}
	if !bytes.Contains(otf, []byte("Aces Sleipnir")) && !bytes.Contains(otf, []byte("A\x00c\x00e\x00s\x00")) {
		t.Errorf("no font name inside")
	}
	if _, err := otfFromUExp([]byte("nothing here OTTO but junk")); err == nil {
		t.Error("junk accepted")
	}
}

// newPilotEnv is a game with Tasha's helmet, a few portraits and the font, for the stand-in retoc.
func newPilotEnv(t *testing.T) *slotEnv {
	t.Helper()
	needFile(t, pilotTex+"_C.uasset")
	needFile(t, portraitTex+".uasset")
	root := t.TempDir()
	e := &slotEnv{game: filepath.Join(root, "gamefiles"), log: filepath.Join(root, "retoc.log")}
	content := filepath.Join(e.game, "Live", "Content")
	for _, dir := range []string{"Character/PI/PI003", "UI/Texture/HUD/SubtitleSpeakerPortrait", "UI/Font"} {
		filepath.Walk(filepath.Join(gameExports, dir), func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				rel, _ := filepath.Rel(gameExports, p)
				dst := filepath.Join(content, rel)
				os.MkdirAll(filepath.Dir(dst), 0o755)
				os.Symlink(p, dst)
			}
			return nil
		})
	}
	e.mods = filepath.Join(root, "Mods")
	os.MkdirAll(e.mods, 0o755)
	paks := filepath.Join(root, "Paks")
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

func TestPilotsEndToEnd(t *testing.T) {
	e := newPilotEnv(t)
	s := e.s
	// 1. find the pilots
	res, err := doFindPilots(testJob(), s)
	if err != nil {
		t.Fatal(err)
	}
	idx := res.(*PilotIndex)
	p := idx.pilot("PI003")
	if p == nil || len(p.Sheets) != 2 || !p.Sheets[0].Preview || !p.Sheets[1].Preview {
		t.Fatalf("index: %+v", idx)
	}
	if strings.Join(p.Sheets[0].Materials, ",") != "MI_PI003_Helmet01,MI_PI003_Helmet03" || strings.Join(p.Sheets[1].Materials, ",") != "MI_PI003_Helmet02" {
		t.Errorf("materials: %v / %v", p.Sheets[0].Materials, p.Sheets[1].Materials)
	}
	if loadPilotIndex(s).pilot("PI003") == nil {
		t.Fatal("index not saved")
	}
	if _, err := os.Stat(filepath.Join(s.pilotsDir(), "tmp")); err == nil {
		t.Error("the full-size preview files were left behind")
	}
	prev, err := os.ReadFile(filepath.Join(s.pilotsDir(), "previews", "T_PI003_Helmet01_01.png"))
	if err != nil || len(prev) < 1000 {
		t.Fatalf("preview: %v", err)
	}
	if b, err := portraitPNG(s, "T_HUD_SubtitleSpeakerPortrait_00_Tasha00_001"); err != nil || len(b) < 1000 {
		t.Fatalf("portrait png: %v", err)
	}
	if b, err := gameFontOTF(s); err != nil || string(b[:4]) != "OTTO" {
		t.Fatalf("font: %v", err)
	}

	// 2. paint-ready pictures of part 1
	pr, err := doPilotPaint(testJob(), s, pilotPaintReq{Pilot: "PI003", Sheets: []string{"Helmet01_01"}, Format: "png"})
	if err != nil {
		t.Fatal(err)
	}
	files := pr.(map[string]any)["files"].([]string)
	var names []string
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}
	if strings.Join(names, " ") != "T_PI003_Helmet01_01_BaseColor.png T_PI003_Helmet01_01_Metallic.png T_PI003_Helmet01_01_Roughness.png T_PI003_Helmet01_01_AO.png T_PI003_Helmet01_01_Normal_DirectX.png" {
		t.Errorf("paint-ready: %v", names)
	}

	// 3. a new colour for part 1, roughness from the paint-ready picture, and a new portrait
	dir := t.TempDir()
	paint := NewImg(1024, 1024)
	for i := 0; i < len(paint.Pix); i += 4 {
		paint.Pix[i], paint.Pix[i+1], paint.Pix[i+2], paint.Pix[i+3] = 30, 200, 90, 255
	}
	SaveImage(filepath.Join(dir, "MyHelmet_Base_color.png"), paint, SaveRGB)
	rough := NewImg(512, 512)
	for i := 0; i < len(rough.Pix); i += 4 {
		rough.Pix[i], rough.Pix[i+1], rough.Pix[i+2], rough.Pix[i+3] = 40, 40, 40, 255
	}
	SaveImage(filepath.Join(dir, "T_PI003_Helmet01_01_Roughness.png"), rough, SaveGrey)
	port := NewImg(428, 540)
	for i := 0; i < len(port.Pix); i += 4 {
		port.Pix[i], port.Pix[i+1], port.Pix[i+2], port.Pix[i+3] = 200, 40, 40, 153
	}
	SaveImage(filepath.Join(dir, "portrait.png"), port, SaveRGBA)

	out, err := doPilotBuild(testJob(), s, pilotBuildReq{
		Pilot:     "PI003",
		Files:     []pilotFile{{Path: filepath.Join(dir, "MyHelmet_Base_color.png"), Sheet: "Helmet01_01"}, {Path: filepath.Join(dir, "T_PI003_Helmet01_01_Roughness.png")}},
		Portraits: []portraitEdit{{Name: "T_HUD_SubtitleSpeakerPortrait_00_Tasha00_001", Picture: filepath.Join(dir, "portrait.png")}},
		ModName:   "Pilot_Tasha", Install: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := out.(map[string]any)
	if r["installedDir"] != filepath.Join(e.mods, "Pilot_Tasha") {
		t.Errorf("installed: %v", r["installedDir"])
	}
	packed := filepath.Join(s.modsOut(), "Pilot_Tasha", "packed", "Pilot_Tasha_P.input")
	got := gameFile(packed, "Live", "/Game/Character/PI/PI003/Texture/Acce/T_PI003_Helmet01_01_C")
	tx, pk, ub, err := LoadTex2D(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, mip := range []int{0, 5, 9} {
		im, _ := tx.DecodeMip(pk.UExp, ub, mip)
		c := im.Pix[(im.W*im.H/2+im.W/2)*4:]
		if d := int(c[0]) - 30 + int(c[1]) - 200 + int(c[2]) - 90; d < -12 || d > 12 {
			t.Errorf("colour mip %d: %v", mip, c[:3])
		}
	}
	// the finish keeps the game's metallic and ambient occlusion, with the new roughness
	orig, op, oub, _ := LoadTex2D(pilotTex + "_MRO")
	before, _ := orig.DecodeMip(op.UExp, oub, 2)
	tx, pk, ub, err = LoadTex2D(gameFile(packed, "Live", "/Game/Character/PI/PI003/Texture/Acce/T_PI003_Helmet01_01_MRO"))
	if err != nil {
		t.Fatal(err)
	}
	after, _ := tx.DecodeMip(pk.UExp, ub, 2)
	if v := psnr(before, after, 0, 2); v < 30 {
		t.Errorf("metallic and AO changed: %.1f dB", v)
	}
	if g := after.Pix[(after.W*after.H/2+after.W/2)*4+1]; g < 30 || g > 50 {
		t.Errorf("roughness %d", g)
	}
	// no normal map was given, so none is in the mod; part 2 is untouched
	for _, n := range []string{"T_PI003_Helmet01_01_N", "T_PI003_Helmet01_02_C"} {
		if _, err := os.Stat(gameFile(packed, "Live", "/Game/Character/PI/PI003/Texture/Acce/"+n) + ".uasset"); err == nil {
			t.Errorf("%s is in the mod", n)
		}
	}
	pp, err := LoadPackage(gameFile(packed, "Live", portraitFolder+"T_HUD_SubtitleSpeakerPortrait_00_Tasha00_001"))
	if err != nil {
		t.Fatal(err)
	}
	im, err := DecodeIcon(pp)
	if err != nil {
		t.Fatal(err)
	}
	if c := im.Pix[(270*428+214)*4:]; c[0] < 190 || c[1] > 55 || c[3] < 145 || c[3] > 160 {
		t.Errorf("portrait pixel %v", c[:4])
	}
	// the rebuilt textures are the same size as the game's, so only pixels changed
	ua, _ := os.ReadFile(portraitTex + ".uasset")
	ua2, _ := os.ReadFile(gameFile(packed, "Live", portraitFolder+"T_HUD_SubtitleSpeakerPortrait_00_Tasha00_001") + ".uasset")
	if !bytes.Equal(ua, ua2) {
		t.Error("portrait header changed")
	}

	// 4. pictures that say no part are refused with a clear message
	_, err = doPilotBuild(testJob(), s, pilotBuildReq{Pilot: "PI003", ModName: "X", Files: []pilotFile{{Path: filepath.Join(dir, "MyHelmet_Base_color.png")}}})
	if err == nil || !strings.Contains(err.Error(), "which helmet part") {
		t.Errorf("unplaced: %v", err)
	}
	_, err = doPilotBuild(testJob(), s, pilotBuildReq{Pilot: "PI003", ModName: "X"})
	if err == nil || !strings.Contains(err.Error(), "first") {
		t.Errorf("nothing to build: %v", err)
	}
}
