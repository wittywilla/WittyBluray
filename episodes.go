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
	// The largest window of playlists within ±15% of its median.
	var best []*feat
	for i := range pool {
		for j := len(pool); j > i; j-- {
			g := pool[i:j]
			if len(g) <= len(best) {
				break
			}
			med := g[len(g)/2].c.Distinct
			if g[0].c.Distinct >= med*(1-episodeTolerant) && g[len(g)-1].c.Distinct <= med*(1+episodeTolerant) {
				best = append([]*feat(nil), g...)
				break
			}
		}
	}
	// Playlists that overlap each other a lot are cuts of one thing (or the
	// same episode with another audio set), not episodes: keep the lowest
	// numbered.
	sort.Slice(best, func(i, j int) bool { return best[i].c.PlaylistNum < best[j].c.PlaylistNum })
	var eps []*feat
	for _, f := range best {
		dup := false
		for _, e := range eps {
			if overlap(f.set, e.set)*2 > min(len(f.set), len(e.set)) {
				dup = true
				break
			}
		}
		if !dup {
			eps = append(eps, f)
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
	for _, f := range feats {
		if in[f] || !f.eligible || isDecoy(f) || len(f.seq) == 0 {
			continue
		}
		if f.c.Distinct < sum*(1-episodeTolerant) || f.c.Distinct > sum*(1+episodeTolerant) {
			continue
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
		e.c.add(0, FlagEpisode, "episode %d of %d: a run of playlists of about the same length", i+1, len(eps))
		r.Episodes = append(r.Episodes, Episode{Index: i + 1, Playlist: e.c.PlaylistNum, Duration: e.c.Distinct})
	}
	if playAll != nil {
		r.PlayAll = playAll.c.PlaylistNum
		playAll.c.add(0, FlagPlayAll, "plays the %d episodes one after another", len(eps))
	}
}

// coversClips says f plays every clip of e for at least as long as e does:
// a play-all playlist often names whole clips where an episode names a
// stretch of them.
func coversClips(f, e *feat) bool {
	have := map[string]float64{}
	for k := range f.set {
		have[k.clip] += ticks45(k.out - min(k.in, k.out))
	}
	need := map[string]float64{}
	for k := range e.set {
		need[k.clip] += ticks45(k.out - min(k.in, k.out))
	}
	for c, t := range need {
		if have[c] < t*0.9 {
			return false
		}
	}
	return len(need) > 0
}

// classify gives every playlist a class: the main one, episodes, decoys,
// then extras by length.
func classify(r *Ranking, feats map[int]*feat) {
	main := feats[r.Main]
	for _, f := range feats {
		c := f.c
		switch {
		case r.Main >= 0 && c.PlaylistNum == r.Main:
			c.Class = ClassMain
		case c.Flags.Has(FlagPlayAll):
			c.Class = ClassPlayAll
		case c.Flags.Has(FlagEpisode):
			c.Class = ClassEpisode
		case isDecoy(f):
			c.Class = ClassDecoy
			c.Flags |= FlagDecoy
			r.Decoys = append(r.Decoys, c.PlaylistNum)
		case c.Flags.Has(FlagDuplicate):
			c.Class = ClassDuplicate
		case main != nil && len(f.set) > 0 && overlap(f.set, main.set)*10 >= len(f.set)*6 && c.Distinct >= minFeatureSec/2:
			c.Class = ClassAlternate
			c.Flags |= FlagAlternate
			c.Reasons = append(c.Reasons, fmt.Sprintf("mostly the same video as playlist %05d: another cut of it", r.Main))
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
	if len(r.Candidates) > 1 {
		next := r.Candidates[1]
		mf, nf := feats[main.PlaylistNum], feats[next.PlaylistNum]
		same := mf != nil && nf != nil && len(nf.set) > 0 && overlap(mf.set, nf.set)*10 >= len(nf.set)*9
		if !same && nf != nil && nf.eligible && main.Score-next.Score < 5 && next.Class != ClassDecoy && next.Class != ClassPlayAll && next.Class != ClassEpisode {
			r.Warnings = append(r.Warnings, fmt.Sprintf("Close call: playlist %05d scores %.1f against playlist %05d's %.1f. The disc's own menu is the surer guide.", main.PlaylistNum, main.Score, next.PlaylistNum, next.Score))
		}
	}
	if main.Flags.Has(FlagOutOfRange) {
		r.Warnings = append(r.Warnings, fmt.Sprintf("The main playlist (%05d) has play items outside their clip's time base: the copy may be incomplete or damaged.", main.PlaylistNum))
	}
	if len(r.Episodes) > 0 {
		s := fmt.Sprintf("Looks like a TV disc: %s", pluralWord(len(r.Episodes), "episode", "episodes"))
		if r.PlayAll >= 0 {
			s += fmt.Sprintf(", with playlist %05d playing them all", r.PlayAll)
		}
		r.Warnings = append(r.Warnings, s+".")
	}
}

// mainBecause picks the strongest positive reasons for the main playlist.
func mainBecause(c *Candidate) string {
	var parts []string
	if c.Flags.Has(FlagFirstPlay) {
		parts = append(parts, "the disc's first play leads to it")
	} else if c.Flags.Has(FlagTitleOne) {
		parts = append(parts, "title 1 leads to it")
	}
	parts = append(parts, fmt.Sprintf("it has the most distinct video (%s)", fmtDur(c.Distinct)))
	if c.Flags&(FlagShuffled|FlagRepeats|FlagOutOfRange) == 0 {
		parts = append(parts, "it plays its clips in a natural order with nothing repeated or out of range")
	}
	return strings.Join(parts, ", ")
}

// Explain words a ranking for people: the pick, the top candidates with
// their reasons, and the warnings.
func (r *Ranking) Explain() string {
	if r == nil || len(r.Candidates) == 0 {
		return "No playlist to rank."
	}
	var b strings.Builder
	if r.Main >= 0 {
		fmt.Fprintf(&b, "Main playlist: %05d\n", r.Main)
	} else {
		b.WriteString("Main playlist: none\n")
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "Warning: %s\n", w)
	}
	if len(r.Episodes) > 0 {
		var list []string
		for _, e := range r.Episodes {
			list = append(list, fmt.Sprintf("%05d", e.Playlist))
		}
		fmt.Fprintf(&b, "Episodes in order: %s\n", strings.Join(list, ", "))
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
