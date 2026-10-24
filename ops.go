package main

// Higher-level operations: splitting pictures into UDIM tiles, turning game textures into
// paint-ready tiles, finding painted files, and composing them back into a game picture.

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Kind is the role a texture plays in a skin.
type Kind string

const (
	KindColour Kind = "colour" // paint colour (_C; _D on aircraft with ordinary textures)
	KindFinish Kind = "finish" // metallic / roughness / extra mask (_MRX; _MRO, _MROS)
	KindNormal Kind = "normal" // normal map, with ambient occlusion in blue (_NO) or without (_N)
	KindOther  Kind = "other"  // anything else: used exactly as it is
)

// The endings the game gives each kind: the player aircraft use _C, _MRX and _NO; aircraft whose
// skins are ordinary textures (such as the Ladon) use _D, _MROS (or _MRO) and _N.
var reKindSuffix = regexp.MustCompile(`(?i)_(C|D|MRX|MROS|MRO|NO|N)(\d*)$`)

// GuessKind works out a texture's role from its name.
func GuessKind(name string) Kind {
	n := strings.TrimSuffix(name, extOf(name))
	m := reKindSuffix.FindStringSubmatch(n)
	if m == nil {
		return KindOther
	}
	switch strings.ToUpper(m[1]) {
	case "C", "D":
		return KindColour
	case "MRX", "MROS", "MRO":
		return KindFinish
	case "NO", "N":
		return KindNormal
	}
	return KindOther
}

// udimNumber is the standard tile number for a square: 1001 is the first, counting across then up by ten.
func udimNumber(bx, by int) int { return 1001 + bx + 10*by }

// splitTile separates a file name into its tile number and the rest of the name (the stem).
// A tile number is four digits from 1001 to 1999 standing on their own: with a dot, underscore,
// dash or space (or the start or end of the name) on each side. The last one in the name wins.
// The number is 0 when there is none, and the stem is then the whole name without its ending.
func splitTile(name string) (stem string, udim int) {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	isSep := func(i int) bool {
		return i < 0 || i >= len(base) || strings.ContainsRune("._- ", rune(base[i]))
	}
	at := -1
	for i := 0; i+4 <= len(base); i++ {
		if base[i] != '1' || !isSep(i-1) || !isSep(i+4) {
			continue
		}
		n, err := strconv.Atoi(base[i : i+4])
		if err == nil && n >= 1001 && n <= 1999 {
			at, udim = i, n
		}
	}
	if at < 0 {
		return base, 0
	}
	left, right := base[:at], base[at+4:]
	switch { // drop the number together with one separator next to it
	case left != "":
		left = left[:len(left)-1]
	case right != "":
		right = right[1:]
	}
	return left + right, udim
}

// ------------------------------------------------------------------ writing tiles

// OutFile describes one written file.
type OutFile struct {
	Path string `json:"path"`
	What string `json:"what"`
}

type channelPlan struct {
	suffix string
	mode   SaveMode
	make   func(tile *Img) *Img // nil = use the tile as it is
}

func greyOf(ch int) func(*Img) *Img {
	return func(t *Img) *Img {
		o := NewImg(t.W, t.H)
		for i := 0; i < t.W*t.H; i++ {
			v := t.Pix[i*4+ch]
			o.Pix[i*4], o.Pix[i*4+1], o.Pix[i*4+2], o.Pix[i*4+3] = v, v, v, 255
		}
		return o
	}
}

// rebuildNormalBlue fills blue from red and green so the tile is an ordinary tangent-space normal map.
func rebuildNormalBlue(t *Img) *Img {
	o := NewImg(t.W, t.H)
	for i := 0; i < t.W*t.H; i++ {
		r, g := t.Pix[i*4], t.Pix[i*4+1]
		x := float64(r)/127.5 - 1
		y := float64(g)/127.5 - 1
		z := math.Sqrt(math.Max(0, 1-x*x-y*y))
		o.Pix[i*4], o.Pix[i*4+1], o.Pix[i*4+2], o.Pix[i*4+3] = r, g, uint8(math.Round((z*0.5+0.5)*255)), 255
	}
	return o
}

func channelIsConstant(m *Img, ch int) bool {
	v := m.Pix[ch]
	for i := 0; i < m.W*m.H; i++ {
		if m.Pix[i*4+ch] != v {
			return false
		}
	}
	return true
}

// plansFor lists the files written for one texture. painter=false writes the picture untouched.
func plansFor(kind Kind, painter bool, full *Img) []channelPlan {
	if !painter {
		mode := SaveRGB
		if full.HasAlpha() {
			mode = SaveRGBA
		}
		return []channelPlan{{"", mode, nil}}
	}
	switch kind {
	case KindColour:
		return []channelPlan{{"_BaseColor", SaveRGB, nil}}
	case KindFinish:
		p := []channelPlan{{"_Metallic", SaveGrey, greyOf(0)}, {"_Roughness", SaveGrey, greyOf(1)}}
		if !channelIsConstant(full, 2) {
			p = append(p, channelPlan{"_BlueMask", SaveGrey, greyOf(2)})
		}
		return p
	case KindNormal:
		p := []channelPlan{{"_Normal_DirectX", SaveRGB, rebuildNormalBlue}}
		if !channelIsConstant(full, 2) { // a two-channel normal map (_N) has no occlusion to paint
			p = append(p, channelPlan{"_AO", SaveGrey, greyOf(2)})
		}
		return p
	}
	return []channelPlan{{"", SaveRGB, nil}}
}

// WriteTiles cuts a full picture into squares (blocksW across, blocksH down) and saves them.
// shrink, when given, says how much smaller than the others a square is stored in the game
// (0 = not at all, 2 = a quarter of the size); that square's tile is then saved at its stored
// size instead of being saved enlarged.
func WriteTiles(full *Img, blocksW, blocksH int, kind Kind, painter bool, outDir, baseName, ext string, progress func(done, total int), shrink ...func(bx, by int) int) ([]OutFile, error) {
	return writeTiles(full, blocksW, blocksH, kind, painter, true, outDir, baseName, ext, progress, shrink...)
}

// writeTiles is WriteTiles; numbered=false leaves the tile number out of the names (for a texture
// that is one picture, not tiles).
func writeTiles(full *Img, blocksW, blocksH int, kind Kind, painter, numbered bool, outDir, baseName, ext string, progress func(done, total int), shrink ...func(bx, by int) int) ([]OutFile, error) {
	if blocksW < 1 || blocksH < 1 || full.W%blocksW != 0 || full.H%blocksH != 0 {
		return nil, fmt.Errorf("a %d x %d picture cannot be cut into %d x %d equal squares", full.W, full.H, blocksW, blocksH)
	}
	tw, th := full.W/blocksW, full.H/blocksH
	plans := plansFor(kind, painter, full)
	type job struct {
		bx, by int
		plan   channelPlan
	}
	var jobs []job
	for by := 0; by < blocksH; by++ {
		for bx := 0; bx < blocksW; bx++ {
			for _, p := range plans {
				jobs = append(jobs, job{bx, by, p})
			}
		}
	}
	outs := make([]OutFile, len(jobs))
	errs := make([]error, len(jobs))
	done := 0
	mu := make(chan struct{}, 1)
	parallelFor(len(jobs), func(i int) {
		j := jobs[i]
		tile := full.Sub(j.bx*tw, j.by*th, tw, th)
		if j.plan.make != nil {
			tile = j.plan.make(tile)
		}
		if len(shrink) > 0 && shrink[0] != nil {
			if s := shrink[0](j.bx, j.by); s > 0 {
				tile = everyNth(tile, 1<<uint(s))
			}
		}
		path := filepath.Join(outDir, fmt.Sprintf("%s%s.%d%s", baseName, j.plan.suffix, udimNumber(j.bx, j.by), ext))
		if !numbered {
			path = filepath.Join(outDir, baseName+j.plan.suffix+ext)
		}
		errs[i] = SaveImage(path, tile, j.plan.mode)
		outs[i] = OutFile{Path: path, What: strings.TrimPrefix(j.plan.suffix, "_")}
		mu <- struct{}{}
		done++
		if progress != nil {
			progress(done, len(jobs))
		}
		<-mu
	})
	for _, e := range errs {
		if e != nil {
			return nil, e
		}
	}
	return outs, nil
}

// SplitPicture cuts a stitched picture file into tiles. pieces=0 means "width divided by height".
func SplitPicture(path string, pieces int, painter bool, outDir, ext string, progress func(done, total int)) ([]OutFile, error) {
	full, err := LoadImage(path)
	if err != nil {
		return nil, err
	}
	if pieces <= 0 {
		if full.H == 0 || full.W%full.H != 0 {
			return nil, fmt.Errorf("%s is %d x %d: the width is not a whole number of squares, so say how many pieces to cut", filepath.Base(path), full.W, full.H)
		}
		pieces = full.W / full.H
	}
	if full.W%pieces != 0 {
		return nil, fmt.Errorf("%s is %d pixels wide, which does not divide into %d equal pieces", filepath.Base(path), full.W, pieces)
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(path))
	}
	return WriteTiles(full, pieces, 1, GuessKind(name), painter, outDir, name, ext, progress)
}

// ExportTexture decodes a raw game texture and saves it as tiles.
// everyNth keeps one pixel out of every n in each direction: the exact undo of repeatEach.
func everyNth(src *Img, n int) *Img {
	w, h := max(src.W/n, 1), max(src.H/n, 1)
	dst := NewImg(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			copy(dst.Pix[(y*w+x)*4:(y*w+x)*4+4], src.Pix[(min(y*n, src.H-1)*src.W+min(x*n, src.W-1))*4:])
		}
	}
	return dst
}

// repeatEach enlarges a picture n times by repeating every pixel, so nothing is blurred.
func repeatEach(src *Img, n int) *Img {
	dst := NewImg(src.W*n, src.H*n)
	for y := 0; y < dst.H; y++ {
		srow := src.Pix[(y/n)*src.W*4:]
		drow := dst.Pix[y*dst.W*4:]
		for x := 0; x < dst.W; x++ {
			copy(drow[x*4:x*4+4], srow[(x/n)*4:])
		}
	}
	return dst
}

func ExportTexture(f TexFiles, painter bool, outDir, ext string, progress func(done, total int)) ([]OutFile, error) {
	t, _, ubulk, err := LoadVTex(f)
	if err != nil {
		return nil, err
	}
	full, err := t.DecodeFull(ubulk, nil)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(filepath.Base(f.UExp), ".uexp")
	if painter && t.Format == "PF_BC5" {
		// two channels: blue is worked out from them when read; it holds no occlusion to paint
		for i := 2; i < len(full.Pix); i += 4 {
			full.Pix[i] = 255
		}
	}
	// an ordinary texture is one picture: its file has no tile number
	return writeTiles(full, t.BlocksW, t.BlocksH, GuessKind(name), painter, !t.IsPlain(), outDir, name, ext, progress, t.BlockMip)
}

// ------------------------------------------------------------------ finding painted files

// Channel names used when sorting painted files.
const (
	ChBase      = "basecolor"
	ChMetallic  = "metallic"
	ChRoughness = "roughness"
	ChNormal    = "normal"
	ChAO        = "ao"
	ChBlue      = "bluemask"
)

type PaintSet struct {
	Tiles    map[int]string // tile number -> file
	Stitched string         // or one picture holding every square side by side
	OpenGL   bool           // normal map uses the OpenGL convention (green flipped)
	Name     string         // what the files are called, without tile number and ending
	Clash    []string       // set when several differently named groups of files compete and none was chosen
	Options  []SetOption    // the competing groups, when there are several
}

// SetOption is one of several differently named groups of pictures found for the same channel.
type SetOption struct {
	Name  string `json:"name"`
	Tiles int    `json:"tiles"` // 0 for one wide picture
}

// wholeKey is the key for pictures that hold a complete texture and are named the way the game
// names that kind of texture (ending in _C, _MRX or _NO) without being named after one texture
// in particular: for example T_PP0018_fa36_0GR_C.1001.png used for T_PP0018_fa36_002_C.
func wholeKey(kind Kind) string { return "kind:" + string(kind) }

var channelWords = []struct {
	ch    string
	words []string
}{
	{ChBase, []string{"basecolor", "base_color", "base color", "albedo", "diffuse"}},
	{ChMetallic, []string{"metallic", "metalness", "metal"}},
	{ChRoughness, []string{"roughness", "rough"}},
	{ChAO, []string{"mixed_ao", "ambient_occlusion", "ambientocclusion", "_ao", " ao", "-ao"}},
	{ChBlue, []string{"bluemask", "blue_mask"}},
	{ChNormal, []string{"normal"}},
}

// classify works out which channel a painted file holds, from its name.
func classify(name string) (ch string, opengl bool, skip bool) {
	n := strings.ToLower(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
	if strings.Contains(n, "height") || strings.Contains(n, "emissive") || strings.Contains(n, "opacity") {
		return "", false, true
	}
	for _, cw := range channelWords {
		for _, w := range cw.words {
			if strings.Contains(n, w) {
				return cw.ch, strings.Contains(n, "opengl"), false
			}
		}
	}
	return "", false, false
}

// FindPainted sorts a list of picture files into channels and tiles.
//
// A file is placed by the first of these that fits its name:
//  1. a channel word such as Base_color, Metallic or Normal (Substance Painter's names);
//  2. the name of one of texNames, the raw textures being rebuilt: a ready-made replacement
//     for exactly that texture (key "tex:<name>");
//  3. a game-style ending, _C, _MRX or _NO: a ready-made picture for any texture of that
//     kind (key wholeKey(kind)).
//
// Files of one channel that are named differently (apart from the tile number) are kept apart:
// if two such groups both hold the same tile, the channel is marked as a clash instead of being
// mixed together silently.
// choices says which group to use for a channel (channel key -> group name) when the person
// has picked one; it may be nil.
func FindPainted(files []string, texNames []string, choices map[string]string) map[string]*PaintSet {
	sets := map[string]*PaintSet{}
	parts := texParts(texNames)
	// longest texture names first so T_X_001_C does not swallow T_X_001_C1
	names := append([]string(nil), texNames...)
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	type cand struct {
		path   string
		ch     string
		opengl bool
		udim   int
		rank   int
		stem   string
	}
	var cands []cand
	for _, f := range files {
		if !isImageName(f) {
			continue
		}
		ch, gl, skip := classify(f)
		if skip {
			continue
		}
		stem, udim := splitTile(f)
		lower := strings.ToLower(filepath.Base(f))
		if ch != "" {
			// a skin with two textures of this kind (body and wing): the name says which one
			if tex := partOf(lower, chKind[ch], parts); tex != "" {
				ch += "@" + tex
			}
		}
		rank := 0
		if strings.HasPrefix(ch, ChNormal) {
			// Painter writes both "Normal" (painted detail only) and "Normal_DirectX" (the complete map).
			switch {
			case strings.Contains(lower, "directx"):
				rank = 3
			case gl:
				rank = 2
			default:
				rank = 1
			}
		}
		if ch == "" {
			for _, tn := range names {
				if strings.Contains(lower, strings.ToLower(tn)) {
					ch = "tex:" + tn
					break
				}
			}
		}
		if ch == "" {
			if k := GuessKind(stem); k != KindOther {
				ch = wholeKey(k)
				if tex := partOf(lower, k, parts); tex != "" {
					ch += "@" + tex
				}
			}
		}
		if ch == "" {
			continue
		}
		cands = append(cands, cand{f, ch, gl, udim, rank, stem})
	}
	best := map[string]int{}
	for _, c := range cands {
		if c.rank > best[c.ch] {
			best[c.ch] = c.rank
		}
	}
	// group the files of each channel by what they are called
	type group struct {
		name     string
		tiles    map[int]string
		stitched string
		opengl   bool
	}
	groups := map[string]map[string]*group{}
	for _, c := range cands {
		if c.rank != best[c.ch] {
			continue
		}
		if groups[c.ch] == nil {
			groups[c.ch] = map[string]*group{}
		}
		key := strings.ToLower(c.stem)
		g := groups[c.ch][key]
		if g == nil {
			g = &group{name: c.stem, tiles: map[int]string{}}
			groups[c.ch][key] = g
		}
		g.opengl = g.opengl || c.opengl
		if c.udim == 0 {
			g.stitched = c.path
		} else {
			g.tiles[c.udim] = c.path
		}
	}
	for ch, byStem := range groups {
		keys := make([]string, 0, len(byStem))
		for k := range byStem {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var tiled, whole []*group
		for _, k := range keys {
			if g := byStem[k]; len(g.tiles) > 0 {
				tiled = append(tiled, g)
			} else {
				whole = append(whole, g)
			}
		}
		s := &PaintSet{Tiles: map[int]string{}}
		use := whole
		if len(tiled) > 0 {
			use = tiled // tiles win over a stitched picture of the same channel
		}
		clash := false
		var used []string
		for _, g := range use {
			used = append(used, g.name)
			s.OpenGL = s.OpenGL || g.opengl
			if len(tiled) == 0 {
				clash = clash || s.Stitched != ""
				s.Stitched = g.stitched
				continue
			}
			for u, p := range g.tiles {
				if _, taken := s.Tiles[u]; taken {
					clash = true
				}
				s.Tiles[u] = p
			}
		}
		s.Name = strings.Join(used, " + ")
		if clash {
			for _, g := range use {
				s.Options = append(s.Options, SetOption{Name: g.name, Tiles: len(g.tiles)})
			}
			// the person's pick settles it
			for _, g := range use {
				if want := choices[ch]; want != "" && strings.EqualFold(g.name, want) {
					s.Tiles, s.Stitched, s.OpenGL, s.Name = map[int]string{}, "", g.opengl, g.name
					for u, p := range g.tiles {
						s.Tiles[u] = p
					}
					if len(g.tiles) == 0 {
						s.Stitched = g.stitched
					}
					clash = false
					break
				}
			}
		}
		if clash {
			s.Clash = used
		}
		sets[ch] = s
	}
	for tex, words := range parts {
		sets[partKey(tex)] = &PaintSet{Name: words[0]}
	}
	return sets
}

// kindWords names a kind of texture in a sentence.
var kindWords = map[Kind]string{KindColour: "colour", KindFinish: "finish", KindNormal: "normal", KindOther: "such"}

// The kind of texture each painted channel goes into.
var chKind = map[string]Kind{ChBase: KindColour, ChMetallic: KindFinish, ChRoughness: KindFinish, ChBlue: KindFinish, ChNormal: KindNormal, ChAO: KindNormal}

// partKey marks, in the sets FindPainted returns, a texture that shares its kind with another
// texture of the skin (a body and a wing colour texture): only pictures named after it count for it.
// Its set's Name is the word that tells it apart (Body_01).
func partKey(texName string) string { return "parts:" + texName }

// texParts finds, for each texture that shares its kind with others in the skin, the words that
// tell it apart: body_01 (and body) for T_OP0045_ladn_Body_01_D next to T_OP0045_ladn_Wing_01_D.
func texParts(texNames []string) map[string][]string {
	byKind := map[Kind][]string{}
	for _, n := range texNames {
		if k := GuessKind(n); k != KindOther {
			byKind[k] = append(byKind[k], n)
		}
	}
	out := map[string][]string{}
	for _, group := range byKind {
		if len(group) < 2 {
			continue
		}
		split := make([][]string, len(group))
		for i, n := range group {
			split[i] = strings.Split(skinPrefix(n), "_")
		}
		common := 0
		for ; ; common++ {
			same := true
			for _, p := range split {
				if common >= len(p)-1 || !strings.EqualFold(p[common], split[0][common]) {
					same = false
					break
				}
			}
			if !same {
				break
			}
		}
		for i, n := range group {
			rest := split[i][common:]
			words := []string{strings.Join(rest, "_")}
			if len(rest) > 1 && len(rest[0]) >= 3 {
				words = append(words, rest[0])
			}
			out[n] = words
		}
	}
	return out
}

// partOf says which texture of kind k a picture is for, from its (lower-case) name: the one whose
// full name it holds, or else the one whose telling words it holds. "" when none fits, or several.
func partOf(lower string, k Kind, parts map[string][]string) string {
	for level := 0; level < 3; level++ {
		var found []string
		for tex, words := range parts {
			if GuessKind(tex) != k {
				continue
			}
			w := tex
			if level > 0 {
				if level > len(words) {
					continue
				}
				w = words[level-1]
			}
			if containsWord(lower, strings.ToLower(w)) {
				found = append(found, tex)
			}
		}
		// T_X_C1 holds T_X_C: the longer name is the one meant
		var best []string
		for _, a := range found {
			inside := false
			for _, b := range found {
				if a != b && len(b) > len(a) && strings.Contains(strings.ToLower(b), strings.ToLower(a)) {
					inside = true
				}
			}
			if !inside {
				best = append(best, a)
			}
		}
		if len(best) == 1 {
			return best[0]
		}
		if len(best) > 1 {
			return ""
		}
	}
	return ""
}

// containsWord tells whether s holds w with no letter or digit right before or after it.
func containsWord(s, w string) bool {
	if w == "" {
		return false
	}
	alnum := func(c byte) bool { return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' }
	for i := 0; ; {
		k := strings.Index(s[i:], w)
		if k < 0 {
			return false
		}
		k += i
		if (k == 0 || !alnum(s[k-1])) && (k+len(w) >= len(s) || !alnum(s[k+len(w)])) {
			return true
		}
		i = k + 1
	}
}

// setsFor gives the picture sets as one texture sees them. Where the skin has several textures of
// its kind, its own sets (named after it) are the only ones that count; otherwise the shared ones
// do, its own taking their place if there are any. keyOf gives each set's key in sets.
func setsFor(sets map[string]*PaintSet, texName string) (view map[string]*PaintSet, keyOf map[string]string) {
	view, keyOf = map[string]*PaintSet{}, map[string]string{}
	own := "@" + texName
	for k, s := range sets {
		if strings.HasSuffix(k, own) {
			local := strings.TrimSuffix(k, own)
			view[local], keyOf[local] = s, k
		}
	}
	shared := sets[partKey(texName)] == nil
	for k, s := range sets {
		if strings.Contains(k, "@") || strings.HasPrefix(k, "parts:") {
			continue
		}
		if _, mine := view[k]; mine {
			continue
		}
		if shared || strings.HasPrefix(k, "tex:") {
			view[k], keyOf[k] = s, k
		}
	}
	return view, keyOf
}

// resize scales a picture to an exact size (area average when shrinking, bilinear when growing).
func resize(src *Img, w, h int) *Img {
	if src.W == w && src.H == h {
		return src
	}
	dst := NewImg(w, h)
	sx, sy := float64(src.W)/float64(w), float64(src.H)/float64(h)
	parallelFor(h, func(y int) {
		for x := 0; x < w; x++ {
			o := dst.Pix[(y*w+x)*4:]
			if sx >= 1 && sy >= 1 {
				x0, x1 := int(float64(x)*sx), int(math.Ceil(float64(x+1)*sx))
				y0, y1 := int(float64(y)*sy), int(math.Ceil(float64(y+1)*sy))
				x1, y1 = min(max(x1, x0+1), src.W), min(max(y1, y0+1), src.H)
				var acc [4]int
				for yy := y0; yy < y1; yy++ {
					for xx := x0; xx < x1; xx++ {
						p := src.Pix[(yy*src.W+xx)*4:]
						acc[0] += int(p[0])
						acc[1] += int(p[1])
						acc[2] += int(p[2])
						acc[3] += int(p[3])
					}
				}
				n := (x1 - x0) * (y1 - y0)
				for c := 0; c < 4; c++ {
					o[c] = uint8((acc[c] + n/2) / n)
				}
				continue
			}
			fx := (float64(x)+0.5)*sx - 0.5
			fy := (float64(y)+0.5)*sy - 0.5
			x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
			tx, ty := fx-float64(x0), fy-float64(y0)
			xa, xb := min(max(x0, 0), src.W-1), min(max(x0+1, 0), src.W-1)
			ya, yb := min(max(y0, 0), src.H-1), min(max(y0+1, 0), src.H-1)
			for c := 0; c < 4; c++ {
				v := (1-ty)*((1-tx)*float64(src.Pix[(ya*src.W+xa)*4+c])+tx*float64(src.Pix[(ya*src.W+xb)*4+c])) +
					ty*((1-tx)*float64(src.Pix[(yb*src.W+xa)*4+c])+tx*float64(src.Pix[(yb*src.W+xb)*4+c]))
				o[c] = uint8(math.Round(v))
			}
		}
	})
	return dst
}

// loadSet loads one channel as a full picture of the texture's size.
// Returns notes about anything that was adjusted (for example a resize).
func loadSet(s *PaintSet, t *VTex) (*Img, []string, error) {
	var notes []string
	if s == nil {
		return nil, nil, errors.New("no painted files for this texture")
	}
	if len(s.Clash) > 0 {
		return nil, nil, errors.New(clashText(s))
	}
	if s.Stitched != "" {
		im, err := LoadImage(s.Stitched)
		if err != nil {
			return nil, nil, err
		}
		if im.W*t.Height != im.H*t.Width {
			return nil, nil, fmt.Errorf("%s is %d x %d, which is not the texture's shape (%d x %d)", filepath.Base(s.Stitched), im.W, im.H, t.Width, t.Height)
		}
		if im.W != t.Width {
			notes = append(notes, fmt.Sprintf("%s was resized from %d x %d to %d x %d", filepath.Base(s.Stitched), im.W, im.H, t.Width, t.Height))
			im = resize(im, t.Width, t.Height)
		}
		return im, notes, nil
	}
	bw, bh := t.Width/t.BlocksW, t.Height/t.BlocksH
	full := NewImg(t.Width, t.Height)
	type slot struct{ bx, by int }
	var slots []slot
	for by := 0; by < t.BlocksH; by++ {
		for bx := 0; bx < t.BlocksW; bx++ {
			if _, ok := s.Tiles[udimNumber(bx, by)]; !ok {
				return nil, nil, fmt.Errorf("tile %d is missing", udimNumber(bx, by))
			}
			slots = append(slots, slot{bx, by})
		}
	}
	errs := make([]error, len(slots))
	msgs := make([]string, len(slots))
	parallelFor(len(slots), func(i int) {
		p := s.Tiles[udimNumber(slots[i].bx, slots[i].by)]
		im, err := LoadImage(p)
		if err != nil {
			errs[i] = err
			return
		}
		if sm := t.BlockMip(slots[i].bx, slots[i].by); sm > 0 {
			// The game stores this tile smaller than the others.
			nw, nh := max(bw>>uint(sm), 1), max(bh>>uint(sm), 1)
			if im.W == nw && im.H == nh {
				im = repeatEach(im, 1<<uint(sm)) // exactly the stored size: used as it is
			} else if im.W*bh == im.H*bw && im.W > nw {
				msgs[i] = fmt.Sprintf("%s is %d x %d; the game stores this tile at %d x %d, so it is reduced to that", filepath.Base(p), im.W, im.H, nw, nh)
			}
		}
		if im.W != bw || im.H != bh {
			if im.W*bh != im.H*bw {
				errs[i] = fmt.Errorf("%s is %d x %d; tiles for this texture must be square like %d x %d", filepath.Base(p), im.W, im.H, bw, bh)
				return
			}
			if msgs[i] == "" {
				msgs[i] = fmt.Sprintf("%s was resized from %d x %d to %d x %d", filepath.Base(p), im.W, im.H, bw, bh)
			}
			im = resize(im, bw, bh)
		}
		full.Paste(im, slots[i].bx*bw, slots[i].by*bh)
	})
	for i := range slots {
		if errs[i] != nil {
			return nil, nil, errs[i]
		}
		if msgs[i] != "" {
			notes = append(notes, msgs[i])
		}
	}
	return full, notes, nil
}

// clashText explains that several groups of files compete for one channel.
func clashText(s *PaintSet) string {
	list := s.Clash
	if len(list) > 4 {
		list = append(append([]string(nil), list[:4]...), "and more")
	}
	return "more than one set of pictures was found (" + strings.Join(list, ", ") + "); choose which one to use"
}

func setComplete(s *PaintSet, t *VTex) bool {
	if s == nil || len(s.Clash) > 0 {
		return false
	}
	if s.Stitched != "" {
		return true
	}
	for by := 0; by < t.BlocksH; by++ {
		for bx := 0; bx < t.BlocksW; bx++ {
			if _, ok := s.Tiles[udimNumber(bx, by)]; !ok {
				return false
			}
		}
	}
	return true
}

// Recipe says how a texture's picture is put together from painted files.
type Recipe struct {
	Ready   bool     `json:"ready"`
	Uses    []string `json:"uses"`    // human-readable list of what goes in
	Missing string   `json:"missing"` // why it is not ready
	Files   int      `json:"files"`
	Sources []Source `json:"sources"` // the pictures that go in, for showing a preview
	Choices []Choice `json:"choices"` // channels with several sets to choose between
	Notes   []string `json:"notes"`   // things worth a second look, such as files named after another skin
}

// Source is one group of pictures used for a texture.
type Source struct {
	Label string   `json:"label"`          // base colour, metallic, ...
	Name  string   `json:"name"`           // what the files are called
	Files []string `json:"files"`          // one file per tile in tile order, or one wide picture
	Wide  bool     `json:"wide"`           // true when Files is a single picture holding every tile
	Cols  int      `json:"cols"`           // tiles across
	Rows  int      `json:"rows"`           // tiles down
	Kept  bool     `json:"kept,omitempty"` // nothing painted was found; the original is kept
}

// Choice is a channel for which several differently named sets of pictures were found.
type Choice struct {
	Key     string      `json:"key"`
	Label   string      `json:"label"`
	Options []SetOption `json:"options"`
	Picked  string      `json:"picked"`
}

var reNamedAfter = regexp.MustCompile(`(?i)^(T_[A-Za-z0-9_]+?)_(C|D|MRX|MROS|MRO|NO|N)[0-9]*(?:_|$)`)

// namedAfter returns the game texture a group of files seems to be named after
// ("T_PP0018_fa36_001_MRX" for "T_PP0018_fa36_001_MRX_Metallic"), or "" when it is not named after one.
func namedAfter(stem string) string {
	if m := reNamedAfter.FindStringSubmatch(stem); m != nil {
		return m[1] + "_" + strings.ToUpper(m[2])
	}
	return ""
}

// skinPrefix removes the kind ending from a texture name: T_X_002_MRX -> T_X_002.
func skinPrefix(texName string) string {
	if loc := reKindSuffix.FindStringIndex(texName); loc != nil {
		return texName[:loc[0]]
	}
	return texName
}

// PlanRecipe checks whether the painted files can rebuild a texture of the given kind.
func PlanRecipe(kind Kind, texName string, t *VTex, all map[string]*PaintSet) Recipe {
	sets, keyOf := setsFor(all, texName)
	rec, used := planRecipe(kind, texName, t, sets)
	// pictures of this kind that do not say which part they are for are left out: say so
	unclaimed := all[wholeKey(kind)] != nil
	for ch, k := range chKind {
		unclaimed = unclaimed || (k == kind && all[ch] != nil)
	}
	if part := all[partKey(texName)]; part != nil && unclaimed && !rec.Ready && rec.Missing != "" {
		rec.Missing += fmt.Sprintf(". This aircraft has more than one %s texture, so the pictures' names must say which part they are for (%s)", kindWords[kind], part.Name)
	}
	rec.Sources, rec.Choices, rec.Notes = []Source{}, []Choice{}, []string{}
	labels := map[string]string{"tex:" + texName: "ready-made picture", wholeKey(kind): "complete picture", ChBase: "base colour",
		ChMetallic: "metallic", ChRoughness: "roughness", ChBlue: "blue mask", ChNormal: "normal map", ChAO: "ambient occlusion",
		"kept:" + ChBlue: "blue mask", "kept:" + ChAO: "ambient occlusion"}
	// every channel this kind of texture can draw on, in the order it is shown
	var consult []string
	switch kind {
	case KindColour:
		consult = []string{"tex:" + texName, ChBase, wholeKey(kind)}
	case KindFinish:
		consult = []string{"tex:" + texName, ChMetallic, ChRoughness, ChBlue, wholeKey(kind)}
	case KindNormal:
		consult = []string{"tex:" + texName, ChNormal, ChAO, wholeKey(kind)}
	default:
		consult = []string{"tex:" + texName}
	}
	for _, key := range consult {
		s := sets[key]
		if s == nil || len(s.Options) == 0 {
			continue
		}
		c := Choice{Key: keyOf[key], Label: labels[key], Options: s.Options}
		if len(s.Clash) == 0 {
			c.Picked = s.Name
		}
		rec.Choices = append(rec.Choices, c)
	}
	if !rec.Ready {
		return rec
	}
	if note := t.SmallTilesNote(); note != "" {
		rec.Notes = append(rec.Notes, note+" A picture of that size is used as it is; a larger one is reduced.")
	}
	target := skinPrefix(texName)
	var foreign []string              // textures the pictures are named after, in the order met
	labelsOf := map[string][]string{} // ... and which channels are named after each
	many := map[string]bool{}         // ... and whether that is more than a single picture
	for _, key := range used {
		s := sets[key]
		src := Source{Label: labels[key], Cols: t.BlocksW, Rows: t.BlocksH}
		if s == nil {
			src.Kept = true // "kept from the original"
			rec.Sources = append(rec.Sources, src)
			continue
		}
		src.Name = s.Name
		if s.Stitched != "" {
			src.Files, src.Wide = []string{s.Stitched}, true
			// One picture for the whole texture: it must have the texture's shape. A different
			// size of the same shape is fine and is resized when the mod is built.
			if w, h, err := ImageSize(s.Stitched); err == nil && w > 0 && h > 0 {
				base := filepath.Base(s.Stitched)
				if w*t.Height != h*t.Width {
					msg := fmt.Sprintf("%s is %d x %d, which is not the shape of this texture (%d x %d)", base, w, h, t.Width, t.Height)
					if t.BlocksW*t.BlocksH > 1 {
						msg += fmt.Sprintf(". A single picture has to hold all %d tiles; a picture of one tile needs its tile number in the name, for example .1001", t.BlocksW*t.BlocksH)
					}
					return Recipe{Missing: msg, Sources: []Source{}, Choices: rec.Choices, Notes: []string{}}
				}
				if w != t.Width {
					rec.Notes = append(rec.Notes, fmt.Sprintf("%s is %d x %d and will be resized to %d x %d.", base, w, h, t.Width, t.Height))
				}
			}
		} else {
			for by := 0; by < t.BlocksH; by++ {
				for bx := 0; bx < t.BlocksW; bx++ {
					src.Files = append(src.Files, s.Tiles[udimNumber(bx, by)])
				}
			}
		}
		rec.Sources = append(rec.Sources, src)
		if from := namedAfter(s.Name); from != "" && !strings.EqualFold(skinPrefix(from), target) {
			if _, seen := labelsOf[from]; !seen {
				foreign = append(foreign, from)
			} else {
				many[from] = true
			}
			if !src.Wide {
				many[from] = true
			}
			if !strings.HasPrefix(key, "tex:") && !strings.HasPrefix(key, "kind:") {
				labelsOf[from] = append(labelsOf[from], labels[key])
			} else {
				labelsOf[from] = append(labelsOf[from][:0:0], labelsOf[from]...)
			}
		}
	}
	for _, from := range foreign {
		what, are, they := "These pictures", "are", "they are the pictures"
		if l := labelsOf[from]; len(l) > 0 {
			what = "The " + strings.Join(l, " and ") + " pictures"
		}
		if !many[from] { // a single wide picture
			what, are, they = strings.TrimSuffix(strings.Replace(what, "These", "This", 1), "s"), "is", "it is the picture"
		}
		rec.Notes = append(rec.Notes, fmt.Sprintf("%s %s named after %s, not after the texture being rebuilt (%s). That is fine if %s you mean to use.", what, are, from, texName, they))
	}
	return rec
}

// planRecipe decides whether a texture can be rebuilt. It also returns the channel keys that
// go into it, in order; a key with no set behind it means "kept from the original".
func planRecipe(kind Kind, texName string, t *VTex, sets map[string]*PaintSet) (Recipe, []string) {
	need := t.BlocksW * t.BlocksH
	describe := func(label string, s *PaintSet) string {
		if s.Stitched != "" {
			return fmt.Sprintf("%s: %s", label, filepath.Base(s.Stitched))
		}
		return fmt.Sprintf("%s: %d tiles", label, len(s.Tiles))
	}
	partial := func(label string, s *PaintSet) string {
		if s == nil {
			return "no " + label + " files found"
		}
		if len(s.Clash) > 0 {
			return label + ": " + clashText(s)
		}
		return fmt.Sprintf("%s: only %d of %d tiles found", label, len(s.Tiles), need)
	}
	if s := sets["tex:"+texName]; s != nil { // a ready-made replacement always wins
		if setComplete(s, t) {
			return Recipe{Ready: true, Uses: []string{describe("ready-made picture", s)}}, []string{"tex:" + texName}
		}
		return Recipe{Missing: partial("ready-made picture", s)}, nil
	}
	// Pictures named like this kind of game texture, but after another skin or texture.
	whole := sets[wholeKey(kind)]
	wholeKeys := []string{wholeKey(kind)}
	wholeReady := func() (Recipe, bool) {
		if !setComplete(whole, t) {
			return Recipe{}, false
		}
		return Recipe{Ready: true, Uses: []string{describe("complete picture", whole) + " (" + whole.Name + ")"}}, true
	}
	// missing prefers to talk about files that are there but unusable over files that are absent
	missing := func(label string, s *PaintSet) Recipe {
		if s == nil && whole != nil {
			return Recipe{Missing: partial("complete picture", whole)}
		}
		return Recipe{Missing: partial(label, s)}
	}
	switch kind {
	case KindColour:
		s := sets[ChBase]
		if setComplete(s, t) {
			return Recipe{Ready: true, Uses: []string{describe("base colour", s)}}, []string{ChBase}
		}
		if r, ok := wholeReady(); ok {
			return r, wholeKeys
		}
		return missing("base colour", s), nil
	case KindFinish:
		m, r := sets[ChMetallic], sets[ChRoughness]
		if !setComplete(m, t) || !setComplete(r, t) {
			if rec, ok := wholeReady(); ok {
				return rec, wholeKeys
			}
			if !setComplete(m, t) {
				return missing("metallic", m), nil
			}
			return missing("roughness", r), nil
		}
		uses := []string{describe("metallic", m), describe("roughness", r)}
		keys := []string{ChMetallic, ChRoughness}
		if b := sets[ChBlue]; setComplete(b, t) {
			uses = append(uses, describe("blue mask", b))
			keys = append(keys, ChBlue)
		} else {
			uses = append(uses, "blue mask: kept from the original")
			keys = append(keys, "kept:"+ChBlue)
		}
		return Recipe{Ready: true, Uses: uses}, keys
	case KindNormal:
		n := sets[ChNormal]
		if !setComplete(n, t) {
			if rec, ok := wholeReady(); ok {
				return rec, wholeKeys
			}
			return missing("normal map", n), nil
		}
		uses := []string{describe("normal map", n)}
		if n.OpenGL {
			uses[0] += " (OpenGL, green will be flipped)"
		}
		keys := []string{ChNormal}
		if t.Format == "PF_BC5" { // two channels: no occlusion is kept
			return Recipe{Ready: true, Uses: uses}, keys
		}
		if a := sets[ChAO]; setComplete(a, t) {
			uses = append(uses, describe("ambient occlusion", a))
			keys = append(keys, ChAO)
		} else {
			uses = append(uses, "ambient occlusion: kept from the original")
			keys = append(keys, "kept:"+ChAO)
		}
		return Recipe{Ready: true, Uses: uses}, keys
	}
	return Recipe{Missing: "no files named after this texture were found"}, nil
}

// Compose builds the full picture for a texture from the painted files.
func Compose(kind Kind, texName string, t *VTex, ubulk []byte, all map[string]*PaintSet) (*Img, []string, error) {
	sets, _ := setsFor(all, texName)
	if s := sets["tex:"+texName]; s != nil {
		return loadSet(s, t)
	}
	var notes []string
	add := func(n []string) { notes = append(notes, n...) }
	whole := sets[wholeKey(kind)]
	switch kind {
	case KindColour:
		if !setComplete(sets[ChBase], t) && setComplete(whole, t) {
			return loadSet(whole, t)
		}
		return loadSet(sets[ChBase], t)
	case KindFinish:
		if (!setComplete(sets[ChMetallic], t) || !setComplete(sets[ChRoughness], t)) && setComplete(whole, t) {
			return loadSet(whole, t)
		}
		met, n1, err := loadSet(sets[ChMetallic], t)
		if err != nil {
			return nil, nil, err
		}
		add(n1)
		rough, n2, err := loadSet(sets[ChRoughness], t)
		if err != nil {
			return nil, nil, err
		}
		add(n2)
		var blue *Img
		if b := sets[ChBlue]; setComplete(b, t) {
			var n3 []string
			if blue, n3, err = loadSet(b, t); err != nil {
				return nil, nil, err
			}
			add(n3)
		} else {
			orig, err := t.DecodeFull(ubulk, nil)
			if err != nil {
				return nil, nil, err
			}
			blue = greyOf(2)(orig)
		}
		for i := 0; i < met.W*met.H; i++ {
			met.Pix[i*4+1] = rough.Pix[i*4]
			met.Pix[i*4+2] = blue.Pix[i*4]
			met.Pix[i*4+3] = 255
		}
		return met, notes, nil
	case KindNormal:
		if !setComplete(sets[ChNormal], t) && setComplete(whole, t) {
			return loadSet(whole, t)
		}
		nrm, n1, err := loadSet(sets[ChNormal], t)
		if err != nil {
			return nil, nil, err
		}
		add(n1)
		var ao *Img
		if t.Format == "PF_BC5" {
			ao = nil // two channels: blue is not kept
		} else if a := sets[ChAO]; setComplete(a, t) {
			var n2 []string
			if ao, n2, err = loadSet(a, t); err != nil {
				return nil, nil, err
			}
			add(n2)
		} else {
			orig, err := t.DecodeFull(ubulk, nil)
			if err != nil {
				return nil, nil, err
			}
			ao = greyOf(2)(orig)
		}
		flip := sets[ChNormal].OpenGL
		for i := 0; i < nrm.W*nrm.H; i++ {
			if flip {
				nrm.Pix[i*4+1] = 255 - nrm.Pix[i*4+1]
			}
			if ao != nil {
				nrm.Pix[i*4+2] = ao.Pix[i*4]
			} else {
				nrm.Pix[i*4+2] = 255
			}
			nrm.Pix[i*4+3] = 255
		}
		return nrm, notes, nil
	}
	return nil, nil, errors.New("no painted files for this texture")
}

// listImages returns the picture files directly inside a folder (and one level below it).
func listImages(dir string) ([]string, error) {
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			sub, _ := os.ReadDir(p)
			for _, s := range sub {
				if !s.IsDir() && isImageName(s.Name()) {
					out = append(out, filepath.Join(p, s.Name()))
				}
			}
		} else if isImageName(e.Name()) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}
