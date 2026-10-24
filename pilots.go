package main

// Pilots: the helmets the pilots wear, and the portraits shown next to radio messages.
//
// A helmet is one or more parts, each with its own set of ordinary (not UDIM) 4096 x 4096
// textures: colour (_C), finish (_MRO: metallic in red, roughness in green, ambient occlusion in
// blue) and normal map (_N). For example Tasha's helmet has part Helmet01_01 (the shell with the
// graffiti) and Helmet01_02 (the mask and fittings). A portrait is a 428 x 540 picture: a photo in
// a soft-edged box, with the name, call sign, rank and unit written underneath. Both are rebuilt
// with Tex2D and packed into an ordinary replacement mod.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	portraitFolder = "/Game/UI/Texture/HUD/SubtitleSpeakerPortrait/"
	portraitPrefix = "T_HUD_SubtitleSpeakerPortrait_00_"
	fontRegular    = "/Game/UI/Font/0708_AcesSleipnirText-Regular"
)

func (s Settings) pilotsDir() string { return filepath.Join(s.Workspace, "pilots") }

// PilotSheet is one part of a helmet and its textures.
type PilotSheet struct {
	Key       string            `json:"key"`       // Helmet01_02
	Helmet    int               `json:"helmet"`    // 1
	Part      int               `json:"part"`      // 2
	Stem      string            `json:"stem"`      // T_PI003_Helmet01_02
	Textures  map[string]string `json:"textures"`  // C, MRO, N -> package path
	Materials []string          `json:"materials"` // materials that use it, for telling pictures apart by name
	Preview   bool              `json:"preview"`
}

type Pilot struct {
	ID        string       `json:"id"`       // PI003
	Name      string       `json:"name"`     // Tasha ("" when not known)
	Callsign  string       `json:"callsign"` // Pinkhead
	Sheets    []PilotSheet `json:"sheets"`
	Portraits []string     `json:"portraits"` // the portraits of this pilot, when known
}

// PortraitText is what a portrait has written under the photo.
type PortraitText struct {
	Name     string `json:"name"`
	Callsign string `json:"callsign"`
	Rank     string `json:"rank"`
	Unit     string `json:"unit"`
}

type PortraitInfo struct {
	Name  string        `json:"name"`  // T_HUD_SubtitleSpeakerPortrait_00_Tasha00_001
	Label string        `json:"label"` // Tasha 2
	Pilot string        `json:"pilot"` // PI003, when known
	Text  *PortraitText `json:"text,omitempty"`
}

type PilotIndex struct {
	Pilots    []Pilot        `json:"pilots"`
	Portraits []PortraitInfo `json:"portraits"`
	Paks      string         `json:"paks"`  // the game's containers when the index was made
	Stale     bool           `json:"stale"` // the game has changed since
}

// Who the pilots are, from the game's wingman and radio speaker tables (DT_WingmanPilotParameter,
// DT_SpeakerTable). The others are shown by their number.
var knownPilots = map[string]struct{ Name, Callsign, Portrait string }{
	"PI001": {"Coster", "Noise", "Coster"},
	"PI002": {"Baxter", "Professor", "Baxter"},
	"PI003": {"Tasha", "Pinkhead", "Tasha"},
	"PI005": {"Kate Evans", "Little Bee", "Evans"},
}

// What some portraits have written on them, as a starting point when writing new text.
var knownPortraitText = map[string]PortraitText{
	"Baxter00_000":    {"E. BAXTER", "PROFESSOR", "LCDR", "FCU NAVY"},
	"Coster00_000":    {"W. COSTER", "NOISE", "LT", "FCU NAVY"},
	"Evans00_000":     {"K. EVANS", "LITTLE BEE", "LCDR", "FCU NAVY"},
	"Jackpot1_00_000": {"C. PEREZ", "GUST", "LCDR", "FCUMC"},
	"Tasha00_000":     {"A. SEVERSKAYA", "PINKY", "ENS", "FCU NAVY"},
	"Tasha00_001":     {"A. SEVERSKAYA", "", "ENS", "FCU NAVY"},
}

var (
	reHelmetTex = regexp.MustCompile(`Content/(Character/PI/(PI\d+)/(?:[^\s/]+/)*(T_PI\d+_(Helmet(\d+)_(\d+))_(C|MRO|N)))\.`)
	reHelmetMat = regexp.MustCompile(`Content/(Character/PI/(PI\d+)/(?:[^\s/]+/)*(MI_PI\d+_Helmet[^\s/.]*))\.`)
	rePortrait  = regexp.MustCompile(`Content/(UI/Texture/HUD/SubtitleSpeakerPortrait/(` + portraitPrefix + `[^\s/.]+))\.`)
	rePortName  = regexp.MustCompile(`^(.*?)_?(\d\d)_(\d{3})$`)
	reSheetRef  = regexp.MustCompile(`T_PI\d+_(Helmet\d+_\d+)_(?:C|MRO|N)$`)
	reGameName  = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
)

// portraitLabel turns T_HUD_SubtitleSpeakerPortrait_00_Tasha00_001 into "Tasha 2".
func portraitLabel(name string) (label, key string) {
	key = strings.TrimPrefix(name, portraitPrefix)
	m := rePortName.FindStringSubmatch(key)
	if m == nil {
		return key, key
	}
	label = m[1]
	if n, _ := strconv.Atoi(m[3]); n > 0 {
		label += fmt.Sprintf(" %d", n+1)
	}
	return label, key
}

// buildPilotIndex reads the rows of the game's file list ("retoc list") and finds every helmet
// texture, helmet material and portrait. It returns the index and the helmet materials to read.
func buildPilotIndex(lines []string) (*PilotIndex, map[string][]string) {
	sheets := map[string]map[string]*PilotSheet{}
	mats := map[string][]string{}
	ports := map[string]bool{}
	seenMat := map[string]bool{}
	for _, line := range lines {
		line = strings.ReplaceAll(line, `\`, "/")
		if m := reHelmetTex.FindStringSubmatch(line); m != nil {
			id, key := m[2], m[4]
			if sheets[id] == nil {
				sheets[id] = map[string]*PilotSheet{}
			}
			sh := sheets[id][key]
			if sh == nil {
				helmet, _ := strconv.Atoi(m[5])
				part, _ := strconv.Atoi(m[6])
				sh = &PilotSheet{Key: key, Helmet: helmet, Part: part, Stem: strings.TrimSuffix(m[3], "_"+m[7]), Textures: map[string]string{}}
				sheets[id][key] = sh
			}
			sh.Textures[m[7]] = "/Game/" + m[1]
			continue
		}
		if m := reHelmetMat.FindStringSubmatch(line); m != nil {
			p := "/Game/" + m[1]
			if !seenMat[p] {
				seenMat[p] = true
				mats[m[2]] = append(mats[m[2]], p)
			}
			continue
		}
		if m := rePortrait.FindStringSubmatch(line); m != nil {
			ports[m[2]] = true
		}
	}
	idx := &PilotIndex{Pilots: []Pilot{}, Portraits: []PortraitInfo{}}
	portraitOf := map[string]string{} // portrait label base -> pilot
	for id, shs := range sheets {
		p := Pilot{ID: id, Portraits: []string{}}
		if k, ok := knownPilots[id]; ok {
			p.Name, p.Callsign = k.Name, k.Callsign
			portraitOf[k.Portrait] = id
		}
		for _, sh := range shs {
			sh.Materials = []string{}
			p.Sheets = append(p.Sheets, *sh)
		}
		sort.Slice(p.Sheets, func(a, b int) bool { return p.Sheets[a].Key < p.Sheets[b].Key })
		idx.Pilots = append(idx.Pilots, p)
	}
	sort.Slice(idx.Pilots, func(a, b int) bool { return idx.Pilots[a].ID < idx.Pilots[b].ID })
	for name := range ports {
		label, key := portraitLabel(name)
		pi := PortraitInfo{Name: name, Label: label}
		if t, ok := knownPortraitText[key]; ok {
			t := t
			pi.Text = &t
		}
		base := label
		if i := strings.IndexByte(base, ' '); i > 0 {
			base = base[:i]
		}
		pi.Pilot = portraitOf[base]
		idx.Portraits = append(idx.Portraits, pi)
	}
	sort.Slice(idx.Portraits, func(a, b int) bool {
		x, y := idx.Portraits[a], idx.Portraits[b]
		if (x.Pilot != "") != (y.Pilot != "") {
			return x.Pilot != ""
		}
		return strings.ToLower(x.Name) < strings.ToLower(y.Name)
	})
	for i := range idx.Pilots {
		for _, pi := range idx.Portraits {
			if pi.Pilot == idx.Pilots[i].ID {
				idx.Pilots[i].Portraits = append(idx.Pilots[i].Portraits, pi.Name)
			}
		}
	}
	return idx, mats
}

// sheetMaterials reads which helmet part each material uses (the material's name table names the
// textures it uses).
func sheetMaterials(s Settings, idx *PilotIndex, mats map[string][]string) {
	for i := range idx.Pilots {
		p := &idx.Pilots[i]
		for _, mp := range mats[p.ID] {
			pkg, err := LoadPackage(gameFile(s.baseDir(), s.Project, mp))
			if err != nil {
				continue
			}
			mat := packageBaseName(mp)
			used := map[string]bool{}
			for _, n := range pkg.Names {
				if m := reSheetRef.FindStringSubmatch(n[strings.LastIndexByte(n, '/')+1:]); m != nil {
					used[m[1]] = true
				}
			}
			for k := range p.Sheets {
				if used[p.Sheets[k].Key] {
					p.Sheets[k].Materials = append(p.Sheets[k].Materials, mat)
				}
			}
		}
		for k := range p.Sheets {
			sort.Strings(p.Sheets[k].Materials)
		}
	}
}

func pilotIndexFile(s Settings) string { return filepath.Join(s.pilotsDir(), "index.json") }

func loadPilotIndex(s Settings) *PilotIndex {
	var idx PilotIndex
	b, err := os.ReadFile(pilotIndexFile(s))
	if err != nil || json.Unmarshal(b, &idx) != nil {
		return &PilotIndex{Pilots: []Pilot{}, Portraits: []PortraitInfo{}}
	}
	if sig := paksSignature(s.PaksDir); sig != "" && idx.Paks != "" && sig != idx.Paks {
		idx.Stale = true
	}
	if idx.Pilots == nil {
		idx.Pilots = []Pilot{}
	}
	if idx.Portraits == nil {
		idx.Portraits = []PortraitInfo{}
	}
	return &idx
}

func (idx *PilotIndex) pilot(id string) *Pilot {
	for i := range idx.Pilots {
		if idx.Pilots[i].ID == id {
			return &idx.Pilots[i]
		}
	}
	return nil
}

func (p *Pilot) sheet(key string) *PilotSheet {
	for i := range p.Sheets {
		if strings.EqualFold(p.Sheets[i].Key, key) {
			return &p.Sheets[i]
		}
	}
	return nil
}

// pilotLabel is how a pilot is named in files and folders: PI003_Tasha.
func pilotLabel(p *Pilot) string {
	if p.Name == "" {
		return p.ID
	}
	return safeName(p.ID + "_" + p.Name)
}

// doFindPilots reads the game's file list, fetches the portraits, the helmet materials and the font,
// and makes a small preview of each helmet part.
func doFindPilots(j *Job, s Settings) (any, error) {
	retoc, err := needExtractSetup(s)
	if err != nil {
		return nil, err
	}
	var rows []string
	tail, err := listGameFiles(j, s, retoc, func(line string) bool {
		if strings.Contains(line, "Character/PI/") || strings.Contains(line, "SubtitleSpeakerPortrait") {
			rows = append(rows, line)
			return true
		}
		return false
	})
	if err != nil {
		return nil, err
	}
	idx, mats := buildPilotIndex(rows)
	if len(idx.Pilots) == 0 && len(idx.Portraits) == 0 {
		for _, l := range tail {
			j.Log("%s", l)
		}
		return nil, errors.New("no pilot helmets or portraits were found in the game's file list; check the Paks folder and AES key")
	}
	j.Log("Found %d pilots with helmets and %d portraits.", len(idx.Pilots), len(idx.Portraits))

	// portraits, helmet materials and the font are small: they are kept in the base folder
	j.SetStep("Fetching the portraits", 0.05)
	want := []string{}
	if len(idx.Portraits) > 0 {
		want = append(want, portraitFolder)
	}
	for _, id := range slices.Sorted(maps.Keys(mats)) {
		want = append(want, mats[id]...)
	}
	if err := ensureBase(j, s, want); err != nil {
		return nil, err
	}
	if err := ensureBase(j, s, []string{fontRegular}); err != nil {
		j.Log("The game's font could not be fetched (%v); new portrait text will use a font of this computer.", err)
	}
	// the radio speaker and wingman lists, for the names on the radio
	if err := ensureBase(j, s, []string{speakerTablePath, wingmanTablePath}); err != nil {
		j.Log("The game's radio speaker list could not be fetched (%v); the names on the radio cannot be changed yet.", err)
	} else if sp, _, err := readRadioLists(s); err == nil {
		j.Log("Found %d named people on the radio.", len(sp))
	}
	sheetMaterials(s, idx, mats)

	// helmet previews: each part's colour texture is fetched, shrunk and deleted again
	prevDir := filepath.Join(s.pilotsDir(), "previews")
	if err := os.MkdirAll(prevDir, 0o755); err != nil {
		return nil, err
	}
	type item struct{ p, k int }
	var todo []item
	for pi := range idx.Pilots {
		for k, sh := range idx.Pilots[pi].Sheets {
			if sh.Textures["C"] != "" {
				todo = append(todo, item{pi, k})
			}
		}
	}
	tmp := filepath.Join(s.pilotsDir(), "tmp")
	defer os.RemoveAll(tmp)
	const batch = 6
	for start := 0; start < len(todo); start += batch {
		if j.Cancelled() {
			return nil, errors.New("cancelled")
		}
		group := todo[start:min(start+batch, len(todo))]
		j.SetStep(fmt.Sprintf("Making helmet previews %d to %d of %d", start+1, start+len(group), len(todo)), 0.1+0.85*float64(start)/float64(len(todo)))
		os.RemoveAll(tmp)
		if err := os.MkdirAll(tmp, 0o755); err != nil {
			return nil, err
		}
		var matches []string
		for _, it := range group {
			matches = append(matches, strings.TrimPrefix(idx.Pilots[it.p].Sheets[it.k].Textures["C"], "/Game/")+".")
		}
		if err := retocToLegacy(j, s, retoc, matches, tmp); err != nil {
			return nil, err
		}
		for _, it := range group {
			sh := &idx.Pilots[it.p].Sheets[it.k]
			png, err := texture2DPreview(gameFile(tmp, s.Project, sh.Textures["C"]), 512)
			if err != nil {
				j.Log("%s: no preview (%v)", sh.Stem, err)
				continue
			}
			if err := os.WriteFile(filepath.Join(prevDir, sh.Stem+".png"), png, 0o644); err != nil {
				return nil, err
			}
			sh.Preview = true
		}
	}
	os.RemoveAll(tmp)
	idx.Paks = paksSignature(s.PaksDir)
	b, _ := json.Marshal(idx)
	if err := os.WriteFile(pilotIndexFile(s), b, 0o644); err != nil {
		return nil, err
	}
	j.Log("The full-size helmet textures fetched for the previews have been deleted again.")
	return idx, nil
}

// texture2DPreview makes a PNG of an ordinary texture, from its largest stored size that is no
// wider than maxW.
func texture2DPreview(base string, maxW int) ([]byte, error) {
	t, p, ubulk, err := LoadTex2D(base)
	if err != nil {
		return nil, err
	}
	mip := 0
	for mip < len(t.Mips)-1 && t.Mips[mip].W > maxW {
		mip++
	}
	im, err := t.DecodeMip(p.UExp, ubulk, mip)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	mode := SaveRGB
	if im.HasAlpha() {
		mode = SaveRGBA
	}
	if err := encodePNG(&buf, im, mode); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// portraitPNG is a portrait as a PNG with its transparency, made once from the base files.
func portraitPNG(s Settings, name string) ([]byte, error) {
	if !reGameName.MatchString(name) {
		return nil, errors.New("bad name")
	}
	cache := filepath.Join(s.pilotsDir(), "portraits", name+".png")
	src := gameFile(s.baseDir(), s.Project, portraitFolder+name)
	if b, err := os.ReadFile(cache); err == nil {
		if ci, e1 := os.Stat(cache); e1 == nil {
			if si, e2 := os.Stat(src + ".uexp"); e2 != nil || !si.ModTime().After(ci.ModTime()) {
				return b, nil
			}
		}
	}
	b, err := texture2DPreview(src, 4096)
	if err != nil {
		return nil, err
	}
	os.MkdirAll(filepath.Dir(cache), 0o755)
	os.WriteFile(cache, b, 0o644)
	return b, nil
}

// otfFromUExp finds the OpenType font a FontFace asset carries and returns exactly its bytes.
func otfFromUExp(b []byte) ([]byte, error) {
	for _, sig := range [][]byte{[]byte("OTTO"), {0, 1, 0, 0}} {
		for from := 0; ; {
			i := bytes.Index(b[from:], sig)
			if i < 0 {
				break
			}
			i += from
			from = i + 1
			if i+12 > len(b) {
				break
			}
			n := int(binary.BigEndian.Uint16(b[i+4:]))
			if n < 4 || n > 64 || i+12+16*n > len(b) {
				continue
			}
			end, ok := 0, true
			for t := 0; t < n; t++ {
				r := b[i+12+16*t:]
				tag := r[:4]
				for _, c := range tag {
					if c < 0x20 || c > 0x7e {
						ok = false
					}
				}
				off, ln := int(binary.BigEndian.Uint32(r[8:])), int(binary.BigEndian.Uint32(r[12:]))
				if off < 12+16*n || i+off+ln > len(b) {
					ok = false
				}
				end = max(end, off+ln)
			}
			if !ok {
				continue
			}
			return b[i : i+end], nil
		}
	}
	return nil, errors.New("no font found in the file")
}

// gameFontOTF is the game's text font, for writing new text on portraits.
func gameFontOTF(s Settings) ([]byte, error) {
	b, err := os.ReadFile(gameFile(s.baseDir(), s.Project, fontRegular) + ".uexp")
	if err != nil {
		return nil, err
	}
	return otfFromUExp(b)
}

// ------------------------------------------------------------------ paint-ready pictures

type pilotPaintReq struct {
	Pilot  string   `json:"pilot"`
	Sheets []string `json:"sheets"` // empty = every part
	Format string   `json:"format"`
}

func pilotPaintDir(s Settings, p *Pilot) string {
	return filepath.Join(s.paintDir(), "Pilots", pilotLabel(p))
}

// doPilotPaint saves a helmet's textures as pictures to paint on: base colour, metallic,
// roughness, ambient occlusion and normal map, for each part.
func doPilotPaint(j *Job, s Settings, r pilotPaintReq) (any, error) {
	idx := loadPilotIndex(s)
	p := idx.pilot(r.Pilot)
	if p == nil {
		return nil, errors.New("pick a pilot first (press Find pilots if the list is empty)")
	}
	sheets := p.Sheets
	if len(r.Sheets) > 0 {
		sheets = nil
		for _, k := range r.Sheets {
			if sh := p.sheet(k); sh != nil {
				sheets = append(sheets, *sh)
			}
		}
	}
	var paths []string
	for _, sh := range sheets {
		for _, kind := range []string{"C", "MRO", "N"} {
			if sh.Textures[kind] != "" {
				paths = append(paths, sh.Textures[kind])
			}
		}
	}
	if len(paths) == 0 {
		return nil, errors.New("this pilot has no helmet textures")
	}
	j.SetStep("Fetching the helmet textures", -1)
	if err := ensureBase(j, s, paths); err != nil {
		return nil, err
	}
	dir := pilotPaintDir(s, p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	ext := imageExt(s, r.Format)
	var files []string
	done := 0
	for _, sh := range sheets {
		for _, kind := range []string{"C", "MRO", "N"} {
			pkg := sh.Textures[kind]
			if pkg == "" {
				continue
			}
			if j.Cancelled() {
				return nil, errors.New("cancelled")
			}
			j.SetStep(fmt.Sprintf("Saving %s_%s", sh.Stem, kind), float64(done)/float64(len(paths)))
			done++
			t, pk, ubulk, err := LoadTex2D(gameFile(s.baseDir(), s.Project, pkg))
			if err != nil {
				return nil, fmt.Errorf("%s: %v", packageBaseName(pkg), err)
			}
			im, err := t.DecodeMip(pk.UExp, ubulk, 0)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", packageBaseName(pkg), err)
			}
			type out struct {
				suffix string
				img    *Img
				mode   SaveMode
			}
			var outs []out
			switch kind {
			case "C":
				outs = []out{{"_BaseColor", im, SaveRGB}}
			case "MRO":
				outs = []out{{"_Metallic", greyOf(0)(im), SaveGrey}, {"_Roughness", greyOf(1)(im), SaveGrey}, {"_AO", greyOf(2)(im), SaveGrey}}
			case "N":
				outs = []out{{"_Normal_DirectX", im, SaveRGB}}
			}
			for _, o := range outs {
				f := filepath.Join(dir, sh.Stem+o.suffix+ext)
				if err := SaveImage(f, o.img, o.mode); err != nil {
					return nil, err
				}
				files = append(files, f)
			}
		}
	}
	j.Log("Saved %d pictures in %s", len(files), dir)
	return map[string]any{"dir": dir, "files": files}, nil
}

// ------------------------------------------------------------------ what goes where

type pilotFile struct {
	Path  string `json:"path"`
	Sheet string `json:"sheet"` // the part the picture was dropped on; "" = tell from its name
}

// helmet picture channels
const (
	hcBase      = "base"
	hcMRO       = "mro" // a finish picture with all three channels packed, like the game's
	hcMetallic  = "metallic"
	hcRoughness = "roughness"
	hcAO        = "ao"
	hcNormal    = "normal"
)

var helmetChannels = []string{hcBase, hcMRO, hcMetallic, hcRoughness, hcAO, hcNormal}

// helmetTextures are a helmet part's textures, in order, with the picture channels each one is
// made from and what it is called in messages.
var helmetTextures = []struct {
	kind, label string
	channels    []string
}{
	{"C", "colour texture", []string{hcBase}},
	{"MRO", "finish texture", []string{hcMRO, hcMetallic, hcRoughness, hcAO}},
	{"N", "normal map", []string{hcNormal}},
}

// helmetChannel works out what a helmet picture holds from its name: Substance Painter's channel
// names, or the game's own endings (_C, _MRO, _N).
func helmetChannel(path string) (ch string, rank int, opengl bool) {
	c, gl, skip := classify(path)
	if skip {
		return "", 0, false
	}
	stem := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	switch c {
	case ChBase:
		return hcBase, 1, false
	case ChMetallic:
		return hcMetallic, 1, false
	case ChRoughness:
		return hcRoughness, 1, false
	case ChAO:
		return hcAO, 1, false
	case ChNormal:
		switch {
		case strings.Contains(stem, "directx"):
			return hcNormal, 3, false
		case gl:
			return hcNormal, 2, true
		}
		return hcNormal, 1, false
	case "":
	default:
		return "", 0, false
	}
	switch {
	case strings.HasSuffix(stem, "_c"):
		return hcBase, 1, false
	case strings.HasSuffix(stem, "_mro"):
		return hcMRO, 1, false
	case strings.HasSuffix(stem, "_n"):
		return hcNormal, 1, false
	}
	return "", 0, false
}

// sheetFromName works out which helmet part a picture is for from its name: the part's name
// (Helmet01_02) or the name of a material that uses it. A helmet with one part needs neither.
func sheetFromName(path string, p *Pilot) string {
	n := strings.ToLower(filepath.Base(path))
	for _, sh := range p.Sheets {
		if strings.Contains(n, strings.ToLower(sh.Key)) {
			return sh.Key
		}
	}
	type mat struct{ name, key string }
	var mats []mat
	for _, sh := range p.Sheets {
		for _, m := range sh.Materials {
			mats = append(mats, mat{strings.ToLower(m), sh.Key})
		}
	}
	sort.Slice(mats, func(a, b int) bool { return len(mats[a].name) > len(mats[b].name) })
	for _, m := range mats {
		if strings.Contains(n, m.name) {
			return m.key
		}
	}
	if len(p.Sheets) == 1 {
		return p.Sheets[0].Key
	}
	return ""
}

type helmetPick struct {
	Path   string `json:"path"`
	OpenGL bool   `json:"opengl,omitempty"`
	rank   int
}

type sheetPlan struct {
	Key  string                `json:"key"`
	Uses map[string]helmetPick `json:"uses"` // channel -> picture
}

type pilotPlan struct {
	Sheets   []sheetPlan `json:"sheets"`
	Unplaced []string    `json:"unplaced"` // pictures whose part could not be told
	Ignored  []string    `json:"ignored"`  // pictures that are none of the channels used
	Problems []string    `json:"problems"`
}

// planPilot sorts the dropped pictures into helmet parts and channels.
func planPilot(p *Pilot, files []pilotFile) pilotPlan {
	res := pilotPlan{Sheets: []sheetPlan{}, Unplaced: []string{}, Ignored: []string{}, Problems: []string{}}
	if p == nil {
		for _, f := range files {
			res.Unplaced = append(res.Unplaced, f.Path)
		}
		return res
	}
	plans := map[string]*sheetPlan{}
	for _, sh := range p.Sheets {
		plans[sh.Key] = &sheetPlan{Key: sh.Key, Uses: map[string]helmetPick{}}
	}
	clash := map[string][]string{}
	for _, f := range files {
		if !isImageName(f.Path) {
			continue
		}
		ch, rank, gl := helmetChannel(f.Path)
		if ch == "" {
			res.Ignored = append(res.Ignored, f.Path)
			continue
		}
		var key string
		if sh := p.sheet(f.Sheet); f.Sheet != "" && sh != nil {
			key = sh.Key
		} else {
			key = sheetFromName(f.Path, p)
		}
		if key == "" {
			res.Unplaced = append(res.Unplaced, f.Path)
			continue
		}
		pl := plans[key]
		old, have := pl.Uses[ch]
		switch {
		case !have || rank > old.rank:
			pl.Uses[ch] = helmetPick{Path: f.Path, OpenGL: gl, rank: rank}
			delete(clash, key+"|"+ch)
		case rank == old.rank && !strings.EqualFold(old.Path, f.Path):
			ck := key + "|" + ch
			if len(clash[ck]) == 0 {
				clash[ck] = []string{filepath.Base(old.Path)}
			}
			clash[ck] = append(clash[ck], filepath.Base(f.Path))
		}
	}
	for _, sh := range p.Sheets {
		pl := plans[sh.Key]
		res.Sheets = append(res.Sheets, *pl)
		for _, ch := range helmetChannels {
			if c := clash[sh.Key+"|"+ch]; len(c) > 0 {
				res.Problems = append(res.Problems, fmt.Sprintf("%s has more than one %s picture: %s. Remove the ones you do not want.", sh.Key, channelLabel(ch), strings.Join(c, ", ")))
			}
		}
		for _, ht := range helmetTextures {
			for _, ch := range ht.channels {
				if _, ok := pl.Uses[ch]; ok && sh.Textures[ht.kind] == "" {
					res.Problems = append(res.Problems, sh.Key+" has no "+ht.label+" in the game, so its "+channelLabel(ch)+" picture cannot be used.")
				}
			}
		}
	}
	return res
}

func channelLabel(ch string) string {
	switch ch {
	case hcBase:
		return "base colour"
	case hcMRO:
		return "finish (_MRO)"
	case hcAO:
		return "ambient occlusion"
	case hcNormal:
		return "normal map"
	}
	return ch
}

func (pl pilotPlan) pictures() int {
	n := 0
	for _, sh := range pl.Sheets {
		n += len(sh.Uses)
	}
	return n
}

// ------------------------------------------------------------------ build

type portraitEdit struct {
	Name    string `json:"name"`    // the game's portrait to replace
	Picture string `json:"picture"` // the finished picture (PNG with transparency)
}

type pilotBuildReq struct {
	Pilot     string         `json:"pilot"`
	Files     []pilotFile    `json:"files"`
	Portraits []portraitEdit `json:"portraits"`
	ModName   string         `json:"modName"`
	Install   bool           `json:"install"`
	Cleanup   []string       `json:"cleanup"`
	KeepInput bool           `json:"keepInput"`
}

// loadChannelPicture loads a picture at the given size.
func loadChannelPicture(path string, w, h int) (*Img, error) {
	im, err := LoadPicture(path)
	if err != nil {
		return nil, err
	}
	return resize(im, w, h), nil
}

// composeHelmet makes the new picture of one helmet texture from the chosen pictures, keeping the
// game's own channels where there is no picture.
func composeHelmet(kind string, t *Tex2D, uexp, ubulk []byte, uses map[string]helmetPick) (*Img, error) {
	w, h := t.W(), t.H()
	switch kind {
	case "C":
		return loadChannelPicture(uses[hcBase].Path, w, h)
	case "N":
		pk := uses[hcNormal]
		im, err := loadChannelPicture(pk.Path, w, h)
		if err != nil {
			return nil, err
		}
		if pk.OpenGL {
			for i := 1; i < len(im.Pix); i += 4 {
				im.Pix[i] = 255 - im.Pix[i]
			}
		}
		return im, nil
	}
	// finish: start from the game's, then the packed picture, then single channels
	out, err := t.DecodeMip(uexp, ubulk, 0)
	if err != nil {
		return nil, err
	}
	if pk, ok := uses[hcMRO]; ok {
		im, err := loadChannelPicture(pk.Path, w, h)
		if err != nil {
			return nil, err
		}
		for i := 0; i < w*h; i++ {
			copy(out.Pix[i*4:i*4+3], im.Pix[i*4:i*4+3])
		}
	}
	for c, ch := range []string{hcMetallic, hcRoughness, hcAO} {
		pk, ok := uses[ch]
		if !ok {
			continue
		}
		im, err := loadChannelPicture(pk.Path, w, h)
		if err != nil {
			return nil, err
		}
		grey := im.IsGrey()
		for i := 0; i < w*h; i++ {
			v := im.Pix[i*4]
			if !grey {
				v = uint8((int(im.Pix[i*4]) + int(im.Pix[i*4+1]) + int(im.Pix[i*4+2]) + 1) / 3)
			}
			out.Pix[i*4+c] = v
		}
	}
	return out, nil
}

// writeRebuilt puts a rebuilt texture into the folder that is packed.
func writeRebuilt(s Settings, input, pkg string, p *Package, uexp, ubulk []byte) error {
	dst := gameFile(input, s.Project, pkg)
	return writePackageFiles(dst, p.UAsset, uexp, ubulk)
}

func doPilotBuild(j *Job, s Settings, r pilotBuildReq) (any, error) {
	if s.findRetoc() == "" {
		return nil, errors.New(s.checks()["retoc"].Note + " Open Settings to fix this.")
	}
	mod := safeName(r.ModName)
	if mod == "" {
		return nil, errors.New("give the mod a name using letters, numbers, dashes or underscores")
	}
	if r.Install {
		if c := s.checks()["mods"]; !c.OK {
			return nil, errors.New("to install automatically, set your mods folder in Settings (" + c.Note + ")")
		}
	}
	idx := loadPilotIndex(s)
	p := idx.pilot(r.Pilot)
	plan := planPilot(p, r.Files)
	if len(r.Files) > 0 && p == nil {
		return nil, errors.New("pick the pilot whose helmet you painted")
	}
	if len(plan.Problems) > 0 {
		return nil, errors.New(plan.Problems[0])
	}
	if len(plan.Unplaced) > 0 {
		return nil, fmt.Errorf("%d pictures do not say which helmet part they are for (for example %s); drop them on the part they belong to", len(plan.Unplaced), filepath.Base(plan.Unplaced[0]))
	}
	for _, pe := range r.Portraits {
		if !reGameName.MatchString(pe.Name) || !strings.HasPrefix(pe.Name, portraitPrefix) {
			return nil, fmt.Errorf("%q is not one of the game's portraits", pe.Name)
		}
	}
	if plan.pictures() == 0 && len(r.Portraits) == 0 {
		return nil, errors.New("add helmet pictures or a new portrait first")
	}

	// the game textures that have new pictures, and the pictures each one is made from
	type texJob struct {
		kind, pkg string
		uses      map[string]helmetPick
		from      []string
	}
	var texJobs []texJob
	var want []string
	for _, sp := range plan.Sheets {
		sh := p.sheet(sp.Key)
		for _, ht := range helmetTextures {
			tj := texJob{kind: ht.kind, pkg: sh.Textures[ht.kind], uses: sp.Uses}
			for _, ch := range ht.channels {
				if u, ok := sp.Uses[ch]; ok {
					tj.from = append(tj.from, filepath.Base(u.Path))
				}
			}
			if len(tj.from) > 0 {
				texJobs = append(texJobs, tj)
				want = append(want, tj.pkg)
			}
		}
	}
	for _, pe := range r.Portraits {
		want = append(want, portraitFolder+pe.Name)
	}
	j.SetStep("Fetching the game's textures", -1)
	if err := ensureBase(j, s, want); err != nil {
		return nil, err
	}

	modDir := filepath.Join(s.modsOut(), mod)
	input := filepath.Join(modDir, "pack_input")
	os.RemoveAll(input)
	total := len(texJobs) + len(r.Portraits)
	step := 0
	var built []string
	for _, tj := range texJobs {
		if j.Cancelled() {
			return nil, errors.New("cancelled")
		}
		name := packageBaseName(tj.pkg)
		base, span := 0.9*float64(step)/float64(total), 0.9/float64(total)
		j.SetStep(fmt.Sprintf("Rebuilding %s (%d of %d)", name, step+1, total), base)
		step++
		t, pk, ubulk, err := LoadTex2D(gameFile(s.baseDir(), s.Project, tj.pkg))
		if err != nil {
			return nil, fmt.Errorf("%s: %v", name, err)
		}
		img, err := composeHelmet(tj.kind, t, pk.UExp, ubulk, tj.uses)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", name, err)
		}
		ue, ub, err := t.Rebuild(pk.UExp, ubulk, img, func(d, n int) {
			j.SetStep(fmt.Sprintf("Rebuilding %s (%d of %d)", name, step, total), base+span*float64(d)/float64(n))
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %v", name, err)
		}
		if err := writeRebuilt(s, input, tj.pkg, pk, ue, ub); err != nil {
			return nil, err
		}
		j.Log("%s rebuilt from %s", name, strings.Join(tj.from, ", "))
		built = append(built, name)
	}
	for _, pe := range r.Portraits {
		if j.Cancelled() {
			return nil, errors.New("cancelled")
		}
		j.SetStep(fmt.Sprintf("Rebuilding %s (%d of %d)", pe.Name, step+1, total), 0.9*float64(step)/float64(total))
		step++
		pkg := portraitFolder + pe.Name
		t, pk, ubulk, err := LoadTex2D(gameFile(s.baseDir(), s.Project, pkg))
		if err != nil {
			return nil, fmt.Errorf("%s: %v", pe.Name, err)
		}
		img, err := LoadPicture(pe.Picture)
		if err != nil {
			return nil, fmt.Errorf("the new portrait for %s: %v", pe.Name, err)
		}
		ue, ub, err := t.Rebuild(pk.UExp, ubulk, img, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", pe.Name, err)
		}
		if err := writeRebuilt(s, input, pkg, pk, ue, ub); err != nil {
			return nil, err
		}
		j.Log("%s replaced", pe.Name)
		built = append(built, pe.Name)
	}
	j.SetStep("Packing the mod", 0.92)
	files, err := packFolder(j, s, input, filepath.Join(modDir, "packed"), mod+"_P")
	if err != nil {
		return nil, err
	}
	res := map[string]any{"mod": mod, "packedDir": filepath.Join(modDir, "packed"), "files": files, "built": built}
	if !r.KeepInput {
		os.RemoveAll(input)
	}
	if r.Install {
		j.SetStep("Copying into the mods folder", 0.96)
		if err := installBuiltMod(j, s, mod, files, res); err != nil {
			return res, err
		}
	}
	cleanInbox(s, r.Cleanup)
	j.Log("Mod %s is ready: %s", mod, filepath.Join(modDir, "packed"))
	return res, nil
}
