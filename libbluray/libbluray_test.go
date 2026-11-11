// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package libbluray

import (
	"strings"
	"testing"

	"github.com/wittywilla/wittybluray"
)

func testDisc() *wittybluray.Disc {
	item := func(clip string, secs uint32, angles int) wittybluray.PlayItem {
		it := wittybluray.PlayItem{Clip: clip, In: 45000, Out: 45000 + secs*45000}
		for i := 0; i < angles; i++ {
			it.Angles = append(it.Angles, wittybluray.AngleClip{Clip: clip})
		}
		return it
	}
	return &wittybluray.Disc{Playlists: map[int]*wittybluray.Playlist{
		1: {Number: 1, Items: []wittybluray.PlayItem{item("00001", 600, 1), item("00002", 600, 1)}, Marks: []wittybluray.Mark{{Type: 1}, {Type: 1, Item: 1}, {Type: 2}}},
		2: {Number: 2, Items: []wittybluray.PlayItem{item("00003", 60, 2)}},
		3: {Number: 3, Items: []wittybluray.PlayItem{item("00004", 10, 1)}},
	}}
}

func TestCompare(t *testing.T) {
	d := testDisc()
	same := &Info{Titles: []TitleInfo{
		{Playlist: 1, Duration: 1200, Clips: []string{"00001", "00002"}, Chapters: 2, Angles: 1},
		{Playlist: 2, Duration: 60.4, Clips: []string{"00003"}, Chapters: 0, Angles: 2},
		{Playlist: 3, Duration: 10, Clips: []string{"00004"}, Angles: 1},
	}}
	if got := Compare(d, same); len(got) != 0 {
		t.Fatalf("expected agreement, got %v", got)
	}
	differ := &Info{Flags: DiscFlags{BDJDetected: true}, Titles: []TitleInfo{
		{Playlist: 1, Duration: 1100, Clips: []string{"00002", "00001"}, Chapters: 3, Angles: 1},
		{Playlist: 2, Duration: 60, Clips: []string{"00003"}, Angles: 1},
		{Playlist: 9, Duration: 5},
	}}
	got := Compare(d, differ)
	var lines []string
	for _, x := range got {
		lines = append(lines, x.String())
	}
	all := strings.Join(lines, "\n")
	for _, want := range []string{"disc bdj", "playlist 00001 duration", "playlist 00001 clips", "playlist 00001 chapters", "playlist 00002 angles", "playlist 00003 present: WittyBluray yes, libbluray no", "playlist 00009 present: WittyBluray no, libbluray yes"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	if len(got) != 7 {
		t.Errorf("%d discrepancies:\n%s", len(got), all)
	}
}

func TestCrossCheckRefusesEncrypted(t *testing.T) {
	d := testDisc()
	d.Encrypted = true
	got := CrossCheck(d)
	if len(got) != 1 || got[0].Field != "refused" {
		t.Fatalf("%v", got)
	}
	if CrossCheck(nil) != nil {
		t.Fatal("nil disc")
	}
}

func TestStubOrReal(t *testing.T) {
	if Available() {
		t.Skip("built with libbluray; the stub isn't in this build")
	}
	if _, err := Read("/nowhere", ReadOptions{}); err != ErrUnavailable {
		t.Fatalf("stub Read: %v", err)
	}
	if got := CrossCheck(testDisc()); len(got) != 1 || got[0].Field != "unavailable" {
		t.Fatalf("stub CrossCheck: %v", got)
	}
}
