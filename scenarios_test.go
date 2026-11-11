// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"testing"
)

// Each pattern commercial discs use to hide or vary the feature, built by
// the synthetic generator and ranked blind: the engine sees only what the
// files say.

// evidence says what on a synthetic disc tells the film's clip order.
type evidence struct {
	carry    bool // the clock runs through the clips (one encode cut up)
	layout   bool // the clips lie on the disc in playing order
	numbered bool // the clip numbers rise in playing order
}

// film adds the reels of a film and returns their names in playing order.
func (s *synDisc) film(reels int, reelSec float64, ev evidence) []string {
	names := s.freeNames(reels)
	if ev.numbered {
		sort.Strings(names)
	}
	lens := make([]float64, reels)
	starts := make([]uint32, reels)
	at := uint32(10 * sec)
	for i := range names {
		lens[i] = reelSec * (0.8 + 0.4*s.rnd.Float64()) // reels of uneven length
		starts[i] = 10 * sec
		if ev.carry {
			starts[i] = at
		}
		at += uint32(lens[i] * sec)
	}
	order := s.rnd.Perm(reels)
	if ev.layout {
		for i := range order {
			order[i] = i
		}
	}
	for _, i := range order {
		s.addClip(names[i], starts[i], lens[i]).epSec = 10
	}
	return names
}

// permuted returns the names in another order.
func (s *synDisc) permuted(names []string) []string {
	out := append([]string(nil), names...)
	for {
		s.rnd.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
		if !slices.Equal(out, names) {
			return out
		}
	}
}

// decoys adds n decoy playlists made from the film's clips, numbered from
// `from` and skipping numbers already taken. Kinds: "permute" (the same
// clips in another order), "repeat" (clips played again to run longer),
// "subset" (some clips, shuffled), "dust" (the film with fake clips of half
// a second between its reels, shuffled), "missing" (names a clip the disc
// does not have).
func (s *synDisc) decoys(film []string, n, from int, kinds ...string) {
	var dust []string
	for k, added := 0, 0; added < n; k++ {
		num := from + k
		if s.playlists[num] != nil {
			continue
		}
		var names []string
		switch kinds[added%len(kinds)] {
		case "permute":
			names = s.permuted(film)
		case "repeat":
			names = s.permuted(film)
			for i := 0; i < len(film); i++ {
				names = append(names, film[s.rnd.Intn(len(film))])
			}
		case "subset":
			// Some of the clips, out of order. (In order they would be a
			// shorter cut of the film, which is what the engine would and
			// should call them.)
			for {
				names = s.permuted(film)[:max(2, len(film)*2/3)]
				at := map[string]int{}
				for i, f := range film {
					at[f] = i
				}
				rising := true
				for i := 1; i < len(names); i++ {
					rising = rising && at[names[i]] > at[names[i-1]]
				}
				if !rising {
					break
				}
			}
		case "dust":
			if dust == nil {
				for _, d := range s.freeNames(8) {
					s.addClip(d, 10*sec, 0.5)
					dust = append(dust, d)
				}
			}
			for _, f := range s.permuted(film) {
				names = append(names, f, dust[s.rnd.Intn(len(dust))])
			}
		case "missing":
			names = append([]string(nil), film...)
			names[s.rnd.Intn(len(names))] = "99999"
		}
		var items []synItem
		for _, nm := range names {
			if s.clips[nm] == nil {
				items = append(items, synItem{clip: nm, in: 10 * sec, out: 700 * sec, audio: []string{"eng", "fra"}, pg: []string{"eng"}})
				continue
			}
			items = append(items, s.item(nm))
		}
		s.pl(num, &synPlaylist{items: items, marks: synChapters(items, 600)})
		added++
	}
}

// feature adds the film's playlist.
func (s *synDisc) feature(num int, film []string) *synPlaylist {
	items := s.items(film...)
	for i := range items {
		if i > 0 {
			items[i].cc = 5
		}
	}
	return s.pl(num, &synPlaylist{items: items, marks: synChapters(items, 600)})
}

// extras adds a trailer and a featurette.
func (s *synDisc) extras(from int) {
	s.addClip("00001", 10*sec, 150)
	s.addClip("00002", 10*sec, 780)
	s.pl(from, &synPlaylist{items: s.items("00001")})
	s.pl(from+1, &synPlaylist{items: s.items("00002")})
}

func sameClips(pl *Playlist, names []string) bool {
	if pl == nil || len(pl.Items) != len(names) {
		return false
	}
	for i := range names {
		if pl.Items[i].Clip != names[i] {
			return false
		}
	}
	return true
}

type scenario struct {
	name  string
	build func(t *testing.T) (*synDisc, []string) // the disc and the film's clips in order
	check func(t *testing.T, d *Disc, r *Ranking, film []string)
}

func wantMain(want int) func(t *testing.T, d *Disc, r *Ranking, film []string) {
	return func(t *testing.T, d *Disc, r *Ranking, film []string) {
		if r.Main != want {
			t.Errorf("main = %05d, want %05d", r.Main, want)
		}
	}
}

func scenarios() []scenario {
	return []scenario{
		{"a plain film with extras", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(1)
			film := s.film(4, 1500, evidence{carry: true, layout: true})
			s.feature(800, film)
			s.extras(1)
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(800)(t, d, r, film)
			if r.ConfidenceLabel == "low" {
				t.Errorf("confidence %s (%.2f): %v", r.ConfidenceLabel, r.Confidence, r.Why)
			}
			if len(r.Decoys) != 0 || len(r.Editions) != 0 || len(r.Episodes) != 0 {
				t.Errorf("decoys %v editions %v episodes %v", r.Decoys, r.Editions, r.Episodes)
			}
			if len(r.Chapters) != 10 || math.Abs(r.Chapters[9].Start-5400) > 0.01 {
				t.Errorf("chapters: %+v", r.Chapters)
			}
		}},
		{"1000 permuted decoys, the clock runs through the film", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(2)
			film := s.film(24, 280, evidence{carry: true})
			s.folderLike = true
			s.feature(613, film)
			s.decoys(film, 1000, 0, "permute")
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(613)(t, d, r, film)
			if len(r.Decoys) != 1000 {
				t.Errorf("%d decoys, want 1000", len(r.Decoys))
			}
			if r.ConfidenceLabel == "low" {
				t.Errorf("confidence %s: %v", r.ConfidenceLabel, r.Why)
			}
			if len(r.Clusters) == 0 || r.Clusters[0].Size != 1001 || r.Clusters[0].Kept != 613 || r.Clusters[0].Ambiguous {
				t.Errorf("clusters: %+v", r.Clusters)
			}
		}},
		{"1000 permuted decoys, only the layout on the disc tells", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(3)
			film := s.film(24, 280, evidence{layout: true})
			s.feature(77, film)
			s.decoys(film, 1000, 0, "permute")
			return s, film
		}, wantMain(77)},
		{"an image laid out in the order of its clips' names gives only the names' evidence", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(19)
			film := s.film(12, 500, evidence{numbered: true, layout: true})
			s.feature(40, film)
			s.decoys(film, 200, 100, "permute")
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(40)(t, d, r, film)
			if c := r.Candidate(40); c.Continuity > 0.5 || r.ConfidenceLabel == "high" {
				t.Errorf("a layout that only repeats the names was taken as evidence: continuity %v, confidence %s", c.Continuity, r.ConfidenceLabel)
			}
		}},
		{"1000 permuted decoys, only the clip numbers tell", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(4)
			film := s.film(24, 280, evidence{numbered: true})
			s.folderLike = true
			s.feature(941, film)
			s.decoys(film, 1000, 0, "permute")
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(941)(t, d, r, film)
			if r.ConfidenceLabel == "high" {
				t.Errorf("clip numbers alone should not give high confidence: %.2f %v", r.Confidence, r.Why)
			}
		}},
		{"500 permuted decoys, only the HDMV program tells (a register chosen in first play)", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(5)
			film := s.film(16, 400, evidence{})
			s.folderLike = true
			// The film is in no natural order by anything but the program.
			s.feature(431, s.permuted(film))
			s.decoys(film, 500, 0, "permute")
			// First play: a logo, then GPR7 = 400 + 31, then title 1. Title 1
			// compares GPR7 against every playlist number and plays the match.
			s.addClip("00003", 10*sec, 12)
			s.pl(900, &synPlaylist{items: s.items("00003")})
			var title []Command
			for n := 0; n <= 500; n++ {
				title = append(title, Command{0x48400200, 7, uint32(n)}, play(uint32(n))) // EQ GPR7, n; PLAY_PL n
			}
			s.objs = []MovieObject{
				{Commands: []Command{play(900), move(7, 400), {0x50400003, 7, 31}, jumpTitle(1)}},
				{Commands: title},
			}
			s.first, s.top, s.titles = hdmv(0), hdmv(0), []IndexObject{hdmv(1)}
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(431)(t, d, r, film)
			if r.FirstPlay != 431 || !slices.Contains(r.Program.Certain, 431) {
				t.Errorf("first play %d, certain %v", r.FirstPlay, r.Program.Certain)
			}
			if c := r.Candidate(900); c == nil || c.Class != ClassIntro {
				t.Errorf("the logo: %+v", c)
			}
		}},
		{"300 permuted decoys, only the Java program's locator tells", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(6)
			film := s.film(16, 400, evidence{})
			s.folderLike = true
			s.feature(207, s.permuted(film))
			s.decoys(film, 300, 0, "permute")
			j := IndexObject{Kind: ObjectBDJ, BDJName: "00000"}
			s.first, s.top, s.titles = j, j, []IndexObject{j}
			s.extra["BDMV/BDJO/00000.bdjo"] = synBDJO("00000")
			s.extra["BDMV/JAR/00000.jar"] = synJar(t, []string{"bd://0.PLAYLIST:00207", "hello", "12345"}, []int{3, 150, 70000})
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(207)(t, d, r, film)
			if !r.BDJ || r.Program.Kind != "bdj" || r.Program.BDJNamed != 2 || r.Program.BDJClasses != 1 {
				t.Errorf("program: %+v", r.Program)
			}
			if c := r.Candidate(207); !c.Flags.Has(FlagBDJTarget) {
				t.Errorf("flags: %s", c.Flags)
			}
		}},
		{"orders that follow on equally well are called ambiguous, not guessed", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(7)
			s.folderLike = true
			for _, n := range []string{"00100", "00200", "00300"} {
				s.addClip(n, 10*sec, 2400)
			}
			// One rising and one falling join each: nothing tells them apart.
			s.pl(10, &synPlaylist{items: s.items("00100", "00300", "00200")})
			s.pl(11, &synPlaylist{items: s.items("00200", "00100", "00300")})
			return s, nil
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			a, b := r.Candidate(10), r.Candidate(11)
			if !a.Flags.Has(FlagAmbiguous) || !b.Flags.Has(FlagAmbiguous) || r.ConfidenceLabel != "low" {
				t.Errorf("flags %s / %s, confidence %s (%.2f)", a.Flags, b.Flags, r.ConfidenceLabel, r.Confidence)
			}
			if len(r.Clusters) != 1 || !r.Clusters[0].Ambiguous {
				t.Errorf("clusters: %+v", r.Clusters)
			}
			if !strings.Contains(strings.Join(r.Warnings, " "), "different orders") {
				t.Errorf("no warning: %v", r.Warnings)
			}
		}},
		{"decoys that repeat clips to run longer, shuffled subsets, fake half-second clips, missing clips", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(8)
			film := s.film(18, 330, evidence{carry: true, layout: true})
			s.feature(5, film)
			s.decoys(film, 400, 0, "repeat", "subset", "dust", "missing", "permute")
			s.extras(2000)
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(5)(t, d, r, film)
			if len(r.Decoys) < 395 {
				t.Errorf("%d decoys of 400", len(r.Decoys))
			}
			for _, e := range r.Editions {
				if !e.Main {
					t.Errorf("a decoy was taken for an edition: %+v", e)
				}
			}
		}},
		{"seamless branching: theatrical and extended cuts sharing clips, each with permuted decoys", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(9)
			film := s.film(14, 420, evidence{layout: true})
			// The extended cut has three more scenes, laid out among the
			// rest as branching puts them; rebuild the layout in that order.
			extra := s.freeNames(3)
			long := append([]string(nil), film[:4]...)
			long = append(long, extra[0])
			long = append(long, film[4:9]...)
			long = append(long, extra[1], extra[2])
			long = append(long, film[9:]...)
			s2 := newSyn(9)
			for _, n := range long {
				dur := 240.0
				if c := s.clips[n]; c != nil {
					dur = ticks45(c.end - c.start)
				}
				s2.addClip(n, 10*sec, dur).epSec = 10
			}
			s2.feature(100, film)
			s2.feature(101, long)
			s2.decoys(film, 200, 200, "permute")
			s2.decoys(long, 200, 400, "permute")
			s2.objs = []MovieObject{{Commands: []Command{jumpTitle(1)}}, {Commands: []Command{play(100)}}, {Commands: []Command{play(101)}}}
			s2.titles = []IndexObject{hdmv(1), hdmv(2)}
			return s2, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(100)(t, d, r, film) // first play leads to the theatrical cut
			var got []int
			for _, e := range r.Editions {
				got = append(got, e.Playlist)
			}
			if !slices.Equal(got, []int{101, 100}) {
				t.Fatalf("editions %v, want [101 100]: %+v", got, r.Editions)
			}
			long, short := r.Editions[0], r.Editions[1]
			if long.Label != "longest cut" || short.Label != "shortest cut" || !short.Main || long.Main {
				t.Errorf("labels: %+v", r.Editions)
			}
			if math.Abs(long.Own-720) > 1 || short.Own != 0 || math.Abs(long.Shared-short.Duration) > 1 {
				t.Errorf("shared and own time: %+v", r.Editions)
			}
			if len(r.Decoys) != 400 {
				t.Errorf("%d decoys, want 400", len(r.Decoys))
			}
		}},
		{"a multi-angle film", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(10)
			film := s.film(5, 1200, evidence{layout: true})
			p := s.feature(1, film)
			alt := s.addClip("00500", 10*sec, ticks45(s.clips[film[2]].end-s.clips[film[2]].start))
			_ = alt
			p.items[2].angles = []string{"00500"}
			s.extras(2)
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(1)(t, d, r, film)
			if c := r.Candidate(1); c.Angles != 2 || !c.Flags.Has(FlagMultiAngle) {
				t.Errorf("angles: %+v", c)
			}
		}},
		{"a TV disc: episodes in playlists out of order, a play-all, and decoys of it", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(11)
			var eps []string
			for i, l := range []float64{1310, 1350, 1290, 1335, 1322, 1301} {
				n := fmt.Sprintf("%05d", 40+i)
				s.addClip(n, 10*sec, l).epSec = 10
				eps = append(eps, n)
			}
			for i, pl := range []int{14, 10, 15, 11, 13, 12} {
				it := s.items(eps[i])
				s.pl(pl, &synPlaylist{items: it, marks: synChapters(it, 300)})
			}
			s.feature(50, eps)
			s.decoys(eps, 60, 100, "permute", "repeat")
			s.extras(1)
			return s, eps
		}, func(t *testing.T, d *Disc, r *Ranking, eps []string) {
			var got []int
			for _, e := range r.Episodes {
				got = append(got, e.Playlist)
			}
			if !slices.Equal(got, []int{14, 10, 15, 11, 13, 12}) || r.PlayAll != 50 {
				t.Fatalf("episodes %v, play-all %d", got, r.PlayAll)
			}
			if len(r.PlayAllParts) != 6 {
				t.Fatalf("parts: %+v", r.PlayAllParts)
			}
			at := 0.0
			for i, p := range r.PlayAllParts {
				if p.FirstItem != i || p.LastItem != i || p.Episode != got[i] || math.Abs(p.Start-at) > 0.01 {
					t.Errorf("part %d: %+v", i, p)
				}
				at += p.Duration
			}
			if r.Episodes[0].Chapters < 4 {
				t.Errorf("episode chapters: %+v", r.Episodes[0])
			}
		}},
		{"a TV disc with only a play-all playlist", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(12)
			var eps []string
			for i := 0; i < 5; i++ {
				n := fmt.Sprintf("%05d", 20+i)
				s.addClip(n, 10*sec, 1290+float64(i)*20)
				eps = append(eps, n)
			}
			items := s.items(eps...)
			s.pl(3, &synPlaylist{items: items, marks: synChapters(items, 430)})
			s.extras(100)
			return s, eps
		}, func(t *testing.T, d *Disc, r *Ranking, eps []string) {
			wantMain(3)(t, d, r, eps)
			if len(r.Episodes) != 0 || len(r.PlayAllParts) != 5 || r.PlayAllParts[4].FirstItem != 4 || r.PlayAllParts[2].Episode != -1 {
				t.Errorf("episodes %v parts %+v", r.Episodes, r.PlayAllParts)
			}
		}},
		{"forced trailers before the film, and a trailer reel longer than half of it", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(13)
			film := s.film(3, 1700, evidence{carry: true, layout: true})
			s.feature(1, film)
			var reel []string
			for i := 0; i < 16; i++ {
				n := fmt.Sprintf("%05d", 300+i)
				s.addClip(n, 10*sec, 170)
				reel = append(reel, n)
				s.pl(300+i, &synPlaylist{items: s.items(n)})
			}
			s.pl(2, &synPlaylist{items: s.items(reel...)}) // 45 minutes of trailers in one playlist
			s.objs = []MovieObject{
				{Commands: []Command{play(300), play(301), play(302), jumpTitle(1)}},
				{Commands: []Command{play(1)}},
				{Commands: []Command{play(2)}},
			}
			s.titles = []IndexObject{hdmv(1), hdmv(2)}
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(1)(t, d, r, film)
			if r.FirstPlay != 1 || r.TitleOne != 1 || r.ConfidenceLabel != "high" {
				t.Errorf("first play %d, title one %d, confidence %s %v", r.FirstPlay, r.TitleOne, r.ConfidenceLabel, r.Why)
			}
			for _, n := range []int{300, 301, 302} {
				if c := r.Candidate(n); c.Class != ClassIntro {
					t.Errorf("trailer %d is %s", n, c.Class)
				}
			}
		}},
		{"missing, empty and cut-short stream files", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(14)
			film := s.film(4, 1500, evidence{layout: true})
			s.feature(1, film)
			gone := s.addClip("00700", 10*sec, 7000)
			gone.size = -1
			empty := s.addClip("00701", 10*sec, 7100)
			empty.size = -2
			cut := s.addClip("00702", 10*sec, 7200)
			cut.epSec = 10
			cut.size = int64(cut.packets) * 192 / 4
			s.pl(2, &synPlaylist{items: s.items("00700")})
			s.pl(3, &synPlaylist{items: s.items("00701")})
			s.pl(4, &synPlaylist{items: s.items("00702")})
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(1)(t, d, r, film)
			if !r.Candidate(2).Flags.Has(FlagMissingClip) || !r.Candidate(3).Flags.Has(FlagZeroSize) || !r.Candidate(4).Flags.Has(FlagTruncated) {
				t.Errorf("flags: %s / %s / %s", r.Candidate(2).Flags, r.Candidate(3).Flags, r.Candidate(4).Flags)
			}
			kinds := map[string]float64{}
			for _, x := range d.Consistency() {
				kinds[x.Kind] = x.PlayableSec
			}
			if _, ok := kinds["streamMissing"]; !ok {
				t.Errorf("no streamMissing in %v", kinds)
			}
			if _, ok := kinds["streamEmpty"]; !ok {
				t.Errorf("no streamEmpty in %v", kinds)
			}
			if p := kinds["streamShort"]; math.Abs(p-1800) > 20 {
				t.Errorf("a quarter of a 7200 s clip should play about 1800 s, got %.0f", p)
			}
			if r.Inconsistencies["streamShort"] != 1 {
				t.Errorf("inconsistencies: %v", r.Inconsistencies)
			}
		}},
		{"a 3D disc: the 2D playlist and its 3D twin", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(15)
			film := s.film(3, 2000, evidence{layout: true})
			s.feature(1, film).ss = true
			s.feature(2, film)
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(2)(t, d, r, film)
			if !d.Stereo3D || !r.Candidate(1).Flags.Has(FlagStereo3D) || r.Candidate(2).Flags.Has(FlagStereo3D) || r.Stereo3D {
				t.Errorf("3D flags: disc %v, 00001 %s, 00002 %s", d.Stereo3D, r.Candidate(1).Flags, r.Candidate(2).Flags)
			}
			if !strings.HasSuffix(r.Candidate(1).Video, "3D") {
				t.Errorf("video: %q", r.Candidate(1).Video)
			}
		}},
		{"an Ultra HD disc: HEVC, HDR10+ and a Dolby Vision layer", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(16)
			s.version = "0300"
			film := s.film(2, 3300, evidence{layout: true})
			for _, n := range film {
				c := s.clips[n]
				c.coding, c.format = 0x24, 8
				// 100 Mbit/s: over 4 GB a clip, as an Ultra HD film's are.
				c.packets = uint32(int64(ticks45(c.end-c.start)*12.5e6) / 192)
			}
			p := s.feature(1, film)
			p.hdr, p.hdrPlus = 1, true
			q := s.feature(2, film[:1])
			q.hdr, q.dv = 1, true
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(1)(t, d, r, film)
			c := r.Candidate(1)
			if !r.UHD || r.HDR != "HDR10+" || c.Video != "2160p HEVC HDR10+" || !c.Flags.Has(FlagUHD|FlagHDR) {
				t.Errorf("UHD %v HDR %q video %q flags %s", r.UHD, r.HDR, c.Video, c.Flags)
			}
			if v := r.Candidate(2).Video; v != "2160p HEVC Dolby Vision" {
				t.Errorf("Dolby Vision playlist: %q", v)
			}
			if c.Size < 8<<30 {
				t.Errorf("size %d: an 80 GB film's size must not wrap", c.Size)
			}
			if d.Index.Version != "0300" {
				t.Errorf("index version %q", d.Index.Version)
			}
		}},
		{"a movie-only copy: no index, no movie objects, decoys kept", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(17)
			film := s.film(10, 600, evidence{carry: true})
			s.folderLike = true
			s.feature(800, film)
			s.decoys(film, 50, 0, "permute", "repeat")
			s.noProgram = true
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(800)(t, d, r, film)
			if !d.MovieOnly || !r.MovieOnly || len(d.Damaged) != 0 || r.Program.Kind != "none" {
				t.Errorf("movie only %v, damaged %v, program %q", d.MovieOnly, d.Damaged, r.Program.Kind)
			}
		}},
		{"everything at once: two cuts, 1000 decoys of every kind, forced trailers, a register-chosen title", func(t *testing.T) (*synDisc, []string) {
			s := newSyn(18)
			film := s.film(20, 300, evidence{carry: true, layout: true})
			s.feature(1500, film)
			s.decoys(film, 1000, 0, "permute", "repeat", "subset", "dust", "missing")
			s.addClip("00003", 10*sec, 20)
			s.pl(1900, &synPlaylist{items: s.items("00003")})
			s.objs = []MovieObject{
				{Commands: []Command{play(1900), move(3, 1500), jumpTitle(1)}},
				{Commands: []Command{{0x22000000, 3, 0}}}, // PLAY_PL GPR3
			}
			s.titles = []IndexObject{hdmv(1)}
			return s, film
		}, func(t *testing.T, d *Disc, r *Ranking, film []string) {
			wantMain(1500)(t, d, r, film)
			if r.ConfidenceLabel != "high" || r.FirstPlay != 1500 {
				t.Errorf("confidence %s (%.2f), first play %d: %v", r.ConfidenceLabel, r.Confidence, r.FirstPlay, r.Why)
			}
		}},
	}
}

func TestSyntheticScenarios(t *testing.T) {
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			s, film := sc.build(t)
			d := s.open(t)
			r := d.Analyze(RankOptions{})
			sc.check(t, d, r, film)
			if t.Failed() {
				t.Log("\n" + r.Explain())
			}
			if r.Main >= 0 {
				if c := r.Candidate(r.Main); len(c.Evidence) == 0 || len(c.Reasons) == 0 || len(r.Why) == 0 || r.Confidence <= 0 {
					t.Errorf("no explanation: %+v, why %v", c.Evidence, r.Why)
				}
				if mp := d.MainPlaylist(); mp == nil || mp.Number != r.Main {
					t.Errorf("MainPlaylist disagrees with Analyze")
				}
			}
		})
	}
}
