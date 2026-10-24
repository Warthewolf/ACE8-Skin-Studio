package main

// New skin slots.
//
// A skin mod made here ("slot mod") adds skins in slots of their own instead of replacing one of the
// game's skins. Its folder in the mods folder holds one container with the skins' own textures, all
// under /Game/SkinStudio/<mod key>/<slot key>/ so two mods can never collide on a file,
// slotfiles/ with each skin's material and icon, and skinstudio.json describing its skins.
//
// Each skin gets a material slot of its own, numbered like the game's skins (see slotfiles.go): the
// skin list builder writes its material and icon under the skin's number and packs them with the
// skin list, so the numbers can change without the mods having to be packed again.
//
// The game learns about the skins from two shared files, the skin list (DT_Skin) and the unlock list
// (DA_Unlock_Skin). Only one copy of each can be in effect, so this program builds one merged copy,
// the "skin list" mod (zz_SkinStudio_SkinList), from the game's own files plus every installed slot
// mod, and gives each skin a free number. A skin keeps its number once it has one; one that arrives
// wanting a number already taken gets the next free one. Names and descriptions are not in either
// file: the skin list refers to text keys, and the SkinSlotText UE4SS mod supplies the texts from
// one small file per mod.

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	manifestName    = "skinstudio.json"
	manifestFormat  = "ace8-skin-studio-mod"
	skinListMod     = "zz_SkinStudio_SkinList"
	skinListState   = "skin-list.json"
	firstSlotNumber = 601
	lastSlotNumber  = 999
	textFilePrefix  = "SkinStudio_"
)

// SlotDef is one new skin.
type SlotDef struct {
	Key         string   `json:"key"`      // s1, s2, ... unique inside its mod
	Aircraft    int      `json:"aircraft"` // the number in PP0021_su57
	Plane       string   `json:"plane"`    // the game's short name, su57
	Vehicle     string   `json:"vehicle"`  // PP0021_su57
	Template    int64    `json:"template"` // skin id of the game skin it is based on
	Number      int      `json:"number"`   // the skin number it had when it was made or shared; 0 = any
	Material    string   `json:"material,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Unlock      string   `json:"unlock"`
	IconFile    string   `json:"iconFile,omitempty"` // picture of the icon inside the mod folder, for showing
	Textures    []string `json:"textures,omitempty"` // the textures it has its own copy of
	// Sources says which game texture each of its own textures stands in for (name -> game path).
	Sources map[string]string `json:"sources,omitempty"`
	// Tex says how its colour texture is stored, when not the game's way (BC7, no mipmaps).
	Tex TexOptions `json:"tex,omitempty"`
}

// SkinText changes the name, description (and icon) of one of the game's own skins.
type SkinText struct {
	SkinID      int64  `json:"skinId"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	NameKey     string `json:"nameKey"`
	DescKey     string `json:"descKey"`
}

// ModManifest is skinstudio.json.
type ModManifest struct {
	Format        string      `json:"format"`
	FormatVersion int         `json:"formatVersion"`
	MadeWith      string      `json:"madeWith"`
	Key           string      `json:"key"`    // unique: used in file paths, text keys and the container name
	Folder        string      `json:"folder"` // the folder name in the mods folder
	Title         string      `json:"title"`  // shown to people
	Author        string      `json:"author,omitempty"`
	Version       string      `json:"version,omitempty"`
	About         string      `json:"about,omitempty"`
	Container     string      `json:"container"` // container file name without the ending
	Slots         []SlotDef   `json:"slots"`
	Emblems       []EmblemDef `json:"emblems,omitempty"` // new emblems (see emblems.go)
	Texts         []SkinText  `json:"texts,omitempty"`
	Created       string      `json:"created,omitempty"`
}

func (m *ModManifest) textKey(slot, what string) string {
	return "SS_" + m.Key + "_" + slot + "_" + what
}

func (m *ModManifest) slot(key string) *SlotDef {
	for i := range m.Slots {
		if m.Slots[i].Key == key {
			return &m.Slots[i]
		}
	}
	return nil
}

func (m *ModManifest) nextSlotKey() string {
	for i := 1; ; i++ {
		k := "s" + strconv.Itoa(i)
		if m.slot(k) == nil {
			return k
		}
	}
}

// slotAssetDir is the game folder a slot's own files live in.
func slotAssetDir(modKey, slotKey string) string {
	return "/Game/SkinStudio/" + modKey + "/" + slotKey + "/"
}

var reKeyChars = regexp.MustCompile(`[^A-Za-z0-9]+`)

// newModKey makes a key from a name: its letters and digits, then an underscore and five random
// letters (letters, so the engine never reads the end as a number).
func newModKey(name string) string {
	base := reKeyChars.ReplaceAllString(name, "")
	if len(base) > 24 {
		base = base[:24]
	}
	if base == "" {
		base = "Skin"
	}
	var sb strings.Builder
	for i := 0; i < 5; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(26))
		sb.WriteByte(byte('a' + n.Int64()))
	}
	return base + "_" + sb.String()
}

var reModKey = regexp.MustCompile(`^[A-Za-z0-9]{1,24}_[a-z]{5}$`)

func readManifest(path string) (*ModManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m ModManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s is not readable: %v", filepath.Base(path), err)
	}
	if m.Format != manifestFormat {
		return nil, fmt.Errorf("%s was not made by ACE8 Skin Studio", filepath.Base(path))
	}
	if err := m.check(); err != nil {
		return nil, err
	}
	return &m, nil
}

// check refuses a manifest whose keys could reach outside the mod's own files.
func (m *ModManifest) check() error {
	if !reModKey.MatchString(m.Key) {
		return fmt.Errorf("the mod key %q is not valid", m.Key)
	}
	if m.Container == "" || reKeyChars.ReplaceAllString(strings.ReplaceAll(m.Container, "_", ""), "") != strings.ReplaceAll(m.Container, "_", "") {
		return fmt.Errorf("the container name %q is not valid", m.Container)
	}
	seen := map[string]bool{}
	for _, sl := range m.Slots {
		if !regexp.MustCompile(`^s[0-9]{1,3}$`).MatchString(sl.Key) || seen[sl.Key] {
			return fmt.Errorf("the skin key %q is not valid", sl.Key)
		}
		seen[sl.Key] = true
		own := slotAssetDir(m.Key, sl.Key)
		for _, p := range []string{sl.Material, sl.Icon} {
			if p != "" && !strings.HasPrefix(p, own) && !strings.HasPrefix(p, "/Game/") {
				return fmt.Errorf("skin %s refers to %s, outside the game's content", sl.Key, p)
			}
		}
		if sl.Aircraft <= 0 || sl.Template <= 0 || aircraftOf(sl.Template) != sl.Aircraft {
			return fmt.Errorf("skin %s does not say which aircraft and skin it is based on", sl.Key)
		}
	}
	for _, e := range m.Emblems {
		if !reEmblemKey.MatchString(e.Key) || seen[e.Key] {
			return fmt.Errorf("the emblem key %q is not valid", e.Key)
		}
		seen[e.Key] = true
	}
	return nil
}

func writeManifest(path string, m *ModManifest) error {
	m.Format, m.FormatVersion, m.MadeWith = manifestFormat, 1, appName+" "+appVersion
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

// InstalledSlotMod is a slot mod found in the mods folder.
type InstalledSlotMod struct {
	Dir      string
	Manifest *ModManifest
}

// scanSlotMods finds the slot mods in the mods folder (folders holding skinstudio.json).
func scanSlotMods(modsDir string) ([]InstalledSlotMod, []string) {
	var out []InstalledSlotMod
	var problems []string
	es, err := os.ReadDir(modsDir)
	if err != nil {
		return nil, nil
	}
	seen := map[string]string{}
	for _, e := range es {
		if !e.IsDir() || e.Name() == skinListMod {
			continue
		}
		p := filepath.Join(modsDir, e.Name(), manifestName)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		m, err := readManifest(p)
		if err != nil {
			problems = append(problems, e.Name()+": "+err.Error())
			continue
		}
		if other, dup := seen[m.Key]; dup {
			problems = append(problems, fmt.Sprintf("%s and %s are the same mod; only %s is used. Remove one of them.", other, e.Name(), other))
			continue
		}
		seen[m.Key] = e.Name()
		out = append(out, InstalledSlotMod{Dir: filepath.Join(modsDir, e.Name()), Manifest: m})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Manifest.Folder) < strings.ToLower(out[j].Manifest.Folder)
	})
	return out, problems
}

// ------------------------------------------------------------------ numbers

// SlotAssign records which number and unlock id a skin has.
type SlotAssign struct {
	Mod      string `json:"mod"`
	Slot     string `json:"slot"`
	Aircraft int    `json:"aircraft"`
	Number   int    `json:"number"`
	UnlockID int64  `json:"unlockId"`
	// UnlockID2 was the second rule of a skin "unlocked from the start" (2.0.0 to 2.0.2); kept so
	// that id is never given out again.
	UnlockID2 int64 `json:"unlockId2,omitempty"`
}

// SkinListState is kept next to the skin list mod (and a copy in the workspace).
type SkinListState struct {
	Format  string       `json:"format"`
	Assign  []SlotAssign `json:"assign"`
	Built   string       `json:"built"`
	Game    string       `json:"game"`    // the game's containers when it was built (a game update changes this)
	Mods    []string     `json:"mods"`    // the skin mods it was built from (key/slot)
	Adopted []string     `json:"adopted"` // other mods whose skin lists were merged in
	// Retired lists every unlock id ever given out. The game remembers which unlock rules have
	// happened by id, so an id is never given to another skin: that skin would count as unlocked
	// already and never be unlocked.
	Retired []int64 `json:"retired,omitempty"`
	// Foreign lists the unlock ids other mods' skins have had here (and the SkinSlotTest
	// experiment's). Those mods keep them; this program's skins never get them.
	Foreign []int64 `json:"foreign,omitempty"`
	// Layout is how the list was built: 2 = with numbered material slots (2.0.2), 3 = with the
	// game's own unlock rule and ids following the game's (2.0.3 on).
	Layout int `json:"layout,omitempty"`
	// Emblems and EmblemRetired are the same records for new emblems (2.2.0 on).
	Emblems       []EmblemAssign `json:"emblems,omitempty"`
	EmblemRetired []int64        `json:"emblemRetired,omitempty"`
}

// The SkinSlotTest experiment (before 2.0.0) gave its two skins the unlock ids right after the
// game's own. A game that had it remembers those as done, so this program's skins never use them.
var experimentIDs = []int64{1400589, 1400590}

func sawSlotExperiment(s Settings) bool {
	if b, err := os.ReadFile(filepath.Join(s.ModsDir, "loadorder.txt")); err == nil && strings.Contains(strings.ToLower(string(b)), "skinslottest") {
		return true
	}
	_, err := os.Stat(filepath.Join(s.ModsDir, "SkinSlotTest"))
	return err == nil
}

func loadListState(s Settings) SkinListState {
	st := SkinListState{Format: "ace8-skin-studio-list"}
	for _, p := range []string{filepath.Join(s.ModsDir, skinListMod, skinListState), filepath.Join(s.skinListDir(), skinListState)} {
		var x SkinListState
		if b, err := os.ReadFile(p); err == nil && json.Unmarshal(b, &x) == nil && x.Format == "ace8-skin-studio-list" {
			st = x
			break
		}
	}
	if sawSlotExperiment(s) {
		st.Foreign = mergeIDs(st.Foreign, experimentIDs)
	}
	return st
}

// mergeIDs gives the ids of both lists, sorted, each once.
func mergeIDs(a, b []int64) []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, list := range [][]int64{a, b} {
		for _, id := range list {
			if id != 0 && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func saveListState(s Settings, st SkinListState) {
	st.Format = "ace8-skin-studio-list"
	st.Built = time.Now().Format(time.RFC3339)
	b, _ := json.MarshalIndent(st, "", "  ")
	os.MkdirAll(s.skinListDir(), 0o755)
	os.WriteFile(filepath.Join(s.skinListDir(), skinListState), b, 0o644)
	if _, err := os.Stat(filepath.Join(s.ModsDir, skinListMod)); err == nil {
		os.WriteFile(filepath.Join(s.ModsDir, skinListMod, skinListState), b, 0o644)
	}
}

// PlacedSlot is where one skin ended up.
type PlacedSlot struct {
	Mod       string `json:"mod"`      // mod key
	ModTitle  string `json:"modTitle"` // shown to people
	Folder    string `json:"folder"`
	Slot      string `json:"slot"`
	Name      string `json:"name"`
	Plane     string `json:"plane"`
	Vehicle   string `json:"vehicle"`
	Aircraft  int    `json:"aircraft"`
	Number    int    `json:"number"`
	Wanted    int    `json:"wanted"`
	Moved     bool   `json:"moved"` // it wanted another number, which was taken
	UnlockID  int64  `json:"unlockId"`
	UnlockID2 int64  `json:"unlockId2,omitempty"`
	SkinID    int64  `json:"skinId"`
	Problem   string `json:"problem,omitempty"`
	// Material and Icon are the skin's own material and icon as written for its number ("" = the
	// paths in its mod's skinstudio.json).
	Material string `json:"material,omitempty"`
	Icon     string `json:"icon,omitempty"`
}

type slotRef struct {
	mod  *InstalledSlotMod
	slot *SlotDef
}

// assignNumbers gives every skin a number and an unlock id. Skins that already have numbers keep
// them; the rest get the number they want when it is free, otherwise the next free one.
func assignNumbers(st *SkinTable, ut *UnlockTable, mods []InstalledSlotMod, old SkinListState) ([]PlacedSlot, []SlotAssign, map[string]slotRef) {
	used := map[int]map[int]bool{} // aircraft -> numbers taken
	usedFor := func(a int) map[int]bool {
		if used[a] == nil {
			used[a] = map[int]bool{}
			for n := range st.UsedSlots(a) {
				used[a][n] = true
			}
		}
		return used[a]
	}
	usedIDs := ut.UsedIDs()
	prev := map[string]SlotAssign{}
	retired := map[int64]bool{}
	for _, id := range append(append([]int64(nil), old.Retired...), old.Foreign...) {
		retired[id] = true
	}
	for _, a := range old.Assign {
		prev[a.Mod+"/"+a.Slot] = a
		retired[a.UnlockID] = true
		if a.UnlockID2 != 0 {
			retired[a.UnlockID2] = true
		}
	}
	// An id a skin had before stays its own; a new one is one nobody ever had. New ids follow on from
	// the game's own, after room for a game update's (see oldUnlockStart). Ids from oldUnlockStart up
	// were given by 2.0.0 to 2.0.2 with rules the game did not act on, so a skin that had one gets a
	// new one.
	last := ut.lastLowID() + unlockRoom
	keepID := func(id int64) bool { return id > ut.MinID() && id < oldUnlockStart && !usedIDs[id] }
	newID := func() int64 {
		for x := last + 1; x <= ut.MaxID(); x++ {
			if !usedIDs[x] && !retired[x] {
				return x
			}
		}
		return 0
	}
	refs := map[string]slotRef{}
	var order []string
	for i := range mods {
		m := &mods[i]
		for k := range m.Manifest.Slots {
			sl := &m.Manifest.Slots[k]
			id := m.Manifest.Key + "/" + sl.Key
			refs[id] = slotRef{m, sl}
			order = append(order, id)
		}
	}
	// Skins that had a number first, in number order, so a newcomer never takes an old skin's number:
	// first the skins that were in the list when it was last built, then skins coming back (their mod
	// was taken out and is back; their old number may have gone to another skin since), then new ones.
	live := map[string]bool{}
	for _, k := range old.Mods {
		live[k] = true
	}
	rank := func(id string) int {
		if _, ok := prev[id]; !ok {
			return 2
		}
		if live[id] {
			return 0
		}
		return 1
	}
	sort.SliceStable(order, func(i, j int) bool {
		ri, rj := rank(order[i]), rank(order[j])
		if ri != rj {
			return ri < rj
		}
		if ri < 2 && prev[order[i]].Number != prev[order[j]].Number {
			return prev[order[i]].Number < prev[order[j]].Number
		}
		return false
	})
	var placed []PlacedSlot
	var assign []SlotAssign
	for _, id := range order {
		r := refs[id]
		sl := r.slot
		ps := PlacedSlot{Mod: r.mod.Manifest.Key, ModTitle: r.mod.Manifest.Title, Folder: filepath.Base(r.mod.Dir), Slot: sl.Key, Name: sl.Name,
			Plane: sl.Plane, Vehicle: sl.Vehicle, Aircraft: sl.Aircraft, Wanted: sl.Number}
		if st.Find(sl.Template) == nil {
			ps.Problem = fmt.Sprintf("the game skin it is based on (%d) is not in the game's skin list", sl.Template)
			placed = append(placed, ps)
			continue
		}
		taken := usedFor(sl.Aircraft)
		num := 0
		if p, ok := prev[id]; ok && p.Aircraft == sl.Aircraft && !taken[p.Number] && p.Number >= firstSlotNumber && p.Number <= lastSlotNumber {
			num = p.Number
		} else if sl.Number >= firstSlotNumber && sl.Number <= lastSlotNumber && !taken[sl.Number] {
			num = sl.Number
		} else {
			for n := firstSlotNumber; n <= lastSlotNumber; n++ {
				if !taken[n] {
					num = n
					break
				}
			}
		}
		if num == 0 {
			ps.Problem = "this aircraft has no free skin numbers left"
			placed = append(placed, ps)
			continue
		}
		taken[num] = true
		p, had := prev[id]
		uid := p.UnlockID
		if !had || !keepID(uid) {
			uid = newID()
		}
		usedIDs[uid] = true
		if uid == 0 {
			ps.Problem = "the unlock list is full"
			placed = append(placed, ps)
			continue
		}
		ps.Number, ps.UnlockID, ps.SkinID = num, uid, skinIDOf(sl.Aircraft, num)
		ps.Moved = sl.Number != 0 && sl.Number != num
		placed = append(placed, ps)
		assign = append(assign, SlotAssign{Mod: r.mod.Manifest.Key, Slot: sl.Key, Aircraft: sl.Aircraft, Number: num, UnlockID: uid})
	}
	return placed, assign, refs
}

// addSlotRows puts the placed skins into the two tables.
func addSlotRows(sc *Schema, st *SkinTable, ut *UnlockTable, placed []PlacedSlot, refs map[string]slotRef) error {
	byAircraft := map[int][]PlacedSlot{}
	for _, p := range placed {
		if p.Problem == "" {
			byAircraft[p.Aircraft] = append(byAircraft[p.Aircraft], p)
		}
	}
	var aircraft []int
	for a := range byAircraft {
		aircraft = append(aircraft, a)
	}
	sort.Ints(aircraft)
	var unlocks []*Obj
	for _, a := range aircraft {
		list := byAircraft[a]
		sort.Slice(list, func(i, j int) bool { return list[i].Number < list[j].Number })
		sortBase := st.SortBase(a)
		var rows []SkinRow
		for i, p := range list {
			r := refs[p.Mod+"/"+p.Slot]
			sl, m := r.slot, r.mod.Manifest
			tmpl := st.Find(sl.Template)
			o := tmpl.Obj.Clone()
			set := func(k string, v any) error {
				if err := o.Set(sc, k, v); err != nil {
					return fmt.Errorf("%s: %v", sl.Name, err)
				}
				return nil
			}
			errs := []error{
				set("SkinID", int32(p.SkinID)),
				set("SkinNameTextID", m.textKey(sl.Key, "Name")),
				set("SkinDescriptionTextID", m.textKey(sl.Key, "Desc")),
				set("SkinCategory", "Campaign"),
				set("DLCID", "Live"),
				set("SortNumber", int32(sortBase+900+int64(i))),
			}
			mat, icon := sl.Material, sl.Icon
			if p.Material != "" {
				mat = p.Material
			}
			if p.Icon != "" {
				icon = p.Icon
			}
			if mat != "" {
				errs = append(errs, set("DecalMaterialRef", softPathTo(mat)))
			}
			if icon != "" {
				errs = append(errs, set("MenuIconImageRef", softPathTo(icon)))
			}
			for _, e := range errs {
				if e != nil {
					return e
				}
			}
			rows = append(rows, SkinRow{Key: skinRowKey(p.SkinID), Obj: o})
			e, err := ut.NewUnlockEntry(p.UnlockID, p.SkinID)
			if err != nil {
				return err
			}
			unlocks = append(unlocks, e)
		}
		st.Insert(a, rows...)
	}
	// the unlock rules go after the game's in id order, as the game's own are
	sort.SliceStable(unlocks, func(i, j int) bool { return unlocks[i].Int("ID") < unlocks[j].Int("ID") })
	ut.Append(unlocks...)
	return nil
}

// ------------------------------------------------------------------ texts (SkinSlotText)

// ue4ssModsDir finds the UE4SS Mods folder: from Settings, next to the mods folder, or from the
// game's folder.
func (s Settings) ue4ssModsDir() string {
	if s.UE4SSMods != "" {
		if st, err := os.Stat(s.UE4SSMods); err == nil && st.IsDir() {
			return s.UE4SSMods
		}
		return ""
	}
	var cands []string
	if s.ModsDir != "" {
		cands = append(cands, filepath.Dir(s.ModsDir))
	}
	if s.PaksDir != "" {
		game := filepath.Dir(filepath.Dir(s.PaksDir)) // ...\Game
		cands = append(cands, filepath.Join(game, "Binaries", "Win64", "ue4ss", "Mods"), filepath.Join(game, "Binaries", "Win64", "Mods"))
	}
	for _, c := range cands {
		if strings.EqualFold(filepath.Base(c), "Mods") {
			if _, err := os.Stat(filepath.Join(c, "..", "UE4SS.dll")); err == nil {
				return c
			}
			if _, err := os.Stat(filepath.Join(c, "mods.txt")); err == nil {
				return c
			}
		}
	}
	return ""
}

func (s Settings) textsDir() string {
	u := s.ue4ssModsDir()
	if u == "" {
		return ""
	}
	return filepath.Join(u, "SkinSlotText", "Scripts", "texts")
}

// iniValue makes a text fit on one line of a SkinSlotText file.
func iniValue(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.TrimSpace(s)
	return strings.ReplaceAll(s, "\n", `\n`)
}

// modTexts is the text file of one mod.
func modTexts(m *ModManifest) string {
	var b strings.Builder
	b.WriteString("; Names and descriptions for the skin mod \"" + strings.ReplaceAll(m.Title, "\n", " ") + "\" (" + m.Key + ").\r\n")
	b.WriteString("; Written by " + appName + "; edit them there, changes made here are replaced.\r\n\r\n[Text]\r\n")
	for _, sl := range m.Slots {
		b.WriteString(m.textKey(sl.Key, "Name") + "=" + iniValue(sl.Name) + "\r\n")
		b.WriteString(m.textKey(sl.Key, "Desc") + "=" + iniValue(sl.Description) + "\r\n")
	}
	for _, e := range m.Emblems {
		b.WriteString(m.textKey(e.Key, "Name") + "=" + iniValue(e.Name) + "\r\n")
		b.WriteString(m.textKey(e.Key, "Desc") + "=" + iniValue(e.Description) + "\r\n")
	}
	for _, t := range m.Texts {
		if t.Name != "" && t.NameKey != "" {
			b.WriteString(t.NameKey + "=" + iniValue(t.Name) + "\r\n")
		}
		if t.Description != "" && t.DescKey != "" {
			b.WriteString(t.DescKey + "=" + iniValue(t.Description) + "\r\n")
		}
	}
	return b.String()
}

// writeAllTexts writes one text file per installed slot mod and removes the files of mods that
// are gone. It returns the folder, or "" when SkinSlotText's folder is not known.
func writeAllTexts(s Settings, mods []InstalledSlotMod) (string, error) {
	dir := s.textsDir()
	if dir == "" {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return dir, err
	}
	keep := map[string]bool{}
	for _, m := range mods {
		name := textFilePrefix + m.Manifest.Key + ".ini"
		keep[strings.ToLower(name)] = true
		if err := os.WriteFile(filepath.Join(dir, name), []byte(modTexts(m.Manifest)), 0o644); err != nil {
			return dir, err
		}
	}
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		n := e.Name()
		if strings.HasPrefix(n, textFilePrefix) && strings.EqualFold(filepath.Ext(n), ".ini") && !keep[strings.ToLower(n)] {
			os.Remove(filepath.Join(dir, n))
		}
	}
	return dir, nil
}

// ------------------------------------------------------------------ the skin list mod

// ListResult is what a rebuild of the skin list did.
type ListResult struct {
	Slots     []PlacedSlot   `json:"slots"`
	Emblems   []PlacedEmblem `json:"emblems"`
	Adopted   []AdoptedMod   `json:"adopted"`
	Problems  []string       `json:"problems"`
	Installed string         `json:"installed"`
	TextsDir  string         `json:"textsDir"`
	Removed   bool           `json:"removed"` // no new skins are left, so the skin list mod was taken out
}

func listInputDir(s Settings) string { return filepath.Join(s.skinListDir(), "input") }

// writeTables writes the two tables as raw files under root.
func writeTables(s Settings, root string, st *SkinTable, ut *UnlockTable) error {
	for _, x := range []struct {
		path string
		w    func() ([]byte, []byte, error)
	}{{skinTablePath, st.Write}, {unlockTablePath, ut.Write}} {
		ua, ue, err := x.w()
		if err != nil {
			return err
		}
		p := gameFile(root, s.Project, x.path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p+".uasset", ua, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(p+".uexp", ue, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// packFolder packs a folder of raw files into <outDir>/<name>.utoc/.ucas/.pak.
func packFolder(j *Job, s Settings, input, outDir, name string) ([]string, error) {
	retoc := s.findRetoc()
	if retoc == "" {
		return nil, errors.New(s.checks()["retoc"].Note + " Open Settings to fix this.")
	}
	os.RemoveAll(outDir)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	if err := runTool(j, retoc, []string{"to-zen", "--version", s.EngineVersion, input, filepath.Join(outDir, name+".utoc")}, nil); err != nil {
		return nil, err
	}
	var files []string
	for _, e := range []string{".utoc", ".ucas", ".pak"} {
		p := filepath.Join(outDir, name+e)
		if st, err := os.Stat(p); err == nil && st.Size() > 0 {
			files = append(files, p)
		}
	}
	if len(files) < 2 {
		return nil, errors.New("retoc finished but did not write the container files; see the log")
	}
	return files, nil
}

// installFiles copies files into a folder of the mods folder, replacing what is there.
func installFiles(dest string, files []string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		if err := copyFile(f, filepath.Join(dest, filepath.Base(f))); err != nil {
			return fmt.Errorf("could not copy %s into the mods folder (%v). If the game is running, close it and try again", filepath.Base(f), err)
		}
	}
	return nil
}

func needModsFolder(s Settings) error {
	if c := s.checks()["mods"]; !c.OK {
		return errors.New("new skin slots go straight into your mods folder; set it in Settings first (" + c.Note + ")")
	}
	return nil
}

// RebuildSkinList builds the merged skin list from the game's files and every installed slot mod,
// installs it and writes the texts.
func RebuildSkinList(j *Job, s Settings) (*ListResult, error) {
	if err := needModsFolder(s); err != nil {
		return nil, err
	}
	res := &ListResult{Slots: []PlacedSlot{}, Emblems: []PlacedEmblem{}, Adopted: []AdoptedMod{}, Problems: []string{}}
	mods, problems := scanSlotMods(s.ModsDir)
	res.Problems = append(res.Problems, problems...)
	adopted := findTableMods(s, mods)
	listDir := filepath.Join(s.ModsDir, skinListMod)
	if len(mods) == 0 && len(adopted) == 0 {
		// nothing to add: take the skin list mod out so the game's own lists are used again
		if _, err := os.Stat(listDir); err == nil {
			if err := os.RemoveAll(listDir); err != nil {
				return nil, fmt.Errorf("the skin list mod could not be removed (%v). If the game is running, close it and try again", err)
			}
			res.Removed = true
		}
		dir, err := writeAllTexts(s, nil)
		res.TextsDir = dir
		if err != nil {
			res.Problems = append(res.Problems, "The names could not be written: "+err.Error())
		}
		return res, nil
	}
	j.SetStep("Reading the game's skin list", 0.05)
	st, ut, err := loadBaseTables(j, s)
	if err != nil {
		return nil, err
	}
	sc := st.sc
	old := loadListState(s)
	// Skins from other mods' lists go in first. They keep their own numbers where they can, but never
	// take a number one of this program's skins already has.
	avoid, avoidIDs := map[int]map[int]bool{}, map[int64]bool{}
	ourKeys := map[string]bool{}
	installed := map[string]bool{}
	for _, m := range mods {
		ourKeys[m.Manifest.Key] = true
		for _, sl := range m.Manifest.Slots {
			installed[m.Manifest.Key+"/"+sl.Key] = true
		}
	}
	for _, a := range old.Assign {
		avoidIDs[a.UnlockID] = true // ids ever given out are never given to another skin
		if a.UnlockID2 != 0 {
			avoidIDs[a.UnlockID2] = true
		}
		if installed[a.Mod+"/"+a.Slot] {
			if avoid[a.Aircraft] == nil {
				avoid[a.Aircraft] = map[int]bool{}
			}
			avoid[a.Aircraft][a.Number] = true
		}
	}
	for _, id := range old.Retired {
		avoidIDs[id] = true
	}
	baseIDs := ut.UsedIDs()
	for i := range adopted {
		adoptRows(j, sc, st, ut, &adopted[i], avoid, avoidIDs, ourKeys)
		res.Adopted = append(res.Adopted, adopted[i])
		if adopted[i].Problem != "" {
			res.Problems = append(res.Problems, adopted[i].Folder+": "+adopted[i].Problem+".")
		}
	}
	// the adopted skins are in the tables now, so their numbers count as taken
	placed, assign, refs := assignNumbers(st, ut, mods, old)
	in := listInputDir(s)
	os.RemoveAll(in)
	j.SetStep("Making the skins' material slots", 0.2)
	for _, n := range writeMaterialSlots(s, in, st, placed, refs) {
		j.Log("Note: %s", n)
	}
	if err := addSlotRows(sc, st, ut, placed, refs); err != nil {
		return nil, err
	}
	res.Slots = placed
	for _, p := range placed {
		if p.Problem != "" {
			res.Problems = append(res.Problems, fmt.Sprintf("%s (%s) was left out: %s.", p.Name, p.ModTitle, p.Problem))
		}
	}
	j.SetStep("Writing the skin list", 0.3)
	if err := writeTables(s, in, st, ut); err != nil {
		return nil, err
	}
	// new emblems: the emblem list and its unlock list go into the same mod
	var emblemAssign []EmblemAssign
	if hasEmblems(mods) {
		j.SetStep("Writing the emblem list", 0.4)
		placedE, assignE, err := addEmblemsToList(j, s, in, mods, old)
		if err != nil {
			return nil, err
		}
		res.Emblems, emblemAssign = placedE, assignE
		for _, p := range placedE {
			if p.Problem != "" {
				res.Problems = append(res.Problems, fmt.Sprintf("The emblem %s (%s) was left out: %s.", p.Name, p.ModTitle, p.Problem))
			}
		}
	}
	j.SetStep("Packing the skin list", 0.5)
	files, err := packFolder(j, s, in, filepath.Join(s.skinListDir(), "packed"), skinListMod+"_P")
	if err != nil {
		return nil, err
	}
	j.SetStep("Installing the skin list", 0.85)
	if err := installFiles(listDir, files); err != nil {
		return nil, err
	}
	res.Installed = listDir
	built := builtKeys(mods)
	var adoptedNames []string
	for _, a := range adopted {
		adoptedNames = append(adoptedNames, a.Folder)
	}
	sort.Strings(adoptedNames)
	retired := map[int64]bool{}
	for _, id := range old.Retired {
		retired[id] = true
	}
	// the unlock ids other mods' skins have here are never given to one of ours later either
	ours := map[int64]bool{}
	for _, a := range assign {
		ours[a.UnlockID] = true
	}
	foreign := old.Foreign
	for id := range ut.UsedIDs() {
		if !baseIDs[id] && !ours[id] {
			foreign = mergeIDs(foreign, []int64{id})
		}
	}
	for _, list := range [][]SlotAssign{old.Assign, assign} {
		for _, a := range list {
			retired[a.UnlockID] = true
			if a.UnlockID2 != 0 {
				retired[a.UnlockID2] = true
			}
		}
	}
	// skins that are not in the list now (their mod was taken out) keep their record, so they get
	// their unlock ids, and their number if it is still free, back when they return
	placedNow := map[string]bool{}
	for _, a := range assign {
		placedNow[a.Mod+"/"+a.Slot] = true
	}
	for _, a := range old.Assign {
		if !placedNow[a.Mod+"/"+a.Slot] {
			assign = append(assign, a)
		}
	}
	var ret []int64
	for id := range retired {
		if id != 0 {
			ret = append(ret, id)
		}
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i] < ret[j] })
	eAssign, eRetired := keepEmblemRecords(old, emblemAssign)
	saveListState(s, SkinListState{Assign: assign, Game: paksSignature(s.PaksDir), Mods: built, Adopted: adoptedNames, Retired: ret, Foreign: foreign, Layout: slotLayout,
		Emblems: eAssign, EmblemRetired: eRetired})
	putListLast(s, adopted)
	if len(mods) > 0 {
		if err := installSkinSlotText(s); err != nil {
			res.Problems = append(res.Problems, "SkinSlotText, which shows the names and descriptions of new skins, could not be installed: "+err.Error())
		}
		keepSaveBackupMod(s)
	}
	dir, err := writeAllTexts(s, mods)
	res.TextsDir = dir
	if err != nil {
		res.Problems = append(res.Problems, "The names and descriptions could not be written: "+err.Error())
	}
	j.Log("Skin list: %d new skins from %d mods%s.", len(assign), len(mods), map[bool]string{true: fmt.Sprintf(", plus skins from %d other mods", len(adopted)), false: ""}[len(adopted) > 0])
	if len(emblemAssign) > 0 {
		j.Log("Emblem list: %d new emblems.", len(emblemAssign))
	}
	for _, p := range res.Emblems {
		if p.Moved {
			j.Log("The emblem %s (%s) wanted number %d, which was taken; it is number %d.", p.Name, p.ModTitle, p.Wanted, p.Number)
		}
	}
	for _, p := range placed {
		if p.Moved {
			j.Log("%s (%s, %s) wanted skin number %d, which was taken; it is number %d.", p.Name, p.ModTitle, p.Plane, p.Wanted, p.Number)
		}
	}
	return res, nil
}

// writeMaterialSlots writes every placed skin's material slot under root and notes in placed where
// its material and icon now are. A skin whose files cannot be made keeps the paths in its mod.
func writeMaterialSlots(s Settings, root string, st *SkinTable, placed []PlacedSlot, refs map[string]slotRef) []string {
	var notes []string
	for i := range placed {
		p := &placed[i]
		if p.Problem != "" {
			continue
		}
		r := refs[p.Mod+"/"+p.Slot]
		tmpl := st.Find(r.slot.Template)
		if tmpl == nil {
			continue
		}
		mat, icon, ns, err := numberedSlotFiles(s, root, tmpl, *r.mod, *r.slot, p.Number)
		notes = append(notes, ns...)
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s: its material slot could not be made (%v); it uses the files in its mod", p.Name, err))
			continue
		}
		p.Material, p.Icon = mat, icon
	}
	return notes
}

// putListLast makes sure the skin list mod loads after every other mod that has a skin list, when
// the loader's loadorder.txt is used (later entries win).
func putListLast(s Settings, tableMods []AdoptedMod) {
	p := filepath.Join(s.ModsDir, "loadorder.txt")
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	listed := map[string]bool{}
	var keep []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		name := strings.TrimSpace(strings.TrimLeft(t, "+-"))
		if strings.EqualFold(name, skinListMod) {
			continue
		}
		if t != "" && !strings.HasPrefix(t, "#") {
			listed[strings.ToLower(name)] = true
		}
		keep = append(keep, l)
	}
	for len(keep) > 0 && strings.TrimSpace(keep[len(keep)-1]) == "" {
		keep = keep[:len(keep)-1]
	}
	// mods with their own skin list must be listed (as they are), so they come before ours
	for _, m := range tableMods {
		if !listed[strings.ToLower(m.Folder)] {
			keep = append(keep, "+"+m.Folder)
		}
	}
	keep = append(keep, "+"+skinListMod)
	os.WriteFile(p, []byte(strings.Join(keep, "\r\n")+"\r\n"), 0o644)
}
