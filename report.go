package main

// The support report: one block of text a person can paste into a chat when asking for help.
// It never contains the AES key, and the person's Windows user name is taken out of paths.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// scrubber hides things that should not leave the computer.
type scrubber struct {
	key  string
	home string
}

func newScrubber(s Settings) scrubber {
	sc := scrubber{key: strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(s.AESKey), "0x"), "0X")}
	if h, err := os.UserHomeDir(); err == nil && len(h) > 3 {
		sc.home = h
	}
	return sc
}

var reLongHex = regexp.MustCompile(`(?i)(0x)?[0-9a-f]{64}`)

func replaceFold(text, old, new string) string {
	if old == "" {
		return text
	}
	lowText, lowOld := strings.ToLower(text), strings.ToLower(old)
	if len(lowText) != len(text) || len(lowOld) != len(old) {
		return strings.ReplaceAll(text, old, new) // unusual letters: fall back to an exact match
	}
	var b strings.Builder
	for {
		i := strings.Index(lowText, lowOld)
		if i < 0 {
			b.WriteString(text)
			return b.String()
		}
		b.WriteString(text[:i])
		b.WriteString(new)
		text, lowText = text[i+len(old):], lowText[i+len(old):]
	}
}

func (sc scrubber) clean(text string) string {
	text = reLongHex.ReplaceAllString(text, "<hidden>") // anything shaped like a key
	if len(sc.key) >= 16 {
		text = replaceFold(text, sc.key, "<hidden>")
	}
	if sc.home != "" {
		text = replaceFold(text, sc.home, "<home>")
		text = replaceFold(text, strings.ReplaceAll(sc.home, `\`, "/"), "<home>")
	}
	return text
}

// retocVersion asks retoc for its version; "" when it does not answer quickly.
func retocVersion(path string) string {
	if path == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if len(v) > 60 {
		v = v[:60]
	}
	return v
}

func tailOfFile(path string, lines int) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if len(b) > 256<<10 {
		b = b[len(b)-256<<10:]
	}
	all := strings.Split(strings.TrimRight(strings.ReplaceAll(string(b), "\r", ""), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return all
}

// buildReport writes the support report.
func buildReport() string {
	s := getSettings()
	sc := newScrubber(s)
	c := s.checks()
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	state := func(k string) string {
		if c[k].OK {
			return "ok"
		}
		return "PROBLEM"
	}
	orNone := func(v string) string {
		if v == "" {
			return "(not set)"
		}
		return v
	}
	line("%s %s report", appName, appVersion)
	line("Made %s on %s/%s", time.Now().Format("2006-01-02 15:04"), runtime.GOOS, runtime.GOARCH)
	line("")
	line("Setup")
	line("  Game Paks folder: %s [%s: %s]", orNone(s.PaksDir), state("paks"), c["paks"].Note)
	aes := "not set"
	if s.AESKey != "" {
		aes = "set, never included in this report"
	}
	line("  AES key: %s [%s: %s]", aes, state("aes"), c["aes"].Note)
	line("  Mappings file: %s [%s]", orNone(s.Usmap), c["usmap"].Note)
	retoc := s.findRetoc()
	rv := retocVersion(retoc)
	if rv == "" {
		rv = "version not reported"
	}
	if retoc == "" {
		line("  retoc: NOT FOUND (setting: %s)", orNone(s.RetocPath))
	} else {
		line("  retoc: %s (%s)", retoc, rv)
		if fileExists(filepath.Join(filepath.Dir(retoc), oodleName)) {
			line("  Oodle (%s): next to retoc", oodleName)
		} else {
			line("  Oodle (%s): NOT next to retoc yet (got on the next extract)", oodleName)
		}
	}
	line("  Mods folder: %s [%s: %s]", orNone(s.ModsDir), state("mods"), c["mods"].Note)
	line("  Workspace: %s", s.Workspace)
	if len(s.ExtraRawDirs) > 0 {
		line("  Extra raw folders: %s", strings.Join(s.ExtraRawDirs, "; "))
	}
	window := "own window (Edge)"
	if s.DefaultBrowser {
		window = "default browser"
	}
	line("  Engine %s, project %s, tiles saved as %s, shown in %s; colour textures stored as %s", s.EngineVersion, s.Project, s.ImageFormat, window, s.Tex.way())
	if s.ModsDir != "" {
		mods := scanMods(s.ModsDir)
		clashes := 0
		for _, m := range mods {
			if len(m.Conflicts) > 0 {
				clashes++
			}
		}
		line("  Installed mods: %d, of which %d replace the same files as another mod", len(mods), clashes)
		ov := slotsOverviewNow(s)
		n := 0
		for _, m := range ov.Mods {
			n += len(m.Slots)
		}
		line("  New skin slots: %d skins in %d skin mods; mod loader %s; SkinSlotText installed %v; UE4SS Mods: %s", n, len(ov.Mods), orNone(ov.Loader), ov.SST.Installed, orNone(ov.SST.Dir))
		if ov.Stale != "" {
			line("  Skin list: %s", ov.Stale)
		}
		for _, p := range append(ov.Setup, ov.Problems...) {
			line("  Note: %s", p)
		}
		for _, m := range ov.Mods {
			for _, sl := range m.Slots {
				line("    %s / %s: %s skin %03d (wanted %d), own paint %v, own icon %v%s", m.Folder, sl.Key, sl.Vehicle, sl.Number, sl.Wanted, sl.Material != "", sl.Icon != "", texNote(sl.Tex))
			}
			for _, e := range m.Emblems {
				line("    %s / %s: emblem %d (wanted %d)", m.Folder, e.Key, e.Number, e.Wanted)
			}
		}
		if len(ov.TableMods) > 0 {
			line("  Mods with their own skin list: %s", strings.Join(ov.TableMods, ", "))
		}
	}
	sv := saveStatus(s)
	line("  Save: found %v, problem %s; new skins and emblems %d, unlocked %d; worn %d, left over %d; backups %d; backup at game start %v; Steam Cloud %v",
		sv.Found, orNone(sv.Problem), len(sv.Items), countUnlocked(sv.Items), sv.Worn, sv.Leftover, len(sv.Backups), sv.AutoBackup.Installed, sv.Cloud)
	names := loadNames(s)
	used, _ := seenTextKeys(s)
	seen := 0
	for f := range names {
		for _, k := range fieldKeys(f) {
			if used[k] {
				seen++
				break
			}
		}
	}
	line("  Names on the radio: %d changed, %d of them seen in the last game; other files setting them first: %s", len(names), seen, orNone(strings.Join(nameClashes(s, names), ", ")))
	line("")
	if j := jobs.Last(); j != nil {
		v := j.view(0)
		line("Last task: %s, %s after %.0f s", v.Title, v.Status, v.Seconds)
		if v.Error != "" {
			line("  Error: %s", v.Error)
		}
		lines := v.Lines
		if len(lines) > 80 {
			line("  Log (last 80 of %d lines):", len(lines))
			lines = lines[len(lines)-80:]
		} else {
			line("  Log:")
		}
		for _, l := range lines {
			line("    %s", l)
		}
	} else {
		line("Last task: none since the program started")
	}
	line("")
	line("Program log (last lines):")
	logMu.Lock()
	if logFile != nil {
		logFile.Sync()
	}
	logMu.Unlock()
	var own []string
	for _, l := range tailOfFile(filepath.Join(dataDir, "ACE8SkinStudio.log"), 400) {
		if !strings.Contains(l, "[job ") { // task lines are already shown above
			own = append(own, l)
		}
	}
	if len(own) > 25 {
		own = own[len(own)-25:]
	}
	for _, l := range own {
		line("  %s", l)
	}
	if len(own) == 0 {
		line("  (empty)")
	}
	text := sc.clean(b.String())
	const limit = 12000 // room to spare in one or two chat messages' worth of attachment
	if len(text) > limit {
		text = text[:limit] + "\n(report cut short)\n"
	}
	return text
}

func countUnlocked(items []saveItem) int {
	n := 0
	for _, it := range items {
		if it.Unlocked {
			n++
		}
	}
	return n
}
