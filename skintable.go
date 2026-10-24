package main

// The two game files that make a skin exist and be unlocked:
//   /Game/Datatables/Information/DT_Skin   one row per skin (LiveSkinDataTable)
//   /Game/Unlock/DA_Unlock_Skin            "when this happens, unlock that skin" entries
// Emblems have the same pair (DT_Emblem and DA_Unlock_Emblem, see emblems.go), read with the same
// code. All are read completely and written back; reading and writing an unchanged file must give
// the same bytes, which is checked every time, so a game update that changes the layout is caught
// before anything is built from it.

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	skinTablePath   = "/Game/Datatables/Information/DT_Skin"
	unlockTablePath = "/Game/Unlock/DA_Unlock_Skin"
	skinRowStruct   = "LiveSkinDataTable"
	unlockStruct    = "LiveConditionActionDataAsset"
)

type SkinRow struct {
	Key string
	Obj *Obj
}

func (r SkinRow) SkinID() int64    { return r.Obj.Int("SkinID") }
func (r SkinRow) Plane() string    { return r.Obj.Str("PlaneStringID") }
func (r SkinRow) Material() string { return r.Obj.Str("DecalMaterialRef") }

// skin numbers: SkinID = aircraft * 1 000 000 + skin * 1000 + 1
func skinIDOf(aircraft, slot int) int64 { return int64(aircraft)*1_000_000 + int64(slot)*1000 + 1 }
func aircraftOf(id int64) int           { return int(id / 1_000_000) }
func slotOf(id int64) int               { return int(id / 1000 % 1000) }
func skinRowKey(id int64) string        { return "I_EM_" + strconv.FormatInt(id, 10) }

// SkinTable is a DataTable: the skin list, or the emblem list (see emblems.go).
type SkinTable struct {
	pkg    *Package // nil when read from another mod's container
	names  []string
	nfe    int // names usable by export data
	sc     *Schema
	prefix []byte // the table object's own data before the row count
	Rows   []SkinRow
}

func readSkinTable(p *Package, sc *Schema) (*SkinTable, error) {
	return readTable(p, sc, skinRowStruct, "skin list")
}

// readTable reads a DataTable package whose rows are rowStruct; what names it in messages.
func readTable(p *Package, sc *Schema, rowStruct, what string) (*SkinTable, error) {
	if len(p.Exports) != 1 {
		return nil, fmt.Errorf("the %s has %d parts, expected 1", what, len(p.Exports))
	}
	t, err := readTableData(p.Names, p.ExportData(0), sc, rowStruct, what)
	if err != nil {
		return nil, err
	}
	t.pkg, t.nfe = p, p.NamesFromExportData
	return t, nil
}

// readSkinTableData reads the skin list from its export data and the package's names.
func readSkinTableData(names []string, data []byte, sc *Schema) (*SkinTable, error) {
	return readTableData(names, data, sc, skinRowStruct, "skin list")
}

func readTableData(names []string, data []byte, sc *Schema, rowStruct, what string) (*SkinTable, error) {
	// 4 bytes, the DataTable's own properties (RowStruct), then 4 more before the row count
	r := &PropReader{d: data, p: 4, names: names, sc: sc}
	var t *SkinTable
	err := func() (err error) {
		defer func() {
			if x := recover(); x != nil {
				err = fmt.Errorf("%v", x)
			}
		}()
		// the table's own header: one fragment, then RowStruct and the guid flag
		for {
			v := r.u16()
			if v&0x80 != 0 {
				return errors.New("unexpected DataTable header")
			}
			if v&0x100 != 0 {
				break
			}
		}
		r.i32() // RowStruct (an import)
		r.i32()
		t = &SkinTable{names: names, nfe: len(names), sc: sc, prefix: append([]byte(nil), data[:r.p]...)}
		n := int(r.i32())
		if n < 0 || n > 100000 {
			return fmt.Errorf("row count %d", n)
		}
		for i := 0; i < n; i++ {
			key := r.fname()
			o := r.object(rowStruct)
			t.Rows = append(t.Rows, SkinRow{key, o})
		}
		if r.p != len(data) {
			return fmt.Errorf("the rows end at %d of %d bytes", r.p, len(data))
		}
		return nil
	}()
	if err != nil {
		return nil, fmt.Errorf("the %s could not be read: %v", what, err)
	}
	// unchanged in, unchanged out
	_, out, err := t.encode()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(out, data) {
		return nil, fmt.Errorf("the %s could not be read exactly (the game may have been updated); set a newer mappings file in Settings", what)
	}
	return t, nil
}

func LoadSkinTable(base string, sc *Schema) (*SkinTable, error) {
	p, err := LoadPackage(base)
	if err != nil {
		return nil, err
	}
	if p.Name != skinTablePath {
		return nil, fmt.Errorf("%s is not the skin list", p.Name)
	}
	return readSkinTable(p, sc)
}

func (t *SkinTable) encode() (*NameTable, []byte, error) {
	nt := NewNameTable(t.names, t.nfe)
	w := &PropWriter{nt: nt, sc: t.sc}
	err := func() (err error) {
		defer func() {
			if x := recover(); x != nil {
				err = fmt.Errorf("%v", x)
			}
		}()
		w.out = append(w.out, t.prefix...)
		w.u32(uint32(len(t.Rows)))
		for _, row := range t.Rows {
			w.fname(row.Key)
			w.object(row.Obj)
		}
		return nil
	}()
	return nt, w.out, err
}

// Write gives the package files of the table as it is now.
func (t *SkinTable) Write() ([]byte, []byte, error) {
	if t.pkg == nil {
		return nil, nil, errors.New("this skin list cannot be written")
	}
	nt, data, err := t.encode()
	if err != nil {
		return nil, nil, err
	}
	return t.pkg.Write(PackageEdit{Names: nt.Names, AddedForExports: len(nt.Added), Exports: [][]byte{data}})
}

// Find returns the row with a skin id, or nil.
func (t *SkinTable) Find(id int64) *SkinRow {
	for i := range t.Rows {
		if t.Rows[i].SkinID() == id {
			return &t.Rows[i]
		}
	}
	return nil
}

// UsedSlots lists the skin numbers an aircraft already has.
func (t *SkinTable) UsedSlots(aircraft int) map[int]bool {
	out := map[int]bool{}
	for _, r := range t.Rows {
		if id := r.SkinID(); id > 0 && aircraftOf(id) == aircraft {
			out[slotOf(id)] = true
		}
	}
	return out
}

// Insert adds rows after the aircraft's last row (or at the end).
func (t *SkinTable) Insert(aircraft int, rows ...SkinRow) {
	at := len(t.Rows)
	for i, r := range t.Rows {
		if id := r.SkinID(); id > 0 && aircraftOf(id) == aircraft {
			at = i + 1
		}
	}
	t.Rows = append(t.Rows[:at], append(rows, t.Rows[at:]...)...)
}

// SortBase is the first sort number of an aircraft's skins (sort numbers are aircraft*1000 + n).
func (t *SkinTable) SortBase(aircraft int) int64 {
	var best int64 = -1
	for _, r := range t.Rows {
		if id := r.SkinID(); id > 0 && aircraftOf(id) == aircraft {
			s := r.Obj.Int("SortNumber")
			if b := s / 1000 * 1000; best < 0 || b < best {
				best = b
			}
		}
	}
	return best
}

// ------------------------------------------------------------------ unlocking

type UnlockTable struct {
	pkg    *Package // nil when read from another mod's container
	names  []string
	nfe    int
	sc     *Schema
	head   []byte // 4 bytes before the object
	tail   []byte // after it
	Obj    *Obj
	action string // what its entries do: UnlockSkin, or UnlockEmblem in the emblem unlock list
	cutoff int64  // new ids follow on from the highest id below this (see lastLowID)
}

func readUnlockTable(p *Package, sc *Schema) (*UnlockTable, error) {
	if len(p.Exports) != 1 {
		return nil, fmt.Errorf("the unlock list has %d parts, expected 1", len(p.Exports))
	}
	u, err := readUnlockTableData(p.Names, p.ExportData(0), sc)
	if err != nil {
		return nil, err
	}
	u.pkg, u.nfe = p, p.NamesFromExportData
	return u, nil
}

func readUnlockTableData(names []string, data []byte, sc *Schema) (*UnlockTable, error) {
	if len(data) < 8 {
		return nil, errors.New("the unlock list is empty")
	}
	o, end, err := ReadObject(sc, names, data, 4, unlockStruct)
	if err != nil {
		return nil, fmt.Errorf("the unlock list could not be read: %v", err)
	}
	u := &UnlockTable{names: names, nfe: len(names), sc: sc, head: append([]byte(nil), data[:4]...), tail: append([]byte(nil), data[end:]...), Obj: o,
		action: "UnlockSkin", cutoff: oldUnlockStart}
	_, out, err := u.encode()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(out, data) {
		return nil, errors.New("the unlock list could not be read exactly (the game may have been updated); set a newer mappings file in Settings")
	}
	return u, nil
}

func LoadUnlockTable(base string, sc *Schema) (*UnlockTable, error) {
	p, err := LoadPackage(base)
	if err != nil {
		return nil, err
	}
	if p.Name != unlockTablePath {
		return nil, fmt.Errorf("%s is not the skin unlock list", p.Name)
	}
	return readUnlockTable(p, sc)
}

func (u *UnlockTable) encode() (*NameTable, []byte, error) {
	nt := NewNameTable(u.names, u.nfe)
	body, err := WriteObject(u.sc, nt, u.Obj)
	if err != nil {
		return nil, nil, err
	}
	out := append(append(append([]byte(nil), u.head...), body...), u.tail...)
	return nt, out, nil
}

func (u *UnlockTable) Write() ([]byte, []byte, error) {
	if u.pkg == nil {
		return nil, nil, errors.New("this unlock list cannot be written")
	}
	nt, data, err := u.encode()
	if err != nil {
		return nil, nil, err
	}
	return u.pkg.Write(PackageEdit{Names: nt.Names, AddedForExports: len(nt.Added), Exports: [][]byte{data}})
}

func (u *UnlockTable) Entries() []*Obj {
	v, _ := u.Obj.Get("ConfigurationEntryList")
	arr, _ := v.([]any)
	out := make([]*Obj, 0, len(arr))
	for _, x := range arr {
		if o, ok := x.(*Obj); ok {
			out = append(out, o)
		}
	}
	return out
}

func (u *UnlockTable) MaxID() int64 { return u.Obj.Int("MaxId") }
func (u *UnlockTable) MinID() int64 { return u.Obj.Int("MinId") }

func (u *UnlockTable) UsedIDs() map[int64]bool {
	out := map[int64]bool{}
	for _, e := range u.Entries() {
		out[e.Int("ID")] = true
	}
	return out
}

func (u *UnlockTable) Append(entries ...*Obj) {
	v, _ := u.Obj.Get("ConfigurationEntryList")
	arr, _ := v.([]any)
	for _, e := range entries {
		arr = append(arr, e)
	}
	u.Obj.Set(u.sc, "ConfigurationEntryList", arr)
}

// unlockedSkin gives the skin id an entry unlocks (0 if none).
func unlockedSkin(e *Obj) int64 { return unlockedBy(e, "UnlockSkin") }

// unlockedBy gives the id an entry's action of the given type unlocks (0 if none).
func unlockedBy(e *Obj, action string) int64 {
	v, _ := e.Get("ActionEntryList")
	arr, _ := v.([]any)
	for _, a := range arr {
		ao, ok := a.(*Obj)
		if !ok || ao.Str("ActionType") != action {
			continue
		}
		for _, part := range strings.Split(ao.Str("Parameters"), ",") {
			if id, ok := strings.CutPrefix(strings.TrimSpace(part), "Id:"); ok {
				n, _ := strconv.ParseInt(id, 10, 64)
				return n
			}
		}
	}
	return 0
}

// templateEntry finds an entry that unlocks something after a mission, to copy for new ones.
func (u *UnlockTable) templateEntry() (*Obj, error) {
	for _, e := range u.Entries() {
		v, _ := e.Get("ConditionEntryList")
		conds, _ := v.([]any)
		if len(conds) != 1 || unlockedBy(e, u.action) == 0 {
			continue
		}
		if c, ok := conds[0].(*Obj); ok && c.Str("ConditionType") == "CompleteLevel" {
			return e, nil
		}
	}
	return nil, errors.New("the unlock list has no entry to model new ones on")
}

// oldUnlockStart is where versions 2.0.0 to 2.0.2 started numbering new unlock rules. Since 2.0.3 new
// rules follow on from the game's own, as the SkinSlotTest skins that the game did unlock (1400589
// and 1400590), with room left for rules a game update may add (unlockRoom ids). The game's own
// numbering has gaps too (1400259 and 1400548 are not used), so a gap does not matter to it.
const (
	oldUnlockStart = 1401000
	unlockRoom     = 100
)

// lastLowID is the highest unlock id in use below the table's cutoff (oldUnlockStart for skins): the
// game's last (1400588 for skins) or, when other mods have added rules after it, theirs.
func (u *UnlockTable) lastLowID() int64 {
	last := u.MinID()
	for id := range u.UsedIDs() {
		if id > last && id < u.cutoff {
			last = id
		}
	}
	return last
}

// The unlock rules a skin's skinstudio.json can name. Since 2.0.3 both mean the same: the game's own
// rule for skins 001 and 002 of every aircraft (CompleteLevel MissionId:AllBase,Mode:Campaign).
// "always" (ConditionType Always, which the game itself never uses) did not unlock anything in game.
const (
	UnlockAlways  = "always"
	UnlockMission = "mission"
)

// NewUnlockEntry makes an entry that unlocks skin (or emblem) id the way the game unlocks its own
// first skins and its emblems: when a campaign mission is finished.
func (u *UnlockTable) NewUnlockEntry(entryID int64, skinID int64) (*Obj, error) {
	tmpl, err := u.templateEntry()
	if err != nil {
		return nil, err
	}
	e := tmpl.Clone()
	if err := e.Set(u.sc, "ID", uint32(entryID)); err != nil {
		return nil, err
	}
	cv, _ := e.Get("ConditionEntryList")
	cond := cv.([]any)[0].(*Obj)
	cond.Set(u.sc, "ConditionType", "CompleteLevel")
	cond.Set(u.sc, "Parameters", "MissionId:AllBase,Mode:Campaign")
	av, _ := e.Get("ActionEntryList")
	act := av.([]any)[0].(*Obj)
	act.Set(u.sc, "ActionType", u.action)
	act.Set(u.sc, "Parameters", fmt.Sprintf("Id:%d,Mode:Campaign", skinID))
	return e, nil
}
