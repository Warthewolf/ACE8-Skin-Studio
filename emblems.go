package main

// New emblems.
//
// The game's emblems are rows of the emblem list (DT_Emblem, LiveEmblemDataTable): an id, name and
// description text keys, and five pictures: the decal painted on the aircraft (512 x 512) and icons
// of 64, 128, 180 and 1024 pixels. The campaign emblems are unlocked by the emblem unlock list
// (DA_Unlock_Emblem) when a campaign mission is finished, the same rule as the first skins.
//
// A new emblem belongs to a skin mod made here, next to (or instead of) new skins. Its pictures are
// in the mod's own container under /Game/Emblem/tga/SS/ and /Game/Emblem/png/SS/, the folders and
// names the game's own emblems use, with the mod's key in the names so two mods never collide. The
// shared skin list mod also carries the emblem list and its unlock list, with a row and an unlock
// rule for every new emblem. The game's emblems use numbers 010 to 450; new ones get numbers from
// 601, and emblem 601 has id 1601000003, written like the game's ids (1, number, 000, 003).

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

const (
	emblemTablePath   = "/Game/Datatables/Information/DT_Emblem"
	emblemUnlockPath  = "/Game/Unlock/DA_Unlock_Emblem"
	emblemRowStruct   = "LiveEmblemDataTable"
	emblemTemplateID  = 1010001003 // the Joker squadron's emblem: a campaign emblem in its own colours
	firstEmblemNumber = 601
	lastEmblemNumber  = 999
)

func emblemIDOf(n int) int64       { return 1_000_000_003 + int64(n)*1_000_000 }
func emblemRowKey(id int64) string { return "SS_Emblem_" + strconv.FormatInt(id, 10) }

// EmblemDef is one new emblem.
type EmblemDef struct {
	Key         string `json:"key"`    // e1, e2, ... unique inside its mod
	Number      int    `json:"number"` // the emblem number it had when it was made or shared; 0 = any
	Name        string `json:"name"`
	Description string `json:"description"`
	IconFile    string `json:"iconFile,omitempty"` // a picture of it inside the mod folder, for showing
}

var reEmblemKey = regexp.MustCompile(`^e[0-9]{1,3}$`)

func (m *ModManifest) emblem(key string) *EmblemDef {
	for i := range m.Emblems {
		if m.Emblems[i].Key == key {
			return &m.Emblems[i]
		}
	}
	return nil
}

func (m *ModManifest) nextEmblemKey() string {
	for i := 1; ; i++ {
		k := "e" + strconv.Itoa(i)
		if m.emblem(k) == nil {
			return k
		}
	}
}

// emblemPictures are an emblem's five pictures: the name ending, and the emblem list field that
// points at it ("" for the 128-pixel icon, which the list does not name).
var emblemPictures = []struct{ suffix, field string }{
	{"", "EmbFilePathIngame"}, {"_s", "IconSFileRef"}, {"_m", ""}, {"_mm", "MenuIconImageRef"}, {"_lll", "IGCIconImageRef"},
}

func emblemStem(modKey, key string) string { return "T_emblem_00_SS_" + modKey + "_" + key }

// emblemPicturePath is where one of a new emblem's pictures is in the game's content.
func emblemPicturePath(modKey, key, suffix string) string {
	if suffix == "" {
		return "/Game/Emblem/tga/SS/" + emblemStem(modKey, key)
	}
	return "/Game/Emblem/png/SS/" + emblemStem(modKey, key) + suffix
}

// ------------------------------------------------------------------ the game's emblem lists

func loadEmblemTable(base string, sc *Schema) (*SkinTable, error) {
	p, err := LoadPackage(base)
	if err != nil {
		return nil, err
	}
	if p.Name != emblemTablePath {
		return nil, fmt.Errorf("%s is not the emblem list", p.Name)
	}
	return readTable(p, sc, emblemRowStruct, "emblem list")
}

func loadEmblemUnlockTable(base string, sc *Schema) (*UnlockTable, error) {
	p, err := LoadPackage(base)
	if err != nil {
		return nil, err
	}
	if p.Name != emblemUnlockPath {
		return nil, fmt.Errorf("%s is not the emblem unlock list", p.Name)
	}
	u, err := readUnlockTable(p, sc)
	if err != nil {
		return nil, err
	}
	// new emblem rules follow on from the last one in use, after room for a game update's
	u.action, u.cutoff = "UnlockEmblem", u.MaxID()+1
	return u, nil
}

// loadEmblemTables reads the game's emblem list and emblem unlock list, fetching them if needed.
func loadEmblemTables(j *Job, s Settings) (*SkinTable, *UnlockTable, error) {
	if err := ensureBase(j, s, []string{emblemTablePath, emblemUnlockPath}); err != nil {
		return nil, nil, err
	}
	var lastErr error
	for _, sc := range []*Schema{activeSchema(s), builtinSchema} {
		et, err := loadEmblemTable(gameFile(s.baseDir(), s.Project, emblemTablePath), sc)
		if err != nil {
			lastErr = err
			continue
		}
		eu, err := loadEmblemUnlockTable(gameFile(s.baseDir(), s.Project, emblemUnlockPath), sc)
		if err != nil {
			lastErr = err
			continue
		}
		return et, eu, nil
	}
	return nil, nil, lastErr
}

func findEmblem(et *SkinTable, id int64) *SkinRow {
	for i := range et.Rows {
		if et.Rows[i].Obj.Int("EmblemID") == id {
			return &et.Rows[i]
		}
	}
	return nil
}

// emblemTemplate is the game emblem new ones are copied from: a campaign emblem shown in its own
// colours (the Joker squadron's, or the first like it after a game update).
func emblemTemplate(et *SkinTable) (*SkinRow, error) {
	if r := findEmblem(et, emblemTemplateID); r != nil {
		return r, nil
	}
	for i := range et.Rows {
		o := et.Rows[i].Obj
		if strings.Contains(o.Str("EmblemStringID"), "_AC8_") && o.Str("ColoringType") == "OriginalColor" && o.Str("IGCIconImageRef") != "" {
			return &et.Rows[i], nil
		}
	}
	return nil, errors.New("the emblem list has no campaign emblem to base new ones on")
}

// emblemTemplatePictures gives the game paths of the template's five pictures, in emblemPictures order.
func emblemTemplatePictures(t *SkinRow) ([]string, error) {
	var out []string
	for _, ep := range emblemPictures {
		var p string
		if ep.field != "" {
			p = packagePathOf(t.Obj.Str(ep.field))
		} else if s := packagePathOf(t.Obj.Str("IconSFileRef")); strings.HasSuffix(s, "_s") {
			p = strings.TrimSuffix(s, "_s") + ep.suffix
		}
		if p == "" || p == "None" {
			return nil, fmt.Errorf("the emblem it is based on has no %s picture", strings.TrimPrefix(ep.suffix, "_"))
		}
		out = append(out, p)
	}
	return out, nil
}

// usedEmblemNumbers are the emblem numbers the game's list has (from its ids: 1, number, ...).
func usedEmblemNumbers(et *SkinTable) map[int]bool {
	out := map[int]bool{}
	for _, r := range et.Rows {
		if id := r.Obj.Int("EmblemID"); id >= 1_000_000_000 {
			out[int(id/1_000_000%1000)] = true
		}
	}
	return out
}

// ------------------------------------------------------------------ numbers

// EmblemAssign records which number and unlock id an emblem has.
type EmblemAssign struct {
	Mod      string `json:"mod"`
	Emblem   string `json:"emblem"`
	Number   int    `json:"number"`
	UnlockID int64  `json:"unlockId"`
}

// PlacedEmblem is where one emblem ended up.
type PlacedEmblem struct {
	Mod      string `json:"mod"`
	ModTitle string `json:"modTitle"`
	Folder   string `json:"folder"`
	Emblem   string `json:"emblem"`
	Name     string `json:"name"`
	Number   int    `json:"number"`
	Wanted   int    `json:"wanted"`
	Moved    bool   `json:"moved"`
	UnlockID int64  `json:"unlockId"`
	EmblemID int64  `json:"emblemId"`
	Problem  string `json:"problem,omitempty"`
}

type emblemRef struct {
	mod    *InstalledSlotMod
	emblem *EmblemDef
}

// assignEmblemNumbers gives every new emblem a number and an unlock id, the way assignNumbers does
// for skins: emblems keep the number they have, a new one gets the number it asks for when it is
// free and otherwise the next free one, and an unlock id is never given to another emblem.
func assignEmblemNumbers(et *SkinTable, ut *UnlockTable, mods []InstalledSlotMod, old SkinListState) ([]PlacedEmblem, []EmblemAssign, map[string]emblemRef) {
	taken := usedEmblemNumbers(et)
	usedIDs := ut.UsedIDs()
	prev := map[string]EmblemAssign{}
	retired := map[int64]bool{}
	for _, id := range old.EmblemRetired {
		retired[id] = true
	}
	for _, a := range old.Emblems {
		prev[a.Mod+"/"+a.Emblem] = a
		retired[a.UnlockID] = true
	}
	last := ut.lastLowID() + unlockRoom
	newID := func() int64 {
		for x := last + 1; x <= ut.MaxID(); x++ {
			if !usedIDs[x] && !retired[x] {
				return x
			}
		}
		return 0
	}
	refs := map[string]emblemRef{}
	var order []string
	for i := range mods {
		m := &mods[i]
		for k := range m.Manifest.Emblems {
			e := &m.Manifest.Emblems[k]
			id := m.Manifest.Key + "/" + e.Key
			refs[id] = emblemRef{m, e}
			order = append(order, id)
		}
	}
	// emblems that had a number first, in number order, then new ones
	sort.SliceStable(order, func(i, j int) bool {
		pi, hi := prev[order[i]]
		pj, hj := prev[order[j]]
		if hi != hj {
			return hi
		}
		return hi && pi.Number < pj.Number
	})
	var placed []PlacedEmblem
	var assign []EmblemAssign
	free := func(n int) bool { return n >= firstEmblemNumber && n <= lastEmblemNumber && !taken[n] }
	for _, id := range order {
		r := refs[id]
		e := r.emblem
		pe := PlacedEmblem{Mod: r.mod.Manifest.Key, ModTitle: r.mod.Manifest.Title, Folder: filepath.Base(r.mod.Dir), Emblem: e.Key, Name: e.Name, Wanted: e.Number}
		num := 0
		p, had := prev[id]
		switch {
		case had && free(p.Number):
			num = p.Number
		case free(e.Number):
			num = e.Number
		default:
			for n := firstEmblemNumber; n <= lastEmblemNumber; n++ {
				if free(n) {
					num = n
					break
				}
			}
		}
		if num == 0 {
			pe.Problem = "there are no free emblem numbers left"
			placed = append(placed, pe)
			continue
		}
		taken[num] = true
		uid := p.UnlockID
		if !had || uid <= ut.MinID() || uid > ut.MaxID() || usedIDs[uid] {
			uid = newID()
		}
		if uid == 0 {
			pe.Problem = "the emblem unlock list is full"
			placed = append(placed, pe)
			continue
		}
		usedIDs[uid] = true
		pe.Number, pe.UnlockID, pe.EmblemID = num, uid, emblemIDOf(num)
		pe.Moved = e.Number != 0 && e.Number != num
		placed = append(placed, pe)
		assign = append(assign, EmblemAssign{Mod: r.mod.Manifest.Key, Emblem: e.Key, Number: num, UnlockID: uid})
	}
	return placed, assign, refs
}

// addEmblemRows puts the placed emblems into the emblem list and its unlock list.
func addEmblemRows(sc *Schema, et *SkinTable, ut *UnlockTable, placed []PlacedEmblem, refs map[string]emblemRef) error {
	tmpl, err := emblemTemplate(et)
	if err != nil {
		return err
	}
	list := append([]PlacedEmblem(nil), placed...)
	sort.Slice(list, func(i, j int) bool { return list[i].Number < list[j].Number })
	var unlocks []*Obj
	for _, p := range list {
		if p.Problem != "" {
			continue
		}
		r := refs[p.Mod+"/"+p.Emblem]
		m, e := r.mod.Manifest, r.emblem
		o := tmpl.Obj.Clone()
		set := func(k string, v any) error {
			if err := o.Set(sc, k, v); err != nil {
				return fmt.Errorf("%s: %v", e.Name, err)
			}
			return nil
		}
		errs := []error{
			set("EmblemID", int32(p.EmblemID)),
			set("EmblemStringID", emblemStem(m.Key, e.Key)),
			set("EmblemNameTextID", m.textKey(e.Key, "Name")),
			set("EmblemDescriptionTextID", m.textKey(e.Key, "Desc")),
			set("SortNumber", int32(100000+p.Number*10)),
		}
		for _, ep := range emblemPictures {
			if ep.field != "" {
				errs = append(errs, set(ep.field, softPathTo(emblemPicturePath(m.Key, e.Key, ep.suffix))))
			}
		}
		for _, x := range errs {
			if x != nil {
				return x
			}
		}
		et.Rows = append(et.Rows, SkinRow{Key: emblemRowKey(p.EmblemID), Obj: o})
		u, err := ut.NewUnlockEntry(p.UnlockID, p.EmblemID)
		if err != nil {
			return err
		}
		unlocks = append(unlocks, u)
	}
	sort.SliceStable(unlocks, func(i, j int) bool { return unlocks[i].Int("ID") < unlocks[j].Int("ID") })
	ut.Append(unlocks...)
	return nil
}

// writeEmblemTables writes the emblem list and its unlock list as raw files under root.
func writeEmblemTables(s Settings, root string, et *SkinTable, ut *UnlockTable) error {
	for _, x := range []struct {
		path string
		w    func() ([]byte, []byte, error)
	}{{emblemTablePath, et.Write}, {emblemUnlockPath, ut.Write}} {
		ua, ue, err := x.w()
		if err != nil {
			return err
		}
		if err := writePackageFiles(gameFile(root, s.Project, x.path), ua, ue, nil); err != nil {
			return err
		}
	}
	return nil
}

// hasEmblems reports whether any of the mods has new emblems.
func hasEmblems(mods []InstalledSlotMod) bool {
	for _, m := range mods {
		if len(m.Manifest.Emblems) > 0 {
			return true
		}
	}
	return false
}

// addEmblemsToList places every emblem of the mods and writes the emblem lists under root.
func addEmblemsToList(j *Job, s Settings, root string, mods []InstalledSlotMod, old SkinListState) ([]PlacedEmblem, []EmblemAssign, error) {
	et, ut, err := loadEmblemTables(j, s)
	if err != nil {
		return nil, nil, err
	}
	placed, assign, refs := assignEmblemNumbers(et, ut, mods, old)
	if err := addEmblemRows(et.sc, et, ut, placed, refs); err != nil {
		return nil, nil, err
	}
	if err := writeEmblemTables(s, root, et, ut); err != nil {
		return nil, nil, err
	}
	return placed, assign, nil
}

// ------------------------------------------------------------------ building an emblem

type emblemReq struct {
	Mod         string   `json:"mod"`      // key of a mod made here to add it to; "" = a new mod
	ModTitle    string   `json:"modTitle"` // name of a new mod
	Author      string   `json:"author"`
	Emblem      string   `json:"emblem"` // key of an emblem to change; "" = a new emblem
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Picture     string   `json:"picture"` // the picture; "" keeps the pictures it has (changing one)
	Number      int      `json:"number"`  // an emblem number to ask for; 0 = any
	Cleanup     []string `json:"cleanup"`
}

type emblemResult struct {
	Mod      string      `json:"mod"`
	ModTitle string      `json:"modTitle"`
	Folder   string      `json:"folder"`
	Emblem   string      `json:"emblem"`
	Name     string      `json:"name"`
	Number   int         `json:"number"`
	Icon     string      `json:"icon"`
	List     *ListResult `json:"list"`
	Setup    []string    `json:"setup"`
}

// emblemSquare fits a picture into a square with see-through edges, keeping its shape, and gives
// the see-through pixels the colour of the picture nearby (so their colour does not show as a
// dark fringe where the game blends the edges).
func emblemSquare(src *Img) *Img {
	n := max(src.W, src.H)
	out := NewImg(n, n)
	out.Paste(src, (n-src.W)/2, (n-src.H)/2)
	return bleedColour(out)
}

// bleedColour fills the colour of fully see-through pixels from their visible neighbours, a few
// pixels wide, and the rest with the picture's average colour.
func bleedColour(m *Img) *Img {
	w, h := m.W, m.H
	known := make([]bool, w*h)
	var sum [3]float64
	n := 0.0
	for i := 0; i < w*h; i++ {
		if a := m.Pix[i*4+3]; a > 0 {
			known[i] = true
			for c := 0; c < 3; c++ {
				sum[c] += float64(m.Pix[i*4+c]) * float64(a)
			}
			n += float64(a)
		}
	}
	for pass := 0; pass < 8; pass++ {
		var fill []int
		var cols [][3]uint8
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := y*w + x
				if known[i] {
					continue
				}
				var acc [3]int
				k := 0
				for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
					xx, yy := x+d[0], y+d[1]
					if xx >= 0 && yy >= 0 && xx < w && yy < h && known[yy*w+xx] {
						j := (yy*w + xx) * 4
						acc[0], acc[1], acc[2] = acc[0]+int(m.Pix[j]), acc[1]+int(m.Pix[j+1]), acc[2]+int(m.Pix[j+2])
						k++
					}
				}
				if k > 0 {
					fill = append(fill, i)
					cols = append(cols, [3]uint8{uint8(acc[0] / k), uint8(acc[1] / k), uint8(acc[2] / k)})
				}
			}
		}
		if len(fill) == 0 {
			break
		}
		for f, i := range fill {
			copy(m.Pix[i*4:i*4+3], cols[f][:])
			known[i] = true
		}
	}
	if n > 0 {
		for i := 0; i < w*h; i++ {
			if !known[i] {
				for c := 0; c < 3; c++ {
					m.Pix[i*4+c] = uint8(sum[c] / n)
				}
			}
		}
	}
	return m
}

func (s Settings) emblemInputDir(m *ModManifest) string {
	return filepath.Join(s.projectDir(m.Key), "input")
}

// doBuildEmblem makes (or changes) a new emblem in a skin mod made here, installs the mod and
// updates the shared lists.
func doBuildEmblem(j *Job, s Settings, r emblemReq) (any, error) {
	if err := needModsFolder(s); err != nil {
		return nil, err
	}
	if _, err := needExtractSetup(s); err != nil {
		return nil, err
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return nil, errors.New("give the emblem a name; it is what the game shows in the emblem list")
	}
	if utf16Len(r.Name) > 80 {
		return nil, errors.New("the emblem name is too long (80 characters at most)")
	}
	if utf16Len(r.Description) > 1000 {
		return nil, errors.New("the description is too long (1000 characters at most)")
	}
	// the mod
	var m *ModManifest
	var err error
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
	key := r.Emblem
	old := (*EmblemDef)(nil)
	if key != "" {
		if old = m.emblem(key); old == nil {
			return nil, errors.New("that emblem is no longer in the mod")
		}
	} else {
		key = m.nextEmblemKey()
	}
	e := EmblemDef{Key: key, Number: r.Number, Name: r.Name, Description: strings.TrimSpace(r.Description)}
	if old != nil {
		if r.Number == 0 {
			e.Number = old.Number
		}
		e.IconFile = old.IconFile
	}
	projDir := s.projectDir(m.Key)
	iconPNG := filepath.Join(projDir, "icons", key+".png")
	pic := strings.TrimSpace(r.Picture)
	if pic == "" && old == nil {
		return nil, errors.New("choose the emblem's picture")
	}
	if pic != "" {
		src, err := LoadPicture(cleanPath(pic))
		if err != nil {
			return nil, err
		}
		if src.W < 16 || src.H < 16 {
			return nil, errors.New("the picture is too small for an emblem")
		}
		img := emblemSquare(src)
		j.SetStep("Reading the game's emblem list", 0.05)
		et, _, err := loadEmblemTables(j, s)
		if err != nil {
			return nil, err
		}
		tmpl, err := emblemTemplate(et)
		if err != nil {
			return nil, err
		}
		srcs, err := emblemTemplatePictures(tmpl)
		if err != nil {
			return nil, err
		}
		if err := ensureBase(j, s, srcs); err != nil {
			return nil, err
		}
		input := s.emblemInputDir(m)
		for i, ep := range emblemPictures {
			if j.Cancelled() {
				return nil, errors.New("cancelled")
			}
			j.SetStep(fmt.Sprintf("Making the emblem's pictures (%d of %d)", i+1, len(emblemPictures)), 0.1+0.5*float64(i)/float64(len(emblemPictures)))
			t, p, ubulk, err := LoadTex2D(gameFile(s.baseDir(), s.Project, srcs[i]))
			if err != nil {
				return nil, fmt.Errorf("%s: %v", packageBaseName(srcs[i]), err)
			}
			ue, ub, err := t.Rebuild(p.UExp, ubulk, img, nil)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", packageBaseName(srcs[i]), err)
			}
			q, err := ParsePackage(p.UAsset, ue)
			if err != nil {
				return nil, err
			}
			dst := emblemPicturePath(m.Key, key, ep.suffix)
			ua, ue2, err := q.Rename(dst, map[string]string{packageBaseName(srcs[i]): packageBaseName(dst)})
			if err != nil {
				return nil, err
			}
			if err := writePackageFiles(gameFile(input, s.Project, dst), ua, ue2, ub); err != nil {
				return nil, err
			}
		}
		os.MkdirAll(filepath.Dir(iconPNG), 0o755)
		if err := SaveImage(iconPNG, resize(img, 256, 256), SaveRGBA); err == nil {
			e.IconFile = "icons/" + key + ".png"
		}
		j.Log("Made the emblem's five pictures from %s", filepath.Base(pic))
	}
	if old != nil {
		*old = e
	} else {
		m.Emblems = append(m.Emblems, e)
	}
	if err := saveAndInstallProject(j, s, m); err != nil {
		return nil, err
	}
	cleanInbox(s, r.Cleanup)
	j.SetStep("Updating the skin and emblem lists", 0.9)
	list, err := RebuildSkinList(j, s)
	if err != nil {
		return nil, fmt.Errorf("the emblem was built and installed, but the emblem list could not be updated: %v", err)
	}
	res := emblemResult{Mod: m.Key, ModTitle: m.Title, Folder: m.Folder, Emblem: key, Name: e.Name, List: list, Setup: slotSetupProblems(s)}
	if e.IconFile != "" {
		res.Icon = filepath.Join(s.ModsDir, m.Folder, filepath.FromSlash(e.IconFile))
	}
	for _, p := range list.Emblems {
		if p.Mod == m.Key && p.Emblem == key {
			res.Number = p.Number
		}
	}
	j.Log("New emblem %q is emblem %d, in the mod %s.", e.Name, res.Number, m.Folder)
	return res, nil
}

// removeEmblemFiles deletes an emblem's pictures from a mod made here.
func removeEmblemFiles(s Settings, m *ModManifest, key string) {
	for _, ep := range emblemPictures {
		base := gameFile(s.emblemInputDir(m), s.Project, emblemPicturePath(m.Key, key, ep.suffix))
		for _, ext := range []string{".uasset", ".uexp", ".ubulk"} {
			os.Remove(base + ext)
		}
	}
	os.Remove(filepath.Join(s.projectDir(m.Key), "icons", key+".png"))
}

// builtKeys lists every skin and emblem of the mods (mod key/skin or emblem key), sorted: what a
// skin list is built from, to tell later whether it is still up to date.
func builtKeys(mods []InstalledSlotMod) []string {
	var out []string
	for _, m := range mods {
		for _, sl := range m.Manifest.Slots {
			out = append(out, m.Manifest.Key+"/"+sl.Key)
		}
		for _, e := range m.Manifest.Emblems {
			out = append(out, m.Manifest.Key+"/"+e.Key)
		}
	}
	sort.Strings(out)
	return out
}

// keepEmblemRecords gives the emblem records to save: the emblems placed now, and those that are
// not in the list now (their mod was taken out) so they get their number and unlock id back when
// they return; and every unlock id ever given to an emblem, which is never given to another.
func keepEmblemRecords(old SkinListState, now []EmblemAssign) ([]EmblemAssign, []int64) {
	out := append([]EmblemAssign(nil), now...)
	placed := map[string]bool{}
	for _, a := range now {
		placed[a.Mod+"/"+a.Emblem] = true
	}
	for _, a := range old.Emblems {
		if !placed[a.Mod+"/"+a.Emblem] {
			out = append(out, a)
		}
	}
	var ids []int64
	for _, a := range out {
		ids = append(ids, a.UnlockID)
	}
	return out, mergeIDs(old.EmblemRetired, ids)
}
