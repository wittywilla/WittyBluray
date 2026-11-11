// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// detectEpisodes finds a TV disc's episodes: several clean playlists of
// about the same length (within 15% of their median) that don't share their
// video, and optionally one "play all" playlist that plays their segments
// one after another. The same rules as WittyDVD's titles.
func detectEpisodes(r *Ranking, feats map[int]*feat) {
	var pool []*feat
	for _, f := range feats {
		c := f.c
		if !f.eligible || !f.video || isDecoy(f) || c.Flags.Has(FlagDuplicate) || len(f.seq) == 0 {
			continue
		}
		if c.Distinct < minEpisodeSec || c.Distinct > maxEpisodeSec {
			continue
		}
		pool = append(pool, f)
	}
	sort.Slice(pool, func(i, j int) bool {
		if pool[i].c.Distinct != pool[j].c.Distinct {
			return pool[i].c.Distinct < pool[j].c.Distinct
		}
		return pool[i].c.PlaylistNum < pool[j].c.PlaylistNum
	})
	// The largest window of playlists within ±15% of its median. Every
	// window is tried on a disc of a few hundred candidates; past that the
	// upper end only moves forward as the lower end does, which is one pass
	// and finds the same window unless lengths are spread very unevenly.
	var best []*feat
	fits := func(i, j int) bool {
		med := pool[i+(j-i)/2].c.Distinct
		return pool[i].c.Distinct >= med*(1-episodeTolerant) && pool[j-1].c.Distinct <= med*(1+episodeTolerant)
	}
	if len(pool) <= 512 {
		for i := range pool {
			for j := len(pool); j > i && j-i > len(best); j-- {
				if fits(i, j) {
					best = append([]*feat(nil), pool[i:j]...)
					break
				}
			}
		}
	} else {
		for i, j := 0, 0; i < len(pool); i++ {
			j = max(j, i+1)
			for j < len(pool) && fits(i, j+1) {
				j++
			}
			if j-i > len(best) && fits(i, j) {
				best = append([]*feat(nil), pool[i:j]...)
			}
		}
	}
	// Playlists that overlap each other a lot are cuts of one thing (or the
	// same episode with another audio set), not episodes: keep the lowest
	// numbered. Each segment remembers the episode that has it, so this
	// costs the segments, not the pairs of playlists.
	sort.Slice(best, func(i, j int) bool { return best[i].c.PlaylistNum < best[j].c.PlaylistNum })
	var eps []*feat
	owner := map[segKey]*feat{}
	for _, f := range best {
		shared := map[*feat]int{}
		dup := false
		for _, k := range f.keys {
			if e := owner[k]; e != nil {
				shared[e]++
				if shared[e]*2 > min(len(f.set), len(e.set)) {
					dup = true
					break
				}
			}
		}
		if !dup {
			eps = append(eps, f)
			for _, k := range f.keys {
				if owner[k] == nil {
					owner[k] = f
				}
			}
		}
	}
	if len(eps) < 2 {
		return
	}
	// A play-all playlist holds (nearly) every segment of every episode, or
	// at least their clips, and lasts about as long as all of them together.
	var sum float64
	for _, e := range eps {
		sum += e.c.Distinct
	}
	in := map[*feat]bool{}
	for _, e := range eps {
		in[e] = true
	}
	var playAll *feat
	tried := 0
	for _, c := range r.Candidates {
		f := feats[c.PlaylistNum]
		if f == nil || in[f] || !f.eligible || isDecoy(f) || len(f.seq) == 0 {
			continue
		}
		if f.c.Distinct < sum*(1-episodeTolerant) || f.c.Distinct > sum*(1+episodeTolerant) {
			continue
		}
		if tried++; tried > 64 {
			break
		}
		covers := true
		for _, e := range eps {
			if overlap(e.set, f.set)*10 < len(e.set)*9 && !coversClips(f, e) {
				covers = false
				break
			}
		}
		if covers && (playAll == nil || f.c.PlaylistNum < playAll.c.PlaylistNum) {
			playAll = f
		}
	}
	if playAll == nil && len(eps) < 3 {
		return // two playlists of a length is no TV disc
	}
	// Viewing order: where each episode starts inside the play-all
	// playlist, else by first clip, else by playlist number.
	pos := map[*feat]int{}
	if playAll != nil {
		at := map[string]int{}
		for i, k := range playAll.seq {
			if _, ok := at[k.clip]; !ok {
				at[k.clip] = i
			}
		}
		for _, e := range eps {
			pos[e] = at[e.seq[0].clip]
		}
	}
	sort.SliceStable(eps, func(i, j int) bool {
		a, b := eps[i], eps[j]
		if playAll != nil && pos[a] != pos[b] {
			return pos[a] < pos[b]
		}
		if a.seq[0].clip != b.seq[0].clip {
			na, oka := clipNumber(a.seq[0].clip)
			nb, okb := clipNumber(b.seq[0].clip)
			if oka && okb {
				return na < nb
			}
			return a.seq[0].clip < b.seq[0].clip
		}
		return a.c.PlaylistNum < b.c.PlaylistNum
	})
	for i, e := range eps {
		e.c.note("episode", 0, FlagEpisode, "episode %d of %d: a run of playlists of about the same length", i+1, len(eps))
		r.Episodes = append(r.Episodes, Episode{Index: i + 1, Playlist: e.c.PlaylistNum, Duration: e.c.Distinct, Chapters: len(e.pl.Chapters())})
	}
	if playAll != nil {
		r.PlayAll = playAll.c.PlaylistNum
		playAll.c.note("playAll", 0, FlagPlayAll, "plays the %d episodes one after another", len(eps))
	}
}

// clipTimes is how long a playlist's distinct segments spend in each clip.
func clipTimes(f *feat) map[string]float64 {
	out := make(map[string]float64, len(f.keys))
	for _, k := range f.keys {
		out[k.clip] += k.dur()
	}
	return out
}

// coversClips says f plays every clip of e for at least as long as e does:
// a play-all playlist often names whole clips where an episode names a
// stretch of them.
func coversClips(f, e *feat) bool {
	have, need := clipTimes(f), clipTimes(e)
	for c, t := range need {
		if have[c] < t*0.9 {
			return false
		}
	}
	return len(need) > 0
}

// maxEditions is how many cuts of one film are reported. A film has two or
// three; more than this is something else.
const maxEditions = 8

// detectEditions finds the cuts of a film that keeps several in playlists
// sharing most of their clips (seamless branching: a theatrical and an
// extended cut differ in a few scenes kept in clips of their own). An
// edition is a clean playlist that never goes back on the disc, is at
// least half as long as the main one, has at least half its video in
// common with it by clip, and is not the same set of segments.
func detectEditions(r *Ranking, feats map[int]*feat) {
	main := feats[r.Main]
	if r.Main < 0 || main == nil || !main.eligible || !main.video || main.c.Distinct < minFeatureSec/2 {
		return
	}
	if main.c.Flags&(FlagEpisode|FlagPlayAll) != 0 {
		return
	}
	mainTimes := clipTimes(main)
	var eds []*feat
	more, looked := 0, 0
	for i := range r.Candidates {
		f := feats[r.Candidates[i].PlaylistNum]
		if f == nil || f == main || !f.eligible || !f.video || isDecoy(f) || f.back > 0 {
			continue
		}
		c := f.c
		if c.Flags&(FlagDuplicate|FlagEpisode|FlagPlayAll) != 0 || f.setHash == main.setHash {
			continue
		}
		if c.Distinct < main.c.Distinct*0.5 || c.Distinct > main.c.Distinct*2 {
			continue
		}
		if looked++; looked > 256 {
			break
		}
		shared := 0.0
		for clip, t := range clipTimes(f) {
			shared += math.Min(t, mainTimes[clip])
		}
		if shared < 0.5*math.Min(c.Distinct, main.c.Distinct) {
			continue
		}
		if len(eds) >= maxEditions-1 {
			more++
			continue
		}
		eds = append(eds, f)
	}
	if len(eds) == 0 {
		return
	}
	eds = append(eds, main)
	sort.SliceStable(eds, func(i, j int) bool {
		if eds[i].c.Distinct != eds[j].c.Distinct {
			return eds[i].c.Distinct > eds[j].c.Distinct
		}
		return eds[i].c.PlaylistNum < eds[j].c.PlaylistNum
	})
	users := map[segKey]int{}
	for _, f := range eds {
		for _, k := range f.keys {
			users[k]++
		}
	}
	for i, f := range eds {
		e := Edition{Playlist: f.c.PlaylistNum, Duration: f.c.Distinct, Chapters: len(f.pl.Chapters()), Main: f == main, ByProgram: f.nav > 0, Label: "cut"}
		switch i {
		case 0:
			e.Label = "longest cut"
		case len(eds) - 1:
			e.Label = "shortest cut"
		}
		for clip, t := range clipTimes(f) {
			e.Shared += math.Min(t, mainTimes[clip])
		}
		for _, k := range f.keys {
			if users[k] == 1 {
				e.Own += k.dur()
			}
		}
		f.c.note("edition", 0, FlagEdition, "one of %d cuts of the same film that share most of their clips (%s)", len(eds), e.Label)
		r.Editions = append(r.Editions, e)
	}
	if more > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%d more playlists look like further cuts of the main film and were not listed: a film has two or three cuts, so these are more likely decoys that leave clips out, which only the disc's program can tell from a real cut.", more))
	}
}

// splitPlayAll says where the parts of a play-all playlist begin. With
// per-episode playlists, each episode is found in it by its first clip.
// Without, a long main playlist made of three or more whole clips of even,
// episode-like length whose clocks each start afresh is offered as a guess.
func splitPlayAll(r *Ranking, feats map[int]*feat) {
	part := func(pl *Playlist, first, last, ep int) {
		p := Part{Index: len(r.PlayAllParts) + 1, FirstItem: first, LastItem: last, Episode: ep}
		for i := 0; i <= last && i < len(pl.Items); i++ {
			d := pl.Items[i].Duration()
			if i < first {
				p.Start += d
			} else {
				p.Duration += d
			}
		}
		r.PlayAllParts = append(r.PlayAllParts, p)
	}
	if all := feats[r.PlayAll]; r.PlayAll >= 0 && all != nil && len(r.Episodes) > 0 {
		pl := all.pl
		at := map[string]int{}
		for i := range pl.Items {
			if _, ok := at[pl.Items[i].Clip]; !ok {
				at[pl.Items[i].Clip] = i
			}
		}
		type cut struct{ item, ep int }
		var cuts []cut
		for _, e := range r.Episodes {
			f := feats[e.Playlist]
			if f == nil || len(f.seq) == 0 {
				continue
			}
			if i, ok := at[f.seq[0].clip]; ok {
				cuts = append(cuts, cut{i, e.Playlist})
			}
		}
		sort.SliceStable(cuts, func(i, j int) bool { return cuts[i].item < cuts[j].item })
		for i, c := range cuts {
			if i > 0 && c.item == cuts[i-1].item {
				continue
			}
			last := len(pl.Items) - 1
			for j := i + 1; j < len(cuts); j++ {
				if cuts[j].item > c.item {
					last = cuts[j].item - 1
					break
				}
			}
			part(pl, c.item, last, c.ep)
		}
		return
	}
	main := feats[r.Main]
	if r.Main < 0 || main == nil || len(r.Episodes) > 0 || len(r.Editions) > 0 || !main.eligible || !main.video {
		return
	}
	pl := main.pl
	n := len(pl.Items)
	if n < 3 || n > 64 || main.carry > 0 || main.back > 0 || len(main.set) != n || main.c.Clips != n {
		return
	}
	durs := make([]float64, n)
	for i := range pl.Items {
		durs[i] = pl.Items[i].Duration()
	}
	sorted := append([]float64(nil), durs...)
	sort.Float64s(sorted)
	med := sorted[n/2]
	if sorted[0] < minEpisodeSec || sorted[n-1] > maxEpisodeSec || sorted[0] < med*(1-episodeTolerant) || sorted[n-1] > med*(1+episodeTolerant) {
		return
	}
	for i := 0; i < n; i++ {
		part(pl, i, i, -1)
	}
	r.Warnings = append(r.Warnings, fmt.Sprintf("Playlist %05d is %d whole clips of about %s each, with nothing carrying on from one to the next: it may be %d episodes played together rather than one film. Its parts are listed.", pl.Number, n, fmtDur(med), n))
}

// classify gives every playlist a class: the main one, episodes, decoys,
// then extras by length.
func classify(r *Ranking, feats map[int]*feat) {
	main := feats[r.Main]
	var mainTimes map[string]float64
	if main != nil {
		mainTimes = clipTimes(main)
	}
	for i := range r.Candidates {
		f := feats[r.Candidates[i].PlaylistNum]
		if f == nil {
			continue
		}
		c := f.c
		switch {
		case r.Main >= 0 && c.PlaylistNum == r.Main:
			c.Class = ClassMain
		case c.Flags.Has(FlagPlayAll):
			c.Class = ClassPlayAll
		case c.Flags.Has(FlagEpisode):
			c.Class = ClassEpisode
		case isMenu(f):
			c.Class = ClassMenu
			c.Flags |= FlagMenu
			c.Reasons = append(c.Reasons, "it carries menu graphics over little video of its own: a menu's background")
			c.Evidence = append(c.Evidence, Evidence{Signal: "menu", Detail: "it carries menu graphics over little video of its own: a menu's background"})
		case isDecoy(f):
			c.Class = ClassDecoy
			c.Flags |= FlagDecoy
			r.Decoys = append(r.Decoys, c.PlaylistNum)
		case c.Flags.Has(FlagDuplicate):
			c.Class = ClassDuplicate
		case c.Flags.Has(FlagEdition):
			c.Class = ClassAlternate
			c.Flags |= FlagAlternate
		case main != nil && len(f.set) > 0 && c.Distinct >= minFeatureSec/2 && sharesMost(f, main, mainTimes):
			c.Class = ClassAlternate
			c.Flags |= FlagAlternate
			c.Reasons = append(c.Reasons, fmt.Sprintf("mostly the same video as playlist %05d: another cut of it", r.Main))
			c.Evidence = append(c.Evidence, Evidence{Signal: "alternate", Detail: fmt.Sprintf("mostly the same video as playlist %05d: another cut of it", r.Main)})
		case c.Flags.Has(FlagIntro) && c.Duration < trailerSec:
			c.Class = ClassIntro
		case c.Duration < stubSec:
			c.Class = ClassStub
		case c.Duration < trailerSec:
			c.Class = ClassTrailer
		case c.Duration < featuretteSec:
			c.Class = ClassFeaturette
		default:
			c.Class = ClassExtra
		}
	}
	sort.Ints(r.Decoys)
	if c := r.Candidate(r.Main); c != nil && r.Main >= 0 {
		c.Reasons = append(c.Reasons, "picked as the main playlist: the highest score among the readable ones")
	}
}

// sharesMost says at least six tenths of f's segments are main's.
func sharesMost(f, main *feat, mainTimes map[string]float64) bool {
	// Cheap test first: a playlist with no clip of main's shares nothing.
	any := false
	for _, k := range f.keys {
		if _, ok := mainTimes[k.clip]; ok {
			any = true
			break
		}
	}
	return any && overlap(f.set, main.set)*10 >= len(f.set)*6
}

// warn writes the disc-wide warnings.
func (r *Ranking) warn(feats map[int]*feat) {
	if r.Main < 0 {
		r.Warnings = append(r.Warnings, "No playlist reads as a whole film: each one names a clip that's missing or empty, or runs past a clip's end.")
		return
	}
	main := r.Candidate(r.Main)
	if n := len(r.Decoys); n > 0 {
		kind := "Decoy playlists like these are a structure-protection pattern or a damaged copy."
		if len(r.Candidates) >= 50 {
			kind = fmt.Sprintf("%d playlists with %d decoys is the layout of playlist obfuscation (as on some Lionsgate and Disney discs).", len(r.Candidates), n)
		}
		r.Warnings = append(r.Warnings, fmt.Sprintf("This disc has %s; picked playlist %05d because %s. %s Nothing is decrypted: the decoys are only skipped.",
			pluralWord(n, "decoy playlist", "decoy playlists"), main.PlaylistNum, mainBecause(main), kind))
	}
	if main.Flags.Has(FlagAmbiguous) {
		r.Warnings = append(r.Warnings, fmt.Sprintf("Playlist %05d is one of %d that play the same segments in different orders, and nothing on the disc that can be read without running it says which order is the film's. The lowest-numbered of the best was picked; the disc's own menu is the surer guide.", main.PlaylistNum, main.ClusterSize))
	}
	if rival := r.rival(feats); rival != nil && main.Score-rival.Score < 5 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("Close call: playlist %05d scores %.1f against playlist %05d's %.1f. The disc's own menu is the surer guide.", main.PlaylistNum, main.Score, rival.PlaylistNum, rival.Score))
	}
	if main.Flags.Has(FlagOutOfRange) {
		r.Warnings = append(r.Warnings, fmt.Sprintf("The main playlist (%05d) has play items outside their clip's time base: the copy may be incomplete or damaged.", main.PlaylistNum))
	}
	if main.Flags.Has(FlagTruncated) {
		r.Warnings = append(r.Warnings, fmt.Sprintf("The main playlist (%05d) plays clips whose stream files are shorter than their clip information says: the copy was cut short, and it will stop early.", main.PlaylistNum))
	}
	if len(r.Editions) > 1 {
		var list []string
		for _, e := range r.Editions {
			list = append(list, fmt.Sprintf("%05d (%s)", e.Playlist, fmtDur(e.Duration)))
		}
		r.Warnings = append(r.Warnings, fmt.Sprintf("This film has %d cuts that share most of their clips: %s.", len(r.Editions), strings.Join(list, ", ")))
	}
	if len(r.Episodes) > 0 {
		s := fmt.Sprintf("Looks like a TV disc: %s", pluralWord(len(r.Episodes), "episode", "episodes"))
		if r.PlayAll >= 0 {
			s += fmt.Sprintf(", with playlist %05d playing them all", r.PlayAll)
		}
		r.Warnings = append(r.Warnings, s+".")
	}
}

// rival is the best-scored playlist that is a different thing from the main
// one: not a copy, a shuffle or another cut of it, not a decoy, and not one
// of its episodes. nil when there is none.
func (r *Ranking) rival(feats map[int]*feat) *Candidate {
	main := r.Candidate(r.Main)
	if main == nil || r.Main < 0 {
		return nil
	}
	mf := feats[main.PlaylistNum]
	for i := range r.Candidates {
		c := &r.Candidates[i]
		f := feats[c.PlaylistNum]
		if c.PlaylistNum == main.PlaylistNum || f == nil || !f.eligible || f.video != (mf != nil && mf.video) {
			continue
		}
		if c.Flags&(FlagDecoy|FlagDuplicate|FlagShuffled|FlagEdition|FlagAlternate|FlagEpisode|FlagPlayAll|FlagIntro) != 0 || isDecoy(f) {
			continue
		}
		if c.ClusterSize > 1 && c.Cluster == main.Cluster {
			continue
		}
		if mf != nil && len(f.set) > 0 && overlap(mf.set, f.set)*10 >= len(f.set)*9 {
			continue // a stretch of the main playlist
		}
		return c
	}
	return nil
}

// confidence says how sure the pick is, and why, from the evidence that
// decided it and the evidence that is missing.
func (r *Ranking) confidence(feats map[int]*feat) {
	main := r.Candidate(r.Main)
	if main == nil || r.Main < 0 {
		r.Confidence, r.ConfidenceLabel = 0, "low"
		r.Why = []string{"No playlist can be read as a feature."}
		return
	}
	f := feats[main.PlaylistNum]
	conf := 0.5
	var why []string
	say := func(delta float64, format string, args ...any) {
		conf += delta
		why = append(why, fmt.Sprintf(format, args...))
	}
	switch {
	case main.Flags.Has(FlagFirstPlay) && hasInt(r.Program.Certain, main.PlaylistNum):
		say(0.25, "The disc's program plays it when the disc is put in, whatever the player's state.")
	case main.Flags.Has(FlagFirstPlay):
		say(0.2, "The disc's first-play program leads to it.")
	case main.Flags.Has(FlagTitleOne):
		say(0.2, "The program of title 1 plays it.")
	case main.Flags.Has(FlagBDJTarget):
		say(0.12, "The disc's Java program names it (read from its class files, not run).")
	case main.Flags.Has(FlagMenuTarget):
		say(0.1, "A title's program plays it.")
	case r.Program.Kind == "bdj":
		say(-0.1, "The disc is driven by a Java program that names no playlist that can be read without running it, so only the structure speaks.")
	case r.Program.Kind == "none":
		say(-0.02, "The copy has no disc program, so only the structure speaks.")
	default:
		say(-0.05, "The disc's program does not lead to it in a way that can be read without running it.")
	}
	rival := r.rival(feats)
	switch {
	case rival == nil:
		say(0.2, "It has %s of distinct video, and no other readable playlist is a different film or programme.", fmtDur(main.Distinct))
	case main.Score-rival.Score >= 15:
		say(0.2, "It has %s of distinct video; the next different playlist (%05d) scores %.0f points less.", fmtDur(main.Distinct), rival.PlaylistNum, main.Score-rival.Score)
	case main.Score-rival.Score >= 5:
		say(0.1, "It scores %.0f points more than the next different playlist (%05d).", main.Score-rival.Score, rival.PlaylistNum)
	default:
		say(-0.15, "Playlist %05d scores within %.1f points of it: a close call.", rival.PlaylistNum, main.Score-rival.Score)
	}
	if f != nil {
		switch {
		case main.Flags.Has(FlagAmbiguous):
			say(-0.35, "%d playlists play the same segments in other orders that follow on just as well; which one is the film cannot be told from structure.", main.ClusterSize-1)
		case main.ClusterSize > 1 && f.joins > 0 && f.natural() >= 0.85:
			say(0.12, "Of %d playlists with the same segments it is the one whose clips follow on from each other (%d of %d joins continue).", main.ClusterSize, f.cont, f.joins)
		case main.ClusterSize > 1 && f.nav >= 2:
			say(0.05, "Of %d playlists with the same segments it is the one the disc's program points at.", main.ClusterSize)
		case main.ClusterSize > 1 && f.joins > 0:
			say(0.0, "Of %d playlists with the same segments it is in the most natural order, on weak evidence (clip numbers or positions only).", main.ClusterSize)
			conf -= 0.1
		case f.joins > 0 && f.back == 0:
			say(0.05, "Its %d play items never go back on the disc.", len(f.pl.Items))
		case f.joins > 0:
			say(-0.1, "%d of its %d joins go back on the disc.", f.back, f.joins)
		}
		if main.Flags&(FlagOutOfRange|FlagTruncated|FlagMissingClip|FlagZeroSize) != 0 || !f.eligible {
			say(-0.2, "Parts of it cannot play as written: clips are missing, cut short, or shorter than the playlist says.")
		}
	}
	if len(r.Editions) > 1 {
		say(-0.05, "The film has %d cuts; this is the one picked, and the others are listed as editions.", len(r.Editions))
	}
	if len(r.Episodes) > 1 {
		say(0, "The disc looks like a TV disc with %d episodes; the main playlist is only the first choice among them.", len(r.Episodes))
	}
	if r.Program.Incomplete || r.Program.Unknown > 0 {
		say(-0.05, "Part of the disc's program could not be read statically (registers of unknown value, or more program than the reading follows).")
	}
	if so := r.SecondOpinion; so != nil {
		switch {
		case so.AgreesOnMain:
			say(0.1, "%s picks the same feature.", so.Source)
		case so.Main >= 0:
			theirs := r.Candidate(so.Main)
			if theirs != nil && theirs.ClusterSize > 1 && theirs.Cluster == main.Cluster {
				say(-0.05, "%s picks playlist %05d, which plays the same segments in another order.", so.Source, so.Main)
			} else {
				say(-0.2, "%s picks playlist %05d instead.", so.Source, so.Main)
			}
		}
	}
	conf = math.Max(0.05, math.Min(0.99, conf))
	r.Confidence = math.Round(conf*100) / 100
	switch {
	case r.Confidence >= 0.75:
		r.ConfidenceLabel = "high"
	case r.Confidence >= 0.5:
		r.ConfidenceLabel = "medium"
	default:
		r.ConfidenceLabel = "low"
	}
	r.Why = why
}

// compareSecondOpinion lists where another reader of the disc differs.
func compareSecondOpinion(r *Ranking, d *Disc, feats map[int]*feat, so *SecondOpinion) {
	if so == nil {
		return
	}
	rep := &SecondOpinionReport{Source: so.Source, Main: so.Main}
	add := func(x Disagreement) {
		if len(rep.Disagreements) >= 50 {
			rep.More++
			return
		}
		rep.Disagreements = append(rep.Disagreements, x)
	}
	if mf, tf := feats[r.Main], feats[so.Main]; so.Main >= 0 && r.Main >= 0 {
		rep.AgreesOnMain = so.Main == r.Main || (mf != nil && tf != nil && len(mf.seq) > 0 && sameSeq(mf.seq, tf.seq))
		if !rep.AgreesOnMain {
			note := ""
			if c := r.Candidate(so.Main); c != nil {
				note = fmt.Sprintf("this engine classes playlist %05d as %s", so.Main, c.Class)
				if fl := c.Flags.String(); fl != "" {
					note += " (" + fl + ")"
				}
			}
			add(Disagreement{Playlist: -1, Field: "main", Ours: fmt.Sprintf("%05d", r.Main), Theirs: fmt.Sprintf("%05d", so.Main), Note: note})
		}
	}
	seen := map[int]bool{}
	for i, t := range so.Titles {
		if i >= 20000 {
			break
		}
		seen[t.Playlist] = true
		pl := d.Playlists[t.Playlist]
		if pl == nil {
			add(Disagreement{Playlist: t.Playlist, Field: "present", Ours: "no", Theirs: "yes", Note: "this engine could not read the playlist"})
			continue
		}
		if dur := d.PlaylistDuration(pl); math.Abs(dur-t.Duration) > 1 {
			add(Disagreement{Playlist: t.Playlist, Field: "duration", Ours: fmt.Sprintf("%.1fs", dur), Theirs: fmt.Sprintf("%.1fs", t.Duration)})
		}
		chs := pl.Chapters()
		if len(chs) != t.Chapters {
			add(Disagreement{Playlist: t.Playlist, Field: "chapters", Ours: fmt.Sprint(len(chs)), Theirs: fmt.Sprint(t.Chapters)})
		} else if len(t.ChapterStarts) == len(chs) {
			for k := range chs {
				if math.Abs(chs[k].Start-t.ChapterStarts[k]) > 1 {
					add(Disagreement{Playlist: t.Playlist, Field: "chapterTimes", Ours: fmt.Sprintf("chapter %d at %.1fs", k+1, chs[k].Start), Theirs: fmt.Sprintf("%.1fs", t.ChapterStarts[k])})
					break
				}
			}
		}
		angles := 1
		for k := range pl.Items {
			angles = max(angles, len(pl.Items[k].Angles))
		}
		if t.Angles > 0 && angles != t.Angles {
			add(Disagreement{Playlist: t.Playlist, Field: "angles", Ours: fmt.Sprint(angles), Theirs: fmt.Sprint(t.Angles)})
		}
	}
	if r.Main >= 0 && len(so.Titles) > 0 && !seen[r.Main] {
		add(Disagreement{Playlist: r.Main, Field: "present", Ours: "yes", Theirs: "no", Note: "the other reader left out the playlist this engine picked (it drops playlists it finds unusable or repeated)"})
	}
	r.SecondOpinion = rep
	if n := len(rep.Disagreements) + rep.More; n > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("%s reads this disc differently in %s; see the second opinion.", so.Source, pluralWord(n, "place", "places")))
	}
}

// mainBecause picks the strongest positive reasons for the main playlist.
func mainBecause(c *Candidate) string {
	var parts []string
	if c.Flags.Has(FlagFirstPlay) {
		parts = append(parts, "the disc's first play leads to it")
	} else if c.Flags.Has(FlagTitleOne) {
		parts = append(parts, "title 1 leads to it")
	} else if c.Flags.Has(FlagBDJTarget) {
		parts = append(parts, "the disc's Java program names it")
	}
	parts = append(parts, fmt.Sprintf("it has the most distinct video (%s)", fmtDur(c.Distinct)))
	if c.Flags&(FlagShuffled|FlagRepeats|FlagOutOfRange) == 0 {
		parts = append(parts, "it plays its clips in a natural order with nothing repeated or out of range")
	}
	return strings.Join(parts, ", ")
}

// Explain words a ranking for people: the pick, how sure it is and why,
// the top candidates with their reasons, and the warnings.
func (r *Ranking) Explain() string {
	if r == nil || len(r.Candidates) == 0 {
		return "No playlist to rank."
	}
	var b strings.Builder
	if r.Main >= 0 {
		fmt.Fprintf(&b, "Main playlist: %05d\n", r.Main)
		fmt.Fprintf(&b, "Confidence: %s (%.2f)\n", r.ConfidenceLabel, r.Confidence)
	} else {
		b.WriteString("Main playlist: none\n")
	}
	for _, w := range r.Why {
		fmt.Fprintf(&b, "Why: %s\n", w)
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "Warning: %s\n", w)
	}
	if len(r.Editions) > 0 {
		var list []string
		for _, e := range r.Editions {
			list = append(list, fmt.Sprintf("%05d %s (%s)", e.Playlist, fmtDur(e.Duration), e.Label))
		}
		fmt.Fprintf(&b, "Editions: %s\n", strings.Join(list, ", "))
	}
	if len(r.Episodes) > 0 {
		var list []string
		for _, e := range r.Episodes {
			list = append(list, fmt.Sprintf("%05d", e.Playlist))
		}
		fmt.Fprintf(&b, "Episodes in order: %s\n", strings.Join(list, ", "))
	}
	if len(r.PlayAllParts) > 0 {
		var list []string
		for _, p := range r.PlayAllParts {
			list = append(list, fmtDur(p.Start))
		}
		fmt.Fprintf(&b, "Play-all parts start at: %s\n", strings.Join(list, ", "))
	}
	if len(r.Chapters) > 0 {
		fmt.Fprintf(&b, "Chapters: %d\n", len(r.Chapters))
	}
	for i, c := range r.Candidates {
		if i >= 10 {
			fmt.Fprintf(&b, "... and %d more\n", len(r.Candidates)-i)
			break
		}
		fmt.Fprintf(&b, "%2d. playlist %05d %s score %.1f [%s]", i+1, c.PlaylistNum, fmtDur(c.Duration), round1(c.Score), c.Class)
		if fl := c.Flags.String(); fl != "" {
			fmt.Fprintf(&b, " {%s}", fl)
		}
		b.WriteByte('\n')
		for _, why := range c.Reasons {
			fmt.Fprintf(&b, "      %s\n", why)
		}
	}
	return b.String()
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
