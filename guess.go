package main

// Working out from the painted pictures' names which game skin they were painted on, so the skin to
// start from (or to replace) can be chosen without the painter having to find it in the list.
// Substance Painter names its exports after the material (MI_PP0021_su57_001_Decal_Base_color_1001)
// and the tiles made here are named after the texture (T_PP0021_su57_001_C_1001), so both carry
// the aircraft and the skin folder.

import (
	"path/filepath"
	"sort"
	"strings"
)

type skinGuess struct {
	Dir     string `json:"dir"`
	Label   string `json:"label"`
	Matched int    `json:"matched"` // pictures whose names name this skin
	Total   int    `json:"total"`   // pictures looked at
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// namesToken reports whether token appears in name as a whole part: PP0021_su57_001 is in
// MI_PP0021_su57_001_Decal but not in MI_PP0021_su57_001c_Decal.
func namesToken(name, token string) bool {
	name, token = strings.ToLower(name), strings.ToLower(token)
	if token == "" {
		return false
	}
	for i := 0; i <= len(name)-len(token); {
		k := strings.Index(name[i:], token)
		if k < 0 {
			return false
		}
		k += i
		end := k + len(token)
		if (k == 0 || !isAlnum(name[k-1])) && (end == len(name) || !isAlnum(name[end])) {
			return true
		}
		i = k + 1
	}
	return false
}

// skinTokens are the ways a skin is named inside file names: PP0021_su57_001 and, shorter, su57_001.
func skinTokens(sk LibSkin) (full, short string) {
	full = sk.Aircraft + "_" + sk.Folder
	if i := strings.Index(sk.Aircraft, "_"); i > 0 && i < len(sk.Aircraft)-1 {
		short = sk.Aircraft[i+1:] + "_" + sk.Folder
	}
	return
}

func usableSkin(sk LibSkin) bool {
	for _, t := range sk.Textures {
		if t.Supported {
			return true
		}
	}
	return false
}

func supportedCount(sk LibSkin) int {
	n := 0
	for _, t := range sk.Textures {
		if t.Supported {
			n++
		}
	}
	return n
}

// preferSkin orders copies of the same game skin found in several places: the workspace's own raw
// folder first, then the raw folders in the order they were added, then the most complete copy,
// then the shortest path (an extracted skin rather than a copy deep inside a full game export).
func preferSkin(a, b LibSkin) bool {
	if a.Root != b.Root {
		return a.Root < b.Root
	}
	if na, nb := supportedCount(a), supportedCount(b); na != nb {
		return na > nb
	}
	if len(a.Dir) != len(b.Dir) {
		return len(a.Dir) < len(b.Dir)
	}
	return a.Dir < b.Dir
}

func skinLabel(sk LibSkin) string { return sk.Aircraft + " / " + sk.Folder }

// guessSkin picks the skin the pictures were painted on. It answers only when the names point at
// one skin more than any other; pictures that name two skins equally are left for the painter.
func guessSkin(files []string, lib []LibSkin) (skinGuess, bool) {
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(strings.ReplaceAll(f, `\`, "/")), filepath.Ext(f)))
	}
	g := skinGuess{Total: len(names)}
	if len(names) == 0 {
		return g, false
	}
	for _, useShort := range []bool{false, true} {
		count := map[string]int{}    // label -> pictures naming it
		best := map[string]LibSkin{} // label -> preferred copy
		for _, sk := range lib {
			if !usableSkin(sk) {
				continue
			}
			full, short := skinTokens(sk)
			token := full
			if useShort {
				token = short
			}
			if token == "" {
				continue
			}
			label := skinLabel(sk)
			if have, ok := best[label]; ok {
				if preferSkin(sk, have) {
					best[label] = sk
				}
				continue
			}
			n := 0
			for _, name := range names {
				if namesToken(name, token) {
					n++
				}
			}
			if n > 0 {
				count[label] = n
				best[label] = sk
			}
		}
		var labels []string
		for l := range count {
			labels = append(labels, l)
		}
		if len(labels) == 0 {
			continue
		}
		sort.Slice(labels, func(i, j int) bool {
			if count[labels[i]] != count[labels[j]] {
				return count[labels[i]] > count[labels[j]]
			}
			return labels[i] < labels[j]
		})
		if len(labels) > 1 && count[labels[0]] == count[labels[1]] {
			return g, false
		}
		sk := best[labels[0]]
		g.Dir, g.Label, g.Matched = sk.Dir, labels[0], count[labels[0]]
		return g, true
	}
	return g, false
}
