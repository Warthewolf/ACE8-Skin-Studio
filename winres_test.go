package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"unicode/utf16"
)

// The Windows program's file details (Properties, Details) show the same version as the program:
// winres/winres.json and rsrc_windows_amd64.syso are updated with appVersion (docs/building.md).
func TestWindowsVersionResource(t *testing.T) {
	syso, err := os.ReadFile("rsrc_windows_amd64.syso")
	if err != nil {
		t.Fatal(err)
	}
	var want []byte
	for _, u := range utf16.Encode([]rune(appVersion)) {
		want = append(want, byte(u), byte(u>>8))
	}
	if !bytes.Contains(syso, want) {
		t.Fatalf("rsrc_windows_amd64.syso does not hold version %s; update winres/winres.json and run go-winres (docs/building.md)", appVersion)
	}
	js, err := os.ReadFile("winres/winres.json")
	if err != nil || !strings.Contains(string(js), `"`+appVersion+`.0"`) || !strings.Contains(string(js), `"`+appVersion+`"`) {
		t.Fatalf("winres/winres.json does not say version %s", appVersion)
	}
}
