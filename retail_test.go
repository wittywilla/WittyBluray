// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

// What ordinary retail discs do that the ranking once took for something
// else. Each disc here is synthetic: it has the structure that an image of a
// physical disc showed the engine getting wrong, and nothing of the disc's.

import (
	"strings"
	"testing"
)

// menuLoopDisc: a film in one clip, a logo before it, and the top menu's
// playlist, which plays one clip of half a minute twelve times over under
// its menu graphics, as discs loop a menu's background.
func menuLoopDisc() *synDisc {
	s := newSyn(41)
	s.addClip("00005", 10*sec, 5172)
	s.addClip("00002", 10*sec, 26)
	s.addClip("00001", 10*sec, 33)
	items := s.items("00005")
	s.pl(2, &synPlaylist{items: items, marks: synChapters(items, 260)})
	s.pl(0, &synPlaylist{items: s.items("00002")})
	var loop []synItem
	for i := 0; i < 12; i++ {
		it := s.item("00001")
		it.ig = true
		loop = append(loop, it)
	}
	s.pl(1, &synPlaylist{items: loop})
	s.objs = []MovieObject{
		{Commands: []Command{play(2), jumpTitle(0)}}, // title 1: the film, then the menu
		{Commands: []Command{play(0), jumpTitle(0)}}, // first play: the logo
		{Commands: []Command{play(1)}},               // the top menu
	}
	s.first, s.top, s.titles = hdmv(1), hdmv(2), []IndexObject{hdmv(0)}
	return s
}

// A menu's looping background is not a decoy, and a disc with one is not
// called copy-protected or damaged.
func TestMenuLoopIsNotADecoy(t *testing.T) {
	d := menuLoopDisc().open(t)
	r := d.Analyze(RankOptions{})
	if r.Main != 2 || len(r.Decoys) != 0 {
		t.Fatalf("main %05d with decoys %v, want 00002 and none\n%s", r.Main, r.Decoys, r.Explain())
	}
	c := r.Candidate(1)
	if c == nil || c.Class != ClassMenu || !c.Flags.Has(FlagMenu) || c.Flags.Has(FlagDecoy) {
		t.Errorf("the menu's playlist: %+v", c)
	}
	for _, w := range r.Warnings {
		if strings.Contains(w, "decoy") || strings.Contains(w, "protection") {
			t.Errorf("warned: %s", w)
		}
	}
	if r.ConfidenceLabel != "high" {
		t.Errorf("confidence %s (%.2f): %v", r.ConfidenceLabel, r.Confidence, r.Why)
	}
}

// Menu graphics don't excuse a decoy: a long playlist that replays its clips
// is still one, whatever streams it claims.
func TestLongRepeatingPlaylistWithGraphicsIsStillADecoy(t *testing.T) {
	s := newSyn(42)
	film := s.film(8, 600, evidence{carry: true})
	s.feature(800, film)
	var items []synItem
	for i := 0; i < 3; i++ {
		for _, n := range film {
			it := s.item(n)
			it.ig = true
			items = append(items, it)
		}
	}
	s.pl(801, &synPlaylist{items: items})
	r := s.open(t).Analyze(RankOptions{})
	if r.Main != 800 || len(r.Decoys) != 1 || r.Decoys[0] != 801 {
		t.Fatalf("main %05d, decoys %v\n%s", r.Main, r.Decoys, r.Explain())
	}
}
