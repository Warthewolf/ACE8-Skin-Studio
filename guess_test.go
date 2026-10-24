package main

import "testing"

func libSkin(dir, aircraft, folder string, root, textures int) LibSkin {
	sk := LibSkin{Dir: dir, Aircraft: aircraft, Folder: folder, Root: root}
	for i := 0; i < textures; i++ {
		sk.Textures = append(sk.Textures, LibTexture{Name: "T", Supported: true})
	}
	return sk
}

func TestGuessSkin(t *testing.T) {
	lib := []LibSkin{
		libSkin(`E:\X\Live\Content\Vehicles\Aircraft\PP\PP0021_su57\Textures\001`, "PP0021_su57", "001", 2, 4),
		libSkin(`E:\X\PP0021_su57\Textures\001`, "PP0021_su57", "001", 2, 4),
		libSkin(`E:\X\PP\PP0021_su57\Textures\001`, "PP0021_su57", "001", 2, 3),
		libSkin(`E:\X\PP0021_su57\Textures\002`, "PP0021_su57", "002", 2, 2),
		libSkin(`E:\X\PP0021_su57\Textures\001c`, "PP0021_su57", "001c", 2, 2),
		libSkin(`E:\X\PP0022_f16c\Textures\001`, "PP0022_f16c", "001", 2, 3),
		libSkin(`E:\X\Empty\001`, "PP0021_su57", "001", 1, 0), // nothing that can be rebuilt
	}
	painter := []string{
		`E:\X\su57_navy_paint\MI_PP0021_su57_001_Decal_Base_color_1001.tga`,
		`E:\X\su57_navy_paint\MI_PP0021_su57_001_Decal_Base_color_1002.tga`,
		`E:\X\su57_navy_paint\MI_PP0021_su57_001_Decal_Normal_1001.tga`,
	}
	g, ok := guessSkin(painter, lib)
	if !ok || g.Dir != `E:\X\PP0021_su57\Textures\001` || g.Label != "PP0021_su57 / 001" || g.Matched != 3 {
		t.Fatalf("painter names: %+v %v", g, ok)
	}
	// tiles made by the program, and a folder number with a letter
	g, ok = guessSkin([]string{"/in/T_PP0021_su57_001c_C_1001.png", "/in/T_PP0021_su57_001c_C_1002.png"}, lib)
	if !ok || g.Label != "PP0021_su57 / 001c" {
		t.Fatalf("001c: %+v %v", g, ok)
	}
	// only the short name
	g, ok = guessSkin([]string{"/in/su57_002_Base_color.png"}, lib)
	if !ok || g.Label != "PP0021_su57 / 002" {
		t.Fatalf("short: %+v %v", g, ok)
	}
	// two skins named equally: left for the painter to choose
	if g, ok = guessSkin([]string{"/in/T_PP0021_su57_001_C.png", "/in/T_PP0021_su57_002_C.png"}, lib); ok {
		t.Fatalf("tie answered: %+v", g)
	}
	// names that say nothing
	if g, ok = guessSkin([]string{"/in/Base_color_1001.png"}, lib); ok {
		t.Fatalf("nothing answered: %+v", g)
	}
	// the workspace's own copy wins over any other
	lib = append(lib, libSkin(`C:\W\raw\PP0021_su57\001`, "PP0021_su57", "001", 0, 1))
	if g, _ = guessSkin(painter, lib); g.Dir != `C:\W\raw\PP0021_su57\001` {
		t.Fatalf("workspace copy not preferred: %+v", g)
	}
	if namesToken("MI_PP0021_su57_0011_Decal", "PP0021_su57_001") || !namesToken("pp0021_SU57_001", "PP0021_su57_001") {
		t.Fatal("namesToken edges")
	}
}
