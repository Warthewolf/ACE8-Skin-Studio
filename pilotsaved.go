package main

// New radio portraits are kept between runs: each finished portrait, how it was made (the photo
// and its place, the text) and the photo itself are saved in workspace/pilots/my-portraits, so
// closing the program loses nothing, and every build of the pilots mod has all of them.
//
// Portraits made before they were kept this way are still inside the mods built from them: their
// pictures can be read back out of the mod's container and saved here again.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (s Settings) myPortraitsDir() string { return filepath.Join(s.pilotsDir(), "my-portraits") }

// SavedPortrait is one new portrait kept for the pilots mod.
type SavedPortrait struct {
	Name    string          `json:"name"`    // the game's portrait it replaces
	Picture string          `json:"picture"` // the finished picture (PNG)
	Photo   string          `json:"photo"`   // the person's own photo, when one was used ("" otherwise)
	Base    string          `json:"base"`    // the picture it was made on, when not the game's portrait ("" otherwise)
	State   json.RawMessage `json:"state"`   // how the window made it, for changing it later
	From    string          `json:"from"`    // "" when made here; the mod's name when read back out of a mod
	Saved   string          `json:"saved"`
}

type portraitMeta struct {
	State json.RawMessage `json:"state"`
	Photo string          `json:"photo"`
	From  string          `json:"from"`
	Saved string          `json:"saved"`
}

func listSavedPortraits(s Settings) []SavedPortrait {
	out := []SavedPortrait{}
	dir := s.myPortraitsDir()
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !reGameName.MatchString(name) {
			continue
		}
		pic := filepath.Join(dir, name+".png")
		if _, err := os.Stat(pic); err != nil {
			continue
		}
		var m portraitMeta
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil || json.Unmarshal(b, &m) != nil {
			continue
		}
		sp := SavedPortrait{Name: name, Picture: pic, State: m.State, From: m.From, Saved: m.Saved}
		if p := filepath.Join(dir, name+".base.png"); fileExists(p) {
			sp.Base = p
		}
		if m.Photo != "" {
			if p := filepath.Join(dir, filepath.Base(m.Photo)); fileExists(p) {
				sp.Photo = p
			}
		}
		if len(sp.State) == 0 {
			sp.State = json.RawMessage("null")
		}
		out = append(out, sp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

type savePortraitReq struct {
	Name    string          `json:"name"`
	Picture string          `json:"picture"` // the finished picture, as uploaded
	Photo   string          `json:"photo"`   // the photo used: uploaded, or one already kept here; "" for none
	Base    string          `json:"base"`    // the picture it is made on, kept here, when not the game's portrait
	State   json.RawMessage `json:"state"`
}

// within tells whether p is inside dir.
func within(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func savePortrait(s Settings, q savePortraitReq) (SavedPortrait, error) {
	if !reGameName.MatchString(q.Name) || !strings.HasPrefix(q.Name, portraitPrefix) {
		return SavedPortrait{}, fmt.Errorf("%q is not one of the game's portraits", q.Name)
	}
	if len(q.State) > 64<<10 {
		return SavedPortrait{}, errors.New("the portrait's settings are too large")
	}
	dir := s.myPortraitsDir()
	src := cleanPath(q.Picture)
	if !within(s.inboxDir(), src) && !within(dir, src) {
		return SavedPortrait{}, errors.New("the finished portrait was not found")
	}
	if _, err := LoadPicture(src); err != nil {
		return SavedPortrait{}, fmt.Errorf("the finished portrait cannot be read: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return SavedPortrait{}, err
	}
	// the picture it is made on (a portrait read back out of a mod) is kept first: it may be the
	// picture that is about to be replaced
	base := filepath.Join(dir, q.Name+".base.png")
	if b := cleanPath(q.Base); b != "" {
		if !within(dir, b) {
			return SavedPortrait{}, errors.New("the picture the portrait was made on was not found")
		}
		if err := copyIfOther(b, base); err != nil {
			return SavedPortrait{}, err
		}
	} else {
		os.Remove(base)
	}
	if err := copyIfOther(src, filepath.Join(dir, q.Name+".png")); err != nil {
		return SavedPortrait{}, err
	}
	m := portraitMeta{State: q.State, Saved: time.Now().Format(time.RFC3339)}
	// the photo goes next to it, under the portrait's name; an older photo of it is replaced
	if ph := cleanPath(q.Photo); ph != "" {
		if !within(s.inboxDir(), ph) && !within(dir, ph) {
			return SavedPortrait{}, errors.New("the photo was not found")
		}
		dst := filepath.Join(dir, q.Name+".photo"+strings.ToLower(filepath.Ext(ph)))
		if err := copyIfOther(ph, dst); err != nil {
			return SavedPortrait{}, err
		}
		removePhotos(dir, q.Name, dst)
		m.Photo = filepath.Base(dst)
	} else {
		removePhotos(dir, q.Name, "")
	}
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, q.Name+".json"), b, 0o644); err != nil {
		return SavedPortrait{}, err
	}
	for _, sp := range listSavedPortraits(s) {
		if sp.Name == q.Name {
			return sp, nil
		}
	}
	return SavedPortrait{}, errors.New("the portrait could not be kept")
}

// copyIfOther copies src to dst unless they are the same file.
func copyIfOther(src, dst string) error {
	if a, b := filepath.Clean(src), filepath.Clean(dst); strings.EqualFold(a, b) {
		return nil
	}
	tmp := dst + ".tmp"
	if err := copyFile(src, tmp); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// removePhotos deletes a portrait's kept photos, all but keep.
func removePhotos(dir, name, keep string) {
	ms, _ := filepath.Glob(filepath.Join(dir, name+".photo.*"))
	for _, m := range ms {
		if !strings.EqualFold(m, keep) {
			os.Remove(m)
		}
	}
}

func removeSavedPortrait(s Settings, name string) error {
	if !reGameName.MatchString(name) {
		return errors.New("bad name")
	}
	dir := s.myPortraitsDir()
	removePhotos(dir, name, "")
	os.Remove(filepath.Join(dir, name+".base.png"))
	os.Remove(filepath.Join(dir, name+".png"))
	return os.Remove(filepath.Join(dir, name+".json"))
}

// ------------------------------------------------------------------ portraits inside built mods

// ModPortraits is a pilots mod built here that holds new portraits that are not kept here.
type ModPortraits struct {
	Mod   string   `json:"mod"`
	Names []string `json:"names"`
}

// builtModTOCs lists the containers of the mods built here (workspace/mods/<mod>/packed).
func builtModTOCs(s Settings) map[string]string {
	out := map[string]string{}
	es, _ := os.ReadDir(s.modsOut())
	for _, e := range es {
		if !e.IsDir() {
			continue
		}
		ms, _ := filepath.Glob(filepath.Join(s.modsOut(), e.Name(), "packed", "*.utoc"))
		if len(ms) == 1 {
			out[e.Name()] = ms[0]
		}
	}
	return out
}

// portraitsInMods finds new portraits in the mods built here that are not kept here.
func portraitsInMods(s Settings) []ModPortraits {
	kept := map[string]bool{}
	for _, sp := range listSavedPortraits(s) {
		kept[sp.Name] = true
	}
	out := []ModPortraits{}
	for mod, toc := range builtModTOCs(s) {
		var m InstalledMod
		readMod(&m, []string{toc})
		mp := ModPortraits{Mod: mod, Names: []string{}}
		for _, f := range m.Replaces {
			base := f[strings.LastIndexByte(f, '/')+1:]
			if strings.HasPrefix(f, strings.TrimPrefix(portraitFolder, "/Game/")) && !kept[base] {
				mp.Names = append(mp.Names, base)
			}
		}
		if len(mp.Names) > 0 {
			out = append(out, mp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mod < out[j].Mod })
	return out
}

// recoverPortraits reads a built mod's new portraits back out of its container and keeps them
// here. Each is read against the game's own portrait: the mod's copy has the same layout, only
// other pixels.
func recoverPortraits(s Settings, mod string) ([]string, error) {
	toc, ok := builtModTOCs(s)[mod]
	if !ok {
		return nil, fmt.Errorf("the mod %s was not built here", mod)
	}
	c, err := OpenZenContainer(toc)
	if err != nil {
		return nil, fmt.Errorf("the mod's files could not be read (%v)", err)
	}
	var want []string
	for _, mp := range portraitsInMods(s) {
		if mp.Mod == mod {
			want = mp.Names
		}
	}
	if len(want) == 0 {
		return nil, errors.New("that mod has no new portraits that are not here already")
	}
	dir := s.myPortraitsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var got []string
	var firstErr error
	for _, name := range want {
		img, err := portraitFromMod(s, c, name)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %v", name, err)
			}
			continue
		}
		if err := SaveImage(filepath.Join(dir, name+".png"), img, SaveRGBA); err != nil {
			return got, err
		}
		b, _ := json.Marshal(portraitMeta{From: mod, Saved: time.Now().Format(time.RFC3339)})
		if err := os.WriteFile(filepath.Join(dir, name+".json"), b, 0o644); err != nil {
			return got, err
		}
		got = append(got, name)
	}
	if len(got) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return got, nil
}

func portraitFromMod(s Settings, c *ZenContainer, name string) (*Img, error) {
	base := gameFile(s.baseDir(), s.Project, portraitFolder+name)
	if !fileExists(base + ".uexp") {
		return nil, errors.New("the game's own portrait is not here; press Look again on the Pilots screen first")
	}
	return textureFromZen(c, strings.TrimPrefix(portraitFolder, "/Game/")+name, base)
}

// textureFromZen reads an ordinary texture out of a container, against a raw package with the same
// layout (the game's own copy): the texture's data is laid out the same, only its pixels differ.
func textureFromZen(c *ZenContainer, pkgSuffix, layout string) (*Img, error) {
	i := c.FindPackage(pkgSuffix)
	if i < 0 {
		return nil, errors.New("not in the mod")
	}
	data, err := c.ReadChunk(i)
	if err != nil {
		return nil, err
	}
	zp, err := ParseZenPackage(data)
	if err != nil {
		return nil, err
	}
	t, pk, ubulk, err := LoadTex2D(layout)
	if err != nil {
		return nil, err
	}
	if t.NeedsBulk() {
		return nil, errors.New("this texture keeps part of its picture outside the package, which cannot be read back")
	}
	exp := pk.Exports[0]
	if len(pk.Exports) != 1 || int(exp.SerialOffset) != pk.totalHeader || len(zp.ExportData) != int(exp.SerialSize) {
		return nil, errors.New("the mod's copy is not laid out like the game's")
	}
	ue := append(append([]byte(nil), zp.ExportData...), pk.UExp[exp.SerialSize:]...)
	if f, at, _ := findPixelFormat(ue, 0); f != t.Format || at != t.pfAt {
		return nil, errors.New("the mod's copy is not laid out like the game's")
	}
	return t.DecodeMip(ue, ubulk, 0)
}
