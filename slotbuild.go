package main

// Building new skin slots: the work behind "New skin slot" on the Repack screen and the Skin slots
// screen.
//
// The mods made on this computer are kept in the workspace (slot-mods/<key>/) with their raw files,
// so a skin can be added, changed or taken out later and the mod packed again. Each build installs
// the mod into the mods folder and then rebuilds the merged skin list.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type slotReq struct {
	planReq
	Mod         string `json:"mod"`      // key of a mod made here to add the skin to; "" = a new mod
	ModTitle    string `json:"modTitle"` // name of a new mod
	Author      string `json:"author"`
	Slot        string `json:"slot"` // key of a skin to rebuild; "" = a new skin
	Name        string `json:"name"`
	Description string `json:"description"`
	Unlock      string `json:"unlock"`
	Icon        string `json:"icon"`        // base, recolour or picture
	IconColour  string `json:"iconColour"`  // #rrggbb for recolour; "" = the paint's main colour
	IconPicture string `json:"iconPicture"` // a picture file for "picture"
	Template    int64  `json:"template"`    // the game skin to base it on; 0 = the one the textures belong to
	Number      int    `json:"number"`      // a skin number to ask for; 0 = any
}

func (s Settings) projectDir(key string) string { return filepath.Join(s.slotModsDir(), key) }

func loadProject(s Settings, key string) (*ModManifest, error) {
	if !reModKey.MatchString(key) {
		return nil, errors.New("unknown skin mod")
	}
	m, err := readManifest(filepath.Join(s.projectDir(key), manifestName))
	if err != nil {
		return nil, fmt.Errorf("the skin mod %s was not made on this computer, so skins cannot be added to it here", key)
	}
	return m, nil
}

// listProjects gives the slot mods made on this computer.
func listProjects(s Settings) []*ModManifest {
	es, _ := os.ReadDir(s.slotModsDir())
	var out []*ModManifest
	for _, e := range es {
		if !e.IsDir() {
			continue
		}
		if m, err := readManifest(filepath.Join(s.slotModsDir(), e.Name(), manifestName)); err == nil && m.Key == e.Name() {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title) })
	return out
}

var reVehicleNumber = regexp.MustCompile(`^[A-Za-z]+0*([0-9]+)_[A-Za-z0-9]+$`)

// vehicleOfTexture works out the aircraft from a texture's game path:
// /Game/Vehicles/Aircraft/PP/PP0021_su57/Textures/001/T_X -> PP0021_su57, 21, "001".
func vehicleOfTexture(pkg string) (vehicle string, aircraft int, folder string, err error) {
	i := strings.Index(pkg, "/Textures/")
	if i < 0 {
		return "", 0, "", fmt.Errorf("%s is not an aircraft skin texture", pkg)
	}
	vehicle = pkg[strings.LastIndexByte(pkg[:i], '/')+1 : i]
	rest := strings.Split(pkg[i+len("/Textures/"):], "/")
	if len(rest) >= 2 {
		folder = rest[0]
	}
	m := reVehicleNumber.FindStringSubmatch(vehicle)
	if m == nil {
		return vehicle, 0, folder, fmt.Errorf("%s is not an aircraft this program knows how to add skins to", vehicle)
	}
	aircraft, _ = strconv.Atoi(m[1])
	return vehicle, aircraft, folder, nil
}

func folderNumber(folder string) int {
	if len(folder) == 3 {
		if n, err := strconv.Atoi(folder); err == nil {
			return n
		}
	}
	return 0
}

func parseColour(s string) ([3]uint8, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return [3]uint8{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return [3]uint8{}, false
	}
	return [3]uint8{uint8(v >> 16), uint8(v >> 8), uint8(v)}, true
}

func hexColour(c [3]uint8) string { return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2]) }

// uniqueFolder picks a mods-folder name that is not in use.
func uniqueFolder(modsDir, want string) string {
	base := safeName(want)
	if base == "" {
		base = "SkinMod"
	}
	name := base
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(modsDir, name)); os.IsNotExist(err) && !strings.EqualFold(name, skinListMod) {
			return name
		}
		name = fmt.Sprintf("%s_%d", base, i)
	}
}

// slotInputDir is where a slot's raw files are kept: <project>/input/Live/Content/SkinStudio/<key>/<slot>
func slotInputDir(s Settings, m *ModManifest, slot string) string {
	return gameFile(filepath.Join(s.projectDir(m.Key), "input"), s.Project, strings.TrimSuffix(slotAssetDir(m.Key, slot), "/"))
}

func writePackageFiles(base string, ua, ue []byte, ubulk []byte) error {
	if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(base+".uasset", ua, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(base+".uexp", ue, 0o644); err != nil {
		return err
	}
	if ubulk != nil {
		return os.WriteFile(base+".ubulk", ubulk, 0o644)
	}
	return nil
}

// slotResult is what the window shows after a build.
type slotResult struct {
	Mod      string      `json:"mod"`
	ModTitle string      `json:"modTitle"`
	Folder   string      `json:"folder"`
	Slot     string      `json:"slot"`
	Name     string      `json:"name"`
	Number   int         `json:"number"`
	Plane    string      `json:"plane"`
	Icon     string      `json:"icon"`
	List     *ListResult `json:"list"`
	Notes    []string    `json:"notes"`
	Setup    []string    `json:"setup"` // things still to set up for the skin to show in game
}

func doBuildSlot(j *Job, s Settings, r slotReq) (any, error) {
	if err := needModsFolder(s); err != nil {
		return nil, err
	}
	if _, err := needExtractSetup(s); err != nil {
		return nil, err
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return nil, errors.New("give the skin a name; it is what the game shows in the skin list")
	}
	if utf16Len(r.Name) > 80 {
		return nil, errors.New("the skin name is too long (80 characters at most)")
	}
	if utf16Len(r.Description) > 1000 {
		return nil, errors.New("the description is too long (1000 characters at most)")
	}
	r.Unlock = UnlockMission // the one rule the game acts on (see NewUnlockEntry)
	// the skin it is based on
	texs, err := skinTextures(r.SkinDir)
	if err != nil {
		return nil, err
	}
	var pkgPath string
	for _, t := range texs {
		if t.Package != "" {
			pkgPath = t.Package
			break
		}
	}
	vehicle, aircraft, folder, err := vehicleOfTexture(pkgPath)
	if err != nil {
		return nil, err
	}
	j.SetStep("Reading the game's skin list", -1)
	st, _, err := loadBaseTables(j, s)
	if err != nil {
		return nil, err
	}
	template := r.Template
	if template == 0 {
		if n := folderNumber(folder); n > 0 {
			template = skinIDOf(aircraft, n)
		}
	}
	trow := st.Find(template)
	if trow == nil || aircraftOf(template) != aircraft {
		var have []string
		for n := range st.UsedSlots(aircraft) {
			have = append(have, fmt.Sprintf("%03d", n))
		}
		sort.Strings(have)
		if len(have) == 0 {
			return nil, fmt.Errorf("%s is not in the game's skin list, so it cannot have skin slots of its own; use Replace a game skin to repaint it", vehicle)
		}
		return nil, fmt.Errorf("the game has no skin %03d for %s to base the new skin on; choose one of %s", slotOf(template), vehicle, strings.Join(have, ", "))
	}
	plane := trow.Plane()
	tmplMat := packagePathOf(trow.Material())
	tmplIcon := packagePathOf(trow.Obj.Str("MenuIconImageRef"))
	// which textures get their own copy
	want := map[string]bool{}
	for _, n := range r.Textures {
		want[n] = true
	}
	var todo []planRow
	if len(want) > 0 {
		rows, _, err := doPlan(r.planReq)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if want[row.Name] {
				if !row.Recipe.Ready {
					return nil, fmt.Errorf("%s cannot be rebuilt: %s", row.Name, row.Recipe.Missing)
				}
				todo = append(todo, row)
			}
		}
	}
	need := []string{materialFolder(tmplMat)}
	if tmplIcon != "" {
		need = append(need, tmplIcon)
	}
	if err := ensureBase(j, s, need); err != nil {
		return nil, err
	}
	// the mod
	var m *ModManifest
	if r.Mod != "" {
		if m, err = loadProject(s, r.Mod); err != nil {
			return nil, err
		}
	} else {
		title := strings.TrimSpace(r.ModTitle)
		if title == "" {
			title = r.Name
		}
		key := newModKey(title)
		m = &ModManifest{Key: key, Title: title, Author: strings.TrimSpace(r.Author), Folder: uniqueFolder(s.ModsDir, title), Container: key + "_P",
			Created: time.Now().Format("2006-01-02"), Slots: []SlotDef{}}
	}
	slotKey := r.Slot
	var old *SlotDef
	if slotKey != "" {
		if old = m.slot(slotKey); old == nil {
			return nil, errors.New("that skin is no longer in the mod")
		}
	} else {
		slotKey = m.nextSlotKey()
	}
	dir := slotInputDir(s, m, slotKey)
	own := slotAssetDir(m.Key, slotKey)
	sl := SlotDef{Key: slotKey, Aircraft: aircraft, Plane: plane, Vehicle: vehicle, Template: template, Number: r.Number,
		Name: r.Name, Description: strings.TrimSpace(r.Description), Unlock: r.Unlock, Sources: map[string]string{}}
	if old != nil && r.Number == 0 {
		sl.Number = old.Number
	}
	// Changing a skin keeps the textures it already has, unless new pictures replace them or it is
	// now based on another game skin. Its material and icon are always made again.
	moves := map[string]string{}
	if old != nil && old.Template == template {
		for name, src := range old.Sources {
			if _, err := os.Stat(filepath.Join(dir, name+".uasset")); err == nil {
				moves[src] = own + name
				sl.Sources[name] = src
				sl.Textures = append(sl.Textures, name)
			}
		}
		sl.Tex = old.Tex // the colour texture as it was stored, unless it is rebuilt below
		for _, n := range []string{packageBaseName(old.Material), packageBaseName(old.Icon), strings.TrimSuffix(packageBaseName(old.Material), "_Decal") + "_DAM"} {
			if n != "" {
				for _, ext := range []string{".uasset", ".uexp"} {
					os.Remove(filepath.Join(dir, n+ext))
				}
			}
		}
	} else {
		os.RemoveAll(dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	// its material and icon are kept apart from the textures: the skin list gives them the skin's
	// number (see slotfiles.go)
	sdir := s.slotFilesDir(m.Key, slotKey)
	os.RemoveAll(sdir)
	if err := os.MkdirAll(sdir, 0o755); err != nil {
		return nil, err
	}
	var notes []string
	var paintColour *[3]uint8
	// 1. textures
	if len(todo) > 0 {
		files, _ := paintedFiles(r.planReq)
		var names []string
		for _, t := range texs {
			names = append(names, t.Name)
		}
		sets := FindPainted(files, names, r.Choices)
		for i, row := range todo {
			if j.Cancelled() {
				return nil, errors.New("cancelled")
			}
			base := 0.1 + float64(i)/float64(len(todo))*0.6
			span := 0.6 / float64(len(todo))
			j.SetStep(fmt.Sprintf("Rebuilding %s (%d of %d)", row.Name, i+1, len(todo)), base)
			f := texFilesFor(row.UExp)
			vt, uexp, ubulk, err := LoadVTex(f)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", row.Name, err)
			}
			img, ns, err := Compose(row.Kind, row.Name, vt, ubulk, sets)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", row.Name, err)
			}
			for _, n := range ns {
				j.Log("Note: %s", n)
				notes = append(notes, n)
			}
			if row.Kind == KindColour && paintColour == nil {
				c := MainColour(img)
				paintColour = &c
			}
			ua, err := os.ReadFile(f.UAsset)
			if err != nil {
				return nil, err
			}
			opts := r.Tex.forKind(row.Kind)
			newUA, newExp, newBulk, err := rebuildTexture(vt, ua, uexp, ubulk, img, opts, func(d, t int) {
				j.SetStep(fmt.Sprintf("Rebuilding %s (%d of %d)", row.Name, i+1, len(todo)), base+span*float64(d)/float64(t))
			})
			if err != nil {
				return nil, fmt.Errorf("%s: %v", row.Name, err)
			}
			if row.Kind == KindColour {
				sl.Tex = opts
			}
			p, err := ParsePackage(newUA, newExp)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", row.Name, err)
			}
			objName := row.Package[strings.LastIndexByte(row.Package, '/')+1:]
			newPkg := own + objName
			nua, nue, err := p.Retarget(newPkg, nil)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", row.Name, err)
			}
			if err := writePackageFiles(filepath.Join(dir, objName), nua, nue, newBulk); err != nil {
				return nil, err
			}
			moves[row.Package] = newPkg
			if _, had := sl.Sources[objName]; !had {
				sl.Textures = append(sl.Textures, row.Name)
			}
			sl.Sources[objName] = row.Package
			j.Log("%s rebuilt as %s (%s)%s", row.Name, newPkg, strings.Join(row.Recipe.Uses, "; "), texNote(opts))
		}
	}
	// 2. the material, pointing at the new textures (and its damage material next to it). Every new
	// skin has a material of its own, even one that only copies the original's paint, so that it
	// has a material slot of its own like the game's skins.
	{
		j.SetStep("Making the skin's material", 0.75)
		mp, err := LoadPackage(gameFile(s.baseDir(), s.Project, tmplMat))
		if err != nil {
			return nil, fmt.Errorf("the material of skin %03d could not be read: %v", slotOf(template), err)
		}
		uses := map[string]bool{}
		for i := range mp.Imports {
			uses[packagePathOf(mp.ImportPath(i))] = true
		}
		for oldTex := range moves {
			if !uses[oldTex] {
				return nil, fmt.Errorf("%s is not used by the material of skin %03d, so it would not show on the new skin; untick it, or base the skin on the skin that texture belongs to", oldTex[strings.LastIndexByte(oldTex, '/')+1:], slotOf(template))
			}
		}
		matName := tmplMat[strings.LastIndexByte(tmplMat, '/')+1:]
		ua, ue, err := mp.Retarget(own+matName, moves)
		if err != nil {
			return nil, err
		}
		if err := writePackageFiles(filepath.Join(sdir, matName), ua, ue, nil); err != nil {
			return nil, err
		}
		sl.Material = own + matName
		if strings.HasSuffix(matName, "_Decal") {
			damName := strings.TrimSuffix(matName, "_Decal") + "_DAM"
			damPath := materialFolder(tmplMat) + damName
			if dp, err := LoadPackage(gameFile(s.baseDir(), s.Project, damPath)); err == nil {
				if ua, ue, err := dp.Retarget(own+damName, nil); err == nil {
					writePackageFiles(filepath.Join(sdir, damName), ua, ue, nil)
				}
			}
		}
	}
	// 3. the icon
	j.SetStep("Making the icon", 0.8)
	projDir := s.projectDir(m.Key)
	os.MkdirAll(filepath.Join(projDir, "icons"), 0o755)
	iconPNG := filepath.Join(projDir, "icons", slotKey+".png")
	var tmplIconImg *Img
	var ip *Package
	if tmplIcon != "" {
		if p, err := LoadPackage(gameFile(s.baseDir(), s.Project, tmplIcon)); err == nil {
			ip = p
			tmplIconImg, _ = DecodeIcon(p)
		}
	}
	var pic *Img
	switch r.Icon {
	case "picture":
		if strings.TrimSpace(r.IconPicture) == "" {
			return nil, errors.New("choose the picture for the icon")
		}
		src, err := LoadPicture(cleanPath(r.IconPicture))
		if err != nil {
			return nil, err
		}
		pic = IconFromPicture(src, 180)
	case "recolour":
		if tmplIconImg == nil {
			return nil, errors.New("the icon of the skin it is based on could not be read, so it cannot be recoloured; choose a picture or keep the original icon")
		}
		c, ok := parseColour(r.IconColour)
		if !ok {
			if paintColour == nil {
				return nil, errors.New("choose a colour for the icon (or rebuild the colour texture, whose main colour is then used)")
			}
			c = *paintColour
		}
		pic = RecolourIcon(tmplIconImg, c)
		notes = append(notes, "The icon was recoloured to "+hexColour(c)+".")
	}
	if pic != nil {
		if ip == nil {
			return nil, errors.New("the icon of the skin it is based on could not be read, so a new icon cannot be made from it")
		}
		ue, err := EncodeIcon(ip, pic)
		if err != nil {
			return nil, err
		}
		q, err := ParsePackage(ip.UAsset, ue)
		if err != nil {
			return nil, err
		}
		iconName := tmplIcon[strings.LastIndexByte(tmplIcon, '/')+1:]
		ua, ue2, err := q.Retarget(own+iconName, nil)
		if err != nil {
			return nil, err
		}
		if err := writePackageFiles(filepath.Join(sdir, iconName), ua, ue2, nil); err != nil {
			return nil, err
		}
		sl.Icon = own + iconName
		back, _ := DecodeIcon(q)
		if back != nil {
			SaveImage(iconPNG, back, SaveRGB)
		}
	} else if tmplIconImg != nil {
		SaveImage(iconPNG, tmplIconImg, SaveRGB)
	}
	if _, err := os.Stat(iconPNG); err == nil {
		sl.IconFile = "icons/" + slotKey + ".png"
	}
	// 4. the mod: save, pack and install
	if old != nil {
		*old = sl
	} else {
		m.Slots = append(m.Slots, sl)
	}
	if err := saveAndInstallProject(j, s, m); err != nil {
		return nil, err
	}
	cleanInbox(s, r.Cleanup)
	// 5. the merged skin list
	j.SetStep("Updating the skin list", 0.9)
	list, err := RebuildSkinList(j, s)
	if err != nil {
		return nil, fmt.Errorf("the skin was built and installed, but the skin list could not be updated: %v", err)
	}
	res := slotResult{Mod: m.Key, ModTitle: m.Title, Folder: m.Folder, Slot: slotKey, Name: sl.Name, Plane: plane, List: list, Notes: notes}
	if sl.IconFile != "" {
		res.Icon = filepath.Join(s.ModsDir, m.Folder, filepath.FromSlash(sl.IconFile))
	}
	for _, p := range list.Slots {
		if p.Mod == m.Key && p.Slot == slotKey {
			res.Number = p.Number
		}
	}
	res.Setup = slotSetupProblems(s)
	j.Log("New skin %q is skin %03d of %s, in the mod %s.", sl.Name, res.Number, vehicle, m.Folder)
	return res, nil
}

// saveAndInstallProject saves a mod made here, packs its files and installs it into the mods folder.
func saveAndInstallProject(j *Job, s Settings, m *ModManifest) error {
	projDir := s.projectDir(m.Key)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		return err
	}
	if err := writeManifest(filepath.Join(projDir, manifestName), m); err != nil {
		return err
	}
	input := filepath.Join(projDir, "input")
	hasFiles := false
	filepath.WalkDir(input, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".uasset") {
			hasFiles = true
		}
		return nil
	})
	var files []string
	if hasFiles {
		j.SetStep("Packing the mod", 0.85)
		var err error
		if files, err = packFolder(j, s, input, filepath.Join(projDir, "packed"), m.Container); err != nil {
			return err
		}
	}
	return installProject(s, m, files)
}

// installProject puts a mod's container files, slot files, skinstudio.json and icons into its
// mods-folder folder.
func installProject(s Settings, m *ModManifest, containerFiles []string) error {
	// the mod may have been put in a folder of another name by hand: keep using that one
	if mods, _ := scanSlotMods(s.ModsDir); mods != nil {
		for _, im := range mods {
			if im.Manifest.Key == m.Key && !strings.EqualFold(filepath.Base(im.Dir), m.Folder) {
				m.Folder = filepath.Base(im.Dir)
				writeManifest(filepath.Join(s.projectDir(m.Key), manifestName), m)
			}
		}
	}
	dest := filepath.Join(s.ModsDir, m.Folder)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	// old container files go first, so a mod that no longer has files of its own does not keep them
	if es, err := os.ReadDir(dest); err == nil {
		for _, e := range es {
			if modFileExts[strings.ToLower(filepath.Ext(e.Name()))] {
				if err := os.Remove(filepath.Join(dest, e.Name())); err != nil {
					return fmt.Errorf("could not replace %s (%v). If the game is running, close it and try again", e.Name(), err)
				}
			}
		}
	}
	if err := installFiles(dest, containerFiles); err != nil {
		return err
	}
	if err := installSlotFiles(filepath.Join(s.projectDir(m.Key), slotFilesFolder), dest); err != nil {
		return fmt.Errorf("could not write the skins' materials and icons into the mods folder: %v", err)
	}
	projIcons := filepath.Join(s.projectDir(m.Key), "icons")
	os.RemoveAll(filepath.Join(dest, "icons"))
	for _, sl := range m.Slots {
		if sl.IconFile != "" {
			copyFile(filepath.Join(projIcons, sl.Key+".png"), filepath.Join(dest, "icons", sl.Key+".png"))
		}
	}
	for _, e := range m.Emblems {
		if e.IconFile != "" {
			copyFile(filepath.Join(projIcons, e.Key+".png"), filepath.Join(dest, "icons", e.Key+".png"))
		}
	}
	return writeManifest(filepath.Join(dest, manifestName), m)
}

// ------------------------------------------------------------------ setup checks

// loaderVersion reads the mod loader's version from its DLL next to the mods folder ("" if unknown).
func loaderVersion(s Settings) string {
	for _, p := range []string{filepath.Join(s.ModsDir, "dlls", "main.dll"), filepath.Join(filepath.Dir(s.ModsDir), "IoStoreLoaderMod", "dlls", "main.dll")} {
		b, err := os.ReadFile(p)
		if err != nil || len(b) > 32<<20 {
			continue
		}
		best := ""
		for _, v := range utf16Strings(b, 5) {
			if reVersion.MatchString(v) && versionLess(best, v) {
				best = v
			}
		}
		if best != "" {
			return best
		}
	}
	return ""
}

var reVersion = regexp.MustCompile(`^[0-9]{1,2}\.[0-9]{1,2}\.[0-9]{1,3}$`)

func versionLess(a, b string) bool {
	if a == "" {
		return b != ""
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			return x < y
		}
	}
	return false
}

// utf16Strings finds runs of printable UTF-16 text in a file.
func utf16Strings(b []byte, minLen int) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) >= minLen {
			out = append(out, string(cur))
		}
		cur = cur[:0]
	}
	for i := 0; i+1 < len(b); i += 2 {
		c := rune(b[i]) | rune(b[i+1])<<8
		if c >= 0x20 && c < 0x7f {
			cur = append(cur, c)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// slotSetupProblems lists what still stands between a new skin and the game.
func slotSetupProblems(s Settings) []string {
	out := []string{}
	switch v := loaderVersion(s); {
	case v == "":
		out = append(out, "The mod loader's version could not be found next to your mods folder. New skins need AC8 Mod Loader 1.1.0 or newer; older loaders do not find files that are new to the game.")
	case versionLess(v, "1.1.0"):
		out = append(out, "Your mod loader is version "+v+". New skins need AC8 Mod Loader 1.1.0 or newer; older loaders do not find files that are new to the game, so the skin would show without its paint and icon.")
	}
	if st := skinSlotTextStatus(s); st.Problem != "" {
		out = append(out, st.Problem)
	}
	return out
}

// ------------------------------------------------------------------ changing and removing

type slotEditReq struct {
	Mod         string `json:"mod"`
	Slot        string `json:"slot"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Unlock      string `json:"unlock"`
}

// editSlotTexts changes a skin's or an emblem's name and description. They are only in the text
// file, so nothing has to be packed again.
func editSlotTexts(j *Job, s Settings, r slotEditReq) (any, error) {
	mods, _ := scanSlotMods(s.ModsDir)
	var im *InstalledSlotMod
	for i := range mods {
		if mods[i].Manifest.Key == r.Mod {
			im = &mods[i]
		}
	}
	if im == nil {
		return nil, errors.New("that skin mod is no longer in the mods folder")
	}
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return nil, errors.New("the name cannot be empty")
	}
	desc := strings.TrimSpace(r.Description)
	// set changes the texts in one copy of the mod's skinstudio.json
	set := func(m *ModManifest) bool {
		if sl := m.slot(r.Slot); sl != nil {
			sl.Name, sl.Description = name, desc
			return true
		}
		if e := m.emblem(r.Slot); e != nil {
			e.Name, e.Description = name, desc
			return true
		}
		return false
	}
	if !set(im.Manifest) {
		return nil, errors.New("that skin is no longer in the mod")
	}
	if err := writeManifest(filepath.Join(im.Dir, manifestName), im.Manifest); err != nil {
		return nil, err
	}
	// a mod made here keeps its own copy in the workspace too
	if pm, err := loadProject(s, r.Mod); err == nil && set(pm) {
		writeManifest(filepath.Join(s.projectDir(r.Mod), manifestName), pm)
	}
	dir, err := writeAllTexts(s, mods)
	if err != nil {
		return nil, err
	}
	return map[string]any{"textsDir": dir}, nil
}

// removeSlot takes one skin or emblem out of a mod made here (and packs the mod again).
func removeSlot(j *Job, s Settings, mod, slot string) (any, error) {
	m, err := loadProject(s, mod)
	if err != nil {
		return nil, err
	}
	keep := []SlotDef{}
	for _, sl := range m.Slots {
		if sl.Key != slot {
			keep = append(keep, sl)
		}
	}
	var keepE []EmblemDef
	for _, e := range m.Emblems {
		if e.Key != slot {
			keepE = append(keepE, e)
		}
	}
	if len(keep) == len(m.Slots) && len(keepE) == len(m.Emblems) {
		return nil, errors.New("that skin is no longer in the mod")
	}
	if len(keep)+len(keepE) == 0 {
		return nil, errors.New("this is all the mod has; remove the whole mod instead")
	}
	if len(keepE) < len(m.Emblems) {
		removeEmblemFiles(s, m, slot)
	} else {
		os.RemoveAll(slotInputDir(s, m, slot))
		os.RemoveAll(s.slotFilesDir(m.Key, slot))
		os.Remove(filepath.Join(s.projectDir(m.Key), "icons", slot+".png"))
	}
	m.Slots, m.Emblems = keep, keepE
	if err := saveAndInstallProject(j, s, m); err != nil {
		return nil, err
	}
	return RebuildSkinList(j, s)
}

// removeSlotMod takes a slot mod out of the mods folder (into removed-mods) and rebuilds the list.
func removeSlotMod(j *Job, s Settings, mod string) (any, error) {
	mods, _ := scanSlotMods(s.ModsDir)
	for _, m := range mods {
		if m.Manifest.Key != mod {
			continue
		}
		dest := filepath.Join(s.removedModsDir(), safeFolder(filepath.Base(m.Dir))+"_"+time.Now().Format("20060102-150405"))
		os.MkdirAll(filepath.Dir(dest), 0o755)
		if err := moveDir(m.Dir, dest); err != nil {
			return nil, fmt.Errorf("the mod could not be moved (%v). If the game is running, close it and try again", err)
		}
		j.Log("%s moved to %s", filepath.Base(m.Dir), dest)
		res, err := RebuildSkinList(j, s)
		if err != nil {
			return nil, fmt.Errorf("the mod was taken out, but the skin list could not be updated: %v", err)
		}
		return map[string]any{"movedTo": dest, "list": res}, nil
	}
	return nil, errors.New("that skin mod is no longer in the mods folder")
}

// reinstallProject puts a mod made here back into the mods folder (after it was removed).
func reinstallProject(j *Job, s Settings, mod string) (any, error) {
	if err := needModsFolder(s); err != nil {
		return nil, err
	}
	m, err := loadProject(s, mod)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(s.ModsDir, m.Folder)); err == nil {
		if cur, err := readManifest(filepath.Join(s.ModsDir, m.Folder, manifestName)); err != nil || cur.Key != m.Key {
			m.Folder = uniqueFolder(s.ModsDir, m.Folder)
		}
	}
	if err := saveAndInstallProject(j, s, m); err != nil {
		return nil, err
	}
	return RebuildSkinList(j, s)
}

// packageBaseName gives the last part of a game path: /Game/A/B -> B ("" for "").
func packageBaseName(p string) string {
	if p == "" {
		return ""
	}
	return p[strings.LastIndexByte(p, '/')+1:]
}
