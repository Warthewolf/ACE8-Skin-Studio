package main

// Command-line use, mainly for testing:  ACE8SkinStudio cli <command> ...

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runCLI(args []string) int {
	fail := func(err error) int {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	if len(args) == 0 {
		fmt.Println("commands: info <tex.uexp> | export <tex.uexp> <outdir> [painter] [png|tga] | split <image> <outdir> [painter] [png|tga] | rebuild <tex.uexp> <painted folder> <outdir>")
		return 2
	}
	has := func(w string) bool {
		for _, a := range args {
			if a == w {
				return true
			}
		}
		return false
	}
	ext := ".png"
	if has("tga") {
		ext = ".tga"
	}
	t0 := time.Now()
	switch args[0] {
	case "info":
		f := texFilesFor(args[1])
		t, _, _, err := LoadVTex(f)
		if err != nil {
			return fail(err)
		}
		ua, _ := os.ReadFile(f.UAsset)
		fmt.Printf("%s %dx%d squares %dx%d tile %d+%d mips %d chunks %d srgb %v package %s\n", t.Format, t.Width, t.Height, t.BlocksW, t.BlocksH, t.Tile, t.Border, t.Mips, len(t.Chunks), t.SRGB, PackageName(ua))
	case "export":
		outs, err := ExportTexture(texFilesFor(args[1]), has("painter"), args[2], ext, nil)
		if err != nil {
			return fail(err)
		}
		for _, o := range outs {
			fmt.Println(o.Path)
		}
	case "split":
		outs, err := SplitPicture(args[1], 0, has("painter"), args[2], map[bool]string{true: ext, false: ""}[has("png") || has("tga")], nil)
		if err != nil {
			return fail(err)
		}
		for _, o := range outs {
			fmt.Println(o.Path)
		}
	case "rebuild":
		f := texFilesFor(args[1])
		t, uexp, ubulk, err := LoadVTex(f)
		if err != nil {
			return fail(err)
		}
		files, err := listImages(args[2])
		if err != nil {
			return fail(err)
		}
		name := strings.TrimSuffix(filepath.Base(f.UExp), ".uexp")
		sets := FindPainted(files, []string{name}, nil)
		kind := GuessKind(name)
		rec := PlanRecipe(kind, name, t, sets)
		fmt.Println("kind", kind, "recipe", rec)
		if !rec.Ready {
			return fail(fmt.Errorf("%s", rec.Missing))
		}
		img, notes, err := Compose(kind, name, t, ubulk, sets)
		if err != nil {
			return fail(err)
		}
		for _, n := range notes {
			fmt.Println("note:", n)
		}
		ne, nb, err := t.Rebuild(uexp, ubulk, img, nil)
		if err != nil {
			return fail(err)
		}
		os.MkdirAll(args[3], 0o755)
		os.WriteFile(filepath.Join(args[3], filepath.Base(f.UExp)), ne, 0o644)
		os.WriteFile(filepath.Join(args[3], filepath.Base(f.UBulk)), nb, 0o644)
	case "model":
		// model <out.glb> <in.glb>...   keeps every material whose name contains "decal"
		mats, err := InspectModels(args[2:])
		if err != nil {
			return fail(err)
		}
		keep := map[string]bool{}
		for _, m := range mats {
			fmt.Printf("%-34s %-34s verts %6d tris %6d u %.2f..%.2f v %.2f..%.2f %v\n", m.File, m.Name, m.Vertices, m.Triangles, m.UMin, m.UMax, m.VMin, m.VMax, m.Suggested)
			if m.Suggested {
				keep[m.File+"|"+m.Name] = true
			}
		}
		tiles, notes, err := BuildPaintModel(args[2:], keep, true, args[1])
		if err != nil {
			return fail(err)
		}
		fmt.Println("tiles", tiles, notes)
	default:
		return fail(fmt.Errorf("unknown command %q", args[0]))
	}
	fmt.Fprintf(os.Stderr, "done in %.1fs\n", time.Since(t0).Seconds())
	return 0
}
