package main

// Settings kept in settings.json next to the program, and the folders the program works in.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const appName = "ACE8 Skin Studio"
const appVersion = "2.4.5"

type Settings struct {
	Theme          string   `json:"theme"` // system, light, dark
	PaksDir        string   `json:"paksDir"`
	AESKey         string   `json:"aesKey"`
	Usmap          string   `json:"usmap"`
	RetocPath      string   `json:"retocPath"`
	ModsDir        string   `json:"modsDir"`
	UE4SSMods      string   `json:"ue4ssMods"` // UE4SS's Mods folder (found by itself when empty)
	Workspace      string   `json:"workspace"`
	ExtraRawDirs   []string `json:"extraRawDirs"`
	EngineVersion  string   `json:"engineVersion"`
	Project        string   `json:"project"`
	ImageFormat    string   `json:"imageFormat"` // png or tga
	DefaultBrowser bool     `json:"defaultBrowser"`
	HideTutorial   bool     `json:"hideTutorial"`  // true once "Don't show this when the program starts" is ticked
	SaveDir        string   `json:"saveDir"`       // the game's SaveGames folder (found by itself when empty)
	SaveBackupOff  bool     `json:"saveBackupOff"` // the game-start save backup was taken out, so it is not put back
	// UnlockAtStart gives every new skin and emblem a second unlock rule with no conditions, which
	// the game may act on at start (an experiment, see docs/dlc-research.md)
	UnlockAtStart bool `json:"unlockAtStart"`
	// Tex is how colour textures were last asked to be stored (the Repack screen's boxes)
	Tex TexOptions `json:"tex"`
}

var (
	dataDir  string // folder that holds settings.json (normally the program's own folder)
	setMu    sync.Mutex
	settings Settings
	saveMu   sync.Mutex // one save at a time, so settings.json and the settings in use stay the same
	logFile  *os.File
	logMu    sync.Mutex

	// settingsProblem says, in plain words, why settings.json could not be read when the program
	// started ("" when it could).
	settingsProblem string
)

func logf(format string, a ...any) {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		fmt.Fprintf(logFile, "%s %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
	}
}

func defaults(s *Settings) {
	if s.Theme != "light" && s.Theme != "system" {
		s.Theme = "dark"
	}
	if s.PaksDir == "" {
		s.PaksDir = platformPaksDir()
	}
	if s.Workspace == "" {
		s.Workspace = defaultWorkspace()
	}
	if s.EngineVersion == "" {
		s.EngineVersion = "UE5_4"
	}
	if s.Project == "" {
		s.Project = "Live"
	}
	if s.ImageFormat != "tga" {
		s.ImageFormat = "png"
	}
	s.AESKey = strings.TrimSpace(s.AESKey)
	for _, p := range []*string{&s.PaksDir, &s.Usmap, &s.RetocPath, &s.ModsDir, &s.Workspace, &s.UE4SSMods, &s.SaveDir} {
		*p = cleanPath(*p)
	}
	var keep []string
	for _, d := range s.ExtraRawDirs {
		if d = cleanPath(d); d != "" {
			keep = append(keep, d)
		}
	}
	s.ExtraRawDirs = keep
}

// cleanPath trims spaces and the quotes Windows adds with "Copy as path".
func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"`)
	return strings.TrimSpace(p)
}

func loadSettings() {
	setMu.Lock()
	defer setMu.Unlock()
	settings = Settings{}
	settingsProblem = ""
	b, err := os.ReadFile(filepath.Join(dataDir, "settings.json"))
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &settings); err != nil {
			settingsProblem = keepBrokenSettings(b, err)
		}
	case !os.IsNotExist(err):
		logf("settings.json could not be read: %v", err)
		settingsProblem = "settings.json could not be read when the program started (" + err.Error() + "), so the settings start empty."
	}
	defaults(&settings)
}

// keepBrokenSettings keeps a copy of a settings.json that could not be read, so the AES key and the
// folders in it are not lost when the settings are saved again, and says so in plain words.
func keepBrokenSettings(b []byte, err error) string {
	logf("settings.json could not be read: %v", err)
	if len(bytes.TrimSpace(b)) == 0 {
		return "settings.json was empty when the program started (the computer may have stopped while it was being saved), so the settings start fresh."
	}
	why, lost := "settings.json could not be read when the program started (it has a mistake in it, such as a single \\ in a folder path where two are needed)", "the settings start empty"
	var se *json.SyntaxError
	if !errors.As(err, &se) { // one value of the wrong kind: the rest was read
		why, lost = "A setting in settings.json could not be read when the program started ("+err.Error()+")", "that setting starts empty"
	}
	// the same file kept on an earlier start is not copied again
	keep := ""
	olds, _ := filepath.Glob(filepath.Join(dataDir, "settings.broken*.json"))
	for _, o := range olds {
		if ob, err := os.ReadFile(o); err == nil && bytes.Equal(ob, b) {
			keep = o
			break
		}
	}
	if keep == "" {
		keep = filepath.Join(dataDir, "settings.broken-"+time.Now().Format("20060102-150405")+".json")
		if err := writeWhole(keep, b, 0o644); err != nil {
			logf("a copy of settings.json could not be kept: %v", err)
			return why + ", and no copy of it could be kept (" + err.Error() + "). Fix it by hand before changing anything here, or the key and folders in it will be replaced."
		}
		logf("settings.json kept as %s", keep)
	}
	return why + ", so " + lost + ". Nothing is lost: a copy of the file is kept as " + keep + ". Enter what is missing again here, or copy it from that file."
}

// settingsNote is why settings.json could not be read at the start ("" when it could).
func settingsNote() string {
	setMu.Lock()
	defer setMu.Unlock()
	return settingsProblem
}

func getSettings() Settings {
	setMu.Lock()
	defer setMu.Unlock()
	s := settings
	s.ExtraRawDirs = append([]string(nil), settings.ExtraRawDirs...)
	return s
}

func saveSettings(s Settings) error {
	defaults(&s)
	b, _ := json.MarshalIndent(s, "", "  ")
	saveMu.Lock()
	defer saveMu.Unlock()
	if err := writeWhole(filepath.Join(dataDir, "settings.json"), b, 0o644); err != nil {
		return fmt.Errorf("could not save settings: %v", err)
	}
	setMu.Lock()
	settings = s
	setMu.Unlock()
	return nil
}

// writeWhole writes data to a new file next to path and only then puts it in path's place, so path
// never holds half a file (a full disk, or the computer stopping part-way through).
func writeWhole(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".part"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// Workspace sub-folders.
func (s Settings) rawDir() string    { return filepath.Join(s.Workspace, "raw") }
func (s Settings) paintDir() string  { return filepath.Join(s.Workspace, "paint-ready") }
func (s Settings) splitDir() string  { return filepath.Join(s.Workspace, "split") }
func (s Settings) inboxDir() string  { return filepath.Join(s.Workspace, "inbox") }
func (s Settings) modsOut() string   { return filepath.Join(s.Workspace, "mods") }
func (s Settings) modelsDir() string { return filepath.Join(s.Workspace, "models") }

// removedModsDir is where mods taken out of the mods folder are kept, so nothing is ever deleted.
func (s Settings) removedModsDir() string { return filepath.Join(s.Workspace, "removed-mods") }

// previewsDir holds the small preview pictures; previewTmp holds full-size files only while previews are made.
func (s Settings) previewsDir() string { return filepath.Join(s.Workspace, "previews") }
func (s Settings) previewTmp() string  { return filepath.Join(s.Workspace, "preview-tmp") }

// findRetoc returns the retoc program to use, or "" when it cannot be found.
func (s Settings) findRetoc() string {
	cands := []string{s.RetocPath}
	if s.RetocPath != "" {
		cands = append(cands, retocCandidates(s.RetocPath)...) // a folder was given
	}
	cands = append(cands, retocCandidates(dataDir)...)
	if p, err := exec.LookPath("retoc"); err == nil {
		cands = append(cands, p)
	}
	// The first one made for this computer; when none is, the first one there, so Settings can say
	// what is wrong with it (the Windows retoc.exe chosen on Linux, for example).
	first := ""
	for _, c := range cands {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			if retocSuitsHere(c) {
				return c
			}
			if first == "" {
				first = c
			}
		}
	}
	return first
}

var reAES = regexp.MustCompile(`^(0x)?[0-9a-fA-F]{64}$`)

type check struct {
	OK   bool   `json:"ok"`
	Note string `json:"note"`
}

// checks reports what is set up and what is still missing, in plain words.
// hostPath writes a Windows path hint the way this system writes paths: on Linux with /, and the
// save folder as it is inside Proton's prefix.
func hostPath(p string) string {
	if runtime.GOOS == "windows" {
		return p
	}
	p = strings.ReplaceAll(p, `\`, "/")
	if strings.HasPrefix(p, "…/BANDAI NAMCO") {
		p = "…/steamapps/compatdata/<game>/pfx/drive_c/users/steamuser/AppData/Local" + strings.TrimPrefix(p, "…")
	}
	return p
}

// defaultWorkspace is the workspace folder used when none is chosen: next to the program.
func defaultWorkspace() string { return filepath.Join(dataDir, "workspace") }

// workspaceProblem tells, in plain words, why the workspace folder cannot be used ("" when it can).
// It makes the folder when it is not there yet.
func workspaceProblem(dir string) string {
	if dir == "" {
		return "No workspace folder is chosen."
	}
	err := os.MkdirAll(dir, 0o755)
	if err == nil {
		probe := filepath.Join(dir, ".write-test")
		if err = os.WriteFile(probe, []byte("ok"), 0o644); err == nil {
			os.Remove(probe)
			return ""
		}
	}
	if vol := filepath.VolumeName(dir); vol != "" {
		if _, e := os.Stat(vol + string(filepath.Separator)); e != nil {
			if strings.HasPrefix(vol, `\\`) {
				return "The workspace folder is on the network share " + vol + ", which cannot be reached right now."
			}
			return "The workspace folder is on drive " + vol + ", which is not on this computer right now (an unplugged drive, or settings copied from another PC?)."
		}
	}
	if st, e := os.Stat(dir); e == nil && !st.IsDir() {
		return "The workspace folder is a file, not a folder."
	}
	if os.IsPermission(err) {
		if runtime.GOOS != "windows" {
			return "This program is not allowed to write to the workspace folder (a folder owned by another user, or a read-only drive?)."
		}
		return "Windows does not let this program write to the workspace folder (a protected folder such as Program Files?)."
	}
	return "The workspace folder cannot be used (" + err.Error() + ")."
}

func (s Settings) checks() map[string]check {
	out := map[string]check{}
	if p := workspaceProblem(s.Workspace); p != "" {
		out["workspace"] = check{false, p + " Choose another folder, or press \"Use the folder next to the program\"."}
	} else {
		out["workspace"] = check{true, "Folder ready"}
	}
	if r := s.findRetoc(); r == "" {
		out["retoc"] = check{false, retocMissingNote()}
	} else if p := retocProblem(r); p != "" {
		out["retoc"] = check{false, p}
	} else {
		out["retoc"] = check{true, r}
	}
	switch {
	case s.PaksDir == "":
		out["paks"] = check{false, "Choose the game's Paks folder (" + hostPath(`…\ACE COMBAT 8\Game\Content\Paks`) + ")."}
	default:
		n := 0
		if es, err := os.ReadDir(s.PaksDir); err == nil {
			for _, e := range es {
				if strings.EqualFold(filepath.Ext(e.Name()), ".utoc") {
					n++
				}
			}
		}
		if n == 0 {
			out["paks"] = check{false, "No .utoc files in that folder. It should be the game's Content\\Paks folder."}
		} else {
			out["paks"] = check{true, fmt.Sprintf("%d game containers found", n)}
		}
	}
	switch {
	case s.AESKey == "":
		out["aes"] = check{false, "Enter the game's AES key (64 hexadecimal characters, optionally starting with 0x)."}
	case !reAES.MatchString(s.AESKey):
		out["aes"] = check{false, "That is not a valid key: it must be 64 hexadecimal characters, optionally starting with 0x."}
	default:
		out["aes"] = check{true, "Key has the right shape"}
	}
	// a settings.json that could not be read is why the key has gone: say so where it is asked for
	if note := settingsNote(); note != "" {
		if a := out["aes"]; !a.OK {
			out["aes"] = check{false, note + " " + a.Note}
		}
		out["settings"] = check{false, note}
	}
	switch {
	case s.Usmap == "":
		out["usmap"] = check{true, "Not set. Nothing needs it; the skin list layouts are built in."}
	default:
		if st, err := os.Stat(s.Usmap); err != nil || st.IsDir() {
			out["usmap"] = check{false, "That mappings file does not exist."}
		} else {
			out["usmap"] = check{true, "File found"}
		}
	}
	if d := s.saveGamesDir(); d == "" {
		out["save"] = check{false, "Not found. Choose the game's SaveGames folder (" + hostPath(`…\BANDAI NAMCO Entertainment\ACE COMBAT 8\Saved\SaveGames`) + ")."}
	} else if _, err := os.Stat(filepath.Join(d, campaignSave)); err == nil {
		out["save"] = check{true, "Found: " + d}
	} else if s.SaveDir != "" {
		out["save"] = check{false, "That folder has no " + campaignSave + "."}
	} else {
		out["save"] = check{false, "No save found yet in " + d + ". Play the game once, or choose the folder that holds " + campaignSave + "."}
	}
	if d := s.ue4ssModsDir(); d != "" {
		out["ue4ss"] = check{true, "Found: " + d}
	} else if s.UE4SSMods != "" {
		out["ue4ss"] = check{false, "That folder does not hold UE4SS's mods (mods.txt)."}
	} else {
		out["ue4ss"] = check{false, "Not found yet. New skins need it for their names; it is found by itself when the mods folder is inside " + hostPath(`ue4ss\Mods`) + "."}
	}
	switch {
	case s.ModsDir == "":
		out["mods"] = check{false, "Choose the folder your mod loader reads (" + hostPath(`…\ACE COMBAT 8\Game\Binaries\Win64\ue4ss\Mods\IoStoreLoaderMod`) + "). New skins, emblems and imported skin mods are installed there."}
	default:
		if st, err := os.Stat(s.ModsDir); err != nil || !st.IsDir() {
			out["mods"] = check{false, "That folder does not exist."}
		} else {
			out["mods"] = check{true, "Folder found"}
		}
	}
	return out
}
