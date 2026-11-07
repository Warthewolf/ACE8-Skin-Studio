package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The save is backed up before the skin list changes, once per different save, and only the newest
// of those backups are kept; backups of other kinds are never deleted.
func TestBackupBeforeListChange(t *testing.T) {
	s := saveEnv(t)
	j := testJob()
	j.Title = "Build new skin"

	// no save yet: nothing to back up, and that is not a problem
	if b, err := backupBeforeListChange(j, s, j.Title); err != nil || b != nil {
		t.Fatalf("no save: %+v %v", b, err)
	}

	camp := testSave([]int32{21001001}, nil, [2]int32{0, 0}, 0)
	os.WriteFile(filepath.Join(s.SaveDir, campaignSave), camp, 0o644)
	manual, err := makeSaveBackup(s, "by hand")
	if err != nil {
		t.Fatal(err)
	}
	// the newest backup already holds this save
	if b, err := backupBeforeListChange(j, s, j.Title); err != nil || b != nil {
		t.Fatalf("unchanged save backed up again: %+v %v", b, err)
	}

	// the save changes (the game was played): it is backed up, marked with its kind and reason
	os.WriteFile(filepath.Join(s.SaveDir, campaignSave), testSave([]int32{21001001, 21002001}, nil, [2]int32{0, 0}, 0), 0o644)
	b, err := backupBeforeListChange(j, s, j.Title)
	if err != nil || b == nil {
		t.Fatalf("changed save not backed up: %+v %v", b, err)
	}
	if b.Kind != backupListChange || b.Reason != "before the skin list changed (build new skin)" {
		t.Fatalf("backup %+v", b)
	}
	var found bool
	for _, x := range listSaveBackups(s) {
		if x.ID == b.ID {
			found = x.Kind == backupListChange && x.Reason == b.Reason
		}
	}
	if !found {
		t.Fatal("the backup's kind and reason were not read back")
	}
	// a System.sav that appears makes the save different too
	os.WriteFile(filepath.Join(s.SaveDir, systemSave), testSaveFile("/Script/Live.LiveSystemSaveGame", gvListBytes(gvTag("Version", tInt, 0, i32b(0)))), 0o644)
	if b2, err := backupBeforeListChange(j, s, ""); err != nil || b2 == nil || b2.Reason != listChangeReason {
		t.Fatalf("new System.sav not backed up: %+v %v", b2, err)
	}

	// only the newest listChangeKeep backups of this kind are kept; the one made by hand stays
	for i := 0; i < listChangeKeep+3; i++ {
		os.WriteFile(filepath.Join(s.SaveDir, campaignSave), testSave([]int32{21001001, int32(21003001 + i*1000)}, nil, [2]int32{0, 0}, 0), 0o644)
		if _, err := backupBeforeListChange(j, s, j.Title); err != nil {
			t.Fatal(err)
		}
	}
	kinds, hand := 0, false
	for _, x := range listSaveBackups(s) {
		if x.Kind == backupListChange {
			kinds++
		}
		if x.ID == manual.ID {
			hand = true
		}
	}
	if kinds != listChangeKeep || !hand {
		t.Fatalf("kept %d skin list backups (want %d), hand-made kept: %v", kinds, listChangeKeep, hand)
	}
}

// A backup folder that cannot be checked (here a file has its name) gives an error straight away,
// instead of looking for a free folder name for ever, and the backup before a skin list change
// says so too.
func TestSaveBackupFolderCannotBeChecked(t *testing.T) {
	s := saveEnv(t)
	os.WriteFile(filepath.Join(s.SaveDir, campaignSave), testSave([]int32{21001001}, nil, [2]int32{0, 0}, 0), 0o644)
	if err := os.WriteFile(s.saveBackupsDir(), []byte("not a folder"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 2)
	go func() {
		_, err := makeSaveBackup(s, "by hand")
		done <- err
	}()
	go func() {
		_, err := backupBeforeListChange(testJob(), s, "Build new skin")
		done <- err
	}()
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err == nil || !strings.Contains(err.Error(), "backup folder") {
				t.Fatalf("backup into a file: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the backup is still looking for a folder name")
		}
	}
	// a folder that already has the name is skipped
	os.Remove(s.saveBackupsDir())
	stamp := time.Now().Format("2006-01-02 15-04-05")
	os.MkdirAll(filepath.Join(s.saveBackupsDir(), stamp), 0o755)
	b, err := makeSaveBackup(s, "by hand")
	if err != nil || b.ID == stamp {
		t.Fatalf("backup %+v %v", b, err)
	}
}

// A game-start backup slot only lists the files written with it: an old file left in the slot
// from an earlier start is not shown under the slot's new date.
func TestAutoBackupSlotOldFile(t *testing.T) {
	s := saveEnv(t)
	auto := filepath.Join(s.saveBackupsDir(), autoBackupFolder)
	os.MkdirAll(auto, 0o755)
	camp := testSave([]int32{21001001}, nil, [2]int32{0, 0}, 0)
	system := testSaveFile("/Script/Live.LiveSystemSaveGame", gvListBytes(gvTag("Version", tInt, 0, i32b(0))))
	for _, f := range []string{"07 Campaign.sav", "08 Campaign.sav", "08 System.sav"} {
		os.WriteFile(filepath.Join(auto, f), camp, 0o644)
	}
	os.WriteFile(filepath.Join(auto, "07 System.sav"), system, 0o644)
	os.WriteFile(filepath.Join(auto, "07.txt"), []byte(time.Now().Format("2006-01-02 15:04:05")+"\n"), 0o644)
	os.WriteFile(filepath.Join(auto, "08.txt"), []byte("2026-01-02 10:11:12\n"), 0o644)
	// slot 07's Campaign.sav is from weeks before: only System.sav was written at the last start
	weeks := time.Now().Add(-21 * 24 * time.Hour)
	os.Chtimes(filepath.Join(auto, "07 Campaign.sav"), weeks, weeks)
	got := map[string][]string{}
	for _, b := range listSaveBackups(s) {
		got[b.ID] = b.Files
	}
	if f := got["auto/07"]; len(f) != 1 || f[0] != systemSave {
		t.Fatalf("slot 07 lists %v", f)
	}
	if f := got["auto/08"]; len(f) != 2 {
		t.Fatalf("slot 08 lists %v", f)
	}
	if _, err := doRestoreBackup(testJob(), s, "auto/07", false); err == nil {
		t.Fatal("put back the old Campaign.sav of slot 07")
	}
}

// A save folder typed with a separator at the end still has its backups next to it, not inside.
func TestSaveFolderTrailingSeparator(t *testing.T) {
	d := filepath.Join(t.TempDir(), "Saved", "SaveGames")
	s := Settings{SaveDir: d + string(filepath.Separator)}
	defaults(&s)
	if got, want := s.saveBackupsDir(), filepath.Join(filepath.Dir(d), saveBackupFolder); got != want {
		t.Fatalf("backups in %s, want %s", got, want)
	}
	if got := s.saveGamesDir(); got != d {
		t.Fatalf("save folder %s, want %s", got, d)
	}
}
