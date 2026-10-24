package main

// The four kinds of work the window can start: split, extract, make tiles, build a mod.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ------------------------------------------------------------------ split

type splitReq struct {
	Paths   []string `json:"paths"`
	Pieces  int      `json:"pieces"`
	Painter bool     `json:"painter"`
	Format  string   `json:"format"` // png, tga or same
	OutDir  string   `json:"outDir"`
	Group   bool     `json:"group"`   // give each skin its own folder inside OutDir
	Cleanup []string `json:"cleanup"` // inbox batches to delete afterwards
}

type fileResult struct {
	Source string    `json:"source"`
	Dir    string    `json:"dir"`
	Files  []OutFile `json:"files"`
	Error  string    `json:"error,omitempty"`
}

var reBadFolderChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)

// skinFolderName names the folder a picture's tiles go into: the picture's name without its
// ending and without the kind (_C, _MRX, _NO), so a skin's colour, finish and normal tiles
// land together: T_X_001_C.png and T_X_001_MRX.png both give T_X_001.
func skinFolderName(pictureName string) string {
	stem := strings.TrimSuffix(pictureName, filepath.Ext(pictureName))
	n := strings.Trim(reBadFolderChars.ReplaceAllString(skinPrefix(stem), "_"), " ._")
	if n == "" {
		n = "tiles"
	}
	if len(n) > 80 {
		n = n[:80]
	}
	return n
}

func doSplit(j *Job, s Settings, r splitReq) (any, error) {
	if len(r.Paths) == 0 {
		return nil, errors.New("add at least one picture first")
	}
	out := cleanPath(r.OutDir)
	if out == "" {
		out = s.splitDir()
	}
	ext := ""
	switch r.Format {
	case "png":
		ext = ".png"
	case "tga":
		ext = ".tga"
	}
	var results []fileResult
	failed := 0
	for i, p := range r.Paths {
		if j.Cancelled() {
			return nil, errors.New("cancelled")
		}
		name := filepath.Base(p)
		j.SetStep(fmt.Sprintf("Splitting %s (%d of %d)", name, i+1, len(r.Paths)), float64(i)/float64(len(r.Paths)))
		dir := out
		if r.Group {
			dir = filepath.Join(out, skinFolderName(name))
		}
		files, err := SplitPicture(p, r.Pieces, r.Painter, dir, ext, func(d, t int) {
			j.SetProgress(i*t+d, len(r.Paths)*t)
		})
		fr := fileResult{Source: name, Dir: dir, Files: files}
		if err != nil {
			fr.Error = err.Error()
			failed++
			j.Log("%s: %v", name, err)
		} else {
			j.Log("%s: wrote %d files to %s", name, len(files), dir)
		}
		results = append(results, fr)
	}
	cleanInbox(s, r.Cleanup)
	res := map[string]any{"outDir": out, "results": results}
	if failed == len(r.Paths) {
		return res, errors.New(results[0].Error)
	}
	return res, nil
}

var reBatch = regexp.MustCompile(`^[a-z0-9]{6,32}$`)

// cleanInbox removes uploaded copies once they are no longer needed.
func cleanInbox(s Settings, batches []string) {
	for _, b := range batches {
		if reBatch.MatchString(b) {
			os.RemoveAll(filepath.Join(s.inboxDir(), b))
		}
	}
}

// ------------------------------------------------------------------ find vehicles

var reVehicle = regexp.MustCompile(`Content/(Vehicles/[^\s]*?)/Textures/([^/\s]+)/([^/\s.]+)\.`)

func vehiclesFile(s Settings) string { return filepath.Join(s.Workspace, "vehicles.json") }

func loadVehicles(s Settings) []Vehicle {
	var v []Vehicle
	if b, err := os.ReadFile(vehiclesFile(s)); err == nil {
		json.Unmarshal(b, &v)
	}
	return v
}

func needExtractSetup(s Settings) (string, error) {
	c := s.checks()
	for _, k := range []string{"retoc", "paks", "aes"} {
		if !c[k].OK {
			return "", errors.New(c[k].Note + " Open Settings to fix this.")
		}
	}
	return s.findRetoc(), nil
}

// listGameFiles reads the game's file list ("retoc list") and passes each row to use, which says
// whether the row was one it wanted. The last messages that were not file rows are returned, for
// the log when nothing useful was found; when retoc fails they are logged straight away.
func listGameFiles(j *Job, s Settings, retoc string, use func(line string) bool) ([]string, error) {
	j.SetStep("Reading the game's file list", -1)
	var tail []string
	err := runTool(j, retoc, []string{"--aes-key", s.AESKey, "list", "--path", s.PaksDir}, func(line string) {
		if use(line) || strings.Contains(line, "Content/") {
			return
		}
		tail = append(tail, line)
		if len(tail) > 40 {
			tail = tail[1:]
		}
	})
	if err != nil {
		for _, l := range tail {
			j.Log("%s", l)
		}
	}
	return tail, err
}

func doFindVehicles(j *Job, s Settings) (any, error) {
	retoc, err := needExtractSetup(s)
	if err != nil {
		return nil, err
	}
	found := map[string]map[string]map[string]bool{}
	lines := 0
	tail, err := listGameFiles(j, s, retoc, func(line string) bool {
		lines++
		m := reVehicle.FindStringSubmatch(line)
		if m == nil {
			return false
		}
		if found[m[1]] == nil {
			found[m[1]] = map[string]map[string]bool{}
		}
		if found[m[1]][m[2]] == nil {
			found[m[1]][m[2]] = map[string]bool{}
		}
		found[m[1]][m[2]][m[3]] = true
		return true
	})
	if err != nil {
		return nil, err
	}
	var out []Vehicle
	for p, skins := range found {
		v := Vehicle{Path: p, Name: filepath.Base(p), Textures: map[string][]string{}}
		for k, names := range skins {
			v.Skins = append(v.Skins, k)
			for n := range names {
				v.Textures[k] = append(v.Textures[k], n)
			}
			sort.Strings(v.Textures[k])
		}
		sort.Strings(v.Skins)
		out = append(out, v)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	j.Log("Read %d file rows and found %d vehicles with skin textures.", lines, len(out))
	if len(out) == 0 {
		for _, l := range tail {
			j.Log("%s", l)
		}
		return nil, errors.New("no vehicles were found in the game's file list; check the Paks folder and AES key")
	}
	os.MkdirAll(s.Workspace, 0o755)
	b, _ := json.Marshal(out)
	os.WriteFile(vehiclesFile(s), b, 0o644)
	return map[string]any{"vehicles": out}, nil
}

// ------------------------------------------------------------------ extract

type extractReq struct {
	Skins   []string `json:"skins"` // texture folders to take; empty = all
	Filter  string   `json:"filter"`
	Scope   string   `json:"scope"` // textures or all
	Convert bool     `json:"convert"`
	Painter bool     `json:"painter"`
	Format  string   `json:"format"`
}

func doExtract(j *Job, s Settings, r extractReq) (any, error) {
	retoc, err := needExtractSetup(s)
	if err != nil {
		return nil, err
	}
	filter := strings.Trim(strings.TrimSpace(strings.ReplaceAll(r.Filter, `\`, "/")), "/")
	if len(filter) < 3 {
		return nil, errors.New("say which aircraft to extract: type at least three characters of its folder name, for example su57")
	}
	// A known vehicle gets an exact folder filter; anything else is used as typed.
	matches := []string{filter}
	for _, v := range loadVehicles(s) {
		if strings.EqualFold(v.Name, filter) || strings.EqualFold(v.Path, filter) {
			matches = []string{v.Path + "/"}
			if r.Scope != "all" {
				matches = []string{v.Path + "/Textures/"}
				var picked []string
				for _, sk := range r.Skins {
					for _, have := range v.Skins {
						if sk == have {
							picked = append(picked, v.Path+"/Textures/"+sk+"/")
						}
					}
				}
				if len(picked) > 0 {
					matches = picked
				}
			}
			break
		}
	}
	raw := s.rawDir()
	if err := os.MkdirAll(raw, 0o755); err != nil {
		return nil, err
	}
	j.SetStep("Extracting from the game files", -1)
	j.Log("Extracting everything whose path contains %s into %s", strings.Join(matches, " or "), raw)
	if err := retocToLegacy(j, s, retoc, matches, raw); err != nil {
		return nil, err
	}
	// what did we get?
	lib := scanLibrary(s)
	var hits []LibTexture
	skins := 0
	for _, sk := range lib {
		n := 0
		for _, t := range sk.Textures {
			hit := false
			for _, m := range matches {
				low := strings.ToLower(m)
				if strings.Contains(strings.ToLower(strings.TrimPrefix(t.Package, "/Game/")+"."), low) ||
					strings.Contains(strings.ToLower(filepath.ToSlash(t.UExp)), low) {
					hit = true
				}
			}
			if hit {
				hits = append(hits, t)
				n++
			}
		}
		if n > 0 {
			skins++
		}
	}
	j.Log("Found %d textures in %d skin folders.", len(hits), skins)
	res := map[string]any{"rawDir": raw, "textures": len(hits), "skins": skins}
	if len(hits) == 0 {
		return res, errors.New("nothing matching that name was extracted; check the spelling (capital letters matter), or use Find aircraft to pick from the game's list")
	}
	if r.Convert {
		out, n, err := exportTextures(j, s, hits, r.Painter, r.Format)
		res["paintDir"], res["files"] = out, n
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

// retocToLegacy pulls every game file whose path contains one of the given texts out into outDir.
func retocToLegacy(j *Job, s Settings, retoc string, matches []string, outDir string) error {
	run := func(extra ...string) (bool, error) {
		args := []string{"--aes-key", s.AESKey, "to-legacy", "--version", s.EngineVersion}
		args = append(args, extra...)
		for _, m := range matches {
			args = append(args, "--filter", m)
		}
		args = append(args, s.PaksDir, outDir)
		unknownFlag := false
		err := runTool(j, retoc, args, func(line string) {
			if strings.Contains(line, "unexpected argument") {
				unknownFlag = true
			}
			j.Log("%s", line)
		})
		return unknownFlag, err
	}
	unknown, err := run("--no-shaders", "--no-script-objects")
	if err != nil && unknown && !j.Cancelled() {
		// An older retoc does not know these two optional switches; go without them.
		j.Log("This retoc is an older version; trying again without the optional switches.")
		_, err = run()
	}
	return err
}

var runToolTTYFn = runToolTTY // tests stand in for retoc here

var reExtracted = regexp.MustCompile(`Extracted (\d+) \((\d+) failed\)`)

// retocWhy runs retoc again for the given filters only, one file at a time, into outDir: a second
// try, and a way to learn why it could not convert them. retoc prints the reasons through its
// progress display, which it leaves out unless its output is a terminal, so this run gives it a
// pipe it takes for one (ttyPipe). It returns the reasons, and how many files matched and failed
// (-1 when that was not said).
func retocWhy(j *Job, s Settings, retoc string, matches []string, outDir string) (why []string, matched, failed int) {
	matched, failed = -1, -1
	args := []string{"--aes-key", s.AESKey, "to-legacy", "--version", s.EngineVersion, "--no-parallel", "--no-shaders", "--no-script-objects"}
	for _, m := range matches {
		args = append(args, "--filter", m)
	}
	args = append(args, s.PaksDir, outDir)
	err := runToolTTYFn(j, retoc, args, func(line string) {
		if i := strings.Index(line, "Failed to convert"); i >= 0 {
			r := strings.ReplaceAll(line[i:], "../../../"+s.Project+"/Content/", "")
			if len(r) > 600 {
				r = r[:600] + "…"
			}
			if len(why) < 10 {
				why = append(why, r)
				j.Log("retoc: %s", r)
			}
		}
		if m := reExtracted.FindStringSubmatch(line); m != nil {
			ok, _ := strconv.Atoi(m[1])
			failed, _ = strconv.Atoi(m[2])
			matched = ok + failed
		}
	})
	if err != nil {
		j.Log("(retoc could not be asked why: %v)", err)
	}
	return why, matched, failed
}

// unreadableGameFiles tries once more to take out game files that did not come out (into dir), one
// at a time, and explains why when they still do not.
func unreadableGameFiles(j *Job, s Settings, retoc string, missing []string, dir string) error {
	var filters, names []string
	for _, gp := range missing {
		filters = append(filters, strings.TrimPrefix(gp, "/Game/")+".")
		names = append(names, packageBaseName(gp))
	}
	what := strings.Join(names, " and ")
	j.SetStep("Trying again to read "+what, -1)
	j.Log("%s did not come out; trying again, one file at a time.", what)
	why, matched, failed := retocWhy(j, s, retoc, filters, dir)
	all := true
	for _, gp := range missing {
		if _, err := os.Stat(gameFile(dir, s.Project, gp) + ".uasset"); err != nil {
			all = false
		}
	}
	if all {
		j.Log("%s came out on the second try.", what)
		return nil
	}
	report := "If it keeps happening, send a support report (Settings > Copy a support report) to @voidcallsreaper on Discord; it has retoc's full message."
	switch {
	case matched == 0:
		return fmt.Errorf("%s was not found in the game files. A game update may have moved it; %s", what, report)
	case len(why) == 0 && failed > 0:
		return fmt.Errorf("retoc found %s in the game files but could not convert it, and did not say why. %s", what, report)
	case len(why) == 0:
		return fmt.Errorf("%s could not be taken out of the game files. %s", what, report)
	}
	advice := report
	low := strings.ToLower(strings.Join(why, " "))
	if strings.Contains(low, "resolve") || strings.Contains(low, "package store entry") || strings.Contains(low, "imported package") {
		advice = "It refers to another game file that could not be read, which usually means a game file is damaged or missing: in Steam, right-click ACE COMBAT 8 > Properties > Installed Files > Verify integrity of game files, then try again. " + report
	}
	return fmt.Errorf("%s could not be taken out of the game files. retoc said: %s. %s", what, why[0], advice)
}

// ------------------------------------------------------------------ previews

// SkinPreview is one small picture of a skin, made before deciding what to extract.
type SkinPreview struct {
	Skin    string `json:"skin"`
	Texture string `json:"texture"`
	Tiles   int    `json:"tiles"`
	Rows    int    `json:"rows"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

type previewIndex struct {
	Vehicle  string        `json:"vehicle"`
	Previews []SkinPreview `json:"previews"`
	Missing  []string      `json:"missing"` // skin folders that have no picture this program can show
}

func findVehicle(s Settings, name string) (Vehicle, bool) {
	name = strings.Trim(strings.TrimSpace(strings.ReplaceAll(name, `\`, "/")), "/")
	for _, v := range loadVehicles(s) {
		if strings.EqualFold(v.Name, name) || strings.EqualFold(v.Path, name) {
			return v, true
		}
	}
	return Vehicle{}, false
}

func previewDirFor(s Settings, v Vehicle) string {
	return filepath.Join(s.previewsDir(), safeName(v.Name))
}

func loadPreviews(s Settings, vehicle string) previewIndex {
	idx := previewIndex{Previews: []SkinPreview{}, Missing: []string{}}
	v, ok := findVehicle(s, vehicle)
	if !ok {
		return idx
	}
	idx.Vehicle = v.Name
	if b, err := os.ReadFile(filepath.Join(previewDirFor(s, v), "index.json")); err == nil {
		json.Unmarshal(b, &idx)
	}
	if idx.Previews == nil {
		idx.Previews = []SkinPreview{}
	}
	if idx.Missing == nil {
		idx.Missing = []string{}
	}
	return idx
}

// faceTexture picks the texture that best shows what a skin looks like: its colour texture.
func faceTexture(names []string) string {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	for _, n := range sorted {
		if GuessKind(n) == KindColour {
			return n
		}
	}
	if len(sorted) > 0 {
		return sorted[0]
	}
	return ""
}

// doPreview pulls out only each skin's colour texture, makes a small picture of it, and deletes
// the full-size files again straight away. Only the small pictures stay on disk.
func doPreview(j *Job, s Settings, vehicle string) (any, error) {
	retoc, err := needExtractSetup(s)
	if err != nil {
		return nil, err
	}
	v, ok := findVehicle(s, vehicle)
	if !ok {
		return nil, errors.New("pick an aircraft from the list first (press Find aircraft in the game files if the list is empty)")
	}
	if len(v.Skins) == 0 {
		return nil, errors.New("this aircraft has no skin folders")
	}
	tmp := s.previewTmp()
	os.RemoveAll(tmp)
	defer os.RemoveAll(tmp)
	outDir := previewDirFor(s, v)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	idx := previewIndex{Vehicle: v.Name, Previews: []SkinPreview{}, Missing: []string{}}
	const batch = 6 // full-size files on disk at any one time
	for start := 0; start < len(v.Skins); start += batch {
		if j.Cancelled() {
			return nil, errors.New("cancelled")
		}
		group := v.Skins[start:min(start+batch, len(v.Skins))]
		j.SetStep(fmt.Sprintf("Fetching skins %d to %d of %d", start+1, start+len(group), len(v.Skins)), float64(start)/float64(len(v.Skins)))
		var matches []string
		for _, skin := range group {
			if face := faceTexture(v.Textures[skin]); face != "" {
				matches = append(matches, v.Path+"/Textures/"+skin+"/"+face+".")
			} else {
				matches = append(matches, v.Path+"/Textures/"+skin+"/") // older aircraft list without texture names
			}
		}
		os.RemoveAll(tmp)
		if err := os.MkdirAll(tmp, 0o755); err != nil {
			return nil, err
		}
		if err := retocToLegacy(j, s, retoc, matches, tmp); err != nil {
			return nil, err
		}
		// what came out, by skin folder
		bySkin := map[string][]string{}
		filepath.WalkDir(tmp, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".uexp") {
				skin := filepath.Base(filepath.Dir(p))
				bySkin[skin] = append(bySkin[skin], p)
			}
			return nil
		})
		for gi, skin := range group {
			j.SetStep(fmt.Sprintf("Making the preview of %s (%d of %d)", skin, start+gi+1, len(v.Skins)), float64(start+gi)/float64(len(v.Skins)))
			var names []string
			paths := map[string]string{}
			for _, p := range bySkin[skin] {
				n := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
				names = append(names, n)
				paths[n] = p
			}
			done := false
			// try the colour texture first, then anything else that can be shown
			order := []string{faceTexture(names)}
			sort.Strings(names)
			order = append(order, names...)
			for _, n := range order {
				if n == "" || done {
					continue
				}
				png, t, err := texturePreviewPNG(paths[n], 1400)
				if err != nil {
					continue
				}
				if err := os.WriteFile(filepath.Join(outDir, safeName(skin)+".png"), png, 0o644); err != nil {
					return nil, err
				}
				idx.Previews = append(idx.Previews, SkinPreview{Skin: skin, Texture: n, Tiles: t.BlocksW, Rows: t.BlocksH, Width: t.Width, Height: t.Height})
				done = true
			}
			if !done {
				idx.Missing = append(idx.Missing, skin)
			}
		}
	}
	os.RemoveAll(tmp)
	b, _ := json.Marshal(idx)
	if err := os.WriteFile(filepath.Join(outDir, "index.json"), b, 0o644); err != nil {
		return nil, err
	}
	j.Log("Made %d previews for %s. The full-size files fetched for them have been deleted.", len(idx.Previews), v.Name)
	if len(idx.Previews) == 0 {
		return idx, errors.New("none of this aircraft's skin textures could be shown")
	}
	return idx, nil
}

// texturePreviewPNG makes a small PNG of a raw texture from one of its stored smaller copies.
func texturePreviewPNG(uexpPath string, maxW int) ([]byte, *VTex, error) {
	t, _, ubulk, err := LoadVTex(texFilesFor(uexpPath))
	if err != nil {
		return nil, nil, err
	}
	mip := 0
	for mip < t.Mips-1 && t.Width>>uint(mip) > maxW {
		mip++
	}
	im, err := t.DecodeMip(ubulk, mip)
	if err != nil {
		return nil, nil, err
	}
	var buf bytes.Buffer
	if err := encodePNG(&buf, im, SaveRGB); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), t, nil
}

func imageExt(s Settings, format string) string {
	switch format {
	case "tga":
		return ".tga"
	case "png":
		return ".png"
	}
	return "." + s.ImageFormat
}

// exportTextures turns raw textures into tiles under the paint-ready folder.
func exportTextures(j *Job, s Settings, texs []LibTexture, painter bool, format string) (string, int, error) {
	ext := imageExt(s, format)
	var todo []LibTexture
	for _, t := range texs {
		if t.Width > 0 && t.Format != "" && (t.Supported || t.Reason != "") && t.BlocksW > 0 {
			todo = append(todo, t)
		}
	}
	if len(todo) == 0 {
		return "", 0, errors.New("none of these textures is a skin texture this program can read")
	}
	written := 0
	root := ""
	var firstErr error
	for i, t := range todo {
		if j.Cancelled() {
			return root, written, errors.New("cancelled")
		}
		j.SetStep(fmt.Sprintf("Making tiles for %s (%d of %d)", t.Name, i+1, len(todo)), float64(i)/float64(len(todo)))
		dir := paintDirFor(s, t)
		if root == "" {
			root = dir
		} else {
			root = commonDir(root, dir)
		}
		files, err := ExportTexture(texFilesFor(t.UExp), painter, dir, ext, nil)
		if err != nil {
			j.Log("%s: %v", t.Name, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %v", t.Name, err)
			}
			continue
		}
		written += len(files)
		j.Log("%s: wrote %d files to %s", t.Name, len(files), dir)
		if t.TileNote != "" {
			j.Log("%s: %s Those tiles are saved at that size.", t.Name, t.TileNote)
		}
	}
	if written == 0 && firstErr != nil {
		return root, 0, firstErr
	}
	return root, written, nil
}

func paintDirFor(s Settings, t LibTexture) string {
	parts := strings.Split(strings.TrimPrefix(t.Package, "/Game/"), "/")
	aircraft, folder := "", ""
	if len(parts) >= 2 {
		folder = parts[len(parts)-2]
		for i := len(parts) - 3; i >= 0; i-- {
			if !strings.EqualFold(parts[i], "Textures") {
				aircraft = parts[i]
				break
			}
		}
	}
	if aircraft == "" {
		aircraft = filepath.Base(filepath.Dir(filepath.Dir(t.UExp)))
	}
	if folder == "" {
		folder = filepath.Base(filepath.Dir(t.UExp))
	}
	return filepath.Join(s.paintDir(), safeName(aircraft), safeName(folder))
}

func commonDir(a, b string) string {
	for a != b {
		if len(a) > len(b) {
			a = filepath.Dir(a)
		} else {
			b = filepath.Dir(b)
		}
		if a == filepath.Dir(a) {
			return a
		}
	}
	return a
}

var reUnsafe = regexp.MustCompile(`[^A-Za-z0-9_\-]+`)

func safeName(n string) string {
	n = strings.Trim(reUnsafe.ReplaceAllString(n, "_"), "_")
	if len(n) > 48 {
		n = n[:48]
	}
	return n
}

type exportReq struct {
	UExps   []string `json:"uexps"`
	Painter bool     `json:"painter"`
	Format  string   `json:"format"`
}

func doExport(j *Job, s Settings, r exportReq) (any, error) {
	var texs []LibTexture
	for _, p := range r.UExps {
		if lt, ok := describeTexture(p); ok {
			texs = append(texs, lt)
		}
	}
	out, n, err := exportTextures(j, s, texs, r.Painter, r.Format)
	return map[string]any{"paintDir": out, "files": n}, err
}

// ------------------------------------------------------------------ plan and build

type planReq struct {
	SkinDir   string   `json:"skinDir"`
	PaintDir  string   `json:"paintDir"`
	Files     []string `json:"files"`
	ModName   string   `json:"modName"`
	Textures  []string `json:"textures"` // names to rebuild (build only)
	Install   bool     `json:"install"`
	Cleanup   []string `json:"cleanup"`
	KeepInput bool     `json:"keepInput"`
	// Choices says which set of pictures to use where several were found (channel key -> set name).
	Choices map[string]string `json:"choices"`
	// Extras changes the name, description or icon the game shows for the replaced skin.
	Extras *skinExtras `json:"extras,omitempty"`
	// Tex says how the colour texture is stored (BC7, no mipmaps); the others keep the game's way.
	Tex TexOptions `json:"tex"`
}

type planRow struct {
	LibTexture
	Recipe  Recipe `json:"recipe"`
	Default bool   `json:"default"`
	Warning string `json:"warning,omitempty"`
}

func skinTextures(dir string) ([]LibTexture, error) {
	dir = cleanPath(dir)
	if dir == "" {
		return nil, errors.New("choose the skin to replace first")
	}
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s", dir)
	}
	var out []LibTexture
	for _, e := range es {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".uexp") {
			continue
		}
		if lt, ok := describeTexture(filepath.Join(dir, e.Name())); ok {
			out = append(out, lt)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("that folder has no raw textures (.uasset, .uexp and .ubulk files)")
	}
	return out, nil
}

func paintedFiles(r planReq) ([]string, error) {
	files := append([]string(nil), r.Files...)
	if d := cleanPath(r.PaintDir); d != "" {
		more, err := listImages(d)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s", d)
		}
		files = append(files, more...)
	}
	return files, nil
}

func doPlan(r planReq) ([]planRow, int, error) {
	texs, err := skinTextures(r.SkinDir)
	if err != nil {
		return nil, 0, err
	}
	files, err := paintedFiles(r)
	if err != nil {
		return nil, 0, err
	}
	var names []string
	for _, t := range texs {
		names = append(names, t.Name)
	}
	sets := FindPainted(files, names, r.Choices)
	var rows []planRow
	for _, t := range texs {
		row := planRow{LibTexture: t}
		if !t.Supported {
			row.Recipe = Recipe{Missing: t.Reason}
			rows = append(rows, row)
			continue
		}
		vt, err := readTexDesc(texFilesFor(t.UExp))
		if err != nil {
			row.Recipe = Recipe{Missing: err.Error()}
			rows = append(rows, row)
			continue
		}
		row.Recipe = PlanRecipe(t.Kind, t.Name, vt, sets)
		_, direct := sets["tex:"+t.Name]
		row.Default = row.Recipe.Ready && t.Kind == KindColour
		switch {
		case !row.Recipe.Ready:
		case t.Kind == KindNormal && !t.Plain:
			row.Warning = "Experimental. This normal map is usually shared by every skin of the aircraft, and the rebuilt copy is a little less sharp."
		case t.Kind == KindFinish && !direct:
			row.Warning = "Not yet confirmed in game. It uses the same format as the colour texture."
		}
		rows = append(rows, row)
	}
	return rows, len(files), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// installBuiltMod copies a packed replacement mod into its own folder in the mods folder, and
// notes in res where it went and which installed mods replace the same game files.
func installBuiltMod(j *Job, s Settings, mod string, files []string, res map[string]any) error {
	dest := filepath.Join(s.ModsDir, mod)
	if err := installFiles(dest, files); err != nil {
		return fmt.Errorf("the mod was built, but %v", err)
	}
	var installed []string
	for _, f := range files {
		installed = append(installed, filepath.Join(dest, filepath.Base(f)))
	}
	res["installedDir"], res["installedFiles"] = dest, installed
	j.Log("Installed into %s", dest)
	for _, m := range scanMods(s.ModsDir) {
		if strings.EqualFold(m.Dir, dest) && len(m.Conflicts) > 0 {
			res["conflicts"] = m.Conflicts
			for _, c := range m.Conflicts {
				j.Log("Note: %s replaces the same game files as the installed mod %s.", mod, c.With)
			}
		}
	}
	return nil
}

func doBuild(j *Job, s Settings, r planReq) (any, error) {
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
	rows, _, err := doPlan(r)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, n := range r.Textures {
		want[n] = true
	}
	var todo []planRow
	for _, row := range rows {
		if want[row.Name] {
			if !row.Recipe.Ready {
				return nil, fmt.Errorf("%s cannot be rebuilt: %s", row.Name, row.Recipe.Missing)
			}
			todo = append(todo, row)
		}
	}
	if len(todo) == 0 {
		return nil, errors.New("tick at least one texture to rebuild")
	}
	files, _ := paintedFiles(r)
	var names []string
	for _, row := range rows {
		names = append(names, row.Name)
	}
	sets := FindPainted(files, names, r.Choices)

	modDir := filepath.Join(s.modsOut(), mod)
	input := filepath.Join(modDir, "pack_input")
	packed := filepath.Join(modDir, "packed")
	os.RemoveAll(input)
	var notes []string
	var paintColour *[3]uint8
	for i, row := range todo {
		if j.Cancelled() {
			return nil, errors.New("cancelled")
		}
		base := float64(i) / float64(len(todo)) * 0.85
		span := 0.85 / float64(len(todo))
		j.SetStep(fmt.Sprintf("Rebuilding %s (%d of %d)", row.Name, i+1, len(todo)), base)
		f := texFilesFor(row.UExp)
		vt, uexp, ubulk, err := LoadVTex(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", row.Name, err)
		}
		img, ns, err := Compose(row.Kind, row.Name, vt, ubulk, sets)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", row.Name, err)
		}
		if row.Kind == KindColour && paintColour == nil {
			c := MainColour(img)
			paintColour = &c
		}
		for _, n := range ns {
			j.Log("Note: %s", n)
			notes = append(notes, n)
		}
		ua, err := os.ReadFile(f.UAsset)
		if err != nil {
			return nil, err
		}
		opts := r.Tex.forKind(row.Kind)
		newUA, newExp, newBulk, err := rebuildTexture(vt, ua, uexp, ubulk, img, opts, func(d, t int) {
			j.SetStep(fmt.Sprintf("Rebuilding %s (%d of %d)", row.Name, i+1, len(todo)), base+span*float64(d)/float64(t))
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %v", row.Name, err)
		}
		rel := filepath.Join(s.Project, "Content", filepath.FromSlash(strings.TrimPrefix(row.Package, "/Game/")))
		dst := filepath.Join(input, rel)
		if err := writePackageFiles(dst, newUA, newExp, newBulk); err != nil {
			return nil, err
		}
		j.Log("%s rebuilt (%s)%s", row.Name, strings.Join(row.Recipe.Uses, "; "), texNote(opts))
	}
	if j.Cancelled() {
		return nil, errors.New("cancelled")
	}
	if r.Extras.wanted() {
		j.SetStep("Changing the skin's name, description and icon", 0.86)
		ns, err := applySkinExtras(j, s, r, mod, input, paintColour)
		if err != nil {
			return nil, err
		}
		notes = append(notes, ns...)
	}
	j.SetStep("Packing the mod", 0.88)
	outFiles, err := packFolder(j, s, input, packed, mod+"_P")
	if err != nil {
		return nil, err
	}
	res := map[string]any{"mod": mod, "packedDir": packed, "files": outFiles, "notes": notes, "textures": len(todo)}
	if !r.KeepInput {
		os.RemoveAll(input)
	}
	if r.Install {
		j.SetStep("Copying into the mods folder", 0.96)
		if err := installBuiltMod(j, s, mod, outFiles, res); err != nil {
			return res, err
		}
	}
	cleanInbox(s, r.Cleanup)
	j.Log("Mod %s is ready: %s", mod, packed)
	return res, nil
}
