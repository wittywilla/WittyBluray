// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"encoding/binary"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// Damage, truncation and hostile input: what a 1:1 copy that went wrong, or
// a file made to break a parser, must not be able to do.

func TestChaptersAcrossPlayItems(t *testing.T) {
	pl := &Playlist{
		Items: []PlayItem{
			{Clip: "00001", In: 10 * sec, Out: 110 * sec},  // 100 s
			{Clip: "00002", In: 500 * sec, Out: 560 * sec}, // 60 s, starts at 100
			{Clip: "00003", In: 20 * sec, Out: 60 * sec},   // 40 s, starts at 160
		},
		Marks: []Mark{
			{Type: 1, Item: 0, Time: 10 * sec},  // 0
			{Type: 2, Item: 0, Time: 50 * sec},  // a link point: not a chapter
			{Type: 1, Item: 1, Time: 530 * sec}, // 130
			{Type: 1, Item: 0, Time: 60 * sec},  // 50, out of order in the file
			{Type: 1, Item: 2, Time: 5 * sec},   // before its item: moved to 160
			{Type: 1, Item: 7, Time: 0},         // no such item: left out
			{Type: 1, Item: 1, Time: 530 * sec}, // the same instant again
			{Type: 1, Item: 2, Time: 900 * sec}, // past its item's end: would start at the very end
		},
	}
	got := pl.Chapters()
	want := []struct {
		start, dur float64
		item, mark int
		adjusted   bool
	}{{0, 50, 0, 0, false}, {50, 80, 0, 3, false}, {130, 30, 1, 2, false}, {160, 40, 2, 4, true}}
	if len(got) != len(want) {
		t.Fatalf("%d chapters: %+v", len(got), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Number != i+1 || math.Abs(g.Start-w.start) > 1e-9 || math.Abs(g.Duration-w.dur) > 1e-9 || g.Item != w.item || g.Mark != w.mark || g.Adjusted != w.adjusted {
			t.Errorf("chapter %d: %+v, want %+v", i+1, g, w)
		}
	}
	if (&Playlist{}).Chapters() != nil || (*Playlist)(nil).Chapters() != nil {
		t.Error("a playlist with nothing has no chapters")
	}
	if noItem, outside := pl.chapterFaults(); noItem != 1 || outside != 2 {
		t.Errorf("faults: %d with no item, %d outside", noItem, outside)
	}
}

// smallDisc is a film with decoys, small enough to write to a folder.
func smallDisc(seed int64) (*synDisc, []string) {
	s := newSyn(seed)
	film := s.film(6, 600, evidence{carry: true})
	s.feature(800, film)
	s.decoys(film, 12, 0, "permute", "repeat")
	s.extras(900)
	s.objs = []MovieObject{{Commands: []Command{jumpTitle(1)}}, {Commands: []Command{play(800)}}}
	s.titles = []IndexObject{hdmv(1)}
	return s.shrink(), film
}

func TestBackupFallbackAndRepair(t *testing.T) {
	s, film := smallDisc(21)
	dir := s.writeFolder(t)
	path := func(rel string) string { return filepath.Join(dir, "BDMV", filepath.FromSlash(rel)) }
	good, _ := os.ReadFile(path("PLAYLIST/00800.mpls"))

	// index.bdmv cut short: the BACKUP copy is read.
	ix, _ := os.ReadFile(path("index.bdmv"))
	os.WriteFile(path("index.bdmv"), ix[:50], 0o644)
	// A clip information file that is not one at all.
	os.WriteFile(path("CLIPINF/"+film[0]+".clpi"), []byte("junk"), 0o644)
	// The feature's playlist damaged in a way that still parses: its second
	// play item names a clip the disc does not have.
	bad := append([]byte(nil), good...)
	at := strings.Index(string(bad), film[1]+"M2TS")
	copy(bad[at:], "99998")
	os.WriteFile(path("PLAYLIST/00800.mpls"), bad, 0o644)
	// A playlist only the backup still has.
	os.Remove(path("PLAYLIST/00900.mpls"))

	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, want := range []string{"index.bdmv", "CLIPINF/" + film[0] + ".clpi", "PLAYLIST/00800.mpls", "PLAYLIST/00900.mpls"} {
		if !slices.Contains(d.FromBackup, want) {
			t.Errorf("%s was not taken from BACKUP: %v", want, d.FromBackup)
		}
	}
	if len(d.Damaged) != 0 {
		t.Errorf("damaged: %v", d.Damaged)
	}
	if !sameClips(d.Playlists[800], film) {
		t.Errorf("playlist 800 was not repaired")
	}
	if !strings.Contains(strings.Join(d.Notes, "\n"), "Playlist 00800 contradicts") {
		t.Errorf("notes: %v", d.Notes)
	}
	if r := d.Analyze(RankOptions{}); r.Main != 800 || r.FirstPlay != 800 {
		t.Errorf("main %d, first play %d", r.Main, r.FirstPlay)
	}

	// Both copies of a playlist damaged the same way: it stays, flagged.
	os.WriteFile(path("BACKUP/PLAYLIST/00800.mpls"), bad, 0o644)
	d2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	r := d2.Analyze(RankOptions{})
	if c := r.Candidate(800); c == nil || !c.Flags.Has(FlagMissingClip) {
		t.Errorf("playlist 800 with a missing clip: %+v", c)
	}
	found := false
	for _, x := range d2.Consistency() {
		found = found || (x.Kind == "clipMissing" && x.Playlist == 800 && x.Severity == "error")
	}
	if !found {
		t.Errorf("consistency does not name the missing clip: %+v", d2.Consistency())
	}
}

func TestMismatchedIndexAndMovieObjects(t *testing.T) {
	s, _ := smallDisc(22)
	dir := s.writeFolder(t)
	// The first copy of MovieObject.bdmv is an older, shorter one: the
	// index names an object it does not have. The backups agree.
	os.WriteFile(filepath.Join(dir, "BDMV", "MovieObject.bdmv"), buildMobj(MovieObject{Commands: []Command{jumpTitle(1)}}), 0o644)
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if len(d.MovieObjects) != 2 || !slices.Contains(d.FromBackup, "MovieObject.bdmv") {
		t.Fatalf("%d movie objects, from backup %v, notes %v", len(d.MovieObjects), d.FromBackup, d.Notes)
	}
	if r := d.Analyze(RankOptions{}); r.FirstPlay != 800 {
		t.Errorf("first play %d", r.FirstPlay)
	}
	// A backup that merely differs from a good first copy is only remarked on.
	s2, _ := smallDisc(23)
	dir2 := s2.writeFolder(t)
	os.WriteFile(filepath.Join(dir2, "BDMV", "BACKUP", "MovieObject.bdmv"), buildMobj(MovieObject{}, MovieObject{}), 0o644)
	d2, err := Open(dir2)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	if len(d2.FromBackup) != 0 || !strings.Contains(strings.Join(d2.Notes, "\n"), "MovieObject.bdmv and its BACKUP copy differ") {
		t.Errorf("from backup %v, notes %v", d2.FromBackup, d2.Notes)
	}
}

func TestMovieOnlyFolders(t *testing.T) {
	s, film := smallDisc(24)
	s.noProgram = true
	dir := s.writeFolder(t)
	os.RemoveAll(filepath.Join(dir, "BDMV", "BACKUP"))
	for _, p := range []string{dir, filepath.Join(dir, "BDMV")} {
		d, err := Open(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if !d.MovieOnly || len(d.Damaged) != 0 || d.Encrypted {
			t.Errorf("movie only %v, damaged %v, encrypted %v", d.MovieOnly, d.Damaged, d.Encrypted)
		}
		if mp := d.MainPlaylist(); !sameClips(mp, film) {
			t.Errorf("main playlist: %+v", mp)
		}
		st := NewNav(d, NavOptions{}).Start()
		if st == nil {
			t.Error("a movie-only copy does not start")
		}
		d.Close()
	}
	// Playlists gone too: each clip plays by itself.
	os.RemoveAll(filepath.Join(dir, "BDMV", "PLAYLIST"))
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if len(d.Playlists) != len(s.clips) {
		t.Errorf("%d playlists made from %d clips", len(d.Playlists), len(s.clips))
	}
}

// No prefix of any structure file, and no file with bytes changed, may
// panic a parser or make it run long.
func TestTruncatedAndCorruptStructures(t *testing.T) {
	s := newSyn(25)
	film := s.film(8, 300, evidence{carry: true})
	p := s.feature(1, film)
	p.ss, p.items[3].angles = true, []string{film[0]}
	p.items[2].ig = true
	s.extra["BDMV/BDJO/00000.bdjo"] = synBDJO("00000", 1, 2)
	files := s.structures()
	parse := func(name string, b []byte) {
		switch {
		case strings.HasSuffix(name, "index.bdmv"):
			ParseIndex(b)
		case strings.HasSuffix(name, "MovieObject.bdmv"):
			ParseMovieObjects(b)
		case strings.HasSuffix(name, ".mpls"):
			if pl, err := ParsePlaylist(1, b); err == nil {
				pl.Chapters()
			}
		case strings.HasSuffix(name, ".clpi"):
			if c, err := ParseClip("00001", b); err == nil {
				c.EntryAt(0x1011, 0, 123456)
				c.PlayableSeconds(1 << 20)
			}
		case strings.HasSuffix(name, ".bdjo"):
			ParseBDJObject("00000", b)
		}
	}
	rnd := rand.New(rand.NewSource(1))
	start := time.Now()
	for name, b := range files {
		if strings.Contains(name, "BACKUP") {
			continue
		}
		for n := 0; n <= len(b); n++ {
			parse(name, b[:n])
		}
		for i := 0; i < 3000; i++ {
			c := append([]byte(nil), b...)
			for k := 0; k < 1+rnd.Intn(4); k++ {
				c[rnd.Intn(len(c))] = byte(rnd.Intn(256))
			}
			parse(name, c)
		}
		for i := 8; i+4 <= len(b) && i < 400; i++ { // every early field at its extremes
			for _, v := range []uint32{0xffffffff, 0x7fffffff, 0x80000000, 0xfffffff0} {
				c := append([]byte(nil), b...)
				binary.BigEndian.PutUint32(c[i:], v)
				parse(name, c)
			}
		}
	}
	if el := time.Since(start); el > 20*time.Second {
		t.Errorf("parsing damaged files took %v", el)
	}
}

// A playlist cut off inside its chapter marks keeps its play items.
func TestDamagedPartsAreLeftOut(t *testing.T) {
	s := newSyn(26)
	film := s.film(4, 600, evidence{carry: true})
	s.feature(1, film)
	b := s.mpls(s.playlists[1])
	pl, err := ParsePlaylist(1, b[:len(b)-5])
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Items) != 4 || len(pl.Warnings) == 0 || len(pl.Marks) == 0 {
		t.Errorf("%d items, %d marks, warnings %v", len(pl.Items), len(pl.Marks), pl.Warnings)
	}
	// A stream table that claims more streams than it holds: the streams
	// before the damage stay, and the next play item is untouched.
	c := append([]byte(nil), b...)
	stn := strings.Index(string(c), film[0]+"M2TS") + 32
	c[stn+5] = 200 // audio streams
	pl, err = ParsePlaylist(1, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Items) != 4 || len(pl.Items[0].Video) != 1 || len(pl.Items[1].Audio) != 2 || len(pl.Warnings) == 0 {
		t.Errorf("items %d, video %d, next item's audio %d, warnings %v", len(pl.Items), len(pl.Items[0].Video), len(pl.Items[1].Audio), pl.Warnings)
	}
	// Clip information with its entry-point map cut off.
	cl := s.clips[film[0]]
	cb := s.clpi(cl)
	clip, err := ParseClip(film[0], cb[:len(cb)-40])
	if err != nil {
		t.Fatal(err)
	}
	if clip.Duration() == 0 || len(clip.Programs) != 1 || len(clip.EPMaps) != 1 {
		t.Errorf("duration %v, programs %d, maps %d, warnings %v", clip.Duration(), len(clip.Programs), len(clip.EPMaps), clip.Warnings)
	}
	binary.BigEndian.PutUint32(cb[12:], 0xfffffff0) // the program table's address is nonsense
	clip, err = ParseClip(film[0], cb)
	if err != nil || len(clip.Warnings) == 0 || clip.Duration() == 0 || len(clip.EPMaps) != 1 {
		t.Errorf("err %v, clip %+v", err, clip)
	}
}

func TestStreamTablesAreShared(t *testing.T) {
	s := newSyn(27)
	film := s.film(20, 100, evidence{carry: true})
	s.feature(1, film)
	s.decoys(film, 50, 2, "permute")
	d := s.open(t)
	a, b := d.Playlists[1].Items, d.Playlists[30].Items
	if &a[0].Audio[0] != &a[19].Audio[0] || &a[0].Audio[0] != &b[5].Audio[0] {
		t.Error("play items with the same stream table do not share it")
	}
	if d.stn.streams > 20 {
		t.Errorf("%d streams kept for one stream table", d.stn.streams)
	}
}

// The entry-point map splits a time across a coarse and a fine entry that
// overlap in one bit; every entry must come back within a fine step (5.7 ms)
// of the time it was written with, and packet numbers past 2^17 exactly.
func TestEntryPointMapTimes(t *testing.T) {
	s := newSyn(28)
	c := s.addClip("00001", 3*sec, 3*3600)
	c.epSec = 1
	c.packets = 9 << 20
	clip, err := ParseClip("00001", s.clpi(c))
	if err != nil || len(clip.EPMaps) != 1 || len(clip.Warnings) != 0 {
		t.Fatalf("%v %+v", err, clip.Warnings)
	}
	es := clip.EPMaps[0].Entries
	if len(es) != 3*3600 {
		t.Fatalf("%d entries", len(es))
	}
	for k, e := range es {
		wantPTS := uint32(3*sec + k*sec)
		wantSPN := uint64(k*sec) * uint64(c.packets) / uint64(c.end-c.start)
		if d := int64(wantPTS) - int64(e.PTS); d < 0 || d > 256 || uint64(e.SPN) != wantSPN {
			t.Fatalf("entry %d: time %d (want %d), packet %d (want %d)", k, e.PTS, wantPTS, e.SPN, wantSPN)
		}
	}
	if e, ok := clip.EntryAt(0x1011, 0, 3*sec+7000*sec+sec/2); !ok || (e.PTS+300)/sec != 7003 {
		t.Errorf("entry at 7000.5 s: %+v", e)
	}
}

// A map whose coarse entries point back and forth would, read naively, give
// 65,535 x 262,143 entries from a file of a megabyte.
func TestHostileEntryPointMap(t *testing.T) {
	const nCoarse, nFine = 65535, 262143
	b := make([]byte, 60)
	copy(b, "HDMV0200")
	binary.BigEndian.PutUint32(b[8:], 60)
	seq := cat(be32(0), []byte{0, 1}, be32(0), []byte{1, 0}, be16(0x1001), be32(0), be32(0), be32(90000))
	b = append(b, seq...)
	binary.BigEndian.PutUint32(b[12:], uint32(len(b)))
	b = append(b, make([]byte, 6)...)
	binary.BigEndian.PutUint32(b[16:], uint32(len(b)))
	v := uint64(1)<<34 | uint64(nCoarse)<<18 | uint64(nFine)
	var coarse []byte
	for k := 0; k < nCoarse; k++ {
		ref := uint32(0)
		if k%2 == 1 {
			ref = nFine
		}
		coarse = append(coarse, cat(be32(ref<<14), be32(0))...)
	}
	body := cat([]byte{0, 1}, be16(0x1011), be16(int(v>>32)), be32(uint32(v)), be32(14), be32(uint32(4+len(coarse))), coarse, make([]byte, nFine*4))
	b = cat(b, be32(uint32(len(body)+2)), be16(1), body)
	start := time.Now()
	clip, err := ParseClip("00001", b)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, m := range clip.EPMaps {
		n += len(m.Entries)
	}
	if n > maxEPEntriesPerClip || time.Since(start) > 2*time.Second {
		t.Errorf("%d entries in %v", n, time.Since(start))
	}
}

// Counts are the file's word: 10,000 objects of 100,000 commands each in a
// file of a few bytes must not make room for 12 GB of commands.
func TestMovieObjectCountsAreNotTrusted(t *testing.T) {
	b := make([]byte, 40)
	copy(b, "MOBJ0200")
	b = append(b, make([]byte, 8)...)
	b = append(b, be16(10000)...)
	for i := 0; i < 40; i++ {
		b = append(b, cat(be16(0), be16(65535))...)
	}
	allocs := testing.AllocsPerRun(3, func() {
		if _, err := ParseMovieObjects(b); err == nil {
			t.Error("a cut-off file parsed")
		}
	})
	if allocs > 200 {
		t.Errorf("%v allocations", allocs)
	}
}

func TestAbstractReadingOfPrograms(t *testing.T) {
	eq := func(gpr, v uint32) Command { return Command{0x48400200, gpr, v} }   // EQ GPRn, v
	gotoPC := func(pc uint32) Command { return Command{0x20810000, pc, 0} }    // GOTO pc
	playReg := func(gpr uint32) Command { return Command{0x22000000, gpr, 0} } // PLAY_PL GPRn
	add := func(gpr, v uint32) Command { return Command{0x50400003, gpr, v} }  // ADD GPRn, v
	mul := func(gpr, v uint32) Command { return Command{0x50400005, gpr, v} }  // MUL GPRn, v
	callObj := func(n uint32) Command { return Command{0x21820000, n, 0} }     // CALL_OBJECT n
	rnd := func(gpr, v uint32) Command { return Command{0x50400008, gpr, v} }  // RND GPRn, v
	moveReg := func(dst, src uint32) Command { return Command{0x50000001, dst, src} }
	disc := func(objs ...MovieObject) *Disc {
		return &Disc{MovieObjects: objs, Index: &Index{FirstPlay: hdmv(0), Titles: []Title{{Number: 1, Object: hdmv(1)}}},
			Playlists: map[int]*Playlist{}}
	}

	// A register computed in first play chooses among a hundred playlists
	// in title 1; only the one it selects is reached.
	var table []Command
	for n := uint32(0); n < 100; n++ {
		table = append(table, eq(9, n), play(n))
	}
	d := disc(MovieObject{Commands: []Command{move(9, 6), mul(9, 7), add(9, 1), jumpTitle(1)}}, MovieObject{Commands: table})
	r := d.FirstPlayReach()
	if !slices.Equal(r.Playlists, []int{43}) || !slices.Equal(r.Certain, []int{43}) || r.Unknown != 0 || r.Incomplete {
		t.Errorf("computed register: %+v", r.Playlists)
	}
	// The same title entered with unknown registers could play any of them.
	if r := d.TitleReach(1); len(r.Playlists) != 100 || len(r.Certain) != 0 {
		t.Errorf("unknown register: %d playlists, certain %v", len(r.Playlists), r.Certain)
	}

	// "Have I been here before": on a disc just put in the registers are
	// zero, so the first-run branch is the one taken.
	d = disc(MovieObject{Commands: []Command{
		eq(1, 1), gotoPC(5), // if GPR1 == 1 goto 5
		move(1, 1), play(10), gotoPC(6),
		play(20),
		play(30),
	}})
	if r := d.FirstPlayReach(); !slices.Equal(r.Playlists, []int{10, 30}) {
		t.Errorf("first run: %v", r.Playlists)
	}
	if r := d.StaticReach(hdmv(0), false); !slices.Equal(r.Playlists, []int{10, 20, 30}) {
		t.Errorf("any run: %v", r.Playlists)
	}

	// A register that may hold several values gives one target each; one
	// drawn at random from a hundred gives none, counted as unknown.
	d = disc(MovieObject{Commands: []Command{rnd(2, 3), add(2, 800), playReg(2), rnd(3, 100), playReg(3), playReg(0x80000004)}})
	r = d.FirstPlayReach()
	if !slices.Equal(r.Playlists, []int{801, 802, 803}) || r.Unknown != 2 || len(r.Certain) != 0 {
		t.Errorf("sets of values: %v, unknown %d, certain %v", r.Playlists, r.Unknown, r.Certain)
	}
	if ts := ObjectTargets(d.MovieObjects[0], 0); len(ts) != 5 || ts[0].Alternatives != 3 || !ts[0].FromRegister {
		t.Errorf("targets: %+v", ts)
	}

	// Registers travel into called objects and back out as unknown.
	d = disc(MovieObject{Commands: []Command{move(5, 77), callObj(1), playReg(5)}}, MovieObject{Commands: []Command{moveReg(6, 5), playReg(6)}})
	if r := d.FirstPlayReach(); !slices.Equal(r.Playlists, []int{77}) || !slices.Equal(r.Objects, []int{0, 1}) {
		t.Errorf("through a call: %+v", r)
	}

	// After a playlist with a menu the registers may be anything, so both
	// ways of a comparison are followed; after one without, they are known.
	menu := &Playlist{Number: 1, Items: []PlayItem{{Clip: "00001", IG: []Stream{{Kind: "ig"}}}}}
	logo := &Playlist{Number: 2, Items: []PlayItem{{Clip: "00002"}}}
	prog := func(first uint32) MovieObject {
		return MovieObject{Commands: []Command{move(4, 1), play(first), eq(4, 1), gotoPC(5), play(50), play(60)}}
	}
	d = disc(prog(1))
	d.Playlists = map[int]*Playlist{1: menu, 2: logo}
	if r := d.FirstPlayReach(); !slices.Equal(r.Playlists, []int{1, 50, 60}) {
		t.Errorf("after a menu: %v", r.Playlists)
	}
	d.MovieObjects[0] = prog(2)
	if r := d.FirstPlayReach(); !slices.Equal(r.Playlists, []int{2, 60}) {
		t.Errorf("after a logo: %v", r.Playlists)
	}

	// Loops end: a counter that never stops, a jump to itself, objects
	// that call each other, and a program far past the budget.
	d = disc(MovieObject{Commands: []Command{add(1, 1), play(5), gotoPC(0)}}, MovieObject{Commands: []Command{gotoPC(0)}},
		MovieObject{Commands: []Command{{0x21800000, 3, 0}}}, MovieObject{Commands: []Command{{0x21800000, 2, 0}}})
	for _, o := range []int{0, 1, 2} {
		if r := d.StaticReach(hdmv(o), true); o == 0 && !slices.Equal(r.Playlists, []int{5}) {
			t.Errorf("loop: %+v", r)
		}
	}
	// A long program is read to its end: 124,000 commands that write every
	// register and compare them all.
	var long []Command
	for i := uint32(0); i < 4096; i++ {
		long = append(long, move(i, i))
	}
	for i := 0; i < 60000; i++ {
		long = append(long, add(uint32(i%4096), 1), eq(uint32(i%4096), 7))
	}
	long = append(long, move(0, 0), play(9))
	d = disc(MovieObject{Commands: long})
	start := time.Now()
	if r := d.FirstPlayReach(); r.Incomplete || !slices.Equal(r.Playlists, []int{9}) {
		t.Errorf("a long program: incomplete %v, playlists %v", r.Incomplete, r.Playlists)
	}
	// One made to cost as much as it can (a loop in which every register
	// keeps taking new values) stops at the budget and says so.
	var loop []Command
	for i := uint32(0); i < 4096; i++ {
		loop = append(loop, add(i, 1))
	}
	loop = append(loop, gotoPC(0), play(9))
	d = disc(MovieObject{Commands: loop})
	if r := d.FirstPlayReach(); !r.Incomplete {
		t.Errorf("a program past the budget was read to the end")
	}
	if el := time.Since(start); el > 10*time.Second {
		t.Errorf("reading two large programs took %v", el)
	}
}

func TestClassConstantsAndBDJHints(t *testing.T) {
	class := synClass([]string{"bd://0.PLAYLIST:00800", "00801", "see 00802.mpls", "12 and 99999 monkeys"}, []int{5, 1000, 40000, -3})
	strs, ints := ClassConstants(class)
	if len(strs) != 4 || !slices.Equal(ints, []int32{5, 1000, 40000, -3}) {
		t.Errorf("strings %q, numbers %v", strs, ints)
	}
	for n := 0; n < len(class); n++ {
		ClassConstants(class[:n])
	}
	if s, i := ClassConstants([]byte("not a class")); s != nil || i != nil {
		t.Error("constants from nothing")
	}
	for s, want := range map[string][]int{
		"bd://0.PLAYLIST:00800": {800}, "00801": {801}, "BDMV/PLAYLIST/00802.mpls": {802},
		"12345 monkeys": nil, "0080": nil, "008000": nil, "bd://1.PLAYLIST:00001,bd://1.PLAYLIST:00002": {1, 2}, "abcde": nil,
	} {
		if got := playlistNumbersIn(s); !slices.Equal(got, want) {
			t.Errorf("%q: %v, want %v", s, got, want)
		}
	}
	s := newSyn(29)
	for _, n := range []int{800, 801, 802, 1000, 5} {
		name := s.freeNames(1)[0]
		s.addClip(name, 10*sec, 600)
		s.pl(n, &synPlaylist{items: s.items(name)})
	}
	j := IndexObject{Kind: ObjectBDJ, BDJName: "00000"}
	s.first, s.titles = j, []IndexObject{j}
	s.extra["BDMV/BDJO/00000.bdjo"] = synBDJO("00000", 5, 801)
	s.extra["BDMV/JAR/00000.jar"] = synJar(t, []string{"bd://0.PLAYLIST:00800", "00801", "see 00802.mpls", "77777"}, []int{5, 1000, 40000})
	s.extra["BDMV/JAR/00001.jar"] = []byte("PK\x03\x04 a damaged archive")
	h := s.open(t).BDJStaticHints()
	if h == nil || h.Jars != 1 || h.Classes != 1 || h.Objects != 1 || len(h.Hints) != 5 {
		t.Fatalf("%+v", h)
	}
	if x := h.Hint(800); x == nil || x.Locators != 1 || x.Listed {
		t.Errorf("800: %+v", x)
	}
	if x := h.Hint(801); x == nil || x.Names != 1 || !x.Listed {
		t.Errorf("801: %+v", x)
	}
	if x := h.Hint(1000); x == nil || x.Numbers != 1 {
		t.Errorf("1000: %+v", x)
	}
	if h.Hint(40000) != nil || h.Hint(77777) != nil {
		t.Error("a playlist the disc does not have was reported")
	}
}

func TestDiscMeta(t *testing.T) {
	xml := `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE disclib [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
<disclib xmlns="urn:BDA:bdmv;disclib"><di:discinfo xmlns:di="urn:BDA:bdmv;discinfo">
<di:title><di:name>A Film &amp; Its Sequel</di:name><di:numSets>1</di:numSets></di:title>
<di:description><di:tableOfContents>
<di:titleName titleNumber="1">Feature &xxe;</di:titleName><di:titleName titleNumber="2">Making of</di:titleName>
</di:tableOfContents></di:description></di:discinfo></disclib>`
	s := newSyn(30)
	s.addClip("00001", 10*sec, 600)
	s.pl(1, &synPlaylist{items: s.items("00001")})
	s.extra["BDMV/META/DL/bdmt_fra.xml"] = []byte(strings.Replace(xml, "A Film", "Un film", 1))
	s.extra["BDMV/META/DL/bdmt_eng.xml"] = []byte(xml)
	m := s.open(t).Meta()
	if m == nil || m.Name != "A Film & Its Sequel" || m.Language != "eng" || m.Titles[2] != "Making of" || !slices.Equal(m.Languages, []string{"eng", "fra"}) {
		t.Fatalf("%+v", m)
	}
	if strings.Contains(m.Titles[1], "root") {
		t.Error("an external entity was followed")
	}
	if ParseDiscMeta([]byte("<a><b>")) != nil || ParseDiscMeta(nil) != nil {
		t.Error("metadata from nothing")
	}
}
