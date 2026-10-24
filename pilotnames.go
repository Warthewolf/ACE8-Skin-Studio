package main

// Names on the radio: the names and callsigns the game shows for the people who talk on the radio,
// and the flight names (Joker 3). They are texts the game looks up by key, like the names of new
// skins, so SkinSlotText gives them: the program writes them into one file in its texts folder
// (SkinStudioNames.ini) and nothing is built or packed. They show the next time the game starts.
//
// The keys come from the game's own lists: the radio speaker list (DT_SpeakerTable) has each
// speaker's name key (LIVE_Tasha) and flight key (Callsign_Joker3), and the wingman list
// (DT_WingmanPilotParameter) has each wingman's callsign key (ContainerAllyCallsign_Name_Pinkhead)
// and flight marker key (ContainerAllyCallsign_Name_Joker3). The briefing maps name wingmen with
// ContainerAllyCallsign_Name_ and their name (Tasha), so that key goes with the name. The hangar's
// pilot list has a title key per wingman (HangarSelectaircraftTitle_Name_Wingman2 is Tasha's,
// found in game) with the name in capitals: it has a box of its own, and when that is empty it
// gets the name in capitals.
//
// SkinSlotText writes "first use: <key>" to SkinSlotText.log the first time the game asks for one
// of its texts, so after a game the program can say which names the game has shown.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	speakerTablePath = "/Game/Datatables/Radio/DT_SpeakerTable"
	wingmanTablePath = "/Game/Datatables/AI/DataInstance/DT_WingmanPilotParameter"
	namesTextFile    = "SkinStudioNames.ini"
	pilotNameTest    = "PilotNameTest.ini" // the test file handed out before this screen existed
	oldFindFile      = "find.txt"          // 2.3.1 and 2.3.2 had SkinSlotText look for texts with it
	allyNamePrefix   = "ContainerAllyCallsign_Name_"
	speakerPrefix    = "LIVE_"
	flightPrefix     = "Callsign_"
	maxNameLen       = 40
)

// RadioSpeaker is one named person on the radio.
type RadioSpeaker struct {
	ID            string `json:"id"`                      // the name's text key: LIVE_Tasha
	Label         string `json:"label"`                   // Tasha
	Portrait      string `json:"portrait,omitempty"`      // T_HUD_SubtitleSpeakerPortrait_00_Tasha00_001
	Side          string `json:"side"`                    // wingman, ally, enemy, other
	Callsign      string `json:"callsign,omitempty"`      // the callsign's text key: ContainerAllyCallsign_Name_Pinkhead
	CallsignLabel string `json:"callsignLabel,omitempty"` // Pinkhead
	Flight        string `json:"flight,omitempty"`        // their flight: Joker3
	Hangar        int    `json:"hangar,omitempty"`        // their place in the hangar's pilot list (1 to 4)
}

// RadioFlight is a flight name: whoever flies in that place in a mission.
type RadioFlight struct {
	ID    string `json:"id"`    // Joker3
	Label string `json:"label"` // Joker 3
}

// names.json: what the person typed, by field ("name:LIVE_Tasha", "callsign:...", "flight:Joker3").
type savedNames struct {
	Names map[string]string `json:"names"`
}

func namesFile(s Settings) string { return filepath.Join(s.pilotsDir(), "names.json") }

func loadNames(s Settings) map[string]string {
	var n savedNames
	if b, err := os.ReadFile(namesFile(s)); err == nil {
		json.Unmarshal(b, &n)
	}
	if n.Names == nil {
		n.Names = map[string]string{}
	}
	return n.Names
}

// ------------------------------------------------------------------ the game's lists

// speakerLabel makes a name to show from a key: LIVE_Harrier_A -> Harrier A, LittleBee -> Little Bee.
func speakerLabel(key string) string {
	k := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(key, speakerPrefix), allyNamePrefix), flightPrefix)
	k = strings.ReplaceAll(k, "＆", " & ")
	k = strings.ReplaceAll(k, "_", " ")
	var b strings.Builder
	rs := []rune(k)
	for i, r := range rs {
		if i > 0 {
			prev := rs[i-1]
			switch {
			case unicode.IsUpper(r) && unicode.IsLower(prev): // LittleBee
				b.WriteRune(' ')
			case unicode.IsDigit(r) && unicode.IsLetter(prev): // Joker3
				b.WriteRune(' ')
			}
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// readRadioLists reads the speaker and wingman lists from the base folder.
func readRadioLists(s Settings) ([]RadioSpeaker, []RadioFlight, error) {
	var lastErr error
	for _, sc := range []*Schema{activeSchema(s), builtinSchema} {
		sp, err := readGameTable(gameFile(s.baseDir(), s.Project, speakerTablePath), speakerTablePath, sc, "LiveRadioSpeakerTable", "radio speaker list")
		if err != nil {
			lastErr = err
			continue
		}
		wm, err := readGameTable(gameFile(s.baseDir(), s.Project, wingmanTablePath), wingmanTablePath, sc, "LiveWingmanPilotParameterTable", "wingman list")
		if err != nil {
			lastErr = err
			continue
		}
		speakers, flights := radioLists(sp, wm)
		return speakers, flights, nil
	}
	return nil, nil, lastErr
}

// readGameTable reads a data table for looking at only.
func readGameTable(base, path string, sc *Schema, rowStruct, what string) (*SkinTable, error) {
	p, err := LoadPackage(base)
	if err != nil {
		return nil, err
	}
	if p.Name != path {
		return nil, fmt.Errorf("%s is not the %s", p.Name, what)
	}
	return readTableData(p.Names, p.ExportData(len(p.Exports)-1), sc, rowStruct, what)
}

// radioLists turns the two lists into the named speakers (LIVE_ keys) and the flights.
func radioLists(sp, wm *SkinTable) ([]RadioSpeaker, []RadioFlight) {
	callsign := map[string]string{}
	for _, r := range wm.Rows {
		if id, cs := r.Obj.Str("SpeakerID"), r.Obj.Str("CallsingID"); id != "" && cs != "" && callsign[id] == "" {
			callsign[id] = cs
		}
	}
	var speakers []RadioSpeaker
	seen := map[string]bool{}
	flights := map[string]bool{}
	for _, r := range sp.Rows {
		o := r.Obj
		id := o.Str("SpeakerID")
		if fl := strings.TrimPrefix(o.Str("CallSign"), flightPrefix); fl != "" && fl != o.Str("CallSign") {
			flights[fl] = true
		}
		if !strings.HasPrefix(id, speakerPrefix) || seen[id] {
			continue
		}
		seen[id] = true
		sk := RadioSpeaker{ID: id, Label: speakerLabel(id), Callsign: callsign[id], Hangar: hangarWingman[id]}
		if img := o.Str("ComWindowImage"); strings.Contains(img, portraitPrefix) {
			sk.Portrait = packageBaseName(strings.SplitN(img, ".", 2)[0])
		}
		if sk.Callsign != "" {
			sk.CallsignLabel = speakerLabel(sk.Callsign)
		}
		if fl := strings.TrimPrefix(o.Str("CallSign"), flightPrefix); fl != o.Str("CallSign") {
			sk.Flight = fl
		}
		proc, colour := o.Str("ProcessID"), o.Str("ColorType")
		switch {
		case proc == "Wingman" || strings.HasSuffix(colour, "Wingman"):
			sk.Side = "wingman"
		case strings.HasSuffix(colour, "Enemy") || strings.HasPrefix(proc, "Enemy"):
			sk.Side = "enemy"
		case strings.HasPrefix(proc, "Ally") || proc == "AWACS" || strings.HasSuffix(colour, "Ally"):
			sk.Side = "ally"
		default:
			sk.Side = "other"
		}
		speakers = append(speakers, sk)
	}
	rank := map[string]int{"wingman": 0, "ally": 1, "other": 2, "enemy": 3}
	sort.SliceStable(speakers, func(i, j int) bool {
		a, b := speakers[i], speakers[j]
		if rank[a.Side] != rank[b.Side] {
			return rank[a.Side] < rank[b.Side]
		}
		return (a.Portrait != "") && (b.Portrait == "")
	})
	// the wingmen's flights first (Joker, Queen), then the others
	wingFlight := map[string]bool{}
	for _, sk := range speakers {
		if sk.Side == "wingman" && sk.Flight != "" {
			wingFlight[sk.Flight] = true
		}
	}
	var fl []RadioFlight
	for id := range flights {
		fl = append(fl, RadioFlight{ID: id, Label: speakerLabel(id)})
	}
	sort.Slice(fl, func(i, j int) bool {
		if wingFlight[fl[i].ID] != wingFlight[fl[j].ID] {
			return wingFlight[fl[i].ID]
		}
		return naturalLess(fl[i].Label, fl[j].Label)
	})
	return speakers, fl
}

// naturalLess sorts Joker 2 before Joker 10.
func naturalLess(a, b string) bool {
	re := regexp.MustCompile(`^(.*?)(\d+)$`)
	ma, mb := re.FindStringSubmatch(a), re.FindStringSubmatch(b)
	if ma != nil && mb != nil && ma[1] == mb[1] {
		if len(ma[2]) != len(mb[2]) {
			return len(ma[2]) < len(mb[2])
		}
		return ma[2] < mb[2]
	}
	return strings.ToLower(a) < strings.ToLower(b)
}

// ------------------------------------------------------------------ fields and keys

// hangarWingman is each wingman's place in the hangar's pilot list, whose title is the key
// HangarSelectaircraftTitle_Name_Wingman<n>: the game's ELiveHangarSelectWingmanType order,
// 1 Baxter, 2 Tasha, 3 Coster, 4 Kate.
var hangarWingman = map[string]int{"LIVE_Baxter": 1, "LIVE_Tasha": 2, "LIVE_Coster": 3, "LIVE_Kate": 4}

// fieldKeys is every text key a field gives its text to.
func fieldKeys(field string) []string {
	kind, id, _ := strings.Cut(field, ":")
	switch kind {
	case "name":
		keys := []string{id}
		if who := strings.TrimPrefix(id, speakerPrefix); who != id && !strings.ContainsAny(who, " ＆") {
			keys = append(keys, allyNamePrefix+who) // the briefing map's name for a wingman
		}
		return keys
	case "hangar": // the title of the hangar's pilot list
		if n := hangarWingman[id]; n > 0 {
			return []string{fmt.Sprintf("HangarSelectaircraftTitle_Name_Wingman%d", n)}
		}
		return nil
	case "callsign":
		return []string{id}
	case "flight":
		return []string{flightPrefix + id, allyNamePrefix + strings.ReplaceAll(id, " ", "")}
	}
	return nil
}

var reNameField = regexp.MustCompile(`^(name|callsign|flight|hangar):[^\s=;#\[\]][^=\r\n\[\]]*$`)

// cleanNames checks what the window sent: known kinds of field, short one-line texts.
func cleanNames(in map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for field, text := range in {
		if !reNameField.MatchString(field) || strings.TrimSpace(field) != field || len(fieldKeys(field)) == 0 {
			return nil, fmt.Errorf("%q is not a name the program knows", field)
		}
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			continue
		}
		if utf16Len(text) > maxNameLen {
			return nil, fmt.Errorf("%q is too long; names can have at most %d characters", text, maxNameLen)
		}
		out[field] = text
	}
	return out, nil
}

// nameEntry is one line of the names file, and the box it comes from.
type nameEntry struct{ Key, Text, Field string }

// nameEntries is every key the names give a text to. A wingman with a new name and an empty
// hangar box gets the name in capitals in the hangar, as the hangar writes names.
func nameEntries(names map[string]string) []nameEntry {
	fields := make([]string, 0, len(names))
	for f := range names {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	var out []nameEntry
	done := map[string]bool{}
	add := func(field, text string, keys []string) {
		for _, k := range keys {
			if !done[k] {
				done[k] = true
				out = append(out, nameEntry{k, text, field})
			}
		}
	}
	for _, f := range fields {
		add(f, names[f], fieldKeys(f))
	}
	for _, f := range fields {
		if kind, id, _ := strings.Cut(f, ":"); kind == "name" && names["hangar:"+id] == "" {
			add(f, strings.ToUpper(names[f]), fieldKeys("hangar:"+id))
		}
	}
	return out
}

// namesIni is the SkinSlotText file for the names.
func namesIni(names map[string]string) string {
	var b strings.Builder
	b.WriteString("; Names on the radio, changed with " + appName + " (Pilots screen). Change them there:\r\n")
	b.WriteString("; this file is written again each time they are saved.\r\n\r\n[Text]\r\n")
	for _, e := range nameEntries(names) {
		b.WriteString(e.Key + "=" + iniValue(e.Text) + "\r\n")
	}
	return b.String()
}

// ------------------------------------------------------------------ what the window sees

type namesView struct {
	Speakers  []RadioSpeaker    `json:"speakers"`
	Flights   []RadioFlight     `json:"flights"`
	Names     map[string]string `json:"names"`     // field -> text
	Seen      map[string]bool   `json:"seen"`      // field -> the game showed it in the last game
	LastGame  time.Time         `json:"lastGame"`  // when SkinSlotText last started
	Saved     time.Time         `json:"saved"`     // when the names were last saved
	NeedLists bool              `json:"needLists"` // the game's lists are not fetched yet
	Problem   string            `json:"problem,omitempty"`
	Clashes   []string          `json:"clashes"` // other text files that give the same keys first
	File      string            `json:"file"`
}

func pilotNamesView(s Settings) namesView {
	v := namesView{Speakers: []RadioSpeaker{}, Flights: []RadioFlight{}, Names: loadNames(s), Seen: map[string]bool{}, Clashes: []string{}}
	if d := s.textsDir(); d != "" {
		v.File = filepath.Join(d, namesTextFile)
	} else {
		v.Problem = "UE4SS's Mods folder was not found, so the names cannot be installed. Set it in Settings (it is the folder that holds mods.txt)."
	}
	speakers, flights, err := readRadioLists(s)
	if err != nil {
		v.NeedLists = true
	} else {
		v.Speakers, v.Flights = speakers, flights
	}
	used, when := seenTextKeys(s)
	v.LastGame = when
	if st, err := os.Stat(namesFile(s)); err == nil {
		v.Saved = st.ModTime()
	}
	for _, e := range nameEntries(v.Names) {
		if used[e.Key] {
			v.Seen[e.Field] = true
		}
	}
	v.Clashes = nameClashes(s, v.Names)
	return v
}

// seenTextKeys reads SkinSlotText.log: the keys the game asked for since it last started.
func seenTextKeys(s Settings) (map[string]bool, time.Time) {
	out := map[string]bool{}
	dir := s.ue4ssModsDir()
	if dir == "" {
		return out, time.Time{}
	}
	p := filepath.Join(dir, "SkinSlotText", "Scripts", "SkinSlotText.log")
	st, err := os.Stat(p)
	if err != nil {
		return out, time.Time{}
	}
	f, err := os.Open(p)
	if err != nil {
		return out, time.Time{}
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		if _, key, ok := strings.Cut(sc.Text(), "first use: "); ok {
			out[strings.TrimSpace(key)] = true
		}
	}
	return out, st.ModTime()
}

// nameClashes names the other text files that give one of the names' keys and are read before
// this program's file (SkinSlotText keeps the first it reads: SkinSlotText.ini, then texts\ in
// name order), so the name typed here would not show.
func nameClashes(s Settings, names map[string]string) []string {
	out := []string{}
	dir := s.textsDir()
	if dir == "" || len(names) == 0 {
		return out
	}
	ours := map[string]bool{}
	for _, e := range nameEntries(names) {
		ours[e.Key] = true
	}
	var files []string
	files = append(files, filepath.Join(filepath.Dir(dir), "SkinSlotText.ini"))
	es, _ := os.ReadDir(dir)
	var inTexts []string
	for _, e := range es {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".ini") {
			inTexts = append(inTexts, e.Name())
		}
	}
	sort.Slice(inTexts, func(i, j int) bool { return strings.ToUpper(inTexts[i]) < strings.ToUpper(inTexts[j]) })
	for _, n := range inTexts {
		if strings.EqualFold(n, namesTextFile) {
			break // files after ours lose to it
		}
		files = append(files, filepath.Join(dir, n))
	}
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			k, _, ok := strings.Cut(line, "=")
			if ok && !strings.HasPrefix(line, ";") && !strings.HasPrefix(line, "#") && ours[strings.TrimSpace(k)] {
				out = append(out, filepath.Base(p))
				break
			}
		}
	}
	return out
}

// ------------------------------------------------------------------ saving

// savePilotNames keeps the names and writes them for SkinSlotText (or takes its file away when
// there are none). The old test file is taken out: its test names would win over these.
func savePilotNames(s Settings, in map[string]string) (namesView, []string, error) {
	names, err := cleanNames(in)
	if err != nil {
		return namesView{}, nil, err
	}
	dir := s.textsDir()
	if dir == "" {
		return namesView{}, nil, errors.New("UE4SS's Mods folder was not found, so the names cannot be installed; set it in Settings")
	}
	var notes []string
	if len(names) > 0 {
		if err := installSkinSlotText(s); err != nil {
			return namesView{}, nil, err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return namesView{}, nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, namesTextFile), []byte(namesIni(names)), 0o644); err != nil {
			return namesView{}, nil, fmt.Errorf("the names could not be written (%v). If the game is running, close it and try again", err)
		}
		keepSaveBackupMod(s)
	} else if err := os.Remove(filepath.Join(dir, namesTextFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return namesView{}, nil, err
	}
	if err := os.Remove(filepath.Join(dir, pilotNameTest)); err == nil {
		notes = append(notes, "The test file "+pilotNameTest+" was taken out of SkinSlotText's texts folder; its ROSA test names would have shown instead.")
	}
	os.Remove(filepath.Join(filepath.Dir(dir), oldFindFile)) // no longer used
	if err := os.MkdirAll(s.pilotsDir(), 0o755); err != nil {
		return namesView{}, nil, err
	}
	b, _ := json.MarshalIndent(savedNames{Names: names}, "", "  ")
	if err := os.WriteFile(namesFile(s), b, 0o644); err != nil {
		return namesView{}, nil, err
	}
	return pilotNamesView(s), notes, nil
}

// doFetchRadioLists fetches the game's speaker and wingman lists (also done by Find pilots).
func doFetchRadioLists(j *Job, s Settings) (any, error) {
	if _, err := needExtractSetup(s); err != nil {
		return nil, err
	}
	j.SetStep("Fetching the game's radio speaker list", 0.2)
	if err := ensureBase(j, s, []string{speakerTablePath, wingmanTablePath}); err != nil {
		return nil, err
	}
	sp, fl, err := readRadioLists(s)
	if err != nil {
		return nil, fmt.Errorf("the game's radio speaker list could not be read (%v)", err)
	}
	j.Log("Found %d named people on the radio and %d flight names.", len(sp), len(fl))
	return pilotNamesView(s), nil
}

func pilotNameRoutes(api func(string, http.HandlerFunc)) {
	api("/api/pilots/names", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 200, pilotNamesView(getSettings()))
			return
		}
		var q struct {
			Names map[string]string `json:"names"`
		}
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		v, notes, err := savePilotNames(getSettings(), q.Names)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]any{"view": v, "notes": notes})
	})
	api("/api/pilots/names/fetch", func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		startJob(w, "Find the names on the radio", []string{s.AESKey}, func(j *Job) (any, error) { return doFetchRadioLists(j, s) })
	})
}
