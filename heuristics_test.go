// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"encoding/binary"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestRankScenarios(t *testing.T) {
	cases := []struct {
		name   string
		disc   func() *fxDisc
		main   int
		decoys []int // must all be decoys
		check  func(t *testing.T, d *Disc, r *Ranking)
	}{
		{name: "simple movie", disc: fxSimpleMovie, main: 1, check: func(t *testing.T, d *Disc, r *Ranking) {
			c := r.Candidate(1)
			if !c.Flags.Has(FlagFirstPlay) || c.Chapters != 15 || c.Audio != 2 || c.Subtitles != 1 || !slices.Equal(c.Languages, []string{"eng", "fra"}) {
				t.Errorf("main: %+v", c)
			}
			if c3 := r.Candidate(3); !c3.Flags.Has(FlagStereo3D) || !c3.Flags.Has(FlagDuplicate) || c3.Class != ClassDuplicate {
				t.Errorf("3D twin: %+v", c3)
			}
			if c4 := r.Candidate(4); !c4.Flags.Has(FlagMultiAngle) || c4.Angles != 2 {
				t.Errorf("multi-angle: %+v", c4)
			}
			if r.Candidate(2).Class != ClassTrailer {
				t.Errorf("trailer: %+v", r.Candidate(2))
			}
			if len(r.Decoys) != 0 || len(r.Episodes) != 0 {
				t.Errorf("decoys %v episodes %v", r.Decoys, r.Episodes)
			}
		}},
		{name: "200 obfuscated playlists", disc: func() *fxDisc { return fxObfuscated(200, 123) }, main: 123, check: func(t *testing.T, d *Disc, r *Ranking) {
			if len(r.Decoys) != 199 {
				t.Errorf("%d decoys", len(r.Decoys))
			}
			if !strings.Contains(strings.Join(r.Warnings, " "), "obfuscation") {
				t.Errorf("warnings %v", r.Warnings)
			}
		}},
		{name: "shuffled decoy longer than the film", disc: fxShuffledLonger, main: 11, decoys: []int{10}, check: func(t *testing.T, d *Disc, r *Ranking) {
			c := r.Candidate(11)
			if !strings.Contains(strings.Join(c.Reasons, "\n"), "9 of 9 joins continue") {
				t.Errorf("SPN continuity not seen: %v", c.Reasons)
			}
			if r.Candidate(10).Distinct <= c.Distinct || !r.Candidate(10).Flags.Has(FlagShuffled) {
				t.Errorf("decoy: %+v", r.Candidate(10))
			}
		}},
		{name: "missing and empty clips", disc: fxMissingClips, main: 5, decoys: []int{1, 2, 3, 4}, check: func(t *testing.T, d *Disc, r *Ranking) {
			if !r.Candidate(1).Flags.Has(FlagMissingClip) || !r.Candidate(2).Flags.Has(FlagZeroSize) || !r.Candidate(3).Flags.Has(FlagOutOfRange) || !r.Candidate(4).Flags.Has(FlagMissingClip) {
				t.Errorf("flags: %v %v %v %v", r.Candidate(1).Flags, r.Candidate(2).Flags, r.Candidate(3).Flags, r.Candidate(4).Flags)
			}
		}},
		{name: "HDMV first play leads to 00800", disc: fxFirstPlay800, main: 800, check: func(t *testing.T, d *Disc, r *Ranking) {
			if r.FirstPlay != 800 || r.TitleOne != 800 || r.BDJ {
				t.Errorf("first play %d title one %d bdj %v", r.FirstPlay, r.TitleOne, r.BDJ)
			}
			if c := r.Candidate(5); !c.Flags.Has(FlagIntro) || c.Class != ClassIntro {
				t.Errorf("intro: %+v", c)
			}
		}},
		{name: "BD-J falls back to structure", disc: fxBDJ, main: 3, decoys: []int{2}, check: func(t *testing.T, d *Disc, r *Ranking) {
			if !r.BDJ || r.FirstPlay != -1 || !strings.Contains(strings.Join(r.Warnings, " "), "BD-J") {
				t.Errorf("bdj %v first play %d warnings %v", r.BDJ, r.FirstPlay, r.Warnings)
			}
		}},
		{name: "TV disc with play all", disc: fxTV, main: 20, check: func(t *testing.T, d *Disc, r *Ranking) {
			var order []int
			for _, e := range r.Episodes {
				order = append(order, e.Playlist)
			}
			if !slices.Equal(order, []int{11, 13, 10, 12}) || r.PlayAll != 20 {
				t.Errorf("episodes %v play all %d", order, r.PlayAll)
			}
			if r.Candidate(30).Class != ClassTrailer {
				t.Errorf("extra: %+v", r.Candidate(30))
			}
		}},
		{name: "segment spam", disc: fxSegmentSpam, main: 1, decoys: []int{2, 3}, check: func(t *testing.T, d *Disc, r *Ranking) {
			if !r.Candidate(2).Flags.Has(FlagSegmentSpam) || !r.Candidate(3).Flags.Has(FlagSegmentSpam) {
				t.Errorf("spam flags %v %v", r.Candidate(2).Flags, r.Candidate(3).Flags)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.disc().open(t)
			r := d.Analyze(RankOptions{})
			if r.Main != tc.main {
				t.Fatalf("main %d, want %d\n%s", r.Main, tc.main, r.Explain())
			}
			if pl := d.MainPlaylist(); pl == nil || pl.Number != tc.main {
				t.Fatalf("MainPlaylist %v", pl)
			}
			if r.Candidates[0].PlaylistNum != tc.main || !r.Candidates[0].Flags.Has(FlagMain) || r.Candidates[0].Class != ClassMain {
				t.Fatalf("first candidate %+v", r.Candidates[0])
			}
			for _, n := range tc.decoys {
				if c := r.Candidate(n); c == nil || c.Class != ClassDecoy || !c.Flags.Has(FlagDecoy) {
					t.Errorf("playlist %05d should be a decoy: %+v", n, c)
				}
			}
			if tc.check != nil {
				tc.check(t, d, r)
			}
			if t.Failed() {
				t.Log(r.Explain())
			}
		})
	}
}

func TestRankingAPI(t *testing.T) {
	d := fxTV().open(t)
	if got := d.RankPlaylists(); len(got) != 6 || got[0].PlaylistNum != 20 {
		t.Fatalf("RankPlaylists %+v", got)
	}
	if eps := d.Episodes(); len(eps) != 4 || eps[0].Index != 1 {
		t.Fatalf("Episodes %+v", eps)
	}
	ex := d.Explain()
	for _, want := range []string{"Main playlist: 00020", "Episodes in order: 00011, 00013, 00010, 00012", "[episode]"} {
		if !strings.Contains(ex, want) {
			t.Errorf("Explain lacks %q:\n%s", want, ex)
		}
	}
	// Flags travel as names, and back.
	b, err := json.Marshal(d.Analyze(RankOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"flags":["main","playAll"]`) {
		t.Errorf("json: %s", b)
	}
	var back Ranking
	if err := json.Unmarshal(b, &back); err != nil || !back.Candidates[0].Flags.Has(FlagMain|FlagPlayAll) {
		t.Fatalf("round trip: %v %+v", err, back.Candidates[0])
	}
	// Weights change the answer: without the distinct-time weight and with
	// a heavy reward for being first, the playlists' order is up to the rest.
	w := DefaultWeights
	w.DistinctPerMinute = 0
	if r := d.Analyze(RankOptions{Weights: &w}); r.Main == 20 && r.Candidates[0].Score != 0 {
		t.Logf("weights: main %d score %.1f", r.Main, r.Candidates[0].Score)
	}
	// Sizes given by the caller replace the files'.
	r := d.Analyze(RankOptions{ClipSizes: map[string]int64{"00001": 0}})
	if !r.Candidate(11).Flags.Has(FlagZeroSize) {
		t.Errorf("caller's sizes ignored: %+v", r.Candidate(11))
	}
}

func TestRankNoDisc(t *testing.T) {
	var d *Disc
	if r := d.Analyze(RankOptions{}); r.Main != -1 || len(r.Candidates) != 0 {
		t.Fatalf("%+v", r)
	}
	d = &Disc{Playlists: map[int]*Playlist{1: {Number: 1}}, Clips: map[string]*Clip{}}
	if r := d.Analyze(RankOptions{}); r.Main != -1 || d.MainPlaylist() != nil {
		t.Fatalf("an empty playlist was picked: %+v", r)
	}
}

func TestObjectTargets(t *testing.T) {
	m := MovieObject{Commands: []Command{
		play(7),                     // PLAY_PL 7
		{0x22C10000, 8, 3},          // PLAY_PL_PI 8, 3
		{0x22C20000, 9, 0x80000000}, // PLAY_PL_PM 9, none
		move(4, 900),                // MOVE GPR4, 900
		{0x22000000, 4, 0},          // PLAY_PL GPR4
		{0x22000000, 5, 0},          // PLAY_PL GPR5 (unknown)
		{0x50400003, 4, 1},          // ADD GPR4, 1: no longer known
		{0x22000000, 4, 0},          // PLAY_PL GPR4 (unknown)
		jumpObject(2),               // JUMP_OBJECT 2
		jumpTitle(3),                // JUMP_TITLE 3
		{0x21820000, 1, 0},          // CALL_OBJECT 1
		{0x21830000, 2, 0},          // CALL_TITLE 2
		{0x48400200, 0x80000004, 1}, // EQ PSR4, 1 (ignored)
		{0x21840000, 0, 0},          // RESUME (ignored)
	}}
	got := ObjectTargets(m, 0)
	want := []struct {
		op         string
		value, arg int
		reg        bool
	}{
		{"PLAY_PL", 7, -1, false}, {"PLAY_PL_PI", 8, 3, false}, {"PLAY_PL_PM", 9, -1, false},
		{"PLAY_PL", 900, -1, true}, {"PLAY_PL", -1, -1, false}, {"PLAY_PL", -1, -1, false},
		{"JUMP_OBJECT", 2, -1, false}, {"JUMP_TITLE", 3, -1, false}, {"CALL_OBJECT", 1, -1, false}, {"CALL_TITLE", 2, -1, false},
	}
	if len(got) != len(want) {
		t.Fatalf("%d targets: %+v", len(got), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Op != w.op || g.Value != w.value || g.Arg != w.arg || g.FromRegister != w.reg {
			t.Errorf("target %d: %+v, want %+v", i, g, w)
		}
	}
	if !got[0].IsPlay() || got[6].IsPlay() {
		t.Error("IsPlay")
	}
}

func TestStaticReach(t *testing.T) {
	objs := []MovieObject{
		{Commands: []Command{play(1), jumpObject(1)}},
		{Commands: []Command{play(2), jumpTitle(2), jumpObject(0)}}, // loops back to 0
		{Commands: []Command{play(3), jumpTitle(0xffff)}},
		{Commands: []Command{play(4)}},
	}
	d := &Disc{MovieObjects: objs, Index: &Index{
		FirstPlay: hdmv(0),
		Titles:    []Title{{Number: 1, Object: hdmv(3)}, {Number: 2, Object: hdmv(2)}, {Number: 3, Object: IndexObject{Kind: ObjectBDJ, BDJName: "00001"}}},
	}}
	r := d.FirstPlayReach()
	if !slices.Equal(r.Playlists, []int{1, 2, 3}) || !slices.Equal(r.Titles, []int{2, 0xffff}) || !slices.Equal(r.Objects, []int{0, 1, 2}) {
		t.Fatalf("first play: %+v", r)
	}
	if r := d.StaticReach(hdmv(0), false); !slices.Equal(r.Playlists, []int{1, 2}) {
		t.Fatalf("without titles: %+v", r)
	}
	if r := d.TitleReach(1); !slices.Equal(r.Playlists, []int{4}) {
		t.Fatalf("title 1: %+v", r)
	}
	if r := d.TitleReach(3); !r.BDJ || len(r.Playlists) != 0 {
		t.Fatalf("BD-J title: %+v", r)
	}
	if r := d.TitleReach(9); len(r.Playlists) != 0 {
		t.Fatalf("no such title: %+v", r)
	}
	// Through the real parser: a MovieObject.bdmv written out byte by byte.
	parsed, err := ParseMovieObjects(buildMobj(objs...))
	if err != nil || len(parsed) != 4 {
		t.Fatalf("parse: %v %d", err, len(parsed))
	}
	d.MovieObjects = parsed
	if r := d.FirstPlayReach(); !slices.Equal(r.Playlists, []int{1, 2, 3}) {
		t.Fatalf("parsed: %+v", r)
	}
}

// The ranking reads whatever a disc's playlists say, so no playlist may
// crash it or make it pick something unreadable.
func FuzzRankPlaylists(f *testing.F) {
	f.Add(fxMpls(fxPlaylist{items: []fxItem{whole("00001", 10)}}), fxMpls(fxPlaylist{items: []fxItem{whole("00002", 1), whole("00001", 10)}}), uint8(3))
	f.Add([]byte("MPLS0200"), []byte{}, uint8(0))
	f.Fuzz(func(t *testing.T, a, b []byte, seed uint8) {
		d := &Disc{Playlists: map[int]*Playlist{}, Clips: map[string]*Clip{}, Index: &Index{FirstPlay: hdmv(0), Titles: []Title{{Number: 1, Object: hdmv(0)}}}}
		for i, raw := range [][]byte{a, b} {
			if pl, err := ParsePlaylist(i, raw); err == nil {
				d.Playlists[i] = pl
			}
		}
		if seed&1 != 0 {
			cb, _ := ParseClip("00001", fxClip(32, sec, sec+uint32(seed)*sec, int(seed>>4)))
			if cb != nil {
				d.Clips["00001"] = cb
			}
		}
		var cmds []Command
		for i := 0; i+12 <= len(b) && len(cmds) < 32; i += 12 {
			cmds = append(cmds, Command{binary.BigEndian.Uint32(b[i:]), binary.BigEndian.Uint32(b[i+4:]), binary.BigEndian.Uint32(b[i+8:])})
		}
		d.MovieObjects = []MovieObject{{Commands: cmds}}
		r := d.Analyze(RankOptions{ClipSizes: map[string]int64{"00001": int64(seed) - 1}})
		_ = r.Explain()
		if r.Main >= 0 {
			if d.Playlists[r.Main] == nil {
				t.Fatalf("picked playlist %d that isn't there", r.Main)
			}
			if c := r.Candidate(r.Main); c.Distinct <= 0 {
				t.Fatalf("picked an unreadable playlist: %+v", c)
			}
		}
	})
}

// A static reading of any program must come back and stay bounded.
func FuzzMovieObject(f *testing.F) {
	f.Add(buildMobj(MovieObject{Commands: []Command{play(1), jumpObject(0), move(1, 5), {0x22000000, 1, 0}}}))
	f.Add([]byte("MOBJ0200"))
	f.Fuzz(func(t *testing.T, b []byte) {
		objs, err := ParseMovieObjects(b)
		if err != nil {
			return
		}
		for i, o := range objs {
			if ts := ObjectTargets(o, i); len(ts) > len(o.Commands) {
				t.Fatalf("%d targets from %d commands", len(ts), len(o.Commands))
			}
		}
		d := &Disc{MovieObjects: objs, Index: &Index{FirstPlay: hdmv(0), Titles: []Title{{Number: 1, Object: hdmv(0)}, {Number: 2, Object: hdmv(1)}}}}
		r := d.FirstPlayReach()
		if len(r.Objects) > maxReachObjects {
			t.Fatalf("visited %d objects", len(r.Objects))
		}
		_ = d.TitleReach(2)
	})
}
