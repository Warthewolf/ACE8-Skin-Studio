package main

// Game files that new skin slots are built from, fetched from the game once and kept in the
// workspace: the skin list, the unlock list, and each base skin's material and icon. They are
// fetched again after a game update (when the game's containers change).

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

func (s Settings) baseDir() string     { return filepath.Join(s.Workspace, "base") }
func (s Settings) slotModsDir() string { return filepath.Join(s.Workspace, "slot-mods") }
func (s Settings) skinListDir() string { return filepath.Join(s.Workspace, "skin-list") }
func (s Settings) exportsDir() string  { return filepath.Join(s.Workspace, "shared") }
func (s Settings) importsDir() string  { return filepath.Join(s.Workspace, "imports") }

// gameFile turns a game path (/Game/A/B) into the place it has under a folder of raw files.
func gameFile(root, project, gamePath string) string {
	return filepath.Join(root, project, "Content", filepath.FromSlash(strings.TrimPrefix(gamePath, "/Game/")))
}

// paksSignature changes whenever the game's containers change (a game update).
func paksSignature(paks string) string {
	es, err := os.ReadDir(paks)
	if err != nil {
		return ""
	}
	var parts []string
	for _, e := range es {
		if !strings.EqualFold(filepath.Ext(e.Name()), ".utoc") {
			continue
		}
		if fi, err := e.Info(); err == nil {
			parts = append(parts, fmt.Sprintf("%s|%d|%d", strings.ToLower(e.Name()), fi.Size(), fi.ModTime().Unix()))
		}
	}
	sort.Strings(parts)
	h := sha1.Sum([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(h[:8])
}

type baseStamp struct {
	Paks string `json:"paks"`
}

var baseMu sync.Mutex

// ensureBase makes sure the given game files are in the base folder, fetching what is missing in
// one go. gamePaths are package paths (/Game/...); a path ending in "/" fetches a whole folder.
func ensureBase(j *Job, s Settings, gamePaths []string) error {
	baseMu.Lock()
	defer baseMu.Unlock()
	dir := s.baseDir()
	sig := paksSignature(s.PaksDir)
	var st baseStamp
	if b, err := os.ReadFile(filepath.Join(dir, "game.json")); err == nil {
		json.Unmarshal(b, &st)
	}
	if st.Paks != "" && sig != "" && st.Paks != sig {
		j.Log("The game files have changed since the base files were fetched (a game update?); fetching them again.")
		os.RemoveAll(dir)
	}
	var want []string
	for _, gp := range gamePaths {
		local := gameFile(dir, s.Project, gp)
		if strings.HasSuffix(gp, "/") {
			if es, err := os.ReadDir(local); err == nil && len(es) > 0 {
				continue
			}
			want = append(want, strings.TrimPrefix(gp, "/Game/"))
			continue
		}
		if _, err := os.Stat(local + ".uasset"); err == nil {
			continue
		}
		want = append(want, strings.TrimPrefix(gp, "/Game/")+".")
	}
	if len(want) == 0 {
		return nil
	}
	retoc, err := needExtractSetup(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	j.Log("Fetching from the game: %s", strings.Join(want, ", "))
	if err := retocToLegacy(j, s, retoc, want, dir); err != nil {
		return err
	}
	b, _ := json.Marshal(baseStamp{Paks: sig})
	os.WriteFile(filepath.Join(dir, "game.json"), b, 0o644)
	var missing []string
	for _, gp := range gamePaths {
		local := gameFile(dir, s.Project, gp)
		if strings.HasSuffix(gp, "/") {
			continue
		}
		if _, err := os.Stat(local + ".uasset"); err != nil {
			missing = append(missing, gp)
		}
	}
	if len(missing) > 0 {
		return unreadableGameFiles(j, s, retoc, missing, dir)
	}
	return nil
}

// activeSchema is the property layout in use: the mappings file from Settings when one is set and
// readable, otherwise the one built into the program.
func activeSchema(s Settings) *Schema {
	if s.Usmap != "" {
		if sc, err := cachedUsmap(s.Usmap); err == nil {
			// it must know the structs this program needs
			ok := true
			for name := range builtinSchema.Structs {
				if _, have := sc.Structs[name]; !have {
					ok = false
				}
			}
			if ok {
				return sc
			}
		}
	}
	return builtinSchema
}

var usmapCache struct {
	sync.Mutex
	key string
	sc  *Schema
}

func cachedUsmap(path string) (*Schema, error) {
	usmapCache.Lock()
	defer usmapCache.Unlock()
	k := fileKey(path)
	if k != "" && k == usmapCache.key {
		return usmapCache.sc, nil
	}
	sc, err := LoadUsmapSchema(path)
	if err != nil {
		return nil, err
	}
	usmapCache.key, usmapCache.sc = k, sc
	return sc, nil
}

// loadBaseTables reads the game's skin list and unlock list from the base folder, fetching them
// first if needed. With a mappings file set, the built-in layout is tried when it fails.
func loadBaseTables(j *Job, s Settings) (*SkinTable, *UnlockTable, error) {
	if err := ensureBase(j, s, []string{skinTablePath, unlockTablePath}); err != nil {
		return nil, nil, err
	}
	dir := s.baseDir()
	var lastErr error
	for _, sc := range []*Schema{activeSchema(s), builtinSchema} {
		st, err := LoadSkinTable(gameFile(dir, s.Project, skinTablePath), sc)
		if err != nil {
			lastErr = err
			continue
		}
		ut, err := LoadUnlockTable(gameFile(dir, s.Project, unlockTablePath), sc)
		if err != nil {
			lastErr = err
			continue
		}
		return st, ut, nil
	}
	return nil, nil, lastErr
}

// materialFolder is the folder of a material: /Game/A/B/MI_X -> /Game/A/B/
func materialFolder(mat string) string {
	mat = strings.SplitN(mat, ".", 2)[0]
	return mat[:strings.LastIndexByte(mat, '/')+1]
}

// packagePathOf strips the object part: /Game/A/B.B -> /Game/A/B
func packagePathOf(ref string) string {
	return strings.SplitN(ref, ".", 2)[0]
}

var errNoBase = errors.New("base files missing")
