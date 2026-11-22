package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Pictures dropped for a skin that no texture is rebuilt from are listed with the reason, such as a
// normal map dropped for a skin that has none of its own (F-22A skin 201 shares one).
func TestPlanUnusedPictures(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, n := range []string{"T_PP0005_f22a_001_C.png", "T_PP0005_f22a_001_MRX.png", "T_PP0005_f22a_001_NO.png", "holiday.png"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, []byte("x"), 0o644)
		files = append(files, p)
	}
	rows := []planRow{
		{LibTexture: LibTexture{Name: "T_PP0005_f22a_201_C", Kind: KindColour}, Recipe: Recipe{Sources: []Source{{Files: files[:1]}}}},
		{LibTexture: LibTexture{Name: "T_PP0005_f22a_201_MRX", Kind: KindFinish}, Recipe: Recipe{Sources: []Source{{Files: files[1:2]}}}},
	}
	got := planUnused(planReq{Files: files}, rows)
	if len(got) != 2 || got[0].File != "T_PP0005_f22a_001_NO.png" || !strings.Contains(got[0].Why, "no normal map of its own") || got[1].File != "holiday.png" || !strings.Contains(got[1].Why, "does not match") {
		t.Fatalf("unused: %+v", got)
	}
	// with a normal map row that uses it, nothing is left over
	rows = append(rows, planRow{LibTexture: LibTexture{Name: "T_PP0005_f22a_201_NO", Kind: KindNormal}, Recipe: Recipe{Sources: []Source{{Files: files[2:3]}}}})
	if got := planUnused(planReq{Files: files[:3]}, rows); len(got) != 0 {
		t.Fatalf("all used: %+v", got)
	}
}
