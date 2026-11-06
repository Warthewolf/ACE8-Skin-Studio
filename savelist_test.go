package main

import (
	"os"
	"path/filepath"
	"testing"
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
