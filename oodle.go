package main

// Oodle for retoc.
//
// The game's files are compressed with Oodle, a library retoc does not carry: the first time it
// needs it, retoc downloads it from GitHub next to itself (oo2core_9_win64.dll on Windows). Where
// that download cannot happen (no internet for it, a firewall or antivirus, GitHub blocked, or a
// folder retoc may not write to), listing the game's files still works but every file taken out
// fails: "Extracted 0 (6 failed)". So before retoc takes files out or packs them, Skin Studio makes
// sure the library is there: copied from the game's own folder when the game has one, otherwise
// downloaded from the same place retoc uses and checked against the same SHA-256.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var oodleBaseURL = "https://github.com/WorkingRobot/OodleUE/raw/refs/heads/main/Engine/Source/Programs/Shared/EpicGames.Oodle/Sdk/2.9.10/"

var oodleMu sync.Mutex

// needsOodle tells whether a retoc command reads or writes compressed game data.
func needsOodle(args []string) bool {
	for _, a := range args {
		if a == "to-legacy" || a == "to-zen" {
			return true
		}
	}
	return false
}

func isRetoc(exe string) bool {
	return strings.HasPrefix(strings.ToLower(exeBase(exe)), "retoc")
}

// gameOodle finds a copy of the library in the game's folder (the folder holding Game\Content\Paks).
func gameOodle(paks, name string) string {
	if paks == "" || name == "" {
		return ""
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(paks)))
	found := ""
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			if rel, _ := filepath.Rel(root, p); strings.Count(rel, string(filepath.Separator)) >= 6 || strings.EqualFold(d.Name(), "Content") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(d.Name(), name) {
			found = p
		}
		return nil
	})
	return found
}

func fileSHA256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ensureOodle makes sure the library is next to retoc.
func ensureOodle(j *Job, retoc string, paks string) error {
	oodleMu.Lock()
	defer oodleMu.Unlock()
	dst := filepath.Join(filepath.Dir(retoc), oodleName)
	if st, err := os.Stat(dst); err == nil && st.Size() > 100_000 {
		return nil
	}
	// 1. the game's own copy
	if src := gameOodle(paks, oodleGameName); src != "" {
		if err := copyFile(src, dst); err == nil {
			j.Log("Copied the game's %s next to retoc (retoc needs it to read the game's files).", oodleName)
			return nil
		} else {
			return oodleWriteProblem(dst, err)
		}
	}
	// 2. the same download retoc makes
	j.SetStep("Getting Oodle for retoc", -1)
	j.Log("retoc needs %s to read the game's files; downloading it.", oodleName)
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Get(oodleBaseURL + oodlePlatformPath)
	if err == nil && resp.StatusCode != 200 {
		err = errors.New(resp.Status)
		resp.Body.Close()
	}
	var b []byte
	if err == nil {
		b, err = io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
	}
	if err == nil && fileSHA256(b) != oodleSHA256 {
		err = errors.New("the download was not the expected file")
	}
	if err != nil {
		logf("oodle download: %v", err)
		return fmt.Errorf("retoc needs the file %s to read the game's files, and it could not be downloaded (%v). "+
			"Check the internet connection (a firewall or antivirus may block it), or get %s from %s and put it next to retoc (%s), then try again",
			oodleName, err, oodleName, oodleBaseURL+oodlePlatformPath, filepath.Dir(retoc))
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return oodleWriteProblem(dst, err)
	}
	j.Log("Downloaded %s next to retoc.", oodleName)
	return nil
}

func oodleWriteProblem(dst string, err error) error {
	return fmt.Errorf("retoc needs the file %s next to it, but it could not be put in %s (%v). "+
		"Put retoc in a folder you can write to, such as next to this program, and choose it again in Settings", oodleName, filepath.Dir(dst), err)
}
