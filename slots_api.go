package main

// The window's requests for skin slots, sharing and importing.

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type slotView struct {
	SlotDef
	Number  int    `json:"number"` // the number it has now (the one it asked for is Wanted)
	Wanted  int    `json:"wanted"`
	Moved   bool   `json:"moved"`
	IconPNG string `json:"iconPng,omitempty"` // full path of the icon picture
}

type emblemView struct {
	EmblemDef
	Number  int    `json:"number"` // the number it has now (the one it asked for is Wanted)
	Wanted  int    `json:"wanted"`
	Moved   bool   `json:"moved"`
	IconPNG string `json:"iconPng,omitempty"`
}

type slotModView struct {
	Key       string       `json:"key"`
	Title     string       `json:"title"`
	Author    string       `json:"author"`
	Folder    string       `json:"folder"`
	Dir       string       `json:"dir"`
	Own       bool         `json:"own"` // made on this computer, so skins can be added and rebuilt
	Slots     []slotView   `json:"slots"`
	Emblems   []emblemView `json:"emblems"`
	Installed bool         `json:"installed"`
}

// emblemViews describes a mod's emblems; dir is where its icons folder is.
func emblemViews(m *ModManifest, dir string, num map[string]int) []emblemView {
	out := []emblemView{}
	for _, e := range m.Emblems {
		v := emblemView{EmblemDef: e, Number: num[m.Key+"/"+e.Key], Wanted: e.Number}
		v.Moved = e.Number != 0 && v.Number != 0 && e.Number != v.Number
		if e.IconFile != "" {
			p := filepath.Join(dir, filepath.FromSlash(e.IconFile))
			if _, err := os.Stat(p); err == nil {
				v.IconPNG = p
			}
		}
		out = append(out, v)
	}
	return out
}

type slotsOverview struct {
	OK        bool          `json:"ok"`
	Note      string        `json:"note"`
	Mods      []slotModView `json:"mods"`
	Removed   []slotModView `json:"removed"` // mods made here that are not in the mods folder
	TableMods []string      `json:"tableMods"`
	Problems  []string      `json:"problems"`
	Setup     []string      `json:"setup"`
	Loader    string        `json:"loader"`
	SST       sstStatus     `json:"sst"`
	ListBuilt string        `json:"listBuilt"`
	Stale     string        `json:"stale"` // why the skin list should be rebuilt ("" when it is up to date)
	ListDir   string        `json:"listDir"`
	Exports   string        `json:"exports"`
}

func slotsOverviewNow(s Settings) slotsOverview {
	ov := slotsOverview{Mods: []slotModView{}, Removed: []slotModView{}, TableMods: []string{}, Problems: []string{}, Setup: []string{}}
	c := s.checks()["mods"]
	ov.OK, ov.Note = c.OK, c.Note
	ov.Exports = s.exportsDir()
	if !c.OK {
		return ov
	}
	st := loadListState(s)
	ov.ListBuilt = st.Built
	if _, err := os.Stat(filepath.Join(s.ModsDir, skinListMod)); err == nil {
		ov.ListDir = filepath.Join(s.ModsDir, skinListMod)
	} else {
		ov.ListBuilt = ""
	}
	num := map[string]int{}
	for _, a := range st.Assign {
		num[a.Mod+"/"+a.Slot] = a.Number
	}
	for _, a := range st.Emblems {
		num[a.Mod+"/"+a.Emblem] = a.Number
	}
	own := map[string]bool{}
	for _, p := range listProjects(s) {
		own[p.Key] = true
	}
	mods, problems := scanSlotMods(s.ModsDir)
	ov.Problems = append(ov.Problems, problems...)
	installed := map[string]bool{}
	for _, im := range mods {
		m := im.Manifest
		installed[m.Key] = true
		v := slotModView{Key: m.Key, Title: m.Title, Author: m.Author, Folder: filepath.Base(im.Dir), Dir: im.Dir, Own: own[m.Key], Installed: true, Slots: []slotView{},
			Emblems: emblemViews(m, im.Dir, num)}
		for _, sl := range m.Slots {
			sv := slotView{SlotDef: sl, Number: num[m.Key+"/"+sl.Key], Wanted: sl.Number}
			sv.Moved = sl.Number != 0 && sv.Number != 0 && sl.Number != sv.Number
			if sl.IconFile != "" {
				p := filepath.Join(im.Dir, filepath.FromSlash(sl.IconFile))
				if _, err := os.Stat(p); err == nil {
					sv.IconPNG = p
				}
			}
			v.Slots = append(v.Slots, sv)
		}
		ov.Mods = append(ov.Mods, v)
	}
	for _, p := range listProjects(s) {
		if !installed[p.Key] {
			v := slotModView{Key: p.Key, Title: p.Title, Author: p.Author, Folder: p.Folder, Own: true, Slots: []slotView{},
				Emblems: emblemViews(p, s.projectDir(p.Key), map[string]int{})}
			for _, sl := range p.Slots {
				sv := slotView{SlotDef: sl, Wanted: sl.Number}
				if sl.IconFile != "" {
					sv.IconPNG = filepath.Join(s.projectDir(p.Key), filepath.FromSlash(sl.IconFile))
				}
				v.Slots = append(v.Slots, sv)
			}
			ov.Removed = append(ov.Removed, v)
		}
	}
	folders := map[string]bool{}
	for _, im := range mods {
		folders[strings.ToLower(filepath.Base(im.Dir))] = true
	}
	for _, am := range findTableMods(s, mods) {
		ov.TableMods = append(ov.TableMods, am.Folder)
		if base, ok := strings.CutSuffix(strings.ToLower(am.Folder), "_skinlist"); ok && folders[base] {
			ov.Problems = append(ov.Problems, am.Folder+" is the stand-alone skin list that comes with "+am.Folder[:len(base)]+" for people without Skin Studio. The shared skin list already covers it, so it can be removed on the Mods screen.")
		}
	}
	sort.Strings(ov.TableMods)
	// without loadorder.txt the loader goes A to Z and the last one wins: a mod with its own skin list
	// named after ours would hide the shared list
	if _, err := os.Stat(filepath.Join(s.ModsDir, "loadorder.txt")); err != nil {
		for _, n := range ov.TableMods {
			if strings.ToLower(n) > strings.ToLower(skinListMod) {
				ov.Problems = append(ov.Problems, n+" comes after "+skinListMod+" in A to Z order, so its own skin list is used instead of the shared one. Rename its folder, or set the order with the mod loader's ModLauncher (loadorder.txt).")
			}
		}
	}
	// is the skin list still right for what is installed (and for this version of the game)?
	if ov.ListDir != "" || len(mods) > 0 {
		now := builtKeys(mods)
		switch {
		case ov.ListDir == "" && len(now) > 0:
			ov.Stale = "The skin mods are installed but the shared skin list is missing, so the game does not know their skins. Press Rebuild the skin list."
		case st.Game != "" && st.Game != paksSignature(s.PaksDir):
			ov.Stale = "The game has been updated since the skin list was built. Rebuild it, so the game's own new skins and changes are kept."
		case strings.Join(now, ",") != strings.Join(st.Mods, ",") || strings.Join(ov.TableMods, ",") != strings.Join(st.Adopted, ","):
			ov.Stale = "Skin mods were added or removed outside this program, so the skin list is out of date. Press Rebuild the skin list."
		case len(now) > 0 && st.Layout < slotLayout:
			ov.Stale = "This version unlocks new skins the way the game unlocks its own first skins, and gives each one a material slot of its own. Press Rebuild the skin list so your skins get both."
		}
	}
	ov.Loader = loaderVersion(s)
	ov.SST = skinSlotTextStatus(s)
	if len(mods) > 0 {
		ov.Setup = append(ov.Setup, slotSetupProblems(s)...)
	}
	return ov
}

// modKind tells the Mods screen what a mod is, as far as skin slots go.
func modKind(m InstalledMod) string {
	if m.Name == skinListMod {
		return "skinlist"
	}
	if !m.Loose {
		if _, err := os.Stat(filepath.Join(m.Dir, manifestName)); err == nil {
			return "slots"
		}
	}
	for _, r := range m.Replaces {
		if r == skinListLabel || r == unlockListLabel {
			return "tables"
		}
	}
	return ""
}

// onlyLists reports whether every file in a conflict is the skin list or unlock list.
func onlyLists(files []string) bool {
	if len(files) == 0 {
		return false
	}
	for _, f := range files {
		if f != skinListLabel && f != unlockListLabel {
			return false
		}
	}
	return true
}

func slotRoutes(api func(string, http.HandlerFunc)) {
	api("/api/slots", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, slotsOverviewNow(getSettings()))
	})
	api("/api/slot/build", func(w http.ResponseWriter, r *http.Request) {
		var q slotReq
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Build new skin", []string{s.AESKey}, func(j *Job) (any, error) { return doBuildSlot(j, s, q) })
	})
	api("/api/emblem/build", func(w http.ResponseWriter, r *http.Request) {
		var q emblemReq
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Build new emblem", []string{s.AESKey}, func(j *Job) (any, error) { return doBuildEmblem(j, s, q) })
	})
	api("/api/slot/edit", func(w http.ResponseWriter, r *http.Request) {
		var q slotEditReq
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Change skin", []string{s.AESKey}, func(j *Job) (any, error) { return editSlotTexts(j, s, q) })
	})
	api("/api/slot/remove", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Mod, Slot string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Remove skin", []string{s.AESKey}, func(j *Job) (any, error) { return removeSlot(j, s, q.Mod, q.Slot) })
	})
	api("/api/slotmod/remove", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Mod string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Remove skin mod", []string{s.AESKey}, func(j *Job) (any, error) { return removeSlotMod(j, s, q.Mod) })
	})
	api("/api/slotmod/reinstall", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Mod string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Put skin mod back", []string{s.AESKey}, func(j *Job) (any, error) { return reinstallProject(j, s, q.Mod) })
	})
	api("/api/slotmod/export", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Mod string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Share skin mod", []string{s.AESKey}, func(j *Job) (any, error) { return doExportMod(j, s, q.Mod) })
	})
	api("/api/slotmod/rename", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Mod, Title, Author string }
		if err := readJSON(r, &q); err != nil || strings.TrimSpace(q.Title) == "" {
			fail(w, 400, errors.New("give the mod a name"))
			return
		}
		if jobs.Running() {
			fail(w, http.StatusConflict, errBusy)
			return
		}
		s := getSettings()
		if err := renameSlotMod(s, q.Mod, strings.TrimSpace(q.Title), strings.TrimSpace(q.Author)); err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	api("/api/import/plan", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Path string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		p := cleanPath(q.Path)
		startJob(w, "Read skin mod", []string{s.AESKey}, func(j *Job) (any, error) { return planImport(j, s, p) })
	})
	api("/api/import", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Path    string
			Cleanup []string
		}
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		p := cleanPath(q.Path)
		startJob(w, "Import skin mod", []string{s.AESKey}, func(j *Job) (any, error) {
			res, err := doImport(j, s, p)
			if err == nil {
				cleanInbox(s, q.Cleanup)
			}
			return res, err
		})
	})
	api("/api/slot/iconpreview", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		img, err := iconPreview(getSettings(), q.Get("skin"), q.Get("template"), q.Get("mode"), q.Get("colour"), q.Get("picture"))
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		var buf bytes.Buffer
		encodePNG(&buf, img, SaveRGB)
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(buf.Bytes())
	})
	api("/api/emblem/preview", func(w http.ResponseWriter, r *http.Request) {
		// the picture as the emblem will have it: squared, with see-through edges
		src, err := LoadPicture(cleanPath(r.URL.Query().Get("picture")))
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		if src.W < 16 || src.H < 16 {
			http.Error(w, "the picture is too small for an emblem", 404)
			return
		}
		var buf bytes.Buffer
		encodePNG(&buf, resize(emblemSquare(src), 256, 256), SaveRGBA)
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(buf.Bytes())
	})
	api("/api/slot/fetchicon", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Skin     string
			Template string
		}
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Fetch the original icon", []string{s.AESKey}, func(j *Job) (any, error) {
			icon, err := templateIcon(j, s, q.Skin, q.Template)
			if err != nil {
				return nil, err
			}
			return map[string]string{"icon": icon}, ensureBase(j, s, []string{icon})
		})
	})
	api("/api/skinlist/rebuild", func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		startJob(w, "Update the skin list", []string{s.AESKey}, func(j *Job) (any, error) { return RebuildSkinList(j, s) })
	})
	api("/api/settings/import", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ Path string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s, err := settingsFromOld(getSettings(), cleanPath(q.Path))
		if err != nil {
			fail(w, 400, err)
			return
		}
		if err := saveSettings(s); err != nil {
			fail(w, 500, err)
			return
		}
		invalidateLibrary()
		writeJSON(w, 200, currentState())
	})
}

// renameSlotMod changes a mod's shown name and author (not its key or folder).
func renameSlotMod(s Settings, key, title, author string) error {
	done := false
	mods, _ := scanSlotMods(s.ModsDir)
	for _, im := range mods {
		if im.Manifest.Key == key {
			im.Manifest.Title, im.Manifest.Author = title, author
			if err := writeManifest(filepath.Join(im.Dir, manifestName), im.Manifest); err != nil {
				return err
			}
			done = true
		}
	}
	if pm, err := loadProject(s, key); err == nil {
		pm.Title, pm.Author = title, author
		writeManifest(filepath.Join(s.projectDir(key), manifestName), pm)
		done = true
	}
	if !done {
		return errors.New("that skin mod was not found")
	}
	writeAllTexts(s, mods)
	return nil
}

// settingsFromOld copies the game setup from AC8 Skin Studio 1.x's settings.json (a file or its folder).
func settingsFromOld(cur Settings, p string) (Settings, error) {
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		p = filepath.Join(p, "settings.json")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return cur, errors.New("no settings.json was found there; choose the folder of AC8 Skin Studio 1.x (where AC8SkinStudio.exe is)")
	}
	var old Settings
	if err := json.Unmarshal(b, &old); err != nil {
		return cur, errors.New("that settings.json could not be read")
	}
	if old.PaksDir != "" {
		cur.PaksDir = old.PaksDir
	}
	if old.AESKey != "" {
		cur.AESKey = old.AESKey
	}
	if old.ModsDir != "" {
		cur.ModsDir = old.ModsDir
	}
	if old.Usmap != "" {
		cur.Usmap = old.Usmap
	}
	if old.ImageFormat != "" {
		cur.ImageFormat = old.ImageFormat
	}
	// raw files extracted with 1.x can be used here too
	oldWork := old.Workspace
	if oldWork == "" {
		oldWork = filepath.Join(filepath.Dir(p), "workspace")
	}
	for _, d := range append([]string{filepath.Join(oldWork, "raw")}, old.ExtraRawDirs...) {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			dup := false
			for _, have := range cur.ExtraRawDirs {
				if strings.EqualFold(have, d) {
					dup = true
				}
			}
			if !dup {
				cur.ExtraRawDirs = append(cur.ExtraRawDirs, d)
			}
		}
	}
	if cur.RetocPath == "" && old.RetocPath != "" {
		cur.RetocPath = old.RetocPath
	}
	return cur, nil
}

// templateOf finds the game skin a new skin starts from and its icon's game path. With a job it may
// fetch the skin list from the game; without one (j == nil) it only uses what is already fetched.
func templateIcon(j *Job, s Settings, skinDir, number string) (string, error) {
	texs, err := skinTextures(skinDir)
	if err != nil {
		return "", err
	}
	var pkg string
	for _, t := range texs {
		if t.Package != "" {
			pkg = t.Package
			break
		}
	}
	_, aircraft, folder, err := vehicleOfTexture(pkg)
	if err != nil {
		return "", err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(number))
	if n <= 0 {
		n = folderNumber(folder)
	}
	var st *SkinTable
	if j != nil {
		if st, _, err = loadBaseTables(j, s); err != nil {
			return "", err
		}
	} else {
		if _, err := os.Stat(gameFile(s.baseDir(), s.Project, skinTablePath) + ".uasset"); err != nil {
			return "", errNoBase
		}
		if st, err = LoadSkinTable(gameFile(s.baseDir(), s.Project, skinTablePath), activeSchema(s)); err != nil {
			return "", err
		}
	}
	row := st.Find(skinIDOf(aircraft, n))
	if row == nil {
		return "", errors.New("no such skin")
	}
	return packagePathOf(row.Obj.Str("MenuIconImageRef")), nil
}

// iconPreview makes the picture the new skin's icon will have (as far as it can be known before building).
func iconPreview(s Settings, skinDir, template, mode, colour, picture string) (*Img, error) {
	if mode == "picture" {
		src, err := LoadPicture(cleanPath(picture))
		if err != nil {
			return nil, err
		}
		return IconFromPicture(src, 180), nil
	}
	icon, err := templateIcon(nil, s, cleanPath(skinDir), template)
	if err != nil {
		return nil, err
	}
	p, err := LoadPackage(gameFile(s.baseDir(), s.Project, icon))
	if err != nil {
		return nil, errNoBase
	}
	im, err := DecodeIcon(p)
	if err != nil {
		return nil, err
	}
	if c, ok := parseColour(colour); ok && mode == "recolour" {
		im = RecolourIcon(im, c)
	}
	return im, nil
}
