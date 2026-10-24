package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The Ladon's skin: ordinary 4096 x 4096 textures (_D colour, _MROS finish, _N normal), a body and a
// wing set, instead of streamed UDIM textures.
const ladonRaw = "/mnt/user-data/uploads/OP0045_ladn"

var ladonTex = []string{"Body_01_D", "Body_01_MROS", "Body_01_N", "Wing_01_D", "Wing_01_MROS", "Wing_01_N"}

// ladonSkin copies the Ladon's raw textures into a skin folder laid out as Extract leaves it.
func ladonSkin(t *testing.T) string {
	t.Helper()
	needFile(t, ladonRaw+"/Textures/01/T_OP0045_ladn_Body_01_D.ubulk")
	dir := filepath.Join(t.TempDir(), "raw", "Live", "Content", "Vehicles", "Aircraft", "OP", "OP0045_ladn", "Textures", "01")
	for _, n := range ladonTex {
		for _, ext := range []string{".uasset", ".uexp", ".ubulk"} {
			if err := copyFile(filepath.Join(ladonRaw, "Textures", "01", "T_OP0045_ladn_"+n+ext), filepath.Join(dir, "T_OP0045_ladn_"+n+ext)); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir
}

func TestPlainSkinTextures(t *testing.T) {
	dir := ladonSkin(t)
	texs, err := skinTextures(dir)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]Kind{}
	for _, lt := range texs {
		if !lt.Supported || !lt.Plain || lt.Width != 4096 || lt.BlocksW != 1 {
			t.Fatalf("%s: %+v", lt.Name, lt)
		}
		kinds[strings.TrimPrefix(lt.Name, "T_OP0045_ladn_")] = lt.Kind
	}
	want := map[string]Kind{"Body_01_D": KindColour, "Body_01_MROS": KindFinish, "Body_01_N": KindNormal, "Wing_01_D": KindColour, "Wing_01_MROS": KindFinish, "Wing_01_N": KindNormal}
	for n, k := range want {
		if kinds[n] != k {
			t.Errorf("%s is %s, want %s", n, kinds[n], k)
		}
	}
}

// Extract writes one picture per channel, without a tile number; the two-channel normal map gets
// no occlusion picture.
func TestPlainExtract(t *testing.T) {
	dir := ladonSkin(t)
	out := t.TempDir()
	var names []string
	for _, n := range []string{"Body_01_D", "Body_01_MROS", "Body_01_N"} {
		files, err := ExportTexture(texFilesFor(filepath.Join(dir, "T_OP0045_ladn_"+n+".uexp")), true, out, ".tga", nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			names = append(names, filepath.Base(f.Path))
		}
	}
	sort.Strings(names)
	want := []string{"T_OP0045_ladn_Body_01_D_BaseColor.tga", "T_OP0045_ladn_Body_01_MROS_BlueMask.tga", "T_OP0045_ladn_Body_01_MROS_Metallic.tga",
		"T_OP0045_ladn_Body_01_MROS_Roughness.tga", "T_OP0045_ladn_Body_01_N_Normal_DirectX.tga"}
	if strings.Join(names, " ") != strings.Join(want, " ") {
		t.Fatalf("wrote %v", names)
	}
	if w, h, err := ImageSize(filepath.Join(out, want[0])); err != nil || w != 4096 || h != 4096 {
		t.Fatalf("colour picture %d x %d (%v)", w, h, err)
	}
}

// Pictures for a body and a wing go to the texture their names point at; a picture that does not
// say which part it is for is used for neither.
func TestPlainPartsMatching(t *testing.T) {
	var names []string
	for _, n := range ladonTex {
		names = append(names, "T_OP0045_ladn_"+n)
	}
	files := []string{
		"/p/T_OP0045_ladn_Body_01_D_BaseColor.png", // as Extract names it
		"/p/MI_OP0045_ladn_Wing_01_Base_color.png", // as Substance Painter names a texture set after the material
		"/p/Base_color.png",                        // no part
		"/p/Ladon_Wing_Metallic.png",               // only "Wing"
		"/p/Ladon_Wing_Roughness.png",
	}
	sets := FindPainted(files, names, nil)
	body, _ := setsFor(sets, "T_OP0045_ladn_Body_01_D")
	wing, _ := setsFor(sets, "T_OP0045_ladn_Wing_01_D")
	if s := body[ChBase]; s == nil || s.Stitched != files[0] {
		t.Fatalf("body base colour %+v", s)
	}
	if s := wing[ChBase]; s == nil || s.Stitched != files[1] {
		t.Fatalf("wing base colour %+v", s)
	}
	wingFinish, _ := setsFor(sets, "T_OP0045_ladn_Wing_01_MROS")
	if wingFinish[ChMetallic] == nil || wingFinish[ChRoughness] == nil {
		t.Fatalf("wing finish %v", wingFinish)
	}
	bodyFinish, _ := setsFor(sets, "T_OP0045_ladn_Body_01_MROS")
	if bodyFinish[ChMetallic] != nil {
		t.Fatal("the wing's metallic picture was given to the body")
	}
	// an aircraft with one texture of each kind still takes pictures that do not say a part
	one := FindPainted([]string{"/p/Base_color.png"}, []string{"T_PP0021_su57_001_C", "T_PP0021_su57_001_MRX"}, nil)
	if v, _ := setsFor(one, "T_PP0021_su57_001_C"); v[ChBase] == nil {
		t.Fatal("a single colour texture lost its base colour")
	}
}

// Rebuilding keeps the game's layout, or lays the texture out as BC7 and/or with one size.
func TestPlainRebuild(t *testing.T) {
	dir := ladonSkin(t)
	f := texFilesFor(filepath.Join(dir, "T_OP0045_ladn_Wing_01_D.uexp"))
	pic := smoothPicture(4096, 4096, 1, 1, 9)
	for _, opts := range []TexOptions{{}, {BC7: true}, {NoMips: true}, {BC7: true, NoMips: true}} {
		vt, uexp, ubulk, err := LoadVTex(f)
		if err != nil {
			t.Fatal(err)
		}
		ua, _ := os.ReadFile(f.UAsset)
		img := &Img{W: pic.W, H: pic.H, Pix: append([]uint8(nil), pic.Pix...)}
		nua, nexp, nbulk, err := rebuildTexture(vt, ua, uexp, ubulk, img, opts, nil)
		if err != nil {
			t.Fatalf("%+v: %v", opts, err)
		}
		p, err := ParsePackage(nua, nexp)
		if err != nil {
			t.Fatalf("%+v: %v", opts, err)
		}
		tx, err := ParseTex2D(p)
		if err != nil {
			t.Fatalf("%+v: %v", opts, err)
		}
		wantFmt, wantMips := "PF_DXT1", 13
		if opts.BC7 {
			wantFmt = "PF_BC7"
		}
		if opts.NoMips {
			wantMips = 1
		}
		if tx.Format != wantFmt || len(tx.Mips) != wantMips || !tx.SRGB {
			t.Fatalf("%+v: %s with %d sizes", opts, tx.Format, len(tx.Mips))
		}
		if opts.NoMips && (nbulk != nil || tx.NeedsBulk()) {
			t.Fatalf("%+v: a single size should be kept inside the .uexp", opts)
		}
		if !opts.NoMips {
			want := 0
			for _, m := range tx.Mips {
				if m.InBulk {
					want += m.Size
				}
			}
			if len(nbulk) != want {
				t.Fatalf("%+v: .ubulk %d bytes for %d", opts, len(nbulk), want)
			}
		}
		rs, _ := p.DataResources()
		if len(rs) != wantMips {
			t.Fatalf("%+v: %d data resources", opts, len(rs))
		}
		got, err := tx.DecodeMip(nexp, nbulk, 0)
		if err != nil {
			t.Fatal(err)
		}
		limit := 2.5
		if opts.BC7 {
			limit = 1.0
		}
		if d := meanAbsDiff(got, pic); d > limit {
			t.Fatalf("%+v: differs from the picture by %.2f", opts, d)
		}
	}
}

// Replace a game skin, end to end, for the Ladon's body: colour as BC7, finish and normal map as
// the game keeps them.
func TestPlainReplaceBuild(t *testing.T) {
	e := newSlotEnv(t)
	dir := ladonSkin(t)
	paint := t.TempDir()
	m := NewImg(1024, 1024)
	for i := 0; i < len(m.Pix); i += 4 {
		m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = 30, 90, 160, 255
	}
	SaveImage(filepath.Join(paint, "Ladon_Body_01_Base_color.png"), m, SaveRGB)
	for _, ch := range []string{"Metallic", "Roughness"} {
		SaveImage(filepath.Join(paint, "Ladon_Body_01_"+ch+".png"), m, SaveRGB)
	}
	SaveImage(filepath.Join(paint, "Ladon_Body_01_Normal_DirectX.png"), m, SaveRGB)
	SaveImage(filepath.Join(paint, "Base_color.png"), m, SaveRGB) // says no part: used for neither
	r := planReq{SkinDir: dir, PaintDir: paint}
	rows, _, err := doPlan(r)
	if err != nil {
		t.Fatal(err)
	}
	ready := map[string]bool{}
	for _, row := range rows {
		ready[row.Name] = row.Recipe.Ready
		if strings.Contains(row.Name, "Wing") && row.Recipe.Ready {
			t.Errorf("%s should have no pictures", row.Name)
		}
		if strings.Contains(row.Name, "Wing_01_D") && !strings.Contains(row.Recipe.Missing, "Wing_01") {
			t.Errorf("the wing's colour texture does not say what its pictures need: %q", row.Recipe.Missing)
		}
	}
	for _, n := range []string{"Body_01_D", "Body_01_MROS", "Body_01_N"} {
		if !ready["T_OP0045_ladn_"+n] {
			t.Fatalf("%s not ready: %+v", n, rows)
		}
	}
	r.ModName, r.KeepInput, r.Tex = "LadonBlue", true, TexOptions{BC7: true}
	r.Textures = []string{"T_OP0045_ladn_Body_01_D", "T_OP0045_ladn_Body_01_MROS", "T_OP0045_ladn_Body_01_N"}
	if _, err := doBuild(testJob(), e.s, r); err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(e.s.modsOut(), "LadonBlue", "pack_input")
	for n, format := range map[string]string{"Body_01_D": "PF_BC7", "Body_01_MROS": "PF_DXT1", "Body_01_N": "PF_BC5"} {
		base := gameFile(in, "Live", "/Game/Vehicles/Aircraft/OP/OP0045_ladn/Textures/01/T_OP0045_ladn_"+n)
		vt, uexp, ubulk, err := LoadVTex(texFilesFor(base + ".uexp"))
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		if vt.Format != format || !vt.IsPlain() || vt.Mips != 13 {
			t.Fatalf("%s: %s, %d sizes", n, vt.Format, vt.Mips)
		}
		_ = uexp
		if n == "Body_01_D" {
			im, err := vt.DecodeMip(ubulk, 4)
			if err != nil {
				t.Fatal(err)
			}
			if c := im.Pix[(100*im.W+100)*4:]; int(c[0])-30 > 3 || 30-int(c[0]) > 3 || int(c[2])-160 > 3 || 160-int(c[2]) > 3 {
				t.Fatalf("colour %v", c[:3])
			}
		}
	}
}
