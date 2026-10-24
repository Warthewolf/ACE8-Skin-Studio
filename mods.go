package main

// Installed mods: what is in the mods folder, which game files each mod replaces, and which
// mods replace the same files.
//
// A mod is a set of container files: Name.utoc (the table of contents), Name.ucas (the data)
// and usually a small Name.pak. The .utoc lists every piece of data in the container by a
// 12-byte id that the game derives from the game file's name, so two mods that hold the same id
// replace the same game file. When the container is not encrypted, the .utoc also carries the
// names, which is what lets the list say "replaces T_PP0021_su57_001_C".

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf16"
)

// ------------------------------------------------------------------ reading a .utoc

type tocInfo struct {
	ContainerID uint64
	ChunkIDs    [][12]byte
	Names       map[int]string // chunk number -> game path, when the container carries names
	Encrypted   bool
}

const tocMagic = "-==--==--==--==-"

// Kinds of data in a container that stand for a game file's own content.
func isFileChunk(id [12]byte) bool {
	t := id[11] & 0x3f
	return t >= 1 && t <= 4 // package data, bulk data, optional bulk data, memory-mapped bulk data
}

func parseTOC(b []byte) (*tocInfo, error) {
	if len(b) < 144 || string(b[:16]) != tocMagic {
		return nil, errors.New("not a container table of contents")
	}
	u32 := func(o int) int { return int(binary.LittleEndian.Uint32(b[o:])) }
	headerSize, entries, blocks := u32(20), u32(24), u32(28)
	methodCount, methodLen, dirSize := u32(36), u32(40), u32(48)
	flags := b[80]
	seeds, noHash := u32(84), u32(96)
	if headerSize < 144 || entries < 0 || entries > 4_000_000 {
		return nil, errors.New("unexpected container header")
	}
	info := &tocInfo{ContainerID: binary.LittleEndian.Uint64(b[56:]), Names: map[int]string{}, Encrypted: flags&2 != 0}
	o := headerSize
	if o+entries*12 > len(b) {
		return nil, errors.New("container table is cut short")
	}
	info.ChunkIDs = make([][12]byte, entries)
	for i := range info.ChunkIDs {
		copy(info.ChunkIDs[i][:], b[o+i*12:])
	}
	if info.Encrypted || dirSize == 0 || flags&8 == 0 {
		return info, nil // ids only: the names are encrypted or absent
	}
	o += entries*12 + entries*10 + seeds*4 + noHash*4 + blocks*12 + methodCount*methodLen
	if flags&4 != 0 { // signed: two signatures and one hash per block come first
		if o+4 > len(b) {
			return info, nil
		}
		sig := u32(o)
		o += 4 + 2*sig + blocks*20
	}
	if o < 0 || o+dirSize > len(b) {
		return info, nil
	}
	readDirectoryIndex(b[o:o+dirSize], info)
	return info, nil
}

// readDirectoryIndex fills info.Names from a container's name table. Anything unexpected simply
// leaves the names out; the ids are enough to compare mods.
func readDirectoryIndex(d []byte, info *tocInfo) {
	pos := 0
	ok := true
	need := func(n int) bool {
		if n < 0 || pos+n > len(d) {
			ok = false
		}
		return ok
	}
	i32 := func() int {
		if !need(4) {
			return 0
		}
		v := int(int32(binary.LittleEndian.Uint32(d[pos:])))
		pos += 4
		return v
	}
	str := func() string {
		n := i32()
		if !ok {
			return ""
		}
		if n >= 0 {
			if !need(n) {
				return ""
			}
			s := strings.TrimRight(string(d[pos:pos+n]), "\x00")
			pos += n
			return s
		}
		n = -n
		if !need(n * 2) {
			return ""
		}
		u := make([]uint16, n)
		for i := range u {
			u[i] = binary.LittleEndian.Uint16(d[pos+i*2:])
		}
		pos += n * 2
		return strings.TrimRight(string(utf16.Decode(u)), "\x00")
	}
	mount := str()
	type dirEntry struct{ name, firstChild, nextSibling, firstFile uint32 }
	type fileEntry struct{ name, nextFile, userData uint32 }
	nd := i32()
	if !ok || nd < 0 || nd > 1_000_000 || !need(nd*16) {
		return
	}
	dirs := make([]dirEntry, nd)
	for i := range dirs {
		dirs[i] = dirEntry{binary.LittleEndian.Uint32(d[pos:]), binary.LittleEndian.Uint32(d[pos+4:]), binary.LittleEndian.Uint32(d[pos+8:]), binary.LittleEndian.Uint32(d[pos+12:])}
		pos += 16
	}
	nf := i32()
	if !ok || nf < 0 || nf > 4_000_000 || !need(nf*12) {
		return
	}
	files := make([]fileEntry, nf)
	for i := range files {
		files[i] = fileEntry{binary.LittleEndian.Uint32(d[pos:]), binary.LittleEndian.Uint32(d[pos+4:]), binary.LittleEndian.Uint32(d[pos+8:])}
		pos += 12
	}
	ns := i32()
	if !ok || ns < 0 || ns > 4_000_000 {
		return
	}
	names := make([]string, ns)
	for i := range names {
		names[i] = str()
		if !ok {
			return
		}
	}
	const none = 0xFFFFFFFF
	name := func(i uint32) string {
		if i == none || int(i) >= len(names) {
			return ""
		}
		return names[i]
	}
	steps := 0
	var walk func(di uint32, path string)
	walk = func(di uint32, path string) {
		for di != none && int(di) < len(dirs) && steps < 8_000_000 {
			steps++
			e := dirs[di]
			here := path
			if n := name(e.name); n != "" {
				here = path + n + "/"
			}
			for f := e.firstFile; f != none && int(f) < len(files) && steps < 8_000_000; f = files[f].nextFile {
				steps++
				if int(files[f].userData) < len(info.ChunkIDs) {
					info.Names[int(files[f].userData)] = here + name(files[f].name)
				}
			}
			walk(e.firstChild, here)
			di = e.nextSibling
		}
	}
	if mount != "" && !strings.HasSuffix(mount, "/") {
		mount += "/"
	}
	walk(0, mount)
}

// gameFileLabel turns a container path into the form shown to people:
// "../../../Live/Content/Vehicles/…/T_X_001_C.uasset" -> "Vehicles/…/T_X_001_C".
func gameFileLabel(p string) string {
	p = strings.TrimPrefix(filepath.ToSlash(p), "../../../")
	if i := strings.Index(p, "/Content/"); i >= 0 && !strings.Contains(p[:i], "/") {
		p = p[i+len("/Content/"):]
	}
	return strings.TrimSuffix(p, filepath.Ext(p))
}

// ------------------------------------------------------------------ the mods folder

type ModFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type ModConflict struct {
	With  string   `json:"with"`
	Files []string `json:"files"` // the game files both mods replace
	Same  bool     `json:"same"`  // both are the same container (same name inside), so only one can load
}

type InstalledMod struct {
	Name      string        `json:"name"`
	Dir       string        `json:"dir"`
	Loose     bool          `json:"loose"` // files sit directly in the mods folder, not in a folder of their own
	Files     []ModFile     `json:"files"`
	Size      int64         `json:"size"`
	Modified  int64         `json:"modified"` // seconds since 1970
	Replaces  []string      `json:"replaces"`
	Replaced  int           `json:"replaced"` // how many game files it replaces (Replaces may be cut short)
	Note      string        `json:"note,omitempty"`
	Conflicts []ModConflict `json:"conflicts"`
	Kind      string        `json:"kind"` // slots, skinlist, tables or ""

	chunks     map[[12]byte]string // id -> label
	containers map[uint64]bool
}

var modFileExts = map[string]bool{".utoc": true, ".ucas": true, ".pak": true, ".sig": true}

const (
	maxModTOC      = 16 << 20 // bytes; a 5 GB mod's table of contents is about 1 MB
	maxReplaceList = 300      // names sent to the window for one mod
)

func readMod(m *InstalledMod, tocPaths []string) {
	m.chunks, m.containers = map[[12]byte]string{}, map[uint64]bool{}
	seen := map[string]bool{}
	unnamed := 0
	for _, p := range tocPaths {
		st, err := os.Stat(p)
		if err != nil {
			m.Note = "One of this mod's files could not be read."
			continue
		}
		if st.Size() > maxModTOC {
			// A mod's table of contents is small; one this large is one of the game's own containers.
			m.Note = filepath.Base(p) + " is too large to be a mod's table of contents, so it was not read. Is this the game's own Paks folder?"
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			m.Note = "One of this mod's files could not be read."
			continue
		}
		info, err := parseTOC(b)
		if err != nil {
			m.Note = filepath.Base(p) + " is not a container this program can read."
			continue
		}
		m.containers[info.ContainerID] = true
		for i, id := range info.ChunkIDs {
			if !isFileChunk(id) {
				continue
			}
			label := ""
			if n, ok := info.Names[i]; ok {
				label = gameFileLabel(n)
				if !seen[label] {
					seen[label] = true
					m.Replaces = append(m.Replaces, label)
				}
			} else {
				unnamed++
			}
			m.chunks[id] = label
		}
		if info.Encrypted {
			m.Note = "This mod's file names are encrypted, so its contents cannot be listed."
		}
	}
	sort.Strings(m.Replaces)
	m.Replaced = len(m.Replaces)
	if len(m.Replaces) > maxReplaceList {
		m.Replaces = m.Replaces[:maxReplaceList]
	}
	if m.Note == "" && unnamed > 0 && len(m.Replaces) == 0 {
		m.Note = "This mod does not carry file names, so its contents cannot be listed."
	}
}

// scanMods lists the mods in a mods folder: every sub-folder that holds a .utoc file, and every
// .utoc file lying directly in the folder. It returns nil when the folder cannot be read.
func scanMods(dir string) []InstalledMod {
	dir = cleanPath(dir)
	if dir == "" {
		return nil
	}
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var mods []InstalledMod
	addFiles := func(m *InstalledMod, folder string, entries []os.DirEntry, onlyBase string) []string {
		var tocs []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if onlyBase != "" && (!modFileExts[ext] || !strings.EqualFold(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())), onlyBase)) {
				continue
			}
			fi, err := e.Info()
			if err != nil {
				continue
			}
			m.Files = append(m.Files, ModFile{e.Name(), fi.Size()})
			m.Size += fi.Size()
			if t := fi.ModTime().Unix(); t > m.Modified {
				m.Modified = t
			}
			if ext == ".utoc" {
				tocs = append(tocs, filepath.Join(folder, e.Name()))
			}
		}
		return tocs
	}
	for _, e := range es {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			sub := filepath.Join(dir, e.Name())
			inner, err := os.ReadDir(sub)
			if err != nil {
				continue
			}
			m := InstalledMod{Name: e.Name(), Dir: sub}
			tocs := addFiles(&m, sub, inner, "")
			if len(tocs) == 0 {
				continue // not a mod: no container directly inside
			}
			readMod(&m, tocs)
			mods = append(mods, m)
		} else if strings.EqualFold(filepath.Ext(e.Name()), ".utoc") {
			base := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			m := InstalledMod{Name: base, Dir: dir, Loose: true}
			readMod(&m, addFiles(&m, dir, es, base))
			mods = append(mods, m)
		}
	}
	sort.Slice(mods, func(i, j int) bool { return strings.ToLower(mods[i].Name) < strings.ToLower(mods[j].Name) })
	// which mods replace the same game files
	for i := range mods {
		mods[i].Conflicts = []ModConflict{}
		if mods[i].Replaces == nil {
			mods[i].Replaces = []string{}
		}
		for j := range mods {
			if i == j {
				continue
			}
			c := ModConflict{With: mods[j].Name}
			seen := map[string]bool{}
			unnamed := 0
			for id, label := range mods[i].chunks {
				other, both := mods[j].chunks[id]
				if !both {
					continue
				}
				if label == "" {
					label = other
				}
				if label == "" {
					unnamed++
				} else if !seen[label] {
					seen[label] = true
					c.Files = append(c.Files, label)
				}
			}
			sort.Strings(c.Files)
			if len(c.Files) > maxReplaceList {
				c.Files = append(c.Files[:maxReplaceList:maxReplaceList], fmt.Sprintf("%d more", len(c.Files)-maxReplaceList))
			}
			if unnamed > 0 && len(c.Files) == 0 {
				c.Files = []string{fmt.Sprintf("%d game files (names not available)", unnamed)}
			}
			for id := range mods[i].containers {
				if mods[j].containers[id] {
					c.Same = true
				}
			}
			if len(c.Files) > 0 || c.Same {
				mods[i].Conflicts = append(mods[i].Conflicts, c)
			}
		}
	}
	return mods
}

// removeMod takes a mod out of the mods folder. Nothing is deleted: the files are moved into
// the workspace, from where they can be put back or deleted by hand.
func removeMod(s Settings, name string, loose bool) (string, error) {
	var mod *InstalledMod
	mods := scanMods(s.ModsDir)
	for i := range mods {
		if mods[i].Name == name && mods[i].Loose == loose {
			mod = &mods[i]
		}
	}
	if mod == nil {
		return "", errors.New("that mod is no longer in the mods folder")
	}
	dest := filepath.Join(s.removedModsDir(), safeFolder(mod.Name)+"_"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	inUse := func(err error) error {
		return fmt.Errorf("the mod could not be moved (%v). If the game is running, close it and try again", err)
	}
	if !mod.Loose {
		if err := moveDir(mod.Dir, dest); err != nil {
			return "", inUse(err)
		}
		moveReplaceTexts(s, mod.Name, dest) // its name and description go with it
		return dest, nil
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	for _, f := range mod.Files {
		if err := moveFile(filepath.Join(mod.Dir, f.Name), filepath.Join(dest, f.Name)); err != nil {
			return "", inUse(err)
		}
	}
	return dest, nil
}

func safeFolder(n string) string {
	n = strings.Trim(reBadFolderChars.ReplaceAllString(n, "_"), " .")
	if n == "" {
		n = "mod"
	}
	return n
}

// moveFile renames a file, or copies and deletes it when the two places are on different drives.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		os.Remove(dst)
		return err
	}
	return os.Remove(src)
}

func moveDir(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Different drive: copy everything first, and only then remove the original.
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		return copyFile(p, filepath.Join(dst, rel))
	})
	if err != nil {
		os.RemoveAll(dst)
		return err
	}
	return os.RemoveAll(src)
}
