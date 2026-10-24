package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// When game files do not come out, retoc is asked why, and the answer it prints on its progress
// display (here with the display's own lines and colour codes around it) goes into the message.
func TestUnreadableGameFiles(t *testing.T) {
	s := Settings{Workspace: t.TempDir(), Project: "Live", EngineVersion: "UE5_4", PaksDir: "/p", AESKey: "k"}
	run := func(lines ...string) error {
		old := runToolTTYFn
		defer func() { runToolTTYFn = old }()
		var gotArgs []string
		runToolTTYFn = func(j *Job, exe string, args []string, onLine func(string)) error {
			gotArgs = args
			for _, l := range lines {
				onLine(l)
			}
			return nil
		}
		err := unreadableGameFiles(testJob(), s, "retoc", []string{"/Game/Datatables/Information/DT_Emblem", "/Game/Unlock/DA_Unlock_Emblem"}, s.baseDir())
		if a := strings.Join(gotArgs, " "); !strings.Contains(a, "--no-parallel") || !strings.Contains(a, "--filter Datatables/Information/DT_Emblem.") {
			t.Fatalf("asked with %q", a)
		}
		return err
	}
	err := run(
		"[00:00:03] ########################-------- 1/2 Live/Content/Datatables/Information/DT_Emblem.uasset",
		"Failed to convert ../../../Live/Content/Datatables/Information/DT_Emblem.uasset: Failed to resolve public export with hash 123 on package /Game/UI/Texture/Emblem/T_Emblem_0001 (imported by /Game/Datatables/Information/DT_Emblem)",
		"info: : Extracted 0 (2 failed) legacy assets to \"/w\"")
	if err == nil || !strings.Contains(err.Error(), "DT_Emblem and DA_Unlock_Emblem could not be taken out") || !strings.Contains(err.Error(), "Failed to resolve public export") ||
		!strings.Contains(err.Error(), "Verify integrity") || strings.Contains(err.Error(), "../../../") {
		t.Fatalf("message: %v", err)
	}
	// nothing matched: not in the game files
	if err := run("info: : Extracted 0 (0 failed) legacy assets to \"/w\""); err == nil || !strings.Contains(err.Error(), "was not found in the game files") {
		t.Fatalf("message: %v", err)
	}
	// failed, but no reason could be had
	if err := run("info: : Extracted 0 (2 failed) legacy assets to \"/w\""); err == nil || !strings.Contains(err.Error(), "did not say why") || !strings.Contains(err.Error(), "support report") {
		t.Fatalf("message: %v", err)
	}
	// the second try brings them out: no error
	for _, gp := range []string{"/Game/Datatables/Information/DT_Emblem", "/Game/Unlock/DA_Unlock_Emblem"} {
		f := gameFile(s.baseDir(), s.Project, gp) + ".uasset"
		os.MkdirAll(filepath.Dir(f), 0o755)
		os.WriteFile(f, []byte("x"), 0o644)
	}
	if err := run("info: : Extracted 2 (0 failed) legacy assets to \"/w\""); err != nil {
		t.Fatalf("after a good second try: %v", err)
	}
}
