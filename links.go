package main

// Useful links: other people's work that helps with making skins, shown on the Useful links
// screen. Only these pages can be opened from the window (in the default browser).

import (
	"errors"
	"net/http"
)

type usefulLink struct {
	Title  string   `json:"title"`
	Author string   `json:"author"`
	Site   string   `json:"site"`
	URL    string   `json:"url"`
	About  string   `json:"about"`
	Points []string `json:"points,omitempty"`
}

var usefulLinks = []usefulLink{
	{
		Title:  "Skin Template Repository - Project Pileus",
		Author: "Nimbus Custom Works, uploaded by M4rkoz",
		Site:   "Nexus Mods",
		URL:    "https://www.nexusmods.com/acecombat8wingsoftheve/mods/87",
		About:  "Ready-made skin templates for the game's aircraft, to paint your own skins on.",
		Points: []string{
			"Layered templates for Photoshop (PSD) and GIMP (XCF), with weathering, decals and the parts that are not colour kept apart",
			"The finish (MRX), normal map (NO) and dirt mask textures, and a Blender model to preview your paint",
			"Templates for the F-22A, Su-33, Su-57, F-35C, F-15C and F-14A so far, with more aircraft being added",
		},
	},
	{
		Title:  "Skin templates by Rythus",
		Author: "Rythus",
		Site:   "Nexus Mods",
		URL:    "https://www.nexusmods.com/acecombat8wingsoftheve/mods/34",
		About:  "Skin templates for the game's aircraft, to paint your own skins on.",
	},
}

func linkRoutes(api func(string, http.HandlerFunc)) {
	api("/api/links", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"links": usefulLinks})
	})
	api("/api/link/open", func(w http.ResponseWriter, r *http.Request) {
		var q struct{ URL string }
		if err := readJSON(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		for _, l := range usefulLinks {
			if l.URL == q.URL {
				if err := openURL(l.URL, false); err != nil {
					fail(w, 500, errors.New("the page could not be opened ("+err.Error()+"); copy the link into your browser instead"))
					return
				}
				writeJSON(w, 200, map[string]bool{"ok": true})
				return
			}
		}
		fail(w, 400, errors.New("unknown link"))
	})
}
