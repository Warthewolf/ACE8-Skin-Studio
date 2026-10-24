package main

// A new skin's material slot.
//
// The game keeps each skin's material in a numbered folder of the aircraft, named after the skin's
// number: .../PP0021_su57/Materials/101/MI_PP0021_su57_101_Decal, with its icon at
// /Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_101. A new skin gets the same: a material slot of
// its own under its own number (Materials/603/MI_PP0021_su57_603_Decal, T_UI_Icon_Skin_su57_603).
//
// A skin's number is only settled when the shared skin list is built (a skin that arrives wanting a
// number already taken gets another), so these small files are made there: the skin's mod carries
// them under its own paths in slotfiles/, and the skin list builder renames them to the number
// the skin has and packs them with the skin list. The big files, the textures, stay in the skin's
// own mod under /Game/SkinStudio/<mod>/<skin>/ and never need renaming.
//
// In the mod's folder these files are kept as they are, in slotfiles/<skin>/ (the mod loader only
// looks at .utoc files, so it leaves them alone). Versions 2.0.2 to 2.0.8 kept them in one
// slotfiles.zip instead; that is still read, but never written, because a zip inside a shared zip
// gets a mod quarantined on Nexus Mods.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	slotFilesFolder = "slotfiles"     // in a mod's folder: slotfiles/<skin>/<name>.uasset and .uexp
	slotFilesZip    = "slotfiles.zip" // the same files as 2.0.2 to 2.0.8 kept them (read only)
	maxSlotFile     = 64 << 20
	// slotLayout is how skin lists are built now (see SkinListState.Layout).
	slotLayout = 3
)

// slotFilesDir is where a skin's material and icon are kept in a mod made here.
func (s Settings) slotFilesDir(key, slot string) string {
	return filepath.Join(s.projectDir(key), slotFilesFolder, slot)
}

// isSlotFile tells whether a slash path inside the slot files is <skin>/<name>.uasset or .uexp.
func isSlotFile(rel string) bool {
	parts := strings.Split(rel, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || !safeRel(rel) {
		return false
	}
	ext := strings.ToLower(path.Ext(parts[1]))
	return ext == ".uasset" || ext == ".uexp"
}

// slotFilesFromZip reads slot files out of a slotfiles.zip (as versions 2.0.2 to 2.0.8 wrote it).
func slotFilesFromZip(b []byte) (map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		if !isSlotFile(name) || f.UncompressedSize64 > maxSlotFile {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(r, maxSlotFile))
		r.Close()
		if err != nil {
			return nil, err
		}
		out[name] = data
	}
	return out, nil
}

// readSlotFolder gives the slot files of a folder holding them as <skin>/<name> (slash path -> bytes).
func readSlotFolder(dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if !isSlotFile(rel) {
			return nil
		}
		if st, err := d.Info(); err != nil || st.Size() > maxSlotFile {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = b
		return nil
	})
	return out, err
}

// modSlotFiles gives a mod's slot files from its folder: slotfiles/, or slotfiles.zip for a mod
// installed by 2.0.2 to 2.0.8. found is false when it has neither.
func modSlotFiles(modDir string) (files map[string][]byte, found bool, err error) {
	if st, err := os.Stat(filepath.Join(modDir, slotFilesFolder)); err == nil && st.IsDir() {
		files, err := readSlotFolder(filepath.Join(modDir, slotFilesFolder))
		return files, true, err
	}
	if b, err := os.ReadFile(filepath.Join(modDir, slotFilesZip)); err == nil {
		files, err := slotFilesFromZip(b)
		return files, true, err
	}
	return nil, false, nil
}

// writeSlotFolder puts slot files into dir (slotfiles/ of a mod), replacing what was there. With no
// files it removes the folder.
func writeSlotFolder(dir string, files map[string][]byte) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for rel, b := range files {
		if !isSlotFile(rel) {
			continue
		}
		dest := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// installSlotFiles copies a project's slot files into its mod's folder, and takes away the
// slotfiles.zip an older version left there.
func installSlotFiles(projectSlotDir, modDir string) error {
	files, err := readSlotFolder(projectSlotDir)
	if err != nil {
		return err
	}
	if err := writeSlotFolder(filepath.Join(modDir, slotFilesFolder), files); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(modDir, slotFilesZip)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// slotPackages gives a skin's own material, damage material and icon, under the paths it was made
// with. They come from the mod's slot files (see modSlotFiles) or, for a skin made here by version
// 2.0.0 or 2.0.1 (which packed them with the mod instead), from the workspace. None found is not an
// error: the skin then uses the files its mod carries.
func slotPackages(s Settings, im InstalledSlotMod, sl SlotDef) ([]*Package, error) {
	raw := map[string]map[string][]byte{} // name -> ".uasset"/".uexp" -> bytes
	put := func(name, ext string, b []byte) {
		if raw[name] == nil {
			raw[name] = map[string][]byte{}
		}
		raw[name][ext] = b
	}
	files, found, err := modSlotFiles(im.Dir)
	if err != nil {
		return nil, fmt.Errorf("the slot files of %s could not be read: %v", filepath.Base(im.Dir), err)
	}
	if found {
		for rel, b := range files {
			dir, file := path.Split(rel)
			if dir != sl.Key+"/" {
				continue
			}
			put(strings.TrimSuffix(file, path.Ext(file)), strings.ToLower(path.Ext(file)), b)
		}
	} else if m, err := loadProject(s, im.Manifest.Key); err == nil {
		dir := slotInputDir(s, m, sl.Key)
		for _, p := range []string{sl.Material, sl.Icon} {
			if p == "" {
				continue
			}
			n := packageBaseName(p)
			names := []string{n}
			if strings.HasSuffix(n, "_Decal") {
				names = append(names, strings.TrimSuffix(n, "_Decal")+"_DAM")
			}
			for _, n := range names {
				ua, e1 := os.ReadFile(filepath.Join(dir, n+".uasset"))
				ue, e2 := os.ReadFile(filepath.Join(dir, n+".uexp"))
				if e1 == nil && e2 == nil {
					put(n, ".uasset", ua)
					put(n, ".uexp", ue)
				}
			}
		}
	}
	var names []string
	for n := range raw {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []*Package
	for _, n := range names {
		ua, ue := raw[n][".uasset"], raw[n][".uexp"]
		if ua == nil || ue == nil {
			continue
		}
		p, err := ParsePackage(ua, ue)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", n, err)
		}
		out = append(out, p)
	}
	return out, nil
}

// swapNumber puts a skin number in place of another inside a game name:
// MI_PP0021_su57_101_Decal -> MI_PP0021_su57_603_Decal, T_UI_Icon_Skin_su57_101 -> ..._603.
// A name without the old number is given back unchanged.
func swapNumber(name, old, nw string) string {
	if i := strings.Index(name, "_"+old+"_"); i >= 0 {
		return name[:i+1] + nw + name[i+1+len(old):]
	}
	if strings.HasSuffix(name, "_"+old) {
		return strings.TrimSuffix(name, old) + nw
	}
	return name
}

func parentPath(p string) string {
	if i := strings.LastIndexByte(p, '/'); i > 0 {
		return p[:i]
	}
	return p
}

// numberedSlotFiles writes a skin's material slot under root (raw files, ready to pack): its
// material and damage material in <aircraft>/Materials/<number>/, its icon next to the game's icons,
// all named after its number. It gives the material and icon paths for the skin's row ("" where the
// skin has none of its own). A file that cannot be renamed safely is written under the path it was
// made with instead, which works just as well, and the reason is given back as a note.
func numberedSlotFiles(s Settings, root string, tmpl *SkinRow, im InstalledSlotMod, sl SlotDef, number int) (mat, icon string, notes []string, err error) {
	if number < firstSlotNumber || number > lastSlotNumber {
		return "", "", nil, fmt.Errorf("skin number %d is out of range", number)
	}
	pkgs, err := slotPackages(s, im, sl)
	if err != nil || len(pkgs) == 0 {
		return "", "", nil, err
	}
	nnn := fmt.Sprintf("%03d", number)
	tmplMat := packagePathOf(tmpl.Material())
	matFolder := packageBaseName(parentPath(tmplMat))           // 101
	matRoot := parentPath(parentPath(tmplMat))                  // .../PP0021_su57/Materials
	tmplIcon := packagePathOf(tmpl.Obj.Str("MenuIconImageRef")) // /Game/UI/Texture/Icon/Skin/T_UI_Icon_Skin_su57_101
	iconNum := fmt.Sprintf("%03d", slotOf(sl.Template))
	if matFolder == "" || !strings.HasPrefix(matRoot, "/Game/") {
		return "", "", nil, fmt.Errorf("the material of the skin it is based on (%s) is not in a folder of its own", tmplMat)
	}
	// where each file goes: package paths and object names
	type move struct {
		p              *Package
		oldObj, newObj string
		newPath        string
	}
	var moves []move
	for _, p := range pkgs {
		obj := packageBaseName(p.Name)
		mv := move{p: p, oldObj: obj}
		switch {
		case p.Name == sl.Icon && tmplIcon == "":
			mv.newObj, mv.newPath = obj, p.Name // no game icon folder to follow: it stays where it is
		case p.Name == sl.Icon:
			mv.newObj = swapNumber(obj, iconNum, nnn)
			if mv.newObj == obj {
				mv.newObj = obj + "_" + nnn
			}
			mv.newPath = parentPath(tmplIcon) + "/" + mv.newObj
		default: // the material and its damage material
			mv.newObj = swapNumber(obj, matFolder, nnn)
			mv.newPath = matRoot + "/" + nnn + "/" + mv.newObj
		}
		moves = append(moves, mv)
	}
	pathMoves := map[string]string{}
	objMoves := map[string]string{}
	for _, mv := range moves {
		pathMoves[mv.p.Name] = mv.newPath
		objMoves[mv.p.Name] = mv.oldObj
	}
	// all of them move, or (when one cannot be renamed safely) none: the paths they were made with
	// are unique to the skin too, so they work just as well
	type out struct {
		at     string
		ua, ue []byte
	}
	var outs []out
	for _, mv := range moves {
		renames := map[string]string{mv.oldObj: mv.newObj}
		// the other files of this skin that this one refers to move with it
		for i := range mv.p.Imports {
			ip := packagePathOf(mv.p.ImportPath(i))
			if np, ok := pathMoves[ip]; ok && ip != mv.p.Name {
				renames[ip] = np
				renames[objMoves[ip]] = packageBaseName(np)
			}
		}
		ua, ue, rerr := mv.p.Rename(mv.newPath, renames)
		if rerr != nil {
			notes = append(notes, fmt.Sprintf("%s keeps the paths it was made with: %v", sl.Name, rerr))
			outs = nil
			for _, mv := range moves {
				outs = append(outs, out{mv.p.Name, mv.p.UAsset, mv.p.UExp})
			}
			break
		}
		outs = append(outs, out{mv.newPath, ua, ue})
	}
	placedAt := map[string]string{} // made-with path -> where it was written
	for i, o := range outs {
		if err := writePackageFiles(gameFile(root, s.Project, o.at), o.ua, o.ue, nil); err != nil {
			return "", "", notes, err
		}
		placedAt[moves[i].p.Name] = o.at
	}
	if sl.Material != "" {
		mat = placedAt[sl.Material]
	}
	if sl.Icon != "" {
		icon = placedAt[sl.Icon]
	}
	return mat, icon, notes, nil
}
