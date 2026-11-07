package main

// Repack can also change what the game shows for the skin it replaces: its name and description
// (through SkinSlotText, like new skins) and its icon (a replacement icon in the same mod).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type skinExtras struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"` // "", recolour or picture
	IconColour  string `json:"iconColour"`
	IconPicture string `json:"iconPicture"`
	Template    int    `json:"template"` // the skin's number; 0 = the folder's
}

func (e *skinExtras) wanted() bool {
	return e != nil && (strings.TrimSpace(e.Name) != "" || strings.TrimSpace(e.Description) != "" || e.Icon == "recolour" || e.Icon == "picture")
}

// Repack mods' texts: a prefix of their own, so they are never mistaken for a skin mod's file.
const replaceTextPrefix = "SkinStudioRepack_"

// replaceTextFile is the text file of a Repack mod (one per mod, named after it).
func replaceTextFile(s Settings, mod string) string {
	if d := s.textsDir(); d != "" {
		return filepath.Join(d, replaceTextPrefix+safeName(mod)+".ini")
	}
	return ""
}

// applySkinExtras puts the skin's new icon into the mod's input folder, and returns the text file
// with its new name and description ("" when neither is changed). That file goes where the game
// reads it at once, so it is written by writeReplaceTexts only once the mod is built.
func applySkinExtras(j *Job, s Settings, r planReq, mod, input string, paintColour *[3]uint8) ([]string, string, error) {
	e := r.Extras
	texs, err := skinTextures(r.SkinDir)
	if err != nil {
		return nil, "", err
	}
	var pkg string
	for _, t := range texs {
		if t.Package != "" {
			pkg = t.Package
			break
		}
	}
	vehicle, aircraft, folder, err := vehicleOfTexture(pkg)
	if err != nil {
		return nil, "", err
	}
	n := e.Template
	if n <= 0 {
		n = folderNumber(folder)
	}
	if n <= 0 {
		return nil, "", errors.New("say which skin number this is, so its name and icon can be changed")
	}
	if _, err := needExtractSetup(s); err != nil {
		return nil, "", err
	}
	st, _, err := loadBaseTables(j, s)
	if err != nil {
		return nil, "", err
	}
	row := st.Find(skinIDOf(aircraft, n))
	if row == nil {
		return nil, "", fmt.Errorf("%s has no skin %03d in the game's skin list", vehicle, n)
	}
	var notes []string
	var texts string
	// texts
	name, desc := strings.TrimSpace(e.Name), strings.TrimSpace(e.Description)
	if name != "" || desc != "" {
		path := replaceTextFile(s, mod)
		if path == "" {
			return nil, "", errors.New("UE4SS's Mods folder was not found, so the name and description cannot be installed; set it in Settings")
		}
		if err := installSkinSlotText(s); err != nil {
			return nil, "", err
		}
		var b strings.Builder
		b.WriteString("; Name and description of " + vehicle + " skin " + fmt.Sprintf("%03d", n) + ", changed by the mod " + mod + ".\r\n")
		b.WriteString("; Written by " + appName + ". Delete this file to get the game's own texts back.\r\n\r\n[Text]\r\n")
		if name != "" {
			b.WriteString(row.Obj.Str("SkinNameTextID") + "=" + iniValue(name) + "\r\n")
		}
		if desc != "" {
			b.WriteString(row.Obj.Str("SkinDescriptionTextID") + "=" + iniValue(desc) + "\r\n")
		}
		texts = b.String()
		notes = append(notes, "The new name and description are in "+filepath.Base(path)+" (SkinSlotText)")
	}
	// icon
	if e.Icon == "recolour" || e.Icon == "picture" {
		icon, err := skinIcon(*row, vehicle, n)
		if err != nil {
			return nil, "", err
		}
		if err := ensureBase(j, s, []string{icon}); err != nil {
			return nil, "", err
		}
		ip, err := LoadPackage(gameFile(s.baseDir(), s.Project, icon))
		if err != nil {
			return nil, "", fmt.Errorf("the skin's icon could not be read: %v", err)
		}
		var pic *Img
		if e.Icon == "picture" {
			src, err := LoadPicture(cleanPath(e.IconPicture))
			if err != nil {
				return nil, "", err
			}
			pic = IconFromPicture(src, 180)
		} else {
			orig, err := DecodeIcon(ip)
			if err != nil {
				return nil, "", err
			}
			c, ok := parseColour(e.IconColour)
			if !ok {
				if paintColour == nil {
					return nil, "", errors.New("choose a colour for the icon (or rebuild the colour texture, whose main colour is then used)")
				}
				c = *paintColour
			}
			pic = RecolourIcon(orig, c)
		}
		ue, err := EncodeIcon(ip, pic)
		if err != nil {
			return nil, "", err
		}
		if err := writePackageFiles(gameFile(input, s.Project, icon), ip.UAsset, ue, nil); err != nil {
			return nil, "", err
		}
		notes = append(notes, "The skin's icon is replaced too ("+icon[strings.LastIndexByte(icon, '/')+1:]+")")
	}
	return notes, texts, nil
}

// skinIcon is the package of a skin's menu icon, as its row in the skin list names it. A row that
// names none has no icon to replace (an empty name would fetch the whole game).
func skinIcon(row SkinRow, vehicle string, n int) (string, error) {
	icon := packagePathOf(row.Obj.Str("MenuIconImageRef"))
	if !strings.HasPrefix(icon, "/Game/") || len(icon) == len("/Game/") {
		return "", fmt.Errorf("%s skin %03d has no icon of its own in the game's skin list, so there is no icon to replace; choose \"Keep the game's icon\"", vehicle, n)
	}
	return icon, nil
}

// writeReplaceTexts puts a Repack mod's new name and description (from applySkinExtras) where
// SkinSlotText reads them, once the mod is built and, when asked for, installed. A build that
// is installed without them takes away the file an earlier build of the mod left, so the game
// shows the skin's own name and description again.
func writeReplaceTexts(j *Job, s Settings, mod, texts string, installed bool) error {
	path := replaceTextFile(s, mod)
	if path == "" {
		return nil
	}
	if texts == "" {
		if installed {
			if err := os.Remove(path); err == nil {
				j.Log("The name and description an earlier build of %s gave the skin were taken out (%s)", mod, filepath.Base(path))
			}
		}
		return nil
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(texts), 0o644); err != nil {
		return err
	}
	j.Log("Name and description written to %s", path)
	return nil
}

// moveReplaceTexts takes a Repack mod's text file along when the mod is removed (or puts it back).
func moveReplaceTexts(s Settings, mod, dest string) {
	p := replaceTextFile(s, mod)
	if p == "" {
		return
	}
	if _, err := os.Stat(p); err == nil {
		moveFile(p, filepath.Join(dest, filepath.Base(p)))
	}
}
