// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"
	"time"
)

// Properties of the ranking that must hold whatever the disc: decoys added
// to a disc never change which playlist is the feature, and neither does
// giving the playlists other numbers.

// propertyDisc builds a film with some evidence of its order and a few
// extras, and returns the feature's playlist number.
func propertyDisc(seed int64) (*synDisc, []string, int) {
	rnd := rand.New(rand.NewSource(seed))
	s := newSyn(seed)
	ev := []evidence{{carry: true}, {layout: true}, {carry: true, layout: true}, {numbered: true}, {carry: true, numbered: true}}[rnd.Intn(5)]
	s.folderLike = !ev.layout
	film := s.film(6+rnd.Intn(18), 200+400*rnd.Float64(), ev)
	feature := 1 + rnd.Intn(1500)
	s.feature(feature, film)
	s.extras(3000)
	return s, film, feature
}

func TestAddingDecoysNeverChangesTheFeature(t *testing.T) {
	kinds := []string{"permute", "repeat", "subset", "dust", "missing"}
	for seed := int64(1); seed <= 40; seed++ {
		s, film, feature := propertyDisc(seed)
		if got := s.open(t).Analyze(RankOptions{}).Main; got != feature {
			t.Fatalf("seed %d: before any decoy the main is %05d, want %05d", seed, got, feature)
		}
		rnd := rand.New(rand.NewSource(seed * 7919))
		from := 0
		for round := 0; round < 3; round++ {
			var mix []string
			for i := 0; i < 1+rnd.Intn(4); i++ {
				mix = append(mix, kinds[rnd.Intn(len(kinds))])
			}
			n := 1 + rnd.Intn(150)
			s.decoys(film, n, from, mix...)
			from += n + 1
			r := s.open(t).Analyze(RankOptions{})
			if r.Main != feature {
				t.Fatalf("seed %d, round %d: %d decoys of %v changed the main from %05d to %05d\n%s", seed, round, n, mix, feature, r.Main, r.Explain())
			}
			if c := r.Candidate(feature); c.Flags.Has(FlagDecoy) || c.Flags.Has(FlagShuffled) {
				t.Fatalf("seed %d: the feature is flagged %s", seed, c.Flags)
			}
			for _, e := range r.Editions {
				if !e.Main {
					t.Fatalf("seed %d, round %d: decoy %05d taken for an edition", seed, round, e.Playlist)
				}
			}
		}
	}
}

func TestRenumberingPlaylistsNeverChangesTheFeature(t *testing.T) {
	for seed := int64(1); seed <= 25; seed++ {
		s, film, feature := propertyDisc(seed)
		s.decoys(film, 120, 0, "permute", "repeat", "subset", "dust")
		// A program that plays the feature by number, to be renumbered too.
		withProgram := seed%2 == 0
		if withProgram {
			s.objs = []MovieObject{{Commands: []Command{jumpTitle(1)}}, {Commands: []Command{play(uint32(feature))}}}
			s.titles = []IndexObject{hdmv(1)}
		}
		rnd := rand.New(rand.NewSource(seed))
		for round := 0; round < 4; round++ {
			var old []int
			for n := range s.playlists {
				old = append(old, n)
			}
			perm := rnd.Perm(5000)
			renamed := map[int]*synPlaylist{}
			to := map[int]int{}
			for i, n := range old {
				to[n] = perm[i]
				renamed[perm[i]] = s.playlists[n]
			}
			s.playlists = renamed
			feature = to[feature]
			if withProgram {
				s.objs[1].Commands[0] = play(uint32(feature))
			}
			r := s.open(t).Analyze(RankOptions{})
			if r.Main != feature {
				t.Fatalf("seed %d, round %d: renumbering moved the main to %05d, want %05d\n%s", seed, round, r.Main, feature, r.Explain())
			}
		}
	}
}

// bigDisc is an obfuscated disc of n playlists: one film of 30 reels and
// decoys of every kind.
func bigDisc(n int) (*synDisc, int) {
	s := newSyn(int64(n))
	film := s.film(30, 220, evidence{carry: true, layout: true})
	feature := n / 3
	s.feature(feature, film)
	s.decoys(film, n-1, 0, "permute", "permute", "repeat", "subset", "dust", "permute")
	return s, feature
}

func openAndRank(tb testing.TB, src *memSource, feature int) *Disc {
	d, err := OpenSource(src)
	if err != nil {
		tb.Fatal(err)
	}
	if r := d.Analyze(RankOptions{}); r.Main != feature {
		tb.Fatalf("main %05d, want %05d", r.Main, feature)
	}
	return d
}

// A disc with thousands of playlists must open and rank quickly and in
// bounded memory: small machines (a Raspberry Pi 3) run this. The time must
// grow with the number of playlists, not with its square.
func TestLargeDiscIsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("builds discs of thousands of playlists")
	}
	timeOf := func(n int) (time.Duration, uint64) {
		s, feature := bigDisc(n)
		src := s.source()
		best := time.Duration(1 << 62)
		var heap uint64
		for run := 0; run < 3; run++ {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			start := time.Now()
			d := openAndRank(t, src, feature)
			if el := time.Since(start); el < best {
				best = el
			}
			runtime.GC()
			runtime.ReadMemStats(&after)
			if after.HeapAlloc > before.HeapAlloc {
				heap = max(heap, after.HeapAlloc-before.HeapAlloc)
			}
			runtime.KeepAlive(d)
		}
		return best, heap
	}
	t2, heap2 := timeOf(2000)
	t8, heap8 := timeOf(8000)
	t.Logf("2,000 playlists: opened and ranked in %v, %d MB kept; 8,000 playlists: %v, %d MB", t2, heap2>>20, t8, heap8>>20)
	if t2 > 5*time.Second {
		t.Errorf("2,000 playlists took %v", t2)
	}
	if heap2 > 96<<20 {
		t.Errorf("2,000 playlists keep %d MB", heap2>>20)
	}
	// Four times the playlists: four times the work if linear, sixteen if
	// every playlist were compared with every other.
	if t8 > 9*t2+200*time.Millisecond {
		t.Errorf("8,000 playlists took %v against %v for 2,000: that grows faster than the disc", t8, t2)
	}
}

func BenchmarkOpenAndRank2000Playlists(b *testing.B) {
	s, feature := bigDisc(2000)
	src := s.source()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		openAndRank(b, src, feature)
	}
}

func BenchmarkRank2000Playlists(b *testing.B) {
	s, feature := bigDisc(2000)
	d := openAndRank(b, s.source(), feature)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if r := d.Analyze(RankOptions{}); r.Main != feature {
			b.Fatal(fmt.Sprint("main ", r.Main))
		}
	}
}
