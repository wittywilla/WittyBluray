// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import "github.com/wittywilla/wittymodule/sdui"

// ModuleID is this module's ID, the same as module.json's.
const ModuleID = "wittybluray"

// The sources of a Blu-ray's sections on its page. Each takes the page's title ($item) and
// answers only for a Blu-ray: on any other title its sections are left out. The server answers
// them (WittyMediaServer internal/server/disc_sources.go), as it does WittyPlay's, because what
// they show is the server's: the disc's kept ranking from this module's helper, an
// administrator's choice of film and the names given to its playlists.
const (
	SourceFeature  = ModuleID + ".feature"
	SourceScenes   = ModuleID + ".scenes"
	SourceEpisodes = ModuleID + ".episodes"
	SourceVersions = ModuleID + ".versions"
	SourceBonus    = ModuleID + ".bonus"
	SourceMenus    = ModuleID + ".menus"
	SourceWhy      = ModuleID + ".why"
	SourceCopy     = ModuleID + ".copy"
	SourceRefusal  = ModuleID + ".refusal"
)

// UI is WittyBluray's part of every app (module API 4, WittyMediaServer docs/SDUI-MODULES.md), written
// to sdui.json beside module.json: a Blu-ray's sections on its page, in every look (Design 2's
// "discpage", "whytitle" and "discrefuse" boards). The player, with the disc's own menus and the
// remote, stays each app's own screen.
func UI() *sdui.Module {
	m := sdui.New(ModuleID, "Blu-rays")
	item := func(more ...sdui.PropSpec) []sdui.PropSpec {
		return append([]sdui.PropSpec{{Name: "item", Type: "ref", Label: "Disc", Required: true, Help: "$item on a Blu-ray's page."}}, more...)
	}
	limit := func(def, most int) sdui.PropSpec {
		return sdui.PropSpec{Name: "limit", Type: "int", Label: "At most", Default: def, Min: sdui.Int(1), Max: sdui.Int(most)}
	}
	src := func(typ, label, icon, title, desc string, params []sdui.PropSpec, card string) {
		m.AddSource(sdui.SourceSpec{Type: typ, Label: label, Icon: icon, Title: title, Description: desc, Params: params,
			Yields: []string{"view"}, CardStyle: card, PageKinds: []string{"detail"}})
	}
	src(SourceRefusal, "Can't play", "warning", "This disc can't be played",
		"An encrypted Blu-ray: a plain message saying so, that nothing was changed or copied, and what the disc's index still says.", item(), "landscape")
	src(SourceFeature, "Feature Presentation", "play", "Feature Presentation",
		"Play the film, the whole of it however the disc cuts it into playlists, and how many scenes it has.", item(), "landscape")
	src(SourceScenes, "Scenes", "list", "Scenes", "The film's chapters as pictures, each playing from its start, with the names given to them.", item(limit(99, 999)), "landscape")
	src(SourceEpisodes, "Episodes", "show", "Episodes", "A TV disc's episodes in viewing order, each from its own place.", item(limit(99, 99)), "landscape")
	src(SourceVersions, "Other versions", "movie", "Other versions", "Other cuts and camera angles of the film than the one Play plays.", item(limit(16, 16)), "landscape")
	src(SourceBonus, "Bonus material", "video", "Bonus material", "The disc's other playlists worth watching, longest first, without its decoys.", item(limit(50, 200)), "landscape")
	src(SourceMenus, "Disc menus", "disc", "Disc menus", "Shortcuts straight into the disc's own menus.", item(), "landscape")
	src(SourceWhy, "Why this playlist", "info", "Why this playlist",
		"Which playlist is the film and why, how sure the disc engine is, and the other candidates, each to play once.", item(limit(20, 32)), "landscape")
	src(SourceCopy, "Diagnosis", "check", "Diagnosis",
		"What was wrong with this copy of the disc and what was mended, and for an administrator the way to check it.", item(limit(32, 32)), "landscape")
	// On a title's page, between its header and its cast and crew, in this order.
	put := func(s sdui.Section) {
		m.Inserts = append(m.Inserts, sdui.ModuleInsert{Page: "detail", Before: "credits", Section: s})
	}
	list := func(id, title string, typ string, more ...any) sdui.Section {
		return sdui.Sec(ModuleID+"-"+id, "List", sdui.Props("title", title), sdui.Src(typ, append([]any{"item", "$item"}, more...)...))
	}
	put(list("refusal", "This disc can't be played", SourceRefusal))
	put(list("feature", "Feature Presentation", SourceFeature))
	put(sdui.Shelf(ModuleID+"-scenes", "Scenes", sdui.Src(SourceScenes, "item", "$item")))
	put(list("episodes", "Episodes", SourceEpisodes))
	put(list("versions", "Other versions", SourceVersions))
	put(list("bonus", "Bonus material", SourceBonus))
	put(list("menus", "Disc menus", SourceMenus))
	put(list("why", "Why this playlist", SourceWhy))
	put(list("copy", "Diagnosis", SourceCopy))
	return m
}
