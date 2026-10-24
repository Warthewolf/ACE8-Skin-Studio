package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureOodle(t *testing.T) {
	dir := t.TempDir()
	retoc := filepath.Join(dir, "tools", "retoc.exe")
	os.MkdirAll(filepath.Dir(retoc), 0o755)
	os.WriteFile(retoc, []byte("x"), 0o755)
	j := testJob()
	// the download: a wrong file is refused, the right one kept
	body := []byte("not oodle")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	old := oodleBaseURL
	oodleBaseURL = srv.URL + "/"
	defer func() { oodleBaseURL = old }()
	err := ensureOodle(j, retoc, "")
	if err == nil || !strings.Contains(err.Error(), "could not be downloaded") {
		t.Fatalf("wrong file accepted: %v", err)
	}
	if fileExists(filepath.Join(filepath.Dir(retoc), oodleName)) {
		t.Fatal("wrong file written")
	}
	// the game's own copy is used when it has one (Windows only: Linux needs its own library)
	if oodleGameName != "" {
		game := filepath.Join(dir, "ACE COMBAT 8")
		paks := filepath.Join(game, "Game", "Content", "Paks")
		os.MkdirAll(paks, 0o755)
		lib := filepath.Join(game, "Engine", "Binaries", "ThirdParty", "Oodle", "Win64", oodleGameName)
		os.MkdirAll(filepath.Dir(lib), 0o755)
		os.WriteFile(lib, make([]byte, 200_000), 0o644)
		if err := ensureOodle(j, retoc, paks); err != nil {
			t.Fatal(err)
		}
		if st, err := os.Stat(filepath.Join(filepath.Dir(retoc), oodleName)); err != nil || st.Size() != 200_000 {
			t.Fatalf("game copy not used: %v", err)
		}
		// there now: nothing more to do, even without a game folder
		if err := ensureOodle(j, retoc, ""); err != nil {
			t.Fatal(err)
		}
	}
	// finding the game's copy
	game := filepath.Join(dir, "Game2", "ACE COMBAT 8")
	paks := filepath.Join(game, "Game", "Content", "Paks")
	os.MkdirAll(paks, 0o755)
	lib := filepath.Join(game, "Engine", "Binaries", "ThirdParty", "Oodle", "Win64", "oo2core_9_win64.dll")
	os.MkdirAll(filepath.Dir(lib), 0o755)
	os.WriteFile(lib, []byte("x"), 0o644)
	if got := gameOodle(paks, "oo2core_9_win64.dll"); got != lib {
		t.Fatalf("game copy found at %q", got)
	}
	if !needsOodle([]string{"--aes-key", "k", "to-legacy"}) || needsOodle([]string{"list"}) || !isRetoc(`C:\x\retoc.exe`) || isRetoc("/x/ace8skinstudio.test") {
		t.Fatal("which runs need Oodle")
	}
}
