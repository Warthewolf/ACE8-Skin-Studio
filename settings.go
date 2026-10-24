package main

// Settings kept in settings.json next to the program, and the folders the program works in.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const appName = "ACE8 Skin Studio"
const appVersion = "2.4.4"

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
	// Tex is how colour textures were last asked to be stored (the Repack screen's boxes)
	Tex TexOptions `json:"tex"`
}

var (
	dataDir  string // folder that holds settings.json (normally the program's own folder)
	setMu    sync.Mutex
	settings Settings
	logFile  *os.File
	logMu    sync.Mutex
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
	if b, err := os.ReadFile(filepath.Join(dataDir, "settings.json")); err == nil {
		json.Unmarshal(b, &settings)
	}
	defaults(&settings)
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
	if err := os.WriteFile(filepath.Join(dataDir, "settings.json"), b, 0o644); err != nil {
		return fmt.Errorf("could not save settings: %v", err)
	}
	setMu.Lock()
	settings = s
	setMu.Unlock()
	return nil
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
		cands = append(cands, filepath.Join(s.RetocPath, exeName)) // a folder was given
	}
	cands = append(cands, filepath.Join(dataDir, exeName), filepath.Join(dataDir, "retoc", exeName))
	for _, c := range cands {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("retoc"); err == nil {
		return p
	}
	return ""
}

var reAES = regexp.MustCompile(`^(0x)?[0-9a-fA-F]{64}$`)

type check struct {
	OK   bool   `json:"ok"`
	Note string `json:"note"`
}

// checks reports what is set up and what is still missing, in plain words.
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
	if r := s.findRetoc(); r != "" {
		out["retoc"] = check{true, r}
	} else {
		out["retoc"] = check{false, "retoc was not found. Download it and choose " + exeName + " here, or put " + exeName + " next to this program."}
	}
	switch {
	case s.PaksDir == "":
		out["paks"] = check{false, "Choose the game's Paks folder (…\\ACE COMBAT 8\\Game\\Content\\Paks)."}
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
		out["save"] = check{false, "Not found. Choose the game's SaveGames folder (…\\BANDAI NAMCO Entertainment\\ACE COMBAT 8\\Saved\\SaveGames)."}
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
		out["ue4ss"] = check{false, "Not found yet. New skins need it for their names; it is found by itself when the mods folder is inside ue4ss\\Mods."}
	}
	switch {
	case s.ModsDir == "":
		out["mods"] = check{false, "Choose your mods folder if you want finished mods copied there automatically."}
	default:
		if st, err := os.Stat(s.ModsDir); err != nil || !st.IsDir() {
			out["mods"] = check{false, "That folder does not exist."}
		} else {
			out["mods"] = check{true, "Folder found"}
		}
	}
	return out
}
