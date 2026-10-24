package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A texture read back out of a container, against the game's own copy for its layout: a skin icon
// a skin slot mod carries (recoloured from the game's) comes out with the game's shape and new colour.
func TestTextureFromContainer(t *testing.T) {
	utoc := "/mnt/user-data/uploads/AceComba8EXPORTS/SkinSlotTest/packed/SkinSlotTest_P.utoc"
	layout := gameExports + "/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_001"
	needFile(t, utoc)
	needFile(t, layout+".uexp")
	c, err := OpenZenContainer(utoc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := textureFromZen(c, "UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_602", layout)
	if err != nil {
		t.Fatal(err)
	}
	tx, pk, ub, err := LoadTex2D(layout)
	if err != nil {
		t.Fatal(err)
	}
	game, err := tx.DecodeMip(pk.UExp, ub, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.W != game.W || got.H != game.H {
		t.Fatalf("%dx%d, the game's is %dx%d", got.W, got.H, game.W, game.H)
	}
	alphaOff, colourOff := 0, 0
	for i := 0; i < len(got.Pix); i += 4 {
		if d := int(got.Pix[i+3]) - int(game.Pix[i+3]); d > 24 || d < -24 {
			alphaOff++
		}
		if got.Pix[i+3] > 200 && got.Pix[i] != game.Pix[i] {
			colourOff++
		}
	}
	if alphaOff > len(got.Pix)/4/100 || colourOff == 0 {
		t.Fatalf("shape differs at %d pixels, colour at %d", alphaOff, colourOff)
	}
	if _, err := textureFromZen(c, "UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_999", layout); err == nil {
		t.Fatal("expected an error for a texture the mod does not have")
	}
}

// New portraits are kept between runs: the picture, the photo and how it was made; changing one
// replaces its photo; taking it out deletes all of it.
func TestKeepPortraits(t *testing.T) {
	s := Settings{Workspace: t.TempDir()}
	inbox := filepath.Join(s.inboxDir(), "b1")
	os.MkdirAll(inbox, 0o755)
	pic := filepath.Join(inbox, "p.png")
	SaveImage(pic, NewImg(428, 540), SaveRGBA)
	photo := filepath.Join(inbox, "me.jpg")
	SaveImage(filepath.Join(inbox, "me.png"), NewImg(64, 64), SaveRGB)
	os.Rename(filepath.Join(inbox, "me.png"), photo)
	name := portraitPrefix + "Evans"
	state := json.RawMessage(`{"photo":"mine","zoom":1.5}`)
	sp, err := savePortrait(s, savePortraitReq{Name: name, Picture: pic, Photo: photo, State: state})
	if err != nil {
		t.Fatal(err)
	}
	if sp.Picture != filepath.Join(s.myPortraitsDir(), name+".png") || filepath.Base(sp.Photo) != name+".photo.jpg" || string(sp.State) != string(state) {
		t.Fatalf("kept %+v", sp)
	}
	os.RemoveAll(s.inboxDir()) // the inbox is emptied; the kept copies stay
	list := listSavedPortraits(s)
	if len(list) != 1 || list[0].Name != name || !fileExists(list[0].Picture) || !fileExists(list[0].Photo) {
		t.Fatalf("after the inbox is emptied: %+v", list)
	}
	// saved again from its own kept files (moved photo, no new upload)
	if _, err := savePortrait(s, savePortraitReq{Name: name, Picture: list[0].Picture, Photo: list[0].Photo, State: state}); err != nil {
		t.Fatal(err)
	}
	// no photo any more: the old one goes
	if sp, err = savePortrait(s, savePortraitReq{Name: name, Picture: list[0].Picture, State: json.RawMessage(`{"photo":"keep"}`)}); err != nil || sp.Photo != "" {
		t.Fatalf("%+v %v", sp, err)
	}
	if ms, _ := filepath.Glob(filepath.Join(s.myPortraitsDir(), "*.photo.*")); len(ms) != 0 {
		t.Fatalf("old photos left: %v", ms)
	}
	// files from elsewhere are refused
	if _, err := savePortrait(s, savePortraitReq{Name: name, Picture: "/etc/passwd"}); err == nil {
		t.Fatal("a picture from outside the program's folders was taken")
	}
	if _, err := savePortrait(s, savePortraitReq{Name: "T_Other", Picture: list[0].Picture}); err == nil {
		t.Fatal("a name that is not a portrait was taken")
	}
	if err := removeSavedPortrait(s, name); err != nil {
		t.Fatal(err)
	}
	if es, _ := os.ReadDir(s.myPortraitsDir()); len(es) != 0 {
		t.Fatalf("left after taking it out: %v", es)
	}
}

// Portraits inside a pilots mod built here that are not kept are listed, once per mod, and stop
// being listed once they are kept.
func TestPortraitsInBuiltMods(t *testing.T) {
	s := Settings{Workspace: t.TempDir()}
	port := "Live/Content/UI/Texture/HUD/SubtitleSpeakerPortrait/" + portraitPrefix
	writeMod(t, filepath.Join(s.modsOut(), "Portraits", "packed"), "Portraits_P", 1, []string{port + "Evans.uasset", port + "Tasha00_000.uasset"})
	writeMod(t, filepath.Join(s.modsOut(), "MySkin", "packed"), "MySkin_P", 2, []string{"Live/Content/Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_PP0021_su57_001_C.uasset"})
	got := portraitsInMods(s)
	if len(got) != 1 || got[0].Mod != "Portraits" || strings.Join(got[0].Names, ",") != portraitPrefix+"Evans,"+portraitPrefix+"Tasha00_000" {
		t.Fatalf("found %+v", got)
	}
	// keeping one leaves the other listed
	inbox := filepath.Join(s.inboxDir(), "b")
	os.MkdirAll(inbox, 0o755)
	SaveImage(filepath.Join(inbox, "p.png"), NewImg(428, 540), SaveRGBA)
	if _, err := savePortrait(s, savePortraitReq{Name: portraitPrefix + "Evans", Picture: filepath.Join(inbox, "p.png")}); err != nil {
		t.Fatal(err)
	}
	if got = portraitsInMods(s); len(got) != 1 || len(got[0].Names) != 1 || got[0].Names[0] != portraitPrefix+"Tasha00_000" {
		t.Fatalf("after keeping Evans: %+v", got)
	}
	// a mod that was not built here is not read from
	if _, err := recoverPortraits(s, "Someone_Elses"); err == nil {
		t.Fatal("expected an error for a mod not built here")
	}
}
