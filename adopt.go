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
}

const (
	skinListLabel   = "Datatables/Information/DT_Skin"
	unlockListLabel = "Unlock/DA_Unlock_Skin"
)

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
		has := false
		for _, r := range m.Replaces {
			if r == skinListLabel || r == unlockListLabel {
				has = true
			}
		}
		if !has {
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

// adoptRows copies another mod's changes into the lists. Numbers in reserved (and unlock ids in
// reservedIDs) are not given to its new skins; the numbers it ends up with are added to them.
func adoptRows(j *Job, sc *Schema, st *SkinTable, ut *UnlockTable, am *AdoptedMod, reserved map[int]map[int]bool, reservedIDs map[int64]bool, ourKeys map[string]bool) {
	var fst *SkinTable
	var fut *UnlockTable
	for _, u := range am.utocs {
		a, b, err := readZenSkinTables(u, sc)
		if err != nil {
			am.Problem = "its skin list could not be read (" + err.Error() + "), so its skins are not in the merged list"
			j.Log("%s: %s", am.Folder, am.Problem)
			return
		}
		if a != nil {
			fst = a
		}
		if b != nil {
			fut = b
		}
	}
	baseRows := map[string]int{}
	for i, r := range st.Rows {
		baseRows[r.Key] = i
	}
	baseEntries := map[int64]int{}
	entries := ut.Entries()
	for i, e := range entries {
		baseEntries[e.Int("ID")] = i
	}
	taken := func(a, n int) bool {
		if reserved[a] != nil && reserved[a][n] {
			return true
		}
		return st.Find(skinIDOf(a, n)) != nil
	}
	usedIDs := ut.UsedIDs()
	for id := range reservedIDs {
		usedIDs[id] = true
	}
	// unlock entries of the other mod by the skin they unlock
	unlockFor := map[int64]*Obj{}
	if fut != nil {
		for _, e := range fut.Entries() {
			id := e.Int("ID")
			if i, ok := baseEntries[id]; ok {
				if !sameObj(sc, e, entries[i]) {
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
			if i, ok := baseRows[r.Key]; ok {
				if !sameObj(sc, r.Obj, st.Rows[i].Obj) {
					st.Rows[i] = SkinRow{Key: r.Key, Obj: r.Obj.Clone()}
					am.Changed++
				}
				continue
			}
			id := r.SkinID()
			if id <= 0 {
				continue
			}
			if nk := r.Obj.Str("SkinNameTextID"); strings.HasPrefix(nk, "SS_") && ourKeys[textKeyMod(nk)] {
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
		num := nr.wanted
		if num <= 0 || num > lastSlotNumber || taken(a, num) {
			num = 0
			for n := firstSlotNumber; n <= lastSlotNumber; n++ {
				if !taken(a, n) {
					num = n
					break
				}
			}
		}
		if num == 0 {
			am.Problem = "an aircraft has no free skin numbers left for its skins"
			continue
		}
		if reserved[a] == nil {
			reserved[a] = map[int]bool{}
		}
		reserved[a][num] = true
		newID := skinIDOf(a, num)
		o := nr.row.Obj
		o.Set(sc, "SkinID", int32(newID))
		st.Insert(a, SkinRow{Key: skinRowKey(newID), Obj: o})
		am.NewSkins = append(am.NewSkins, AdoptedSkin{Plane: nr.row.Plane(), Aircraft: a, Wanted: nr.wanted, Number: num, NameKey: o.Str("SkinNameTextID"), Moved: num != nr.wanted})
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
		if e != nil && !usedIDs[e.Int("ID")] && e.Int("ID") >= ut.MinID() && e.Int("ID") <= ut.MaxID() {
			uid = e.Int("ID")
		} else {
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
		reservedIDs[uid] = true
		if e == nil {
			var err error
			if e, err = ut.NewUnlockEntry(uid, newID); err != nil {
				continue
			}
		} else {
			e.Set(sc, "ID", uint32(uid))
		}
		ut.Append(e)
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
