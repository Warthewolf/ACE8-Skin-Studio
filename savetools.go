package main

// Save tools: backups of the game's save, "Unlock now" (new skins and emblems unlocked without
// finishing a mission), "Prepare my save for the game without mods", and "Restore a backup"; and
// SkinStudioSaveBackup, the small UE4SS mod that backs the save up each time the game starts.
//
// Only Campaign.sav is ever changed. Before every change both saves (Campaign.sav, System.sav) are
// copied into a dated folder in SkinStudioSaveBackups, next to the game's SaveGames folder. The
// new save is read back before it replaces the old one, and it is written to a new file first and
// then put in place, so a failure leaves the old save as it was. The game must be closed: it writes
// its save when it closes, which would undo the change.

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	campaignSave     = "Campaign.sav"
	systemSave       = "System.sav"
	saveBackupFolder = "SkinStudioSaveBackups"
	autoBackupFolder = "auto"
	saveBackupMod    = "SkinStudioSaveBackup"
	saveBackupInfo   = "backup.json"
)

//go:embed embed/SkinStudioSaveBackup
var saveBackupModFS embed.FS

// saveGamesDir is the game's SaveGames folder: from Settings, or the usual place.
func (s Settings) saveGamesDir() string {
	if s.SaveDir != "" {
		return s.SaveDir
	}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		return filepath.Join(la, "BANDAI NAMCO Entertainment", "ACE COMBAT 8", "Saved", "SaveGames")
	}
	return platformSaveGamesDir()
}

// saveBackupsDir is next to SaveGames, so the game and Steam Cloud leave it alone.
func (s Settings) saveBackupsDir() string {
	d := s.saveGamesDir()
	if d == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(d), saveBackupFolder)
}

// gameRunning names the game's program when it is running ("" when not). Tests replace it.
var gameRunning = findRunningGame

func isGameExe(name string) bool {
	n := strings.ToLower(name)
	return strings.HasPrefix(n, "acecombat8") && strings.HasSuffix(n, ".exe")
}

func needGameClosed() error {
	if name := gameRunning(); name != "" {
		return fmt.Errorf("the game is running (%s). Close it first and try again: the game writes its save when it closes, which would undo the change", name)
	}
	return nil
}

func (s Settings) campaignPath() (string, error) {
	d := s.saveGamesDir()
	if d == "" {
		return "", errors.New("the game's save folder was not found; set it in Settings")
	}
	p := filepath.Join(d, campaignSave)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("there is no %s in %s. Has the game been played on this computer? If the save is somewhere else, set its folder in Settings", campaignSave, d)
	}
	return p, nil
}

// ------------------------------------------------------------------ backups

type saveBackup struct {
	ID     string    `json:"id"` // folder name, or auto/07 for the backups made at game start
	When   time.Time `json:"when"`
	Reason string    `json:"reason"`
	Auto   bool      `json:"auto"`
	Files  []string  `json:"files"` // Campaign.sav, System.sav
	Bytes  int64     `json:"bytes"`
}

type backupInfo struct {
	When    time.Time `json:"when"`
	Reason  string    `json:"reason"`
	Program string    `json:"program"`
}

// makeSaveBackup copies both saves into a new dated folder.
func makeSaveBackup(s Settings, reason string) (saveBackup, error) {
	from := s.saveGamesDir()
	root := s.saveBackupsDir()
	if from == "" || root == "" {
		return saveBackup{}, errors.New("the game's save folder was not found; set it in Settings")
	}
	now := time.Now()
	name := now.Format("2006-01-02 15-04-05")
	dir := filepath.Join(root, name)
	for i := 2; ; i++ {
		if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s (%d)", now.Format("2006-01-02 15-04-05"), i)
		dir = filepath.Join(root, name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return saveBackup{}, fmt.Errorf("the backup folder could not be made (%v)", err)
	}
	b := saveBackup{ID: name, When: now, Reason: reason}
	for _, f := range []string{campaignSave, systemSave} {
		data, err := os.ReadFile(filepath.Join(from, f))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			os.RemoveAll(dir)
			return saveBackup{}, fmt.Errorf("%s could not be read for the backup (%v)", f, err)
		}
		if err := os.WriteFile(filepath.Join(dir, f), data, 0o644); err != nil {
			os.RemoveAll(dir)
			return saveBackup{}, fmt.Errorf("the backup could not be written (%v)", err)
		}
		if back, err := os.ReadFile(filepath.Join(dir, f)); err != nil || !bytes.Equal(back, data) {
			os.RemoveAll(dir)
			return saveBackup{}, errors.New("the backup did not read back the same; nothing was changed")
		}
		b.Files = append(b.Files, f)
		b.Bytes += int64(len(data))
	}
	if len(b.Files) == 0 {
		os.RemoveAll(dir)
		return saveBackup{}, fmt.Errorf("there is no save in %s to back up", from)
	}
	info, _ := json.MarshalIndent(backupInfo{When: now, Reason: reason, Program: appName + " " + appVersion}, "", "  ")
	os.WriteFile(filepath.Join(dir, saveBackupInfo), info, 0o644)
	return b, nil
}

var reAutoBackup = regexp.MustCompile(`^(\d\d) (Campaign|System)\.sav$`)

// listSaveBackups lists the backups, newest first.
func listSaveBackups(s Settings) []saveBackup {
	out := []saveBackup{}
	root := s.saveBackupsDir()
	if root == "" {
		return out
	}
	es, _ := os.ReadDir(root)
	for _, e := range es {
		if !e.IsDir() || e.Name() == autoBackupFolder {
			continue
		}
		dir := filepath.Join(root, e.Name())
		b := saveBackup{ID: e.Name()}
		for _, f := range []string{campaignSave, systemSave} {
			if st, err := os.Stat(filepath.Join(dir, f)); err == nil {
				b.Files = append(b.Files, f)
				b.Bytes += st.Size()
				if b.When.IsZero() {
					b.When = st.ModTime()
				}
			}
		}
		if len(b.Files) == 0 {
			continue
		}
		var info backupInfo
		if raw, err := os.ReadFile(filepath.Join(dir, saveBackupInfo)); err == nil && json.Unmarshal(raw, &info) == nil {
			b.Reason = info.Reason
			if !info.When.IsZero() {
				b.When = info.When
			}
		}
		out = append(out, b)
	}
	// the backups the game-start mod makes: "07 Campaign.sav", "07 System.sav", "07.txt"
	auto := filepath.Join(root, autoBackupFolder)
	es, _ = os.ReadDir(auto)
	slots := map[string]*saveBackup{}
	for _, e := range es {
		m := reAutoBackup.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		st, err := os.Stat(filepath.Join(auto, e.Name()))
		if err != nil {
			continue
		}
		b := slots[m[1]]
		if b == nil {
			b = &saveBackup{ID: autoBackupFolder + "/" + m[1], Auto: true, Reason: "when the game started", When: st.ModTime()}
			if raw, err := os.ReadFile(filepath.Join(auto, m[1]+".txt")); err == nil {
				if t, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(string(raw)), time.Local); err == nil {
					b.When = t
				}
			}
			slots[m[1]] = b
		}
		b.Files = append(b.Files, m[2]+".sav")
		b.Bytes += st.Size()
	}
	for _, b := range slots {
		sort.Strings(b.Files)
		out = append(out, *b)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].When.After(out[j].When) })
	return out
}

// backupPath is where one file of a backup is.
func (s Settings) backupPath(id, file string) (string, error) {
	root := s.saveBackupsDir()
	if root == "" {
		return "", errors.New("the game's save folder was not found; set it in Settings")
	}
	if strings.HasPrefix(id, autoBackupFolder+"/") {
		n := strings.TrimPrefix(id, autoBackupFolder+"/")
		if !regexp.MustCompile(`^\d\d$`).MatchString(n) {
			return "", errors.New("unknown backup")
		}
		return filepath.Join(root, autoBackupFolder, n+" "+file), nil
	}
	if id == "" || id != filepath.Base(id) || strings.ContainsAny(id, `/\`) || id == "." || id == ".." {
		return "", errors.New("unknown backup")
	}
	return filepath.Join(root, id, file), nil
}

// replaceFile writes data next to path and then puts it in path's place.
func replaceFile(path string, data []byte) error {
	tmp := path + ".skinstudio-new"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("the new save could not be written (%v)", err)
	}
	if back, err := os.ReadFile(tmp); err != nil || !bytes.Equal(back, data) {
		os.Remove(tmp)
		return errors.New("the new save did not read back the same; the save was not changed")
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("the save could not be replaced (%v); it was not changed", err)
	}
	return nil
}

// ------------------------------------------------------------------ changing the save

// saveChange is what one of the tools did.
type saveChange struct {
	Changed bool       `json:"changed"`
	Backup  saveBackup `json:"backup"`
	Lines   []string   `json:"lines"` // what changed, for people
	Cloud   bool       `json:"cloud"` // Steam Cloud keeps this save
}

// changeSave reads Campaign.sav, lets change make its changes (it returns what it did, nothing
// when there is nothing to do), and writes the save back after backing both saves up.
func changeSave(j *Job, s Settings, reason string, change func(f *saveFile) ([]string, error), check func(f *saveFile) error) (*saveChange, error) {
	if err := needGameClosed(); err != nil {
		return nil, err
	}
	path, err := s.campaignPath()
	if err != nil {
		return nil, err
	}
	before, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("the save could not be read (%v)", err)
	}
	j.SetStep("Reading the save", 0.2)
	f, err := parseSave(raw)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(f.bytes(), raw) {
		return nil, errors.New("this save could not be read exactly, so it is left alone (the game may have changed how it saves)")
	}
	lines, err := change(f)
	if err != nil {
		return nil, err
	}
	res := &saveChange{Lines: lines, Cloud: hasSteamCloud(s)}
	if len(lines) == 0 {
		return res, nil
	}
	out := f.bytes()
	j.SetStep("Checking the new save", 0.5)
	g, err := parseSave(out)
	if err != nil {
		return nil, fmt.Errorf("the changed save did not read back (%v); the save was not changed", err)
	}
	if check != nil {
		if err := check(g); err != nil {
			return nil, fmt.Errorf("the changed save is not right (%v); the save was not changed", err)
		}
	}
	j.SetStep("Backing up the save", 0.7)
	b, err := makeSaveBackup(s, reason)
	if err != nil {
		return nil, err
	}
	res.Backup = b
	j.Log("Backed up the save to %s.", filepath.Join(s.saveBackupsDir(), b.ID))
	if err := needGameClosed(); err != nil {
		return nil, err
	}
	if now, err := os.Stat(path); err != nil || !now.ModTime().Equal(before.ModTime()) || now.Size() != before.Size() {
		return nil, errors.New("the save changed while this was working (was the game started?); it was not changed. Try again")
	}
	j.SetStep("Writing the save", 0.85)
	if err := replaceFile(path, out); err != nil {
		return nil, err
	}
	res.Changed = true
	for _, l := range lines {
		j.Log("%s", l)
	}
	return res, nil
}

// countOf writes "1 new skin", "2 new skins" ("aircraft" stays as it is).
func countOf(n int, what string) string {
	if n != 1 {
		first, rest, _ := strings.Cut(what, " that ")
		if !strings.HasSuffix(first, "aircraft") {
			first += "s"
		}
		if rest != "" {
			return fmt.Sprintf("%d %s that %s", n, first, rest)
		}
		return fmt.Sprintf("%d %s", n, first)
	}
	return "1 " + what
}

func hasSteamCloud(s Settings) bool {
	_, err := os.Stat(filepath.Join(s.saveGamesDir(), "steam_autocloud.vdf"))
	return err == nil
}

// The unlock lists in the save's common data.
const (
	saveSkinList       = "UnlockedSkinIdList"
	saveNewSkinList    = "NewlyUnlockedSkinIdList"
	saveEmblemList     = "UnlockedEmblemIdList"
	saveNewEmblemList  = "NewlyUnlockedEmblemIdList"
	saveUnlockRules    = "UnlockData"
	actionUnlockSkin   = "ELiveCAActionType::UnlockSkin"
	actionUnlockEmblem = "ELiveCAActionType::UnlockEmblem"
)

func saveIDs(f *saveFile, list string) ([]uint32, *gvProp, error) {
	p := f.prop(list)
	if p == nil {
		return nil, nil, fmt.Errorf("the save has no %s", list)
	}
	v, err := p.uint32s()
	return v, p, err
}

// isModSkin and isModEmblem tell the ids this program gives new skins and emblems (numbers 601 to
// 999) from the game's, unless the game's own list has the id (a game update could use them).
func isModSkin(id int64, game map[int64]bool) bool {
	return id > 0 && slotOf(id) >= firstSlotNumber && !game[id]
}

func isModEmblem(id int64, game map[int64]bool) bool {
	return id >= 1_000_000_000 && int(id/1_000_000%1000) >= firstEmblemNumber && !game[id]
}

// saveItem is one new skin or emblem, and how the save has it.
type saveItem struct {
	Kind     string `json:"kind"` // skin or emblem
	Mod      string `json:"mod"`
	ModTitle string `json:"modTitle"`
	Key      string `json:"key"`
	Name     string `json:"name"`
	Plane    string `json:"plane,omitempty"`
	Number   int    `json:"number"`
	ID       int64  `json:"id"`
	UnlockID int64  `json:"unlockId"`
	Unlocked bool   `json:"unlocked"`
	Worn     int    `json:"worn"` // aircraft (and pilots) wearing it
}

// installedSaveItems lists the new skins and emblems that are in the game's lists now: in the
// skin list mod as it was last built, and still installed.
func installedSaveItems(s Settings) []saveItem {
	out := []saveItem{}
	if s.ModsDir == "" {
		return out
	}
	if _, err := os.Stat(filepath.Join(s.ModsDir, skinListMod)); err != nil {
		return out
	}
	st := loadListState(s)
	built := map[string]bool{}
	for _, k := range st.Mods {
		built[k] = true
	}
	mods, _ := scanSlotMods(s.ModsDir)
	slots := map[string]*SlotDef{}
	emblems := map[string]*EmblemDef{}
	titles := map[string]string{}
	for _, m := range mods {
		titles[m.Manifest.Key] = m.Manifest.Title
		for i := range m.Manifest.Slots {
			slots[m.Manifest.Key+"/"+m.Manifest.Slots[i].Key] = &m.Manifest.Slots[i]
		}
		for i := range m.Manifest.Emblems {
			emblems[m.Manifest.Key+"/"+m.Manifest.Emblems[i].Key] = &m.Manifest.Emblems[i]
		}
	}
	for _, a := range st.Assign {
		k := a.Mod + "/" + a.Slot
		sl := slots[k]
		if !built[k] || sl == nil || a.Number == 0 {
			continue
		}
		out = append(out, saveItem{Kind: "skin", Mod: a.Mod, ModTitle: titles[a.Mod], Key: a.Slot, Name: sl.Name, Plane: sl.Plane,
			Number: a.Number, ID: skinIDOf(a.Aircraft, a.Number), UnlockID: a.UnlockID})
	}
	for _, a := range st.Emblems {
		k := a.Mod + "/" + a.Emblem
		e := emblems[k]
		if !built[k] || e == nil || a.Number == 0 {
			continue
		}
		out = append(out, saveItem{Kind: "emblem", Mod: a.Mod, ModTitle: titles[a.Mod], Key: a.Emblem, Name: e.Name,
			Number: a.Number, ID: emblemIDOf(a.Number), UnlockID: a.UnlockID})
	}
	return out
}

// unlockRuleTemplate is a rule the game has marked done, of the given kind, to copy.
func unlockRuleTemplate(rules *gvProp, action string) *gvList {
	for _, e := range rules.Elems {
		if e.find("ActionType").text() == action && e.find("bIsActivated").boolVal() {
			if _, ok := e.find("ID").int32Val(); ok {
				return e
			}
		}
	}
	return nil
}

// unlockItems puts the items into the save's unlock lists (and "new" lists, so the game marks
// them new), and marks their unlock rules done so the game does not unlock them a second time.
func unlockItems(f *saveFile, items []saveItem) ([]string, error) {
	var lines []string
	add := func(kind, list, newList string) error {
		have, p, err := saveIDs(f, list)
		if err != nil {
			return err
		}
		fresh, np, err := saveIDs(f, newList)
		if err != nil {
			return err
		}
		in := map[uint32]bool{}
		for _, id := range have {
			in[id] = true
		}
		inNew := map[uint32]bool{}
		for _, id := range fresh {
			inNew[id] = true
		}
		for _, it := range items {
			id := uint32(it.ID)
			if it.Kind != kind || in[id] {
				continue
			}
			have = append(have, id)
			in[id] = true
			if !inNew[id] {
				fresh = append(fresh, id)
				inNew[id] = true
			}
			lines = append(lines, fmt.Sprintf("Unlocked the %s %s (%s).", kind, it.Name, it.ModTitle))
		}
		p.setUint32s(have)
		np.setUint32s(fresh)
		return nil
	}
	if err := add("skin", saveSkinList, saveNewSkinList); err != nil {
		return nil, err
	}
	if err := add("emblem", saveEmblemList, saveNewEmblemList); err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, nil
	}
	if rules := f.prop(saveUnlockRules); rules != nil && rules.Elems != nil {
		done := map[uint32]bool{}
		for _, e := range rules.Elems {
			if v, ok := e.find("ID").int32Val(); ok {
				done[uint32(v)] = true
			}
		}
		for _, it := range items {
			action := map[string]string{"skin": actionUnlockSkin, "emblem": actionUnlockEmblem}[it.Kind]
			if it.UnlockID <= 0 || done[uint32(it.UnlockID)] {
				continue
			}
			tmpl := unlockRuleTemplate(rules, action)
			if tmpl == nil {
				continue // the game marks it done itself after the next mission
			}
			e := tmpl.clone()
			e.find("ID").setInt32(int32(it.UnlockID))
			rules.Elems = append(rules.Elems, e)
			done[uint32(it.UnlockID)] = true
		}
	}
	return lines, nil
}

// gameIDs is what the game itself has, from its own lists; nil maps when they could not be read.
type gameIDs struct {
	Skins, Emblems, SkinRules, EmblemRules map[int64]bool
}

func readGameIDs(j *Job, s Settings) gameIDs {
	var g gameIDs
	if st, ut, err := loadBaseTables(j, s); err == nil {
		g.Skins, g.SkinRules = map[int64]bool{}, ut.UsedIDs()
		for _, r := range st.Rows {
			g.Skins[r.SkinID()] = true
		}
	} else {
		j.Log("The game's skin list could not be read (%v); new skins are told apart by their numbers only.", err)
	}
	if et, eu, err := loadEmblemTables(j, s); err == nil {
		g.Emblems, g.EmblemRules = map[int64]bool{}, eu.UsedIDs()
		for _, r := range et.Rows {
			g.Emblems[r.Obj.Int("EmblemID")] = true
		}
	} else {
		j.Log("The game's emblem list could not be read (%v); new emblems are told apart by their numbers only.", err)
	}
	return g
}

// ourRuleIDs are the unlock rule ids this program has ever given out.
func ourRuleIDs(st SkinListState) map[int64]bool {
	out := map[int64]bool{}
	for _, a := range st.Assign {
		out[a.UnlockID], out[a.UnlockID2] = true, true
	}
	for _, id := range st.Retired {
		out[id] = true
	}
	for _, a := range st.Emblems {
		out[a.UnlockID] = true
	}
	for _, id := range st.EmblemRetired {
		out[id] = true
	}
	for _, id := range experimentIDs {
		out[id] = true
	}
	delete(out, 0)
	return out
}

// vanillaSave takes the new skins and emblems out of the save, so it is like a save the game
// without mods would make: out of the unlock lists, off every aircraft wearing them (a skin goes
// back to the aircraft's first skin, an emblem to none), and their unlock rules forgotten (the
// game unlocks them again when the mods are back and a mission is finished, or with Unlock now).
func vanillaSave(f *saveFile, g gameIDs, ours map[int64]bool) ([]string, error) {
	var lines []string
	skins, _, err := saveIDs(f, saveSkinList)
	if err != nil {
		return nil, err
	}
	unlocked := map[int64]bool{}
	for _, id := range skins {
		unlocked[int64(id)] = true
	}
	drop := func(list string, mod func(int64) bool) (int, error) {
		v, p, err := saveIDs(f, list)
		if err != nil {
			return 0, err
		}
		keep := v[:0:0]
		for _, id := range v {
			if !mod(int64(id)) {
				keep = append(keep, id)
			}
		}
		if len(keep) != len(v) {
			p.setUint32s(keep)
		}
		return len(v) - len(keep), nil
	}
	modSkin := func(id int64) bool { return isModSkin(id, g.Skins) }
	modEmblem := func(id int64) bool { return isModEmblem(id, g.Emblems) }
	n, err := drop(saveSkinList, modSkin)
	if err != nil {
		return nil, err
	}
	if _, err := drop(saveNewSkinList, modSkin); err != nil {
		return nil, err
	}
	if n > 0 {
		lines = append(lines, "Took "+countOf(n, "new skin")+" out of the unlocked skins.")
	}
	if n, err = drop(saveEmblemList, modEmblem); err != nil {
		return nil, err
	}
	if _, err := drop(saveNewEmblemList, modEmblem); err != nil {
		return nil, err
	}
	if n > 0 {
		lines = append(lines, "Took "+countOf(n, "new emblem")+" out of the unlocked emblems.")
	}
	wornSkins, wornEmblems := 0, 0
	f.walk(func(p *gvProp) {
		switch p.Name {
		case "SkinID":
			v, ok := p.int32Val()
			if !ok || !modSkin(int64(v)) {
				return
			}
			first := skinIDOf(aircraftOf(int64(v)), 1)
			if unlocked[first] && (g.Skins == nil || g.Skins[first]) {
				p.setInt32(int32(first))
			} else {
				p.setInt32(-1)
			}
			wornSkins++
		case "EmblemIDs":
			v, err := p.uint32s()
			if err != nil {
				return
			}
			changed := false
			for i, id := range v {
				if modEmblem(int64(int32(id))) {
					v[i] = 0
					changed = true
					wornEmblems++
				}
			}
			if changed {
				p.setUint32s(v)
			}
		}
	})
	if wornSkins > 0 {
		lines = append(lines, fmt.Sprintf("Put %s back on the game's own skin.", countOf(wornSkins, "aircraft that wore a new skin")))
	}
	if wornEmblems > 0 {
		lines = append(lines, "Took "+countOf(wornEmblems, "new emblem")+" off aircraft.")
	}
	if rules := f.prop(saveUnlockRules); rules != nil && rules.Elems != nil {
		keep := rules.Elems[:0:0]
		for _, e := range rules.Elems {
			v, ok := e.find("ID").int32Val()
			id := int64(uint32(v))
			gone := false
			switch e.find("ActionType").text() {
			case actionUnlockSkin:
				gone = ok && (ours[id] || (g.SkinRules != nil && !g.SkinRules[id]))
			case actionUnlockEmblem:
				gone = ok && (ours[id] || (g.EmblemRules != nil && !g.EmblemRules[id]))
			}
			if !gone {
				keep = append(keep, e)
			}
		}
		if d := len(rules.Elems) - len(keep); d > 0 {
			rules.Elems = keep
			lines = append(lines, "Forgot "+countOf(d, "unlock rule")+" the game does not have.")
		}
	}
	return lines, nil
}

// ------------------------------------------------------------------ the tools

func doUnlockNow(j *Job, s Settings) (*saveChange, error) {
	items := installedSaveItems(s)
	if len(items) == 0 {
		return nil, errors.New("there are no new skins or emblems in the game's lists yet. Build one first (or, if they are installed, press Rebuild the skin list on the Skin slots page)")
	}
	return changeSave(j, s, "before Unlock now", func(f *saveFile) ([]string, error) {
		return unlockItems(f, items)
	}, func(g *saveFile) error {
		for _, kind := range []struct{ kind, list string }{{"skin", saveSkinList}, {"emblem", saveEmblemList}} {
			v, _, err := saveIDs(g, kind.list)
			if err != nil {
				return err
			}
			in := map[int64]bool{}
			for _, id := range v {
				in[int64(id)] = true
			}
			for _, it := range items {
				if it.Kind == kind.kind && !in[it.ID] {
					return fmt.Errorf("%s is not in the new save", it.Name)
				}
			}
		}
		return nil
	})
}

func doPrepareVanilla(j *Job, s Settings) (*saveChange, error) {
	j.SetStep("Reading the game's lists", 0.05)
	g := readGameIDs(j, s)
	ours := ourRuleIDs(loadListState(s))
	return changeSave(j, s, "before Prepare my save for the game without mods", func(f *saveFile) ([]string, error) {
		return vanillaSave(f, g, ours)
	}, func(f *saveFile) error {
		for _, l := range []string{saveSkinList, saveNewSkinList} {
			v, _, err := saveIDs(f, l)
			if err != nil {
				return err
			}
			for _, id := range v {
				if isModSkin(int64(id), g.Skins) {
					return fmt.Errorf("skin %d is still in %s", id, l)
				}
			}
		}
		return nil
	})
}

func doRestoreBackup(j *Job, s Settings, id string, withSystem bool) (*saveChange, error) {
	if err := needGameClosed(); err != nil {
		return nil, err
	}
	dir := s.saveGamesDir()
	if dir == "" {
		return nil, errors.New("the game's save folder was not found; set it in Settings")
	}
	var bk *saveBackup
	for _, b := range listSaveBackups(s) {
		if b.ID == id {
			b := b
			bk = &b
		}
	}
	if bk == nil {
		return nil, errors.New("that backup is not there any more")
	}
	files := []string{campaignSave}
	if withSystem {
		files = append(files, systemSave)
	}
	data := map[string][]byte{}
	for _, f := range files {
		p, err := s.backupPath(id, f)
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) && f == systemSave {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("the backup's %s could not be read (%v)", f, err)
		}
		g, err := parseSave(b)
		if err != nil {
			return nil, fmt.Errorf("the backup's %s is damaged (%v); nothing was changed", f, err)
		}
		if want := strings.TrimSuffix(f, ".sav"); !strings.Contains(g.Class, want) {
			return nil, fmt.Errorf("the backup's %s is not a %s save; nothing was changed", f, strings.ToLower(want))
		}
		data[f] = b
	}
	j.SetStep("Backing up the save as it is now", 0.3)
	res := &saveChange{Cloud: hasSteamCloud(s)}
	if _, err := os.Stat(filepath.Join(dir, campaignSave)); err == nil {
		b, err := makeSaveBackup(s, "before putting back the backup of "+bk.When.Format("2 Jan 2006 15:04"))
		if err != nil {
			return nil, err
		}
		res.Backup = b
	}
	if err := needGameClosed(); err != nil {
		return nil, err
	}
	j.SetStep("Putting the backup back", 0.7)
	for _, f := range files {
		if b, ok := data[f]; ok {
			if err := replaceFile(filepath.Join(dir, f), b); err != nil {
				return nil, err
			}
			res.Lines = append(res.Lines, fmt.Sprintf("Put back %s from %s.", f, bk.When.Format("2 Jan 2006 15:04")))
		}
	}
	res.Changed = true
	for _, l := range res.Lines {
		j.Log("%s", l)
	}
	return res, nil
}

// ------------------------------------------------------------------ what the save has

type saveStatusView struct {
	Dir         string       `json:"dir"`
	Found       bool         `json:"found"`
	Modified    time.Time    `json:"modified"`
	Problem     string       `json:"problem,omitempty"`
	GameRunning string       `json:"gameRunning,omitempty"`
	Items       []saveItem   `json:"items"`
	Leftover    int          `json:"leftover"` // new skins and emblems unlocked in the save that are not installed
	Worn        int          `json:"worn"`     // aircraft wearing a new skin or emblem
	Backups     []saveBackup `json:"backups"`
	BackupsDir  string       `json:"backupsDir"`
	Cloud       bool         `json:"cloud"`
	AutoBackup  autoModView  `json:"autoBackup"`
}

func saveStatus(s Settings) saveStatusView {
	v := saveStatusView{Dir: s.saveGamesDir(), BackupsDir: s.saveBackupsDir(), Items: installedSaveItems(s), Backups: listSaveBackups(s),
		GameRunning: gameRunning(), Cloud: hasSteamCloud(s), AutoBackup: saveBackupModStatus(s)}
	if v.AutoBackup.Installed && v.BackupsDir != "" {
		os.MkdirAll(filepath.Join(v.BackupsDir, autoBackupFolder), 0o755) // the game-start backup cannot make it itself
	}
	path, err := s.campaignPath()
	if err != nil {
		v.Problem = err.Error()
		return v
	}
	st, _ := os.Stat(path)
	v.Found, v.Modified = true, st.ModTime()
	raw, err := os.ReadFile(path)
	if err != nil {
		v.Problem = "The save could not be read: " + err.Error()
		return v
	}
	f, err := parseSave(raw)
	if err != nil {
		v.Problem = "The save could not be read: " + err.Error()
		return v
	}
	installed := map[int64]int{}
	for i, it := range v.Items {
		installed[it.ID] = i
	}
	for _, l := range [][2]string{{saveSkinList, "skin"}, {saveEmblemList, "emblem"}} {
		ids, _, _ := saveIDs(f, l[0])
		for _, id := range ids {
			x := int64(id)
			if i, ok := installed[x]; ok && v.Items[i].Kind == l[1] {
				v.Items[i].Unlocked = true
			} else if (l[1] == "skin" && isModSkin(x, nil)) || (l[1] == "emblem" && isModEmblem(x, nil)) {
				v.Leftover++
			}
		}
	}
	f.walk(func(p *gvProp) {
		switch p.Name {
		case "SkinID":
			if x, ok := p.int32Val(); ok && isModSkin(int64(x), nil) {
				v.Worn++
				if i, ok := installed[int64(x)]; ok {
					v.Items[i].Worn++
				}
			}
		case "EmblemIDs":
			ids, _ := p.uint32s()
			for _, id := range ids {
				if x := int64(id); isModEmblem(x, nil) {
					v.Worn++
					if i, ok := installed[x]; ok {
						v.Items[i].Worn++
					}
				}
			}
		}
	})
	return v
}

// ------------------------------------------------------------------ SkinStudioSaveBackup

type autoModView struct {
	Dir       string `json:"dir"`       // UE4SS's Mods folder
	Installed bool   `json:"installed"` // the mod is there
	Current   bool   `json:"current"`   // and is this program's version
	Off       bool   `json:"off"`       // the person took it out, so it is not put back by itself
	Problem   string `json:"problem,omitempty"`
}

func saveBackupModFiles() map[string][]byte {
	out := map[string][]byte{}
	fs.WalkDir(saveBackupModFS, "embed/"+saveBackupMod, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := saveBackupModFS.ReadFile(p)
			out[strings.TrimPrefix(p, "embed/"+saveBackupMod+"/")] = b
		}
		return nil
	})
	return out
}

func saveBackupModStatus(s Settings) autoModView {
	v := autoModView{Dir: s.ue4ssModsDir(), Off: s.SaveBackupOff}
	if v.Dir == "" {
		v.Problem = "UE4SS was not found, so the save cannot be backed up when the game starts (Skin Studio still backs it up before every change it makes)."
		return v
	}
	_, err := os.Stat(filepath.Join(v.Dir, saveBackupMod, "Scripts", "main.lua"))
	v.Installed = err == nil
	v.Current = v.Installed
	for name, want := range saveBackupModFiles() {
		if have, err := os.ReadFile(filepath.Join(v.Dir, saveBackupMod, filepath.FromSlash(name))); err != nil || !bytes.Equal(have, want) {
			v.Current = false
		}
	}
	return v
}

// installSaveBackupMod puts SkinStudioSaveBackup into UE4SS's Mods folder and makes the folder it
// writes its backups to (it cannot make folders itself).
func installSaveBackupMod(s Settings) error {
	dir := s.ue4ssModsDir()
	if dir == "" {
		return errors.New("UE4SS's Mods folder was not found; set it in Settings")
	}
	root := s.saveBackupsDir()
	if root == "" {
		return errors.New("the game's save folder was not found; set it in Settings")
	}
	if err := os.MkdirAll(filepath.Join(root, autoBackupFolder), 0o755); err != nil {
		return fmt.Errorf("the backup folder could not be made (%v)", err)
	}
	dest := filepath.Join(dir, saveBackupMod)
	for name, b := range saveBackupModFiles() {
		target := filepath.Join(dest, filepath.FromSlash(name))
		if old, err := os.ReadFile(target); err == nil && bytes.Equal(old, b) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, b, 0o644); err != nil {
			return fmt.Errorf("could not write %s (%v). If the game is running, close it and try again", filepath.Base(target), err)
		}
	}
	// paths.txt tells it where the save is when that is not the usual place
	paths := filepath.Join(dest, "Scripts", "paths.txt")
	if s.SaveDir != "" {
		txt := s.saveGamesDir() + "\r\n" + filepath.Join(root, autoBackupFolder) + "\r\n"
		if err := os.WriteFile(paths, []byte(txt), 0o644); err != nil {
			return err
		}
	} else {
		os.Remove(paths)
	}
	return nil
}

func removeSaveBackupMod(s Settings) error {
	dir := s.ue4ssModsDir()
	if dir == "" {
		return nil
	}
	if err := os.RemoveAll(filepath.Join(dir, saveBackupMod)); err != nil {
		return fmt.Errorf("SkinStudioSaveBackup could not be taken out (%v). If the game is running, close it and try again", err)
	}
	return nil
}

// keepSaveBackupMod installs or updates the game-start backup with SkinSlotText, unless the
// person took it out.
func keepSaveBackupMod(s Settings) {
	if s.SaveBackupOff || s.ue4ssModsDir() == "" || s.saveBackupsDir() == "" {
		return
	}
	if st := saveBackupModStatus(s); st.Current {
		return
	}
	if err := installSaveBackupMod(s); err != nil {
		logf("SkinStudioSaveBackup could not be installed: %v", err)
	}
}

// ------------------------------------------------------------------ requests

func saveRoutes(api func(string, http.HandlerFunc)) {
	api("/api/save/status", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, saveStatus(getSettings())) })
	api("/api/save/unlock", func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		startJob(w, "Unlock new skins and emblems", []string{s.AESKey}, func(j *Job) (any, error) { return doUnlockNow(j, s) })
	})
	api("/api/save/vanilla", func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		startJob(w, "Prepare the save for the game without mods", []string{s.AESKey}, func(j *Job) (any, error) { return doPrepareVanilla(j, s) })
	})
	api("/api/save/restore", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			ID         string `json:"id"`
			WithSystem bool   `json:"withSystem"`
		}
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		startJob(w, "Restore a save backup", []string{s.AESKey}, func(j *Job) (any, error) { return doRestoreBackup(j, s, q.ID, q.WithSystem) })
	})
	api("/api/save/backup", func(w http.ResponseWriter, r *http.Request) {
		b, err := makeSaveBackup(getSettings(), "by hand")
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, 200, b)
	})
	api("/api/save/automod", func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			On bool `json:"on"`
		}
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s := getSettings()
		var err error
		if q.On {
			err = installSaveBackupMod(s)
		} else {
			err = removeSaveBackupMod(s)
		}
		if err != nil {
			fail(w, 500, err)
			return
		}
		s.SaveBackupOff = !q.On
		if err := saveSettings(s); err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, 200, saveStatus(getSettings()))
	})
}
