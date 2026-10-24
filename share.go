package main

// Sharing skin mods: a .zip that people can install by hand or hand to ACE8 Skin Studio.
//
//   <Folder>.zip
//     skinstudio.json                 what the mod is (read by Skin Studio)
//     README.txt                      how to install it, with or without Skin Studio
//     icons/s1.png ...
//     Mods/<Folder>/                  the mod's own files: <key>_P.utoc/.ucas/.pak (textures),
//                                     slotfiles/<skin>/ (materials and icons), skinstudio.json, icons
//     Mods/<Folder>_SkinList/         a skin list with only this mod's skins, for people without
//                                     Skin Studio (two of these cannot be used at once)
//     ue4ss/Mods/SkinSlotText/        SkinSlotText (shows the names and descriptions), with this
//                                     mod's names in Scripts/texts/SkinStudio_<key>.ini
//
// There is no zip inside the zip: Nexus Mods quarantines a file that has one (zips made by 2.0.2
// to 2.0.8 had slotfiles.zip in them; they still import). SkinSlotText's DLL is fine there.
//
// Importing a zip made here installs only Mods/<Folder>; the skin list and texts are made again
// for everything installed, and a skin whose number is taken gets a free one. A zip made some other
// way is installed as it is; when it holds its own skin list, its skins are taken into the merged list.

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// ------------------------------------------------------------------ export

func findInstalledSlotMod(s Settings, key string) (*InstalledSlotMod, error) {
	mods, _ := scanSlotMods(s.ModsDir)
	for i := range mods {
		if mods[i].Manifest.Key == key {
			return &mods[i], nil
		}
	}
	return nil, errors.New("that skin mod is not in the mods folder")
}

// standaloneTables builds the skin list with only one mod's skins, numbered as they are now.
func standaloneTables(j *Job, s Settings, im *InstalledSlotMod) (string, error) {
	st, ut, err := loadBaseTables(j, s)
	if err != nil {
		return "", err
	}
	old := loadListState(s)
	placed, _, refs := assignNumbers(st, ut, []InstalledSlotMod{*im}, old)
	for _, p := range placed {
		if p.Problem != "" {
			return "", fmt.Errorf("%s: %s", p.Name, p.Problem)
		}
	}
	in := filepath.Join(s.exportsDir(), "work", "input")
	os.RemoveAll(filepath.Join(s.exportsDir(), "work"))
	for _, n := range writeMaterialSlots(s, in, st, placed, refs) {
		j.Log("Note: %s", n)
	}
	if err := addSlotRows(st.sc, st, ut, placed, refs); err != nil {
		return "", err
	}
	if err := writeTables(s, in, st, ut); err != nil {
		return "", err
	}
	// the numbers it was shared with become the numbers it asks for
	for _, p := range placed {
		if sl := im.Manifest.slot(p.Slot); sl != nil {
			sl.Number = p.Number
		}
	}
	if len(im.Manifest.Emblems) > 0 {
		placedE, _, err := addEmblemsToList(j, s, in, []InstalledSlotMod{*im}, old)
		if err != nil {
			return "", err
		}
		for _, p := range placedE {
			if p.Problem != "" {
				return "", fmt.Errorf("%s: %s", p.Name, p.Problem)
			}
			if e := im.Manifest.emblem(p.Emblem); e != nil {
				e.Number = p.Number
			}
		}
	}
	return in, nil
}

const shareReadme = `%[1]s
%[2]s

%[3]sThis skin mod was made with ACE8 Skin Studio %[4]s. It adds %[5]s; nothing of the game
is replaced.

%[8]s

Installing it with ACE8 Skin Studio (recommended)
--------------------------------------------------
Open ACE8 Skin Studio, go to "Skin slots" and drop this .zip onto it (or press "Import a skin mod").
Skin Studio installs it next to your other skin mods, gives each skin and emblem a free number if
the one it wants is already taken, and updates the names and descriptions. Any number of skin mods
can be installed this way at the same time. New skins and emblems unlock when you finish a campaign
mission, or straight away with "Unlock now" on Skin Studio's Save tools screen (game closed).

Installing it by hand
---------------------
Only do this if you do not have ACE8 Skin Studio, and only for one new-slot skin mod at a time: the
"_SkinList" folder replaces the game's skin list, so two of them cannot work together.
1. Copy both folders inside "Mods" (%[9]s and %[9]s_SkinList) into the folder your mod loader reads
   (for AC8 Mod Loader: ...\ue4ss\Mods\IoStoreLoaderMod).
2. Copy the "ue4ss" folder from this zip into the game's ...\Binaries\Win64 folder and let Windows
   merge it. It adds SkinSlotText, the small UE4SS mod that gives the new skins and emblems their
   names and descriptions, with this mod's names in ue4ss\Mods\SkinSlotText\Scripts\texts.
3. Start the game. New skin and emblem files need AC8 Mod Loader 1.1.0 or newer. They are unlocked
   when you finish a campaign mission, the way the game unlocks its own first skins and emblems.

The "slotfiles" folder holds the skins' materials and icons for ACE8 Skin Studio; the mod loader
does not use it, and it does no harm where it is.

Use modded files offline and in single player only.
`

// doExportMod writes the shareable zip of an installed slot mod.
func doExportMod(j *Job, s Settings, key string) (any, error) {
	im, err := findInstalledSlotMod(s, key)
	if err != nil {
		return nil, err
	}
	m := im.Manifest
	j.SetStep("Making a skin list with only this mod's skins", 0.1)
	in, err := standaloneTables(j, s, im)
	if err != nil {
		return nil, err
	}
	listName := m.Key + "_SkinList_P"
	files, err := packFolder(j, s, in, filepath.Join(s.exportsDir(), "work", "packed"), listName)
	if err != nil {
		return nil, err
	}
	j.SetStep("Writing the zip", 0.6)
	if err := os.MkdirAll(s.exportsDir(), 0o755); err != nil {
		return nil, err
	}
	out := filepath.Join(s.exportsDir(), m.Folder+".zip")
	tmp := out + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return nil, err
	}
	zw := zip.NewWriter(f)
	add := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	addFile := func(name, src string) error {
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, in)
		return err
	}
	mb, _ := json.MarshalIndent(m, "", "  ")
	var errs []error
	errs = append(errs, add(manifestName, mb))
	var lines []string
	for _, sl := range m.Slots {
		lines = append(lines, fmt.Sprintf("- %s (%s): %s", sl.Name, sl.Vehicle, strings.ReplaceAll(firstLine(sl.Description), "\n", " ")))
	}
	for _, e := range m.Emblems {
		lines = append(lines, fmt.Sprintf("- %s (emblem): %s", e.Name, strings.ReplaceAll(firstLine(e.Description), "\n", " ")))
	}
	by := ""
	if m.Author != "" {
		by = "By " + m.Author + ". "
	}
	readme := fmt.Sprintf(shareReadme, m.Title, strings.Repeat("=", max(3, len([]rune(m.Title)))), by, appVersion, modContents(m), "", "",
		strings.Join(lines, "\n"), m.Folder)
	errs = append(errs, add("README.txt", []byte(strings.ReplaceAll(readme, "\n", "\r\n"))))
	es, _ := os.ReadDir(im.Dir)
	for _, e := range es {
		if e.IsDir() || strings.EqualFold(e.Name(), slotFilesZip) {
			continue
		}
		errs = append(errs, addFile("Mods/"+m.Folder+"/"+e.Name(), filepath.Join(im.Dir, e.Name())))
	}
	// the skins' materials and icons as plain files (a mod installed by 2.0.2 to 2.0.8 has them in
	// slotfiles.zip: they come out of it here, so the shared zip has no zip inside)
	slot, _, err := modSlotFiles(im.Dir)
	if err != nil {
		errs = append(errs, err)
	}
	var rels []string
	for rel := range slot {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		errs = append(errs, add("Mods/"+m.Folder+"/"+slotFilesFolder+"/"+rel, slot[rel]))
	}
	icons := []string{}
	for _, sl := range m.Slots {
		icons = append(icons, sl.IconFile)
	}
	for _, e := range m.Emblems {
		icons = append(icons, e.IconFile)
	}
	for _, icon := range icons {
		if icon != "" {
			p := filepath.Join(im.Dir, filepath.FromSlash(icon))
			if _, err := os.Stat(p); err == nil {
				errs = append(errs, addFile("Mods/"+m.Folder+"/"+icon, p), addFile(icon, p))
			}
		}
	}
	for _, p := range files {
		errs = append(errs, addFile("Mods/"+m.Folder+"_SkinList/"+filepath.Base(p), p))
	}
	sst := skinSlotTextFiles()
	var names []string
	for n := range sst {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		errs = append(errs, add("ue4ss/Mods/SkinSlotText/"+n, sst[n]))
	}
	errs = append(errs, add("ue4ss/Mods/SkinSlotText/Scripts/texts/"+textFilePrefix+m.Key+".ini", []byte(modTexts(m))))
	errs = append(errs, zw.Close(), f.Close())
	for _, e := range errs {
		if e != nil {
			os.Remove(tmp)
			return nil, fmt.Errorf("the zip could not be written: %v", e)
		}
	}
	os.Remove(out)
	if err := os.Rename(tmp, out); err != nil {
		return nil, err
	}
	os.RemoveAll(filepath.Join(s.exportsDir(), "work"))
	st, _ := os.Stat(out)
	j.Log("Wrote %s (%s).", out, humanSize(st.Size()))
	return map[string]any{"zip": out, "dir": s.exportsDir(), "size": st.Size(), "skins": len(m.Slots), "emblems": len(m.Emblems)}, nil
}

// modContents says what a mod adds: "2 new skins in slots of their own and 1 new emblem".
func modContents(m *ModManifest) string {
	var parts []string
	switch n := len(m.Slots); n {
	case 0:
	case 1:
		parts = append(parts, "1 new skin in a slot of its own")
	default:
		parts = append(parts, fmt.Sprintf("%d new skins in slots of their own", n))
	}
	switch n := len(m.Emblems); n {
	case 0:
	case 1:
		parts = append(parts, "1 new emblem")
	default:
		parts = append(parts, fmt.Sprintf("%d new emblems", n))
	}
	if len(parts) == 0 {
		return "no new skins"
	}
	return strings.Join(parts, " and ")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%d KB", max(1, n/1024))
}

// ------------------------------------------------------------------ reading a zip or folder

// pkgSource is a zip or folder being imported.
type pkgSource struct {
	zr    *zip.ReadCloser
	dir   string
	files []string // slash paths
}

func openSource(p string) (*pkgSource, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, errors.New("that file or folder does not exist")
	}
	src := &pkgSource{}
	if st.IsDir() {
		src.dir = p
		count := 0
		err := filepath.WalkDir(p, func(f string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if count++; count > 20000 {
				return filepath.SkipAll
			}
			if !d.IsDir() {
				rel, _ := filepath.Rel(p, f)
				src.files = append(src.files, filepath.ToSlash(rel))
			}
			return nil
		})
		return src, err
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, errors.New("that is not a .zip file this program can open (.7z and .rar files need unpacking first)")
	}
	src.zr = zr
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() {
			src.files = append(src.files, strings.ReplaceAll(f.Name, `\`, "/"))
		}
	}
	return src, nil
}

func (src *pkgSource) Close() {
	if src.zr != nil {
		src.zr.Close()
	}
}

func safeRel(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, ":") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

func (src *pkgSource) open(name string) (io.ReadCloser, error) {
	if !safeRel(name) {
		return nil, errors.New("unsafe path in the archive")
	}
	if src.dir != "" {
		return os.Open(filepath.Join(src.dir, filepath.FromSlash(name)))
	}
	for _, f := range src.zr.File {
		if strings.ReplaceAll(f.Name, `\`, "/") == name {
			return f.Open()
		}
	}
	return nil, os.ErrNotExist
}

func (src *pkgSource) read(name string, limit int64) ([]byte, error) {
	r, err := src.open(name)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if int64(len(b)) > limit {
		return nil, errors.New("file too large")
	}
	return b, err
}

func (src *pkgSource) extract(name, dest string) error {
	r, err := src.open(name)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	os.Remove(dest)
	return os.Rename(tmp, dest)
}

// ------------------------------------------------------------------ import plan

type ImportSkin struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Desc    string `json:"description"`
	Vehicle string `json:"vehicle"`
	Plane   string `json:"plane"`
	Wanted  int    `json:"wanted"`
	Number  int    `json:"number"` // what it will get
	Taken   string `json:"taken,omitempty"`
	Icon    string `json:"icon,omitempty"` // a picture of the icon, saved for showing
}

type ImportEmblem struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Desc   string `json:"description"`
	Wanted int    `json:"wanted"`
	Number int    `json:"number"` // what it will get
	Icon   string `json:"icon,omitempty"`
}

type ImportContainer struct {
	Base     string   `json:"base"`  // path inside the archive without the ending
	Files    []string `json:"files"` // archive paths
	Folder   string   `json:"folder"`
	SkinList bool     `json:"skinList"`
}

type ImportPlan struct {
	Source     string            `json:"source"`
	Studio     bool              `json:"studio"` // made by ACE8 Skin Studio
	Manifest   *ModManifest      `json:"manifest,omitempty"`
	Update     bool              `json:"update"` // the same mod is installed already and will be replaced
	Folder     string            `json:"folder"`
	Skins      []ImportSkin      `json:"skins"`
	Emblems    []ImportEmblem    `json:"emblems"`
	Containers []ImportContainer `json:"containers"`
	Texts      []string          `json:"texts"` // SkinSlotText files it brings
	Notes      []string          `json:"notes"`
	SlotDir    string            `json:"-"` // where its slotfiles/ folder is inside the zip or folder ("x/slotfiles/")
	SlotZip    string            `json:"-"` // or its slotfiles.zip (made by 2.0.2 to 2.0.8)
}

// findStudioManifest finds the mod's skinstudio.json: the one at the top, or the one next to its container.
func findStudioManifest(src *pkgSource) (*ModManifest, string) {
	var cands []string
	for _, f := range src.files {
		if path.Base(f) == manifestName {
			cands = append(cands, f)
		}
	}
	sort.Slice(cands, func(i, j int) bool { return strings.Count(cands[i], "/") < strings.Count(cands[j], "/") })
	for _, c := range cands {
		b, err := src.read(c, 1<<20)
		if err != nil {
			continue
		}
		var m ModManifest
		if json.Unmarshal(b, &m) != nil || m.Format != manifestFormat || m.check() != nil {
			continue
		}
		return &m, c
	}
	return nil, ""
}

// containersIn groups the archive's container files by name.
func containersIn(src *pkgSource) []ImportContainer {
	groups := map[string]*ImportContainer{}
	var order []string
	for _, f := range src.files {
		ext := strings.ToLower(path.Ext(f))
		if !modFileExts[ext] || !safeRel(f) {
			continue
		}
		base := strings.TrimSuffix(f, path.Ext(f))
		g := groups[strings.ToLower(base)]
		if g == nil {
			g = &ImportContainer{Base: base}
			groups[strings.ToLower(base)] = g
			order = append(order, strings.ToLower(base))
		}
		g.Files = append(g.Files, f)
	}
	var out []ImportContainer
	for _, k := range order {
		g := groups[k]
		hasToc := false
		for _, f := range g.Files {
			if strings.EqualFold(path.Ext(f), ".utoc") {
				hasToc = true
			}
		}
		if hasToc {
			out = append(out, *g)
		}
	}
	return out
}

// tocNames reads a container table from the archive and tells whether it holds a skin list.
func tocHasSkinList(src *pkgSource, c ImportContainer) bool {
	for _, f := range c.Files {
		if !strings.EqualFold(path.Ext(f), ".utoc") {
			continue
		}
		b, err := src.read(f, maxModTOC)
		if err != nil {
			continue
		}
		info, err := parseTOC(b)
		if err != nil {
			continue
		}
		for _, n := range info.Names {
			l := gameFileLabel(n)
			if l == skinListLabel || l == unlockListLabel {
				return true
			}
		}
	}
	return false
}

func planImport(j *Job, s Settings, p string) (*ImportPlan, error) {
	if err := needModsFolder(s); err != nil {
		return nil, err
	}
	src, err := openSource(p)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	plan := &ImportPlan{Source: p, Skins: []ImportSkin{}, Emblems: []ImportEmblem{}, Containers: []ImportContainer{}, Texts: []string{}, Notes: []string{}}
	m, mpath := findStudioManifest(src)
	all := containersIn(src)
	if m != nil {
		plan.Studio, plan.Manifest = true, m
		// the mod's own container: the one named in the manifest
		for _, c := range all {
			if strings.EqualFold(path.Base(c.Base), m.Container) {
				c.Folder = m.Folder
				plan.Containers = append(plan.Containers, c)
				break
			}
		}
		// the skins' materials and icons: a slotfiles folder (2.0.9 on) or slotfiles.zip (2.0.2 to 2.0.8)
		for _, base := range []string{"Mods/" + m.Folder, path.Dir(mpath)} {
			dirCand := strings.TrimPrefix(path.Join(base, slotFilesFolder)+"/", "./")
			zipCand := strings.TrimPrefix(path.Join(base, slotFilesZip), "./")
			for _, f := range src.files {
				if plan.SlotDir == "" && len(f) > len(dirCand) && strings.EqualFold(f[:len(dirCand)], dirCand) && isSlotFile(f[len(dirCand):]) {
					plan.SlotDir = f[:len(dirCand)]
				}
				if plan.SlotZip == "" && strings.EqualFold(f, zipCand) {
					plan.SlotZip = f
				}
			}
		}
		if plan.SlotDir != "" {
			plan.SlotZip = ""
		}
		hasSlotFiles := plan.SlotDir != "" || plan.SlotZip != ""
		if len(plan.Containers) == 0 && len(m.Emblems) > 0 {
			return nil, fmt.Errorf("the zip says it holds %s, but that container is not in it", m.Container)
		}
		if len(plan.Containers) == 0 && len(m.Slots) > 0 {
			for _, sl := range m.Slots {
				// textures are always in the container; a material or icon is, when there are no slot files
				if len(sl.Textures) > 0 || !hasSlotFiles && (sl.Material != "" || sl.Icon != "") {
					return nil, fmt.Errorf("the zip says it holds %s, but that container is not in it", m.Container)
				}
			}
		}
		installed, _ := scanSlotMods(s.ModsDir)
		plan.Folder = uniqueFolder(s.ModsDir, m.Folder)
		for _, im := range installed {
			if im.Manifest.Key == m.Key {
				plan.Update = true
				plan.Folder = filepath.Base(im.Dir)
			}
		}
		// what numbers its skins would get, without changing anything yet
		st, ut, err := loadBaseTables(j, s)
		if err != nil {
			return nil, err
		}
		var others []InstalledSlotMod
		for _, im := range installed {
			if im.Manifest.Key != m.Key {
				others = append(others, im)
			}
		}
		incoming := InstalledSlotMod{Dir: filepath.Join(s.ModsDir, plan.Folder), Manifest: m}
		placed, _, _ := assignNumbers(st, ut, append(others, incoming), loadListState(s))
		owner := map[string]string{}
		for _, ps := range placed {
			if ps.Mod != m.Key && ps.Number > 0 {
				owner[fmt.Sprintf("%d/%d", ps.Aircraft, ps.Number)] = ps.Name + " (" + ps.ModTitle + ")"
			}
		}
		iconDir := filepath.Join(s.importsDir(), "preview", m.Key)
		os.RemoveAll(iconDir)
		for _, ps := range placed {
			if ps.Mod != m.Key {
				continue
			}
			sl := m.slot(ps.Slot)
			sk := ImportSkin{Key: ps.Slot, Name: sl.Name, Desc: sl.Description, Vehicle: sl.Vehicle, Plane: sl.Plane, Wanted: sl.Number, Number: ps.Number}
			if ps.Problem != "" {
				plan.Notes = append(plan.Notes, sl.Name+": "+ps.Problem+".")
			}
			if sl.Number != 0 && ps.Number != sl.Number {
				if o := owner[fmt.Sprintf("%d/%d", sl.Aircraft, sl.Number)]; o != "" {
					sk.Taken = o
				} else {
					sk.Taken = "the game or another mod"
				}
			}
			if sl.IconFile != "" {
				for _, cand := range []string{path.Join(path.Dir(mpath), sl.IconFile), sl.IconFile, "Mods/" + m.Folder + "/" + sl.IconFile} {
					if b, err := src.read(cand, 4<<20); err == nil {
						dst := filepath.Join(iconDir, sl.Key+".png")
						os.MkdirAll(iconDir, 0o755)
						if os.WriteFile(dst, b, 0o644) == nil {
							sk.Icon = dst
						}
						break
					}
				}
			}
			plan.Skins = append(plan.Skins, sk)
		}
		// and the numbers its emblems would get
		if len(m.Emblems) > 0 {
			et, eut, err := loadEmblemTables(j, s)
			if err != nil {
				return nil, err
			}
			placedE, _, _ := assignEmblemNumbers(et, eut, append(others, incoming), loadListState(s))
			for _, pe := range placedE {
				if pe.Mod != m.Key {
					continue
				}
				e := m.emblem(pe.Emblem)
				ie := ImportEmblem{Key: e.Key, Name: e.Name, Desc: e.Description, Wanted: e.Number, Number: pe.Number}
				if pe.Problem != "" {
					plan.Notes = append(plan.Notes, e.Name+": "+pe.Problem+".")
				}
				if e.IconFile != "" {
					for _, cand := range []string{path.Join(path.Dir(mpath), e.IconFile), e.IconFile, "Mods/" + m.Folder + "/" + e.IconFile} {
						if b, err := src.read(cand, 4<<20); err == nil {
							dst := filepath.Join(iconDir, e.Key+".png")
							os.MkdirAll(iconDir, 0o755)
							if os.WriteFile(dst, b, 0o644) == nil {
								ie.Icon = dst
							}
							break
						}
					}
				}
				plan.Emblems = append(plan.Emblems, ie)
			}
		}
		return plan, nil
	}
	// not made here: install every container as it is
	if len(all) == 0 {
		return nil, errors.New("no mod files (.utoc, .ucas, .pak) were found in it")
	}
	used := map[string]bool{}
	topName := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	for _, c := range all {
		folder := path.Base(path.Dir(c.Base))
		if folder == "." || folder == "" || strings.EqualFold(folder, "Mods") || strings.EqualFold(folder, "~mods") || strings.EqualFold(folder, "Paks") || strings.EqualFold(folder, "IoStoreLoaderMod") {
			folder = path.Base(c.Base)
			if len(all) == 1 {
				folder = topName
			}
		}
		folder = strings.TrimSuffix(folder, "_P")
		name := uniqueFolder(s.ModsDir, folder)
		for used[strings.ToLower(name)] {
			name = uniqueFolder(s.ModsDir, name+"_2")
		}
		used[strings.ToLower(name)] = true
		c.Folder = name
		c.SkinList = tocHasSkinList(src, c)
		plan.Containers = append(plan.Containers, c)
		if c.SkinList {
			plan.Notes = append(plan.Notes, path.Base(c.Base)+" has its own skin list. Its skins are taken into the merged skin list, so it works next to your other skin mods.")
		}
	}
	for _, f := range src.files {
		if strings.EqualFold(path.Ext(f), ".ini") && strings.Contains(strings.ToLower(f), "skinslottext/") {
			plan.Texts = append(plan.Texts, f)
		}
	}
	return plan, nil
}

// doImport installs what planImport described.
func doImport(j *Job, s Settings, p string) (any, error) {
	plan, err := planImport(j, s, p)
	if err != nil {
		return nil, err
	}
	src, err := openSource(p)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	installed := []string{}
	if plan.Studio {
		m := plan.Manifest
		m.Folder = plan.Folder
		dest := filepath.Join(s.ModsDir, plan.Folder)
		// an update replaces the old files of the same mod
		if plan.Update {
			if es, err := os.ReadDir(dest); err == nil {
				for _, e := range es {
					if modFileExts[strings.ToLower(filepath.Ext(e.Name()))] {
						if err := os.Remove(filepath.Join(dest, e.Name())); err != nil {
							return nil, fmt.Errorf("the old copy could not be replaced (%v). If the game is running, close it and try again", err)
						}
					}
				}
			}
		}
		j.SetStep("Copying the mod into the mods folder", 0.2)
		for _, c := range plan.Containers {
			for _, f := range c.Files {
				if err := src.extract(f, filepath.Join(dest, path.Base(f))); err != nil {
					return nil, fmt.Errorf("could not copy %s (%v). If the game is running, close it and try again", path.Base(f), err)
				}
			}
		}
		// the skins' materials and icons always go into a slotfiles folder, also from an older zip
		slot := map[string][]byte{}
		switch {
		case plan.SlotDir != "":
			for _, f := range src.files {
				if len(f) <= len(plan.SlotDir) || f[:len(plan.SlotDir)] != plan.SlotDir {
					continue
				}
				rel := f[len(plan.SlotDir):]
				if !isSlotFile(rel) || m.slot(path.Dir(rel)) == nil {
					continue
				}
				b, err := src.read(f, maxSlotFile)
				if err != nil {
					return nil, fmt.Errorf("could not read %s (%v)", f, err)
				}
				slot[rel] = b
			}
		case plan.SlotZip != "":
			b, err := src.read(plan.SlotZip, 256<<20)
			if err == nil {
				slot, err = slotFilesFromZip(b)
			}
			if err != nil {
				return nil, fmt.Errorf("could not read %s (%v)", slotFilesZip, err)
			}
			for rel := range slot {
				if m.slot(path.Dir(rel)) == nil {
					delete(slot, rel)
				}
			}
		}
		if err := writeSlotFolder(filepath.Join(dest, slotFilesFolder), slot); err != nil {
			return nil, fmt.Errorf("could not copy the skins' materials and icons (%v)", err)
		}
		os.Remove(filepath.Join(dest, slotFilesZip)) // from an older copy of the mod
		for _, sk := range plan.Skins {
			if sk.Icon != "" {
				copyFile(sk.Icon, filepath.Join(dest, "icons", sk.Key+".png"))
			}
		}
		for _, e := range plan.Emblems {
			if e.Icon != "" {
				copyFile(e.Icon, filepath.Join(dest, "icons", e.Key+".png"))
			}
		}
		if err := writeManifest(filepath.Join(dest, manifestName), m); err != nil {
			return nil, err
		}
		installed = append(installed, dest)
	} else {
		j.SetStep("Copying the mod files into the mods folder", 0.2)
		for _, c := range plan.Containers {
			dest := filepath.Join(s.ModsDir, c.Folder)
			for _, f := range c.Files {
				if err := src.extract(f, filepath.Join(dest, path.Base(f))); err != nil {
					return nil, fmt.Errorf("could not copy %s (%v)", path.Base(f), err)
				}
			}
			installed = append(installed, dest)
		}
		if dir := s.textsDir(); dir != "" {
			for _, t := range plan.Texts {
				name := path.Base(t)
				if strings.HasPrefix(name, textFilePrefix) {
					name = "Imported_" + name // never mistaken for one of this program's own files
				}
				src.extract(t, filepath.Join(dir, name))
			}
		}
	}
	res := map[string]any{"plan": plan, "installed": installed}
	needList := plan.Studio
	for _, c := range plan.Containers {
		if c.SkinList {
			needList = true
		}
	}
	if needList {
		j.SetStep("Updating the skin list", 0.6)
		list, err := RebuildSkinList(j, s)
		if err != nil {
			return res, fmt.Errorf("the mod was installed, but the skin list could not be updated: %v", err)
		}
		res["list"] = list
		res["setup"] = slotSetupProblems(s)
	}
	return res, nil
}
