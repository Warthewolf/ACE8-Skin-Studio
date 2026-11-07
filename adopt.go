package main

// Other mods that carry their own copy of the skin list or unlock list (made with other tools, or
// by hand). Only one copy of each list can be in effect, so instead of letting one mod's list hide
// everyone else's, their changes are copied into the merged list: skins they add get a free number
// like any new skin, and changes they make to the game's own skins and unlock rules are kept.

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type AdoptedSkin struct {
	Plane    string `json:"plane"`
	Aircraft int    `json:"aircraft"`
	Wanted   int    `json:"wanted"`
	Number   int    `json:"number"`
	NameKey  string `json:"nameKey"`
	Moved    bool   `json:"moved"`
}

// AdoptedMod is another mod with its own skin list or unlock list.
type AdoptedMod struct {
	Folder   string        `json:"folder"`
	Dir      string        `json:"dir"`
	NewSkins []AdoptedSkin `json:"newSkins"`
	Changed  int           `json:"changed"` // the game's own skins it changes
	Unlocks  int           `json:"unlocks"` // unlock rules it adds or changes
	Skipped  int           `json:"skipped"` // skins that are already in the list through a Skin Studio mod
	Problem  string        `json:"problem,omitempty"`
	utocs    []string
	read     bool            // its lists were read
	assign   []AdoptedAssign // the numbers and unlock ids its new skins got
}

// AdoptedAssign records the number and unlock id a skin from another mod's list was given, so it
// keeps them while this program's skins come and go, and a skin from a mod installed later never
// takes them.
type AdoptedAssign struct {
	Folder   string `json:"folder"` // the mod's folder
	Row      string `json:"row"`    // the skin's row in that mod's list
	Aircraft int    `json:"aircraft"`
	Number   int    `json:"number"`
	UnlockID int64  `json:"unlockId,omitempty"`
}

func adoptedKey(folder, row string) string { return strings.ToLower(folder) + "/" + row }

const (
	skinListLabel   = "Datatables/Information/DT_Skin"
	unlockListLabel = "Unlock/DA_Unlock_Skin"
)

// carriesLists tells whether a mod holds a skin list or unlock list of its own. It looks at every
// file of the mod: the list of its files for the window (Replaces) is cut short for a big mod.
func carriesLists(m InstalledMod) bool {
	for _, label := range m.chunks {
		if label == skinListLabel || label == unlockListLabel {
			return true
		}
	}
	for _, r := range m.Replaces {
		if r == skinListLabel || r == unlockListLabel {
			return true
		}
	}
	return false
}

// findTableMods lists the mods (other than this program's own) that hold a skin list or unlock list.
func findTableMods(s Settings, slotMods []InstalledSlotMod) []AdoptedMod {
	var out []AdoptedMod
	ours := map[string]bool{}
	for _, m := range slotMods {
		ours[strings.ToLower(filepath.Clean(m.Dir))] = true
	}
	for _, m := range scanMods(s.ModsDir) {
		if m.Name == skinListMod || ours[strings.ToLower(filepath.Clean(m.Dir))] && !m.Loose {
			continue
		}
		if !carriesLists(m) {
			continue
		}
		am := AdoptedMod{Folder: m.Name, Dir: m.Dir, NewSkins: []AdoptedSkin{}}
		for _, f := range m.Files {
			if strings.EqualFold(filepath.Ext(f.Name), ".utoc") {
				am.utocs = append(am.utocs, filepath.Join(m.Dir, f.Name))
			}
		}
		out = append(out, am)
	}
	return out
}

// sameObj tells whether two rows hold the same values (they may come from packages with different name maps).
func sameObj(sc *Schema, a, b *Obj) bool {
	nt := NewNameTable(nil, 0)
	x, err1 := WriteObject(sc, nt, a)
	y, err2 := WriteObject(sc, nt, b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

// gameLists are the game's own skin rows and unlock rules, as they are before any other mod's list
// is taken in. A mod's row or rule with the key or id of one of them is a change to the game's;
// anything else is new, also when a mod taken in earlier added one with the same key or id.
type gameLists struct {
	rows    map[string]*Obj
	entries map[int64]*Obj
}

func gameListsOf(st *SkinTable, ut *UnlockTable) gameLists {
	g := gameLists{rows: map[string]*Obj{}, entries: map[int64]*Obj{}}
	for _, r := range st.Rows {
		g.rows[r.Key] = r.Obj.Clone()
	}
	for _, e := range ut.Entries() {
		g.entries[e.Int("ID")] = e.Clone()
	}
	return g
}

// adoption is what taking in the other mods' lists shares between them.
type adoption struct {
	game        gameLists
	reserved    map[int]map[int]bool     // aircraft -> numbers not given to their new skins (ours, and those placed so far)
	reservedIDs map[int64]bool           // unlock ids not given to them
	ourKeys     map[string]bool          // this program's mod keys
	kept        map[string]AdoptedAssign // adoptedKey -> what that skin had before
	held        map[int]map[int]string   // aircraft -> number -> adoptedKey of the skin, still there, that had it
}

func (ad *adoption) reserve(a, n int) {
	if ad.reserved[a] == nil {
		ad.reserved[a] = map[int]bool{}
	}
	ad.reserved[a][n] = true
}

// adoptAll takes the lists of the other mods (adopted) into the tables, which hold the game's own
// lists and nothing else yet. Their skins keep the numbers they had here before, or get the ones
// they ask for when free, but never take a number one of this program's skins (mods) has, nor an
// unlock id this program ever gave out.
func adoptAll(j *Job, st *SkinTable, ut *UnlockTable, mods []InstalledSlotMod, adopted []AdoptedMod, old SkinListState) {
	read := make([]adoptedLists, len(adopted))
	for i := range adopted {
		read[i].st, read[i].ut = readAdopted(st.sc, &adopted[i])
		if !adopted[i].read {
			j.Log("%s: %s", adopted[i].Folder, adopted[i].Problem)
		}
	}
	adoptLists(j, st, ut, mods, adopted, read, old)
}

// adoptedLists are the skin list and unlock list of another mod (nil for one it does not have).
type adoptedLists struct {
	st *SkinTable
	ut *UnlockTable
}

// adoptLists is adoptAll with the other mods' lists read (read[i] are those of adopted[i]).
func adoptLists(j *Job, st *SkinTable, ut *UnlockTable, mods []InstalledSlotMod, adopted []AdoptedMod, read []adoptedLists, old SkinListState) {
	if len(adopted) == 0 {
		return
	}
	sc := st.sc
	ad := &adoption{game: gameListsOf(st, ut), reserved: map[int]map[int]bool{}, reservedIDs: map[int64]bool{}, ourKeys: map[string]bool{},
		kept: map[string]AdoptedAssign{}, held: map[int]map[int]string{}}
	for _, a := range old.AdoptedSkins {
		ad.kept[adoptedKey(a.Folder, a.Row)] = a
	}
	var present []AdoptedAssign // records of skins still in their mod's list
	for i := range adopted {
		am := &adopted[i]
		if !am.read || read[i].st == nil {
			continue
		}
		for _, r := range read[i].st.Rows {
			if a, ok := ad.kept[adoptedKey(am.Folder, r.Key)]; ok && ad.game.rows[r.Key] == nil {
				present = append(present, a)
			}
		}
	}
	// The numbers they had before stay theirs. When two had the same one (the mod of one was taken
	// out, and is back), it stays with the skin that was in the list when it was last built.
	wasIn := map[string]bool{}
	for _, f := range old.Adopted {
		wasIn[strings.ToLower(f)] = true
	}
	sort.SliceStable(present, func(i, j int) bool {
		return !wasIn[strings.ToLower(present[i].Folder)] && wasIn[strings.ToLower(present[j].Folder)]
	})
	for _, a := range present {
		if ad.held[a.Aircraft] == nil {
			ad.held[a.Aircraft] = map[int]string{}
		}
		ad.held[a.Aircraft][a.Number] = adoptedKey(a.Folder, a.Row)
	}
	installed := map[string]bool{}
	for _, m := range mods {
		ad.ourKeys[m.Manifest.Key] = true
		for _, sl := range m.Manifest.Slots {
			installed[m.Manifest.Key+"/"+sl.Key] = true
		}
	}
	live := map[string]bool{}
	for _, k := range old.Mods {
		live[k] = true
	}
	for _, a := range old.Assign {
		ad.reservedIDs[a.UnlockID] = true // ids ever given out are never given to another skin
		if a.StartID != 0 {
			ad.reservedIDs[a.StartID] = true
		}
		if a.UnlockID2 != 0 {
			ad.reservedIDs[a.UnlockID2] = true
		}
		k := a.Mod + "/" + a.Slot
		if !installed[k] {
			continue
		}
		// a skin of ours coming back (its mod was taken out) does not take back a number that a skin
		// from another mod's list has had since; it gets a free one (see assignNumbers)
		if !live[k] && ad.held[a.Aircraft][a.Number] != "" {
			continue
		}
		ad.reserve(a.Aircraft, a.Number)
	}
	for _, id := range old.Retired {
		ad.reservedIDs[id] = true
	}
	for i := range adopted {
		if adopted[i].read {
			adoptRows(j, sc, st, ut, &adopted[i], read[i].st, read[i].ut, ad)
		}
	}
}

// readAdopted reads another mod's skin list and unlock list (nil for one it does not have).
func readAdopted(sc *Schema, am *AdoptedMod) (*SkinTable, *UnlockTable) {
	var fst *SkinTable
	var fut *UnlockTable
	for _, u := range am.utocs {
		a, b, err := readZenSkinTables(u, sc)
		if err != nil {
			am.Problem = "its skin list could not be read (" + err.Error() + "), so its skins are not in the merged list"
			return nil, nil
		}
		if a != nil {
			fst = a
		}
		if b != nil {
			fut = b
		}
	}
	am.read = true
	return fst, fut
}

// adoptedRecords gives the records of other mods' skins to keep: those placed now, and those of
// mods whose lists are not in the merged list now (taken out, or not readable), for when they are back.
func adoptedRecords(old SkinListState, adopted []AdoptedMod) []AdoptedAssign {
	var out []AdoptedAssign
	now := map[string]bool{}
	for _, am := range adopted {
		if am.read {
			now[strings.ToLower(am.Folder)] = true
			out = append(out, am.assign...)
		}
	}
	for _, a := range old.AdoptedSkins {
		if !now[strings.ToLower(a.Folder)] {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if fi, fj := strings.ToLower(out[i].Folder), strings.ToLower(out[j].Folder); fi != fj {
			return fi < fj
		}
		return out[i].Row < out[j].Row
	})
	return out
}

// adoptRows copies another mod's changes (from its lists fst and fut, either may be nil) into the lists.
func adoptRows(j *Job, sc *Schema, st *SkinTable, ut *UnlockTable, am *AdoptedMod, fst *SkinTable, fut *UnlockTable, ad *adoption) {
	entries := ut.Entries()
	entryAt := map[int64]int{}
	for i, e := range entries {
		entryAt[e.Int("ID")] = i
	}
	taken := func(a, n int, self string) bool {
		if ad.reserved[a][n] {
			return true
		}
		if h := ad.held[a][n]; h != "" && h != self {
			return true
		}
		return st.Find(skinIDOf(a, n)) != nil
	}
	usedIDs := ut.UsedIDs()
	for id := range ad.reservedIDs {
		usedIDs[id] = true
	}
	inRange := func(id int64) bool { return id >= ut.MinID() && id <= ut.MaxID() }
	// unlock entries of the other mod by the skin they unlock
	unlockFor := map[int64]*Obj{}
	if fut != nil {
		for _, e := range fut.Entries() {
			id := e.Int("ID")
			if g, ok := ad.game.entries[id]; ok {
				if i, ok := entryAt[id]; ok && !sameObj(sc, e, g) {
					entries[i] = e.Clone()
					am.Unlocks++
				}
				continue
			}
			if sk := unlockedSkin(e); sk != 0 {
				unlockFor[sk] = e
			}
		}
	}
	type newRow struct {
		row    SkinRow
		wanted int
	}
	var added []newRow
	if fst != nil {
		for _, r := range fst.Rows {
			if g, ok := ad.game.rows[r.Key]; ok {
				if !sameObj(sc, r.Obj, g) {
					for i := range st.Rows {
						if st.Rows[i].Key == r.Key {
							st.Rows[i] = SkinRow{Key: r.Key, Obj: r.Obj.Clone()}
						}
					}
					am.Changed++
				}
				continue
			}
			id := r.SkinID()
			if id <= 0 {
				continue
			}
			if nk := r.Obj.Str("SkinNameTextID"); strings.HasPrefix(nk, "SS_") && ad.ourKeys[textKeyMod(nk)] {
				am.Skipped++ // one of this program's own skins, in the copy of the list its mod shares
				continue
			}
			added = append(added, newRow{SkinRow{r.Key, r.Obj.Clone()}, slotOf(id)})
		}
	}
	sort.SliceStable(added, func(i, j int) bool { return added[i].row.SkinID() < added[j].row.SkinID() })
	setEntries := func() {
		arr := make([]any, len(entries))
		for i := range entries {
			arr[i] = entries[i]
		}
		ut.Obj.Set(sc, "ConfigurationEntryList", arr)
	}
	setEntries()
	for _, nr := range added {
		oldID := nr.row.SkinID()
		a := aircraftOf(oldID)
		self := adoptedKey(am.Folder, nr.row.Key)
		rec, had := ad.kept[self]
		had = had && rec.Aircraft == a
		num := 0
		switch {
		case had && rec.Number > 0 && rec.Number <= lastSlotNumber && !taken(a, rec.Number, self):
			num = rec.Number // the number it had here before
		case nr.wanted > 0 && nr.wanted <= lastSlotNumber && !taken(a, nr.wanted, self):
			num = nr.wanted
		default:
			for n := firstSlotNumber; n <= lastSlotNumber; n++ {
				if !taken(a, n, self) {
					num = n
					break
				}
			}
		}
		if num == 0 {
			am.Problem = "an aircraft has no free skin numbers left for its skins"
			continue
		}
		ad.reserve(a, num)
		newID := skinIDOf(a, num)
		o := nr.row.Obj
		o.Set(sc, "SkinID", int32(newID))
		st.Insert(a, SkinRow{Key: skinRowKey(newID), Obj: o})
		am.NewSkins = append(am.NewSkins, AdoptedSkin{Plane: nr.row.Plane(), Aircraft: a, Wanted: nr.wanted, Number: num, NameKey: o.Str("SkinNameTextID"), Moved: num != nr.wanted})
		am.assign = append(am.assign, AdoptedAssign{Folder: am.Folder, Row: nr.row.Key, Aircraft: a, Number: num})
		// its unlock rule, pointed at the skin's number; a skin without one is unlocked straight away
		var e *Obj
		if src := unlockFor[oldID]; src != nil {
			e = src.Clone()
			av, _ := e.Get("ActionEntryList")
			for _, x := range av.([]any) {
				if ao, ok := x.(*Obj); ok && ao.Str("ActionType") == "UnlockSkin" {
					ao.Set(sc, "Parameters", strings.Replace(ao.Str("Parameters"), fmt.Sprintf("Id:%d", oldID), fmt.Sprintf("Id:%d", newID), 1))
				}
			}
		}
		uid := int64(0)
		switch {
		case had && rec.Number == num && rec.UnlockID != 0 && !usedIDs[rec.UnlockID] && inRange(rec.UnlockID):
			uid = rec.UnlockID // the id its rule had here before
		case e != nil && !usedIDs[e.Int("ID")] && inRange(e.Int("ID")):
			uid = e.Int("ID")
		default:
			for x := ut.lastLowID() + unlockRoom + 1; x <= ut.MaxID(); x++ {
				if !usedIDs[x] {
					uid = x
					break
				}
			}
		}
		if uid == 0 {
			continue
		}
		usedIDs[uid] = true
		ad.reservedIDs[uid] = true
		if e == nil {
			var err error
			if e, err = ut.NewUnlockEntry(uid, newID); err != nil {
				continue
			}
		} else {
			e.Set(sc, "ID", uint32(uid))
		}
		ut.Append(e)
		am.assign[len(am.assign)-1].UnlockID = uid
		am.Unlocks++
	}
	if len(am.NewSkins) > 0 || am.Changed > 0 || am.Unlocks > 0 {
		j.Log("%s: %d new skins, %d changed skins and %d unlock rules taken into the merged skin list.", am.Folder, len(am.NewSkins), am.Changed, am.Unlocks)
	}
}

// textKeyMod gives the mod key of one of this program's text keys: SS_<Name>_<abcde>_<slot>_Name.
func textKeyMod(k string) string {
	parts := strings.SplitN(strings.TrimPrefix(k, "SS_"), "_", 3)
	if len(parts) < 3 {
		return ""
	}
	return parts[0] + "_" + parts[1]
}
