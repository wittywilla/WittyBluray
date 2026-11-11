// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"encoding/binary"
	"math"
	"sort"
	"strings"
	"testing"
	"time"
)

// The parsers read whatever is on a disc image, so no input may crash them.
// `go test -fuzz=FuzzParsers` explores further; the seeds run with every test.

func FuzzParsers(f *testing.F) {
	f.Add([]byte("INDX0200"), uint8(0))
	f.Add([]byte("MOBJ0200\x00\x00\x00\x00"), uint8(1))
	f.Add([]byte("MPLS0200\x00\x00\x00\x3a\x00\x00\x00\x00"), uint8(2))
	f.Add([]byte("HDMV0200\x00\x00\x00\x00\x00\x00\x00\x00"), uint8(3))
	f.Add([]byte{0x16, 0x00, 0x10}, uint8(4))
	f.Add([]byte{0x15, 0x00, 0x05, 0x00, 0x01}, uint8(5))
	f.Fuzz(func(t *testing.T, b []byte, which uint8) {
		switch which % 7 {
		case 0:
			_, _ = ParseIndex(b)
		case 1:
			_, _ = ParseMovieObjects(b)
		case 2:
			_, _ = ParsePlaylist(1, b)
		case 3:
			_, _ = ParseClip("00001", b)
		case 4:
			m := &Menu{}
			_ = m.parseIC(b)
		case 5:
			_, _ = parsePDS(b, matrix709)
			_, _ = ParseBDJObject("00000", b)
			if s, err := ParseSounds(b); err == nil {
				for i := range s.Clips {
					_, _ = s.WAV(i)
				}
			}
			_, _ = parseDialogStyle(b)
			_ = parseDialog(b, nil, &Palette{}, 1)
			_, _ = parseCompObject(b)
			_ = parseWindows(b)
		case 6:
			_, _ = decodeRLE(b, 64, 64)
		}
	})
}

func navDisc(objs []MovieObject) *Disc {
	first := IndexObject{Kind: ObjectHDMV, MovieObject: 0}
	return &Disc{
		Index:        &Index{FirstPlay: first, TopMenu: first, Titles: []Title{{Object: first}}},
		MovieObjects: objs,
		Playlists:    map[int]*Playlist{1: {Number: 1, Items: []PlayItem{{Clip: "00001"}}}},
		Clips:        map[string]*Clip{},
	}
}

// A disc's program jumping to its own first-play title recursed until the Go
// stack overflowed, which ends the whole server: unlike a panic it can't be
// recovered, so a single crafted disc image took everything down.
func TestNavProgramsCantRecurse(t *testing.T) {
	for name, cmd := range map[string]Command{
		"jump to first play": {0x21810000, 0xffff, 0},
		"jump to top menu":   {0x21810000, 0, 0},
		"jump to title 1":    {0x21810000, 1, 0},
		"call title 1":       {0x21830000, 1, 0},
		"call first play":    {0x21830000, 0xffff, 0},
	} {
		n := NewNav(navDisc([]MovieObject{{Commands: []Command{cmd}}}), NavOptions{})
		if st := n.Start(); st == nil || st.Kind != StepStop {
			t.Errorf("%s: %+v, want a stop", name, st)
		}
	}
}

// Running a disc's programs must always come back, however they are written.
func FuzzNav(f *testing.F) {
	f.Add([]byte{0x21, 0x81, 0, 0, 0, 0, 0xff, 0xff, 0, 0, 0, 0, 0x20, 0x81, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, uint8(0))
	f.Add([]byte{0x22, 0x80, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0x21, 0x84, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, uint8(3))
	f.Fuzz(func(t *testing.T, b []byte, script uint8) {
		var cmds []Command
		for i := 0; i+12 <= len(b) && len(cmds) < 64; i += 12 {
			cmds = append(cmds, Command{binary.BigEndian.Uint32(b[i:]), binary.BigEndian.Uint32(b[i+4:]), binary.BigEndian.Uint32(b[i+8:])})
		}
		n := NewNav(navDisc([]MovieObject{{Commands: cmds}, {Commands: cmds}}), NavOptions{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			n.Start()
			for i := uint8(0); i < script%8; i++ {
				n.PressButton(cmds)
				n.PlaylistEnded()
				n.MenuCall()
				n.Resume()
				n.NavTimerFired()
				n.PlayTitle(int(script))
			}
		}()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Fatal("a disc's program never came back")
		}
	})
}

// One fuzzer for each file a disc gives the engine, seeded with files the
// synthetic generator writes (a film with angles, a 3D table, a menu
// stream, entry-point maps, a BD-J object and a JAR) and with damaged
// copies of them. Each checks more than "no panic": what comes back must
// stay inside the limits the parsers promise.

func fuzzSeedDisc(f *testing.F) (*synDisc, map[string][]byte) {
	s := newSyn(99)
	film := s.film(6, 300, evidence{carry: true})
	p := s.feature(1, film)
	p.ss, p.hdr, p.dv = true, 1, true
	p.items[1].angles = []string{film[0], film[2]}
	p.items[2].ig = true
	s.decoys(film, 3, 2, "permute", "repeat", "dust")
	s.objs = []MovieObject{
		{Commands: []Command{play(1), move(7, 2), {0x50400003, 7, 1}, jumpTitle(1)}},
		{ResumeIntention: true, Commands: []Command{{0x48400200, 7, 3}, {0x22000000, 7, 0}, {0x20810000, 0, 0}, {0x21820000, 0, 0}}},
	}
	s.titles = []IndexObject{hdmv(1), {Kind: ObjectBDJ, BDJName: "00000"}}
	s.extra["BDMV/BDJO/00000.bdjo"] = synBDJO("00000", 1, 2)
	return s, s.structures()
}

// seeds adds a file, the file cut short in a few places, and the file with
// its early length and address fields at their extremes.
func seeds(f *testing.F, b []byte) {
	f.Add(b)
	for _, n := range []int{len(b) / 4, len(b) / 2, len(b) - 1} {
		if n > 0 && n < len(b) {
			f.Add(b[:n])
		}
	}
	for _, at := range []int{8, 12, 16} {
		if at+4 <= len(b) {
			c := append([]byte(nil), b...)
			binary.BigEndian.PutUint32(c[at:], 0xfffffff0)
			f.Add(c)
		}
	}
}

func FuzzIndex(f *testing.F) {
	_, files := fuzzSeedDisc(f)
	seeds(f, files["BDMV/index.bdmv"])
	f.Add([]byte("INDX0300"))
	f.Fuzz(func(t *testing.T, b []byte) {
		ix, err := ParseIndex(b)
		if err != nil {
			return
		}
		if len(ix.Titles) > 1000 {
			t.Fatalf("%d titles", len(ix.Titles))
		}
		for i, tt := range ix.Titles {
			if tt.Number != i+1 || tt.Object.MovieObject < 0 || len(tt.Object.BDJName) > 5 {
				t.Fatalf("title %d: %+v", i, tt)
			}
		}
	})
}

func FuzzMovieObjects(f *testing.F) {
	_, files := fuzzSeedDisc(f)
	seeds(f, files["BDMV/MovieObject.bdmv"])
	f.Fuzz(func(t *testing.T, b []byte) {
		objs, err := ParseMovieObjects(b)
		if err != nil {
			return
		}
		n := 0
		for _, o := range objs {
			n += len(o.Commands)
		}
		if n*12 > len(b) {
			t.Fatalf("%d commands from %d bytes", n, len(b))
		}
		d := &Disc{MovieObjects: objs, Playlists: map[int]*Playlist{}, Index: &Index{FirstPlay: hdmv(0), Titles: []Title{{Number: 1, Object: hdmv(1)}}}}
		for _, r := range []Reach{d.FirstPlayReach(), d.TitleReach(1), d.StaticReach(hdmv(0), false)} {
			if len(r.Objects) > maxReachObjects || len(r.Targets) > maxAbsTargets {
				t.Fatalf("%d objects, %d targets", len(r.Objects), len(r.Targets))
			}
		}
	})
}

func FuzzPlaylist(f *testing.F) {
	_, files := fuzzSeedDisc(f)
	for _, n := range []string{"00001", "00002", "00003", "00004"} {
		seeds(f, files["BDMV/PLAYLIST/"+n+".mpls"])
	}
	seeds(f, fxMpls(fxPlaylist{items: []fxItem{whole("00001", 10)}, ss: true}))
	f.Fuzz(func(t *testing.T, b []byte) {
		pl, err := ParsePlaylist(7, b)
		if err != nil {
			return
		}
		if len(pl.Items) > maxPlayItems || len(pl.Marks) > maxMarks || len(pl.SubPaths) > 255 || len(pl.Extensions) > 32 {
			t.Fatalf("%d items, %d marks, %d sub-paths, %d extensions", len(pl.Items), len(pl.Marks), len(pl.SubPaths), len(pl.Extensions))
		}
		streams := 0
		for i := range pl.Items {
			it := &pl.Items[i]
			k := len(it.Video) + len(it.Audio) + len(it.PG) + len(it.IG) + len(it.SecAudio) + len(it.SecVideo) + len(it.PiPPG) + len(it.DV)
			if k > maxStreamsPerItem || len(it.Angles) < 1 || len(it.Angles) > 10 {
				t.Fatalf("item %d: %d streams, %d angles", i, k, len(it.Angles))
			}
			streams += k
		}
		if streams > maxPlaylistStreams {
			t.Fatalf("%d streams", streams)
		}
		chs := pl.Chapters()
		total := (&Disc{}).PlaylistDuration(pl)
		for i, c := range chs {
			if c.Number != i+1 || c.Start < 0 || c.Start > total+0.001 || c.Duration < 0 || c.Item < 0 || c.Item >= len(pl.Items) || (i > 0 && c.Start <= chs[i-1].Start) {
				t.Fatalf("chapter %d of %d in %.3f s: %+v", i, len(chs), total, c)
			}
		}
		// Ranked by itself and beside a copy: never a pick that cannot play.
		d := &Disc{Playlists: map[int]*Playlist{7: pl, 8: pl}, Clips: map[string]*Clip{}}
		if r := d.Analyze(RankOptions{}); r.Main >= 0 && r.Candidate(r.Main).Distinct <= 0 {
			t.Fatalf("picked a playlist with nothing to play")
		}
	})
}

func checkClip(t *testing.T, c *Clip, size int) {
	n := 0
	for _, m := range c.EPMaps {
		n += len(m.Entries)
	}
	if n > maxEPEntriesPerClip || n*4 > size || len(c.STC) > maxSTCPerClip || len(c.EPMaps) > 32 || len(c.Programs) > 255 {
		t.Fatalf("%d entries, %d time bases, %d maps, %d programs from %d bytes", n, len(c.STC), len(c.EPMaps), len(c.Programs), size)
	}
	if d := c.Duration(); d < 0 || math.IsNaN(d) {
		t.Fatalf("duration %v", d)
	}
	for stc := -1; stc < 3; stc++ {
		for _, pts := range []uint32{0, 45000, 1 << 31, 0xffffffff} {
			c.EntryAt(c.VideoPID(), stc, pts)
			c.EntryAfter(c.VideoPID(), stc, pts)
		}
	}
	if s, ok := c.PlayableSeconds(int64(size) * 1000); ok && (s < 0 || math.IsNaN(s)) {
		t.Fatalf("playable %v", s)
	}
	c.EndPTS(0)
	c.STCRange(1)
}

func FuzzClip(f *testing.F) {
	s, files := fuzzSeedDisc(f)
	for n := range s.clips {
		seeds(f, files["BDMV/CLIPINF/"+n+".clpi"])
		break
	}
	seeds(f, buildClip(32, STCSequence{PCRPID: 0x1001, Start: 45000, End: 90000}, STCSequence{PCRPID: 0x1001, StartSPN: 16, Start: 45000, End: 90000}))
	f.Fuzz(func(t *testing.T, b []byte) {
		if c, err := ParseClip("00001", b); err == nil {
			checkClip(t, c, len(b))
		}
	})
}

// FuzzEPMap fuzzes the entry-point map alone: the bytes are the map of one
// stream, set behind a clip header that is always valid, so the fuzzer
// spends its time on the coarse and fine tables.
func FuzzEPMap(f *testing.F) {
	head := buildClip(4096, STCSequence{PCRPID: 0x1001, Start: 45000, End: 45000 * 600})
	binary.BigEndian.PutUint32(head[16:], uint32(len(head)))
	wrap := func(counts uint64, tables []byte) []byte {
		body := cat([]byte{0, 1}, be16(0x1011), be16(int(counts>>32)), be32(uint32(counts)), be32(14), tables)
		return cat(head, be32(uint32(len(body)+2)), be16(1), body)
	}
	s := newSyn(98)
	c := s.addClip("00001", 3*sec, 4000)
	c.epSec = 2
	whole := s.clpi(c)
	cpi := int(binary.BigEndian.Uint32(whole[16:]))
	f.Add(uint16(1), uint32(2000), whole[cpi+6+2+12:])
	f.Add(uint16(2), uint32(2), cat(be32(20), be32(0), be32(0), be32(1<<14), be32(1<<17), be32(5<<17|9), be32(9<<17|3)))
	f.Add(uint16(65535), uint32(262143), cat(be32(4), be32(0xffffc000)))
	f.Fuzz(func(t *testing.T, nCoarse uint16, nFine uint32, tables []byte) {
		b := wrap(uint64(1)<<34|uint64(nCoarse)<<18|uint64(nFine&0x3ffff), tables)
		c, err := ParseClip("00001", b)
		if err != nil {
			t.Fatalf("a clip with a valid header and a damaged map must still parse: %v", err)
		}
		checkClip(t, c, len(b))
	})
}

func FuzzBDJO(f *testing.F) {
	_, files := fuzzSeedDisc(f)
	seeds(f, files["BDMV/BDJO/00000.bdjo"])
	f.Fuzz(func(t *testing.T, b []byte) {
		o, err := ParseBDJObject("00000", b)
		if err != nil {
			return
		}
		if len(o.Playlists) > 2047 || len(o.Apps) > 255 || len(o.Cache) > 255 {
			t.Fatalf("%d playlists, %d applications, %d cache items", len(o.Playlists), len(o.Apps), len(o.Cache))
		}
		o.Autostart()
	})
}

func FuzzClassConstants(f *testing.F) {
	seeds(f, synClass([]string{"bd://0.PLAYLIST:00800", "00801"}, []int{5, 1000, 70000}))
	f.Add([]byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 52, 0, 3, 5, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		strs, ints := ClassConstants(b)
		total := 0
		for _, s := range strs {
			total += len(s)
			playlistNumbersIn(s)
		}
		if total > len(b) || len(ints) > len(b) {
			t.Fatalf("%d bytes of strings and %d numbers from %d bytes", total, len(ints), len(b))
		}
	})
}

func FuzzDiscMeta(f *testing.F) {
	f.Add([]byte(`<disclib><di:discinfo xmlns:di="urn:BDA:bdmv;discinfo"><di:title><di:name>X</di:name></di:title><di:tableOfContents><di:titleName titleNumber="1">Y</di:titleName></di:tableOfContents></di:discinfo></disclib>`))
	f.Add([]byte(`<!DOCTYPE a [<!ENTITY b "c">]><a>&b;`))
	f.Fuzz(func(t *testing.T, b []byte) {
		if m := ParseDiscMeta(b); m != nil && (len(m.Name) > 512 || len(m.Titles) > 1000) {
			t.Fatalf("%d-byte name, %d titles", len(m.Name), len(m.Titles))
		}
	})
}

// FuzzDisc changes one file of a whole disc and opens and ranks it: the
// cross-checks, the backup repair and the ranking all see the damage.
func FuzzDisc(f *testing.F) {
	s, files := fuzzSeedDisc(f)
	var names []string
	for n := range files {
		if !strings.Contains(n, "BACKUP") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for i, n := range names {
		f.Add(uint8(i), files[n], true)
		f.Add(uint8(i), files[n][:len(files[n])/2], false)
	}
	src := s.source()
	f.Fuzz(func(t *testing.T, which uint8, b []byte, backup bool) {
		name := names[int(which)%len(names)]
		m := &memSource{files: map[string][]byte{}, names: src.names, streams: src.streams}
		for k, v := range src.files {
			m.files[k] = v
		}
		m.files[strings.ToUpper(name)] = b
		if !backup {
			m.files[strings.ToUpper(strings.Replace(name, "BDMV/", "BDMV/BACKUP/", 1))] = b
		}
		d, err := OpenSource(m)
		if err != nil {
			return
		}
		r := d.Analyze(RankOptions{})
		_ = r.Explain()
		d.Consistency()
		d.BDJStaticHints()
		if r.Main >= 0 {
			if d.Playlists[r.Main] == nil || r.Candidate(r.Main).Distinct <= 0 || r.Confidence <= 0 || r.Confidence > 1 {
				t.Fatalf("main %d: %+v", r.Main, r.Candidate(r.Main))
			}
		}
		if n := NewNav(d, NavOptions{}); n.Start() == nil {
			t.Fatal("the disc does not start")
		}
	})
}
