package main

// The library: raw game textures found on disk, grouped into skins.

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type LibTexture struct {
	Name      string `json:"name"`
	Kind      Kind   `json:"kind"`
	Format    string `json:"format"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	BlocksW   int    `json:"blocksW"`
	BlocksH   int    `json:"blocksH"`
	Supported bool   `json:"supported"`
	Plain     bool   `json:"plain,omitempty"`    // an ordinary texture: one picture, no tiles
	TileNote  string `json:"tileNote,omitempty"` // which tiles the game stores smaller than the rest
	Reason    string `json:"reason,omitempty"`
	UExp      string `json:"uexp"`
	Package   string `json:"package"`
}

type LibSkin struct {
	Dir      string       `json:"dir"`
	Aircraft string       `json:"aircraft"`
	Folder   string       `json:"folder"`
	Package  string       `json:"package"`
	Root     int          `json:"root"` // which raw folder it was found in: 0 is the workspace's own
	Textures []LibTexture `json:"textures"`
}

func describeTexture(uexpPath string) (LibTexture, bool) {
	name := strings.TrimSuffix(filepath.Base(uexpPath), ".uexp")
	lt := LibTexture{Name: name, Kind: GuessKind(name), UExp: uexpPath}
	f := texFilesFor(uexpPath)
	st, err := os.Stat(uexpPath)
	if err != nil || st.Size() > 4<<20 {
		return lt, false
	}
	uexp, err := os.ReadFile(uexpPath)
	if err != nil {
		return lt, false
	}
	if fm, _, _ := findPixelFormat(uexp, 0); fm == "" {
		return lt, false // not a texture
	} else {
		lt.Format = fm
	}
	ua, err := os.ReadFile(f.UAsset)
	if err == nil {
		lt.Package = PackageName(ua)
	}
	t, err := parseAnyTex(ua, uexp)
	if err != nil {
		lt.Reason = err.Error()
		return lt, true
	}
	lt.Width, lt.Height, lt.BlocksW, lt.BlocksH, lt.Plain = t.Width, t.Height, t.BlocksW, t.BlocksH, t.IsPlain()
	lt.TileNote = t.SmallTilesNote()
	switch {
	case lt.Package == "":
		lt.Reason = "the .uasset file is missing or has no game path, so the texture cannot be packed"
	case !strings.HasPrefix(lt.Package, "/Game/"):
		lt.Reason = "the texture is not under the game's content folder"
	default:
		if _, err := os.Stat(f.UBulk); err != nil {
			lt.Reason = "the .ubulk file is missing"
		} else {
			lt.Supported = true
		}
	}
	return lt, true
}

// scanLibrary walks the raw folders and returns the skins found, sorted by aircraft and folder.
func scanLibrary(s Settings) []LibSkin {
	roots := append([]string{s.rawDir()}, s.ExtraRawDirs...)
	byDir := map[string]*LibSkin{}
	seen := map[string]bool{}
	visited := 0
	for ri, root := range roots {
		if root == "" {
			continue
		}
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			visited++
			if visited > 400000 {
				return filepath.SkipAll
			}
			if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".uexp") {
				return nil
			}
			abs, _ := filepath.Abs(p)
			if seen[abs] {
				return nil
			}
			seen[abs] = true
			if _, err := os.Stat(strings.TrimSuffix(abs, filepath.Ext(abs)) + ".ubulk"); err != nil {
				return nil
			}
			lt, ok := describeTexture(abs)
			if !ok {
				return nil
			}
			dir := filepath.Dir(abs)
			sk := byDir[dir]
			if sk == nil {
				sk = &LibSkin{Dir: dir, Root: ri, Folder: filepath.Base(dir), Aircraft: filepath.Base(filepath.Dir(dir))}
				byDir[dir] = sk
			}
			if lt.Package != "" && sk.Package == "" {
				parts := strings.Split(strings.TrimPrefix(lt.Package, "/Game/"), "/")
				if len(parts) >= 2 {
					sk.Package = "/Game/" + strings.Join(parts[:len(parts)-1], "/")
					sk.Folder = parts[len(parts)-2]
					sk.Aircraft = ""
					for i := len(parts) - 3; i >= 0; i-- {
						if !strings.EqualFold(parts[i], "Textures") {
							sk.Aircraft = parts[i]
							break
						}
					}
					if sk.Aircraft == "" {
						sk.Aircraft = sk.Folder
					}
				}
			}
			sk.Textures = append(sk.Textures, lt)
			return nil
		})
	}
	var out []LibSkin
	for _, sk := range byDir {
		sort.Slice(sk.Textures, func(i, j int) bool { return sk.Textures[i].Name < sk.Textures[j].Name })
		out = append(out, *sk)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Aircraft != out[j].Aircraft {
			return out[i].Aircraft < out[j].Aircraft
		}
		return out[i].Folder < out[j].Folder
	})
	return out
}

// Vehicle is one aircraft (or other vehicle) found in the game's file list.
type Vehicle struct {
	Path  string   `json:"path"`  // for example Vehicles/Aircraft/PP/PP0021_su57
	Name  string   `json:"name"`  // PP0021_su57
	Skins []string `json:"skins"` // texture folders: 001, 002, ...
	// Textures lists the texture names inside each folder, so single textures can be asked for.
	Textures map[string][]string `json:"textures,omitempty"`
}
