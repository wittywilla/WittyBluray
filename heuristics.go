package wittybluray

// Playlist ranking heuristics: which playlist is the feature, which are
// episodes, decoys or extras. Everything here works from the unencrypted
// navigation and metadata of the disc: the playlists (MPLS), the clip
// information (CLPI), index.bdmv, a static reading of MovieObject.bdmv, the
// presence of BD-J objects and the sizes of the .m2ts files. Nothing is
// decrypted and nothing here reads, derives or guesses AACS or BD+ keys: an
// encrypted disc is still refused elsewhere, whatever these scores say.
//
// The shape of the API mirrors WittyDVD's title ranking (RankTitles,
// Analyze, Explain, Episodes, DefaultWeights) so a server can treat both
// kinds of disc alike.

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Flag marks what the engine found out about a playlist.
type Flag uint32

const (
	FlagMain        Flag = 1 << iota // the engine's pick
	FlagDecoy                        // structure-protection decoy or junk
	FlagMenuTarget                   // a title's HDMV program plays it, or a BD-J object lists it
	FlagFirstPlay                    // first play reaches this long playlist (static reading)
	FlagTitleOne                     // title 1 reaches this long playlist (static reading)
	FlagIntro                        // a short playlist first play passes on its way
	FlagEpisode                      // one episode of a TV disc
	FlagPlayAll                      // plays the episodes one after another
	FlagDuplicate                    // the same segments in the same order as a lower-numbered playlist
	FlagShuffled                     // another playlist's segments in a less natural order
	FlagOutOfRange                   // in or out times outside the clip's time base
	FlagRepeats                      // replays the same segments
	FlagSegmentSpam                  // many play items under a second or a few seconds long
	FlagMissingClip                  // names a clip whose .clpi or .m2ts is missing
	FlagZeroSize                     // names a clip whose .m2ts is empty
	FlagBadChapters                  // chapter marks name play items that don't exist
	FlagAlternate                    // mostly the main playlist's video (another cut)
	FlagStereo3D                     // has a 3D (MVC) dependent view
	FlagMultiAngle                   // has a multi-angle play item
	FlagAudioOnly                    // no video stream: a music disc's playlist
)

var flagNames = []string{"main", "decoy", "menuTarget", "firstPlay", "titleOne", "intro", "episode", "playAll", "duplicate", "shuffled", "outOfRange", "repeats", "segmentSpam", "missingClip", "zeroSize", "badChapters", "alternate", "stereo3D", "multiAngle", "audioOnly"}

// Has reports whether every flag in g is set.
func (f Flag) Has(g Flag) bool { return f&g == g }

// Names lists the flags set, in a fixed order.
func (f Flag) Names() []string {
	out := []string{}
	for i, n := range flagNames {
		if f&(1<<i) != 0 {
			out = append(out, n)
		}
	}
	return out
}

func (f Flag) String() string { return strings.Join(f.Names(), ",") }

// MarshalJSON writes the flags as a list of names.
func (f Flag) MarshalJSON() ([]byte, error) { return json.Marshal(f.Names()) }

// UnmarshalJSON reads a list of flag names; unknown names are ignored.
func (f *Flag) UnmarshalJSON(b []byte) error {
	var names []string
	if err := json.Unmarshal(b, &names); err != nil {
		return err
	}
	*f = 0
	for _, n := range names {
		for i, k := range flagNames {
			if k == n {
				*f |= 1 << i
			}
		}
	}
	return nil
}

// Playlist classes (the same words WittyDVD uses for titles).
const (
	ClassMain       = "main"
	ClassPlayAll    = "playAll"
	ClassEpisode    = "episode"
	ClassDecoy      = "decoy"
	ClassDuplicate  = "duplicate"
	ClassAlternate  = "alternate"
	ClassIntro      = "intro"
	ClassStub       = "stub"
	ClassTrailer    = "trailer"
	ClassFeaturette = "featurette"
	ClassExtra      = "extra"
)

// Candidate is one playlist as the engine scored it.
type Candidate struct {
	PlaylistNum int      `json:"playlist"`
	Duration    float64  `json:"durationSec"`
	Distinct    float64  `json:"distinctSec"` // each readable segment counted once
	Chapters    int      `json:"chapters"`
	Items       int      `json:"items"`
	Clips       int      `json:"clips"`     // distinct clips named
	Size        int64    `json:"sizeBytes"` // the distinct clips' .m2ts bytes
	Audio       int      `json:"audioStreams"`
	Subtitles   int      `json:"subtitleStreams"` // PG streams
	Languages   []string `json:"audioLanguages,omitempty"`
	Angles      int      `json:"angles"`
	Score       float64  `json:"score"`
	Class       string   `json:"class"`
	Flags       Flag     `json:"flags"`
	Reasons     []string `json:"reasons"`

	pl *Playlist
}

// Playlist is the playlist the candidate was scored from.
func (c Candidate) Playlist() *Playlist { return c.pl }

// Episode is one episode of a TV disc, in viewing order.
type Episode struct {
	Index    int     `json:"index"` // 1-based viewing order
	Playlist int     `json:"playlist"`
	Duration float64 `json:"durationSec"`
}

// Ranking is the engine's full answer for a disc. Playlist numbers start at
// 0 (00000.mpls is a real playlist), so "none" is -1.
type Ranking struct {
	Candidates []Candidate `json:"candidates"` // the pick first, then best first
	Main       int         `json:"main"`       // -1 when no playlist can be the feature
	PlayAll    int         `json:"playAll"`    // -1 when none
	Episodes   []Episode   `json:"episodes,omitempty"`
	Decoys     []int       `json:"decoys,omitempty"`
	FirstPlay  int         `json:"firstPlay"` // long playlist first play reaches; -1 none
	TitleOne   int         `json:"titleOne"`  // long playlist title 1 reaches; -1 none
	// BDJ: the disc's first play or title 1 is a Java program, which this
	// engine doesn't run; the ranking falls back to structure alone.
	BDJ      bool     `json:"bdj,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Candidate returns the scored playlist by number, or nil.
func (r *Ranking) Candidate(playlist int) *Candidate {
	for i := range r.Candidates {
		if r.Candidates[i].PlaylistNum == playlist {
			return &r.Candidates[i]
		}
	}
	return nil
}

// Weights tune the scoring. Scores are in "minutes of distinct video": a
// weight of 15 is worth as much as a quarter of an hour of unique segments.
type Weights struct {
	DistinctPerMinute float64 // per minute of distinct, readable video
	MissingClip       float64 // names a clip whose .clpi or .m2ts is missing
	ZeroSize          float64 // names a clip whose .m2ts is empty
	OutOfRangeAny     float64 // any item's in/out time is outside the clip's time base
	OutOfRangeShare   float64 // times the share of such items
	Repeats           float64 // times the share of play time spent on repeats
	BackJumpAny       float64 // any join goes back (a lower clip, or earlier in the same clip)
	BackJumpShare     float64 // times the share of joins that go back
	Continuity        float64 // times the share of joins that continue seamlessly (PTS or SPN)
	Shuffled          float64 // another playlist's segments in a less natural order
	Duplicate         float64 // identical to a lower-numbered playlist (tie breaker)
	SegmentSpam       float64 // most items are under a second, or a few seconds, long
	BadChapters       float64 // chapter marks point nowhere
	RegularChapters   float64 // three or more chapters of sensible, even length
	AudioMatch        float64 // among the long playlists, has the most audio streams
	SubtitleMatch     float64 // among the long playlists, has the most PG streams
	LargestSize       float64 // its clips hold the most bytes
	Stereo3D          float64 // a 3D playlist (the 2D one is preferred: it plays here as 2D)
	FirstPlay         float64 // first play reaches it by itself
	TitleOne          float64 // title 1 reaches it
	MenuReference     float64 // a title's HDMV program plays it
	BDJListed         float64 // the BD-J object of first play or title 1 lists it among a few
}

// DefaultWeights are the weights MainPlaylist uses.
var DefaultWeights = Weights{
	DistinctPerMinute: 1,
	MissingClip:       -40,
	ZeroSize:          -40,
	OutOfRangeAny:     -10,
	OutOfRangeShare:   -20,
	Repeats:           -30,
	BackJumpAny:       -5,
	BackJumpShare:     -35,
	Continuity:        5,
	Shuffled:          -25,
	Duplicate:         -0.5,
	SegmentSpam:       -30,
	BadChapters:       -10,
	RegularChapters:   3,
	AudioMatch:        2,
	SubtitleMatch:     1,
	LargestSize:       5,
	Stereo3D:          -1,
	FirstPlay:         15,
	TitleOne:          10,
	MenuReference:     4,
	BDJListed:         4,
}

// RankOptions add what the caller knows beyond the disc's structures.
type RankOptions struct {
	// ClipSizes is the size in bytes of each clip's .m2ts file (-1 when the
	// file is missing). nil reads them from the disc's Source when it has
	// one; a disc without a Source is ranked without them.
	ClipSizes map[string]int64
	Weights   *Weights
}

// Durations that steer the classification.
const (
	minFeatureSec   = 20 * 60 // first play reaching a playlist this long has found the film
	stubSec         = 15
	trailerSec      = 4 * 60
	featuretteSec   = 45 * 60
	minEpisodeSec   = 8 * 60
	maxEpisodeSec   = 75 * 60
	episodeTolerant = 0.15
	maxBDJListed    = 10
)

// segKey is one stretch of a clip.
type segKey struct {
	clip    string
	stc     int
	in, out uint32
}

// feat is what one playlist's structure says.
type feat struct {
	c        *Candidate
	pl       *Playlist
	seq      []segKey // readable segments in play order
	set      map[segKey]bool
	sig      string // the sorted set of readable segments
	total    float64
	valid    float64 // play time of readable segments
	missing  int
	zero     int
	pastEnd  int
	before   int
	invalid  int // items that can't be read (no clip information, or past the clip's end)
	joins    int
	cont     int
	back     int
	tiny     int
	short    int
	video    bool
	audio    bool
	eligible bool
	marks    []float64
	badMarks bool
}

func fmtDur(sec float64) string {
	if sec < 0 || math.IsNaN(sec) || math.IsInf(sec, 0) {
		sec = 0
	}
	s := int(sec + 0.5)
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
}

func (c *Candidate) add(points float64, flag Flag, format string, args ...any) {
	c.Score += points
	c.Flags |= flag
	msg := fmt.Sprintf(format, args...)
	if points != 0 {
		msg = fmt.Sprintf("%+.1f %s", points, msg)
	}
	c.Reasons = append(c.Reasons, msg)
}

func pluralWord(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// RankPlaylists scores every playlist, the pick first, then best first.
func (d *Disc) RankPlaylists() []Candidate { return d.Analyze(RankOptions{}).Candidates }

// Explain words the ranking for people: the pick, why, and warnings.
func (d *Disc) Explain() string { return d.Analyze(RankOptions{}).Explain() }

// Episodes lists the episodes of a TV disc in viewing order; nil when the
// disc doesn't look like one.
func (d *Disc) Episodes() []Episode { return d.Analyze(RankOptions{}).Episodes }

// clipFileSizes reads the size of every clip's stream file once.
func (d *Disc) clipFileSizes() map[string]int64 {
	if d.Source == nil {
		return nil
	}
	d.sizesOnce.Do(func() {
		m := make(map[string]int64, len(d.Clips))
		for n := range d.Clips {
			ext, err := d.ClipExtents(n)
			if err != nil {
				m[n] = -1
				continue
			}
			var t int64
			for _, e := range ext {
				t += e.Length
			}
			m[n] = t
		}
		d.sizes = m
	})
	return d.sizes
}

// Analyze scores and classifies every playlist.
func (d *Disc) Analyze(opt RankOptions) *Ranking {
	r := &Ranking{Main: -1, PlayAll: -1, FirstPlay: -1, TitleOne: -1}
	if d == nil || len(d.Playlists) == 0 {
		r.Warnings = append(r.Warnings, "The disc has no playlist.")
		return r
	}
	w := DefaultWeights
	if opt.Weights != nil {
		w = *opt.Weights
	}
	sizes := opt.ClipSizes
	if sizes == nil {
		sizes = d.clipFileSizes()
	}
	var pls []*Playlist
	for _, pl := range d.sortedPlaylists() {
		if pl != nil {
			pls = append(pls, pl)
		}
	}
	r.Candidates = make([]Candidate, len(pls))
	feats := make([]*feat, len(pls))
	for i, pl := range pls {
		feats[i] = d.measure(&r.Candidates[i], pl, sizes)
	}
	for _, f := range feats {
		scorePlaylist(f, w)
	}
	crossPlaylist(feats, w)
	d.discSignals(r, feats, w)

	tier := func(c *Candidate, f *feat) int {
		t := 0
		switch {
		case !f.eligible:
			return 9
		case f.video:
		case f.audio:
			t = 2
		default:
			return 9
		}
		if isDecoy(f) {
			t++
		}
		return t
	}
	byNum := map[int]*feat{}
	tiers := map[int]int{}
	for _, f := range feats {
		byNum[f.c.PlaylistNum] = f
		tiers[f.c.PlaylistNum] = tier(f.c, f)
	}
	sort.SliceStable(r.Candidates, func(i, j int) bool {
		a, b := r.Candidates[i], r.Candidates[j]
		if ta, tb := tiers[a.PlaylistNum], tiers[b.PlaylistNum]; ta != tb {
			return ta < tb
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Distinct != b.Distinct {
			return a.Distinct > b.Distinct
		}
		if a.Duration != b.Duration {
			return a.Duration > b.Duration
		}
		return a.PlaylistNum < b.PlaylistNum
	})
	// feats point into the candidates before sorting: re-point them.
	for i := range r.Candidates {
		byNum[r.Candidates[i].PlaylistNum].c = &r.Candidates[i]
	}
	if tiers[r.Candidates[0].PlaylistNum] < 9 {
		r.Candidates[0].Flags |= FlagMain
		r.Main = r.Candidates[0].PlaylistNum
	}
	detectEpisodes(r, byNum)
	classify(r, byNum)
	r.warn(byNum)
	return r
}

func clipNumber(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// clipStart is where a clip's time base begins (45 kHz).
func clipStart(c *Clip, stc int) (uint32, bool) {
	for i := range c.STC {
		if c.STC[i].ID == stc {
			return c.STC[i].Start, true
		}
	}
	if len(c.STC) > 0 {
		return c.STC[0].Start, true
	}
	return 0, false
}

func near(a, b uint32) bool {
	d := int64(a) - int64(b)
	return d > -45000 && d < 45000
}

// measure reads one playlist's structure.
func (d *Disc) measure(c *Candidate, pl *Playlist, sizes map[string]int64) *feat {
	f := &feat{c: c, pl: pl, set: map[segKey]bool{}}
	*c = Candidate{PlaylistNum: pl.Number, Items: len(pl.Items), pl: pl}
	clips := map[string]bool{}
	starts := make([]float64, len(pl.Items))
	for i := range pl.Items {
		it := &pl.Items[i]
		dur := it.Duration()
		starts[i] = f.total
		f.total += dur
		switch {
		case dur < 1:
			f.tiny++
		case dur < 10:
			f.short++
		}
		bad := false
		if len(d.Clips) > 0 {
			cl := d.Clips[it.Clip]
			if cl == nil {
				f.missing++
				bad = true
			} else {
				if end := cl.EndPTS(it.STCID); end > 0 && it.Out > end+45000 {
					f.pastEnd++
					bad = true
				} else if st, ok := clipStart(cl, it.STCID); ok && st > 0 && uint64(it.In)+45000 < uint64(st) {
					f.before++
				}
			}
		}
		// A stream file that is missing or empty marks a decoy, but doesn't
		// rule the playlist out: a copy of a disc's structure without its
		// video must still be described.
		if s, ok := sizes[it.Clip]; ok && !bad {
			switch {
			case s < 0:
				f.missing++
			case s == 0:
				f.zero++
			}
		}
		if bad {
			f.invalid++
		} else {
			k := segKey{it.Clip, it.STCID, it.In, it.Out}
			f.seq = append(f.seq, k)
			f.valid += dur
			if !f.set[k] {
				f.set[k] = true
				c.Distinct += dur
			}
			if !clips[it.Clip] {
				clips[it.Clip] = true
				if s := sizes[it.Clip]; s > 0 {
					c.Size += s
				} else if cl := d.Clips[it.Clip]; cl != nil {
					c.Size += cl.Size()
				}
			}
		}
		c.Angles = max(c.Angles, len(it.Angles))
		if i > 0 {
			f.joins++
			cont, back := d.join(&pl.Items[i-1], it)
			if cont {
				f.cont++
			}
			if back {
				f.back++
			}
		}
	}
	c.Duration, c.Clips = f.total, len(clips)
	if len(pl.Items) > 0 {
		first := &pl.Items[0]
		f.video, f.audio = len(first.Video) > 0, len(first.Audio) > 0
		c.Audio, c.Subtitles = len(first.Audio), len(first.PG)
		seen := map[string]bool{}
		for _, a := range first.Audio {
			if a.Lang != "" && !seen[a.Lang] {
				seen[a.Lang] = true
				c.Languages = append(c.Languages, a.Lang)
			}
		}
	}
	for _, m := range pl.Marks {
		if m.Type != 1 {
			continue
		}
		c.Chapters++
		if m.Item < 0 || m.Item >= len(pl.Items) {
			f.badMarks = true
			continue
		}
		t := starts[m.Item]
		if in := pl.Items[m.Item].In; m.Time > in {
			t += ticks45(m.Time - in)
		}
		f.marks = append(f.marks, t)
	}
	keys := make([]string, 0, len(f.set))
	for k := range f.set {
		keys = append(keys, fmt.Sprintf("%s/%d/%d/%d", k.clip, k.stc, k.in, k.out))
	}
	sort.Strings(keys)
	f.sig = strings.Join(keys, ";")
	f.eligible = len(pl.Items) > 0 && f.invalid == 0 && c.Distinct > 0 && (f.video || f.audio)
	return f
}

// join judges the seam between two play items: cont when the next one
// continues where the last one ended (its presentation time follows on, it
// starts at the same entry point of the same clip's packet map, or a whole
// clip follows a whole clip of a higher number), back when it goes back on
// the disc (a lower-numbered clip, or earlier in the same clip) without
// following on in time.
func (d *Disc) join(a, b *PlayItem) (cont, back bool) {
	ascending := false
	if a.Clip == b.Clip {
		ascending = uint64(b.In)+45000 >= uint64(a.Out)
	} else {
		na, oka := clipNumber(a.Clip)
		nb, okb := clipNumber(b.Clip)
		if oka && okb {
			ascending = nb > na
		} else {
			ascending = b.Clip > a.Clip
		}
	}
	pts := near(a.Out, b.In)
	spn := false
	whole := false
	ca, cb := d.Clips[a.Clip], d.Clips[b.Clip]
	if a.Clip == b.Clip && ca != nil && len(ca.EPMaps) > 0 {
		pid := ca.VideoPID()
		ea, oka := ca.EntryAt(pid, a.STCID, a.Out)
		eb, okb := ca.EntryAt(pid, b.STCID, b.In)
		spn = oka && okb && a.STCID == b.STCID && ea.SPN == eb.SPN && b.In >= a.In
	}
	if a.Clip != b.Clip && ca != nil && cb != nil {
		endA := ca.EndPTS(a.STCID)
		stB, okB := clipStart(cb, b.STCID)
		whole = endA > 0 && near(a.Out, endA) && okB && near(b.In, stB)
	}
	cont = pts || spn || (whole && ascending)
	back = !ascending && !pts && !spn
	return cont, back
}

// scorePlaylist applies the signals one playlist carries by itself.
func scorePlaylist(f *feat, w Weights) {
	c := f.c
	n := len(f.pl.Items)
	c.add(w.DistinctPerMinute*c.Distinct/60, 0, "distinct video: %s of unique, readable segments (plays %s)", fmtDur(c.Distinct), fmtDur(c.Duration))
	if f.missing > 0 {
		c.add(w.MissingClip, FlagMissingClip, "%d of %d play items name a clip whose information or stream file is missing", f.missing, n)
	}
	if f.zero > 0 {
		c.add(w.ZeroSize, FlagZeroSize, "%d of %d play items name a clip whose stream file is empty", f.zero, n)
	}
	if bad := f.pastEnd + f.before; bad > 0 {
		share := float64(bad) / float64(max(n, 1))
		c.add(w.OutOfRangeAny+w.OutOfRangeShare*share, FlagOutOfRange, "%d of %d play items start before or end after their clip's time base", bad, n)
	}
	if f.valid > 0 && len(f.seq) > len(f.set) {
		if rep := 1 - c.Distinct/f.valid; rep >= 0.2 {
			c.add(w.Repeats*rep, FlagRepeats, "%.0f%% of its play time replays segments it already played (%d repeats)", rep*100, len(f.seq)-len(f.set))
		}
	}
	if f.back > 0 {
		share := float64(f.back) / float64(max(f.joins, 1))
		c.add(w.BackJumpAny+w.BackJumpShare*share, 0, "plays its clips out of order (%d of %d joins go back)", f.back, f.joins)
	}
	if f.joins > 0 && f.cont > 0 {
		c.add(w.Continuity*float64(f.cont)/float64(f.joins), 0, "%d of %d joins continue where the last segment ended (presentation time or clip packet map)", f.cont, f.joins)
	}
	if (f.tiny >= 10 && f.tiny*2 >= n) || (n >= 20 && (f.tiny+f.short)*2 >= n) {
		c.add(w.SegmentSpam, FlagSegmentSpam, "%d of %d play items are under 10 seconds long (%d under a second)", f.tiny+f.short, n, f.tiny)
	}
	if f.badMarks {
		c.add(w.BadChapters, FlagBadChapters, "its chapter marks name play items that don't exist")
	}
	if k := len(f.marks); k >= 3 && c.Duration > 0 {
		marks := append([]float64(nil), f.marks...)
		sort.Float64s(marks)
		var lens []float64
		for i, s := range marks {
			e := c.Duration
			if i+1 < k {
				e = marks[i+1]
			}
			lens = append(lens, e-s)
		}
		mean := (c.Duration - marks[0]) / float64(k)
		var v float64
		for _, l := range lens {
			v += (l - mean) * (l - mean)
		}
		if mean > 0 {
			cv := math.Sqrt(v/float64(k)) / mean
			if cv < 0.8 && mean >= 60 && mean <= 20*60 {
				c.add(w.RegularChapters, 0, "%d chapters of even length (about %s each)", k, fmtDur(mean))
			}
		}
	}
	for _, sp := range f.pl.SubPaths {
		if sp.Type == 8 || sp.Type == 9 {
			c.add(w.Stereo3D, FlagStereo3D, "has a 3D (MVC) dependent view; it plays here as 2D")
			break
		}
	}
	if c.Angles > 1 {
		c.add(0, FlagMultiAngle, "has %d angles", c.Angles)
	}
	if !f.video && f.audio {
		c.add(0, FlagAudioOnly, "has sound and no video")
	}
}

// natural is the share of joins that continue in time or go forward on
// the disc; 1 for a single item.
func (f *feat) natural() float64 {
	if f.joins == 0 {
		return 1
	}
	return float64(f.cont+f.joins-f.back) / float64(2*f.joins)
}

func overlap(a, b map[segKey]bool) int {
	if len(a) > len(b) {
		a, b = b, a
	}
	n := 0
	for k := range a {
		if b[k] {
			n++
		}
	}
	return n
}

func sameSeq(a, b []segKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// crossPlaylist compares playlists: duplicates and shuffled copies. Feats
// are in playlist number order.
func crossPlaylist(feats []*feat, w Weights) {
	// Siblings that play the same set of segments: the one in the most
	// natural order stands; the others are shuffled copies, or duplicates.
	groups := map[string][]*feat{}
	for _, f := range feats {
		if len(f.set) > 0 {
			groups[f.sig] = append(groups[f.sig], f)
		}
	}
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		best := 0.0
		for _, f := range g {
			best = max(best, f.natural())
		}
		var keep *feat
		for _, f := range g {
			if f.natural() >= best-1e-9 {
				keep = f
				break
			}
		}
		for i, f := range g {
			dup := false
			for _, e := range g[:i] {
				if sameSeq(f.seq, e.seq) {
					f.c.add(w.Duplicate, FlagDuplicate, "plays the same segments in the same order as playlist %05d", e.c.PlaylistNum)
					dup = true
					break
				}
			}
			if !dup && f.natural() < best-1e-9 {
				f.c.add(w.Shuffled, FlagShuffled, "plays the same segments as playlist %05d in a less natural order", keep.c.PlaylistNum)
			}
		}
	}
	// A playlist that jumps about more than another and mostly plays its
	// segments is a scrambled copy of it.
	for _, a := range feats {
		if a.back == 0 || len(a.set) == 0 || a.c.Flags.Has(FlagShuffled) {
			continue
		}
		for _, b := range feats {
			if a == b || len(b.set) == 0 || a.back <= b.back || a.sig == b.sig {
				continue
			}
			if ov := overlap(a.set, b.set); ov*5 >= len(a.set)*4 && ov*2 >= len(b.set) {
				a.c.add(w.Shuffled, FlagShuffled, "plays playlist %05d's segments in a scrambled order", b.c.PlaylistNum)
				break
			}
		}
	}
}

// reachTarget walks the playlists a static reach plays, in order, and
// returns the first long readable one with those passed before it.
func reachTarget(reach Reach, byNum map[int]*feat, minDur float64) (int, []int) {
	var intros []int
	for _, n := range reach.Playlists {
		f := byNum[n]
		if f == nil || !f.eligible {
			continue
		}
		if f.c.Distinct >= minDur {
			return n, intros
		}
		intros = append(intros, n)
	}
	return -1, intros
}

// discSignals applies what the disc as a whole says: its program, stream
// counts and sizes.
func (d *Disc) discSignals(r *Ranking, feats []*feat, w Weights) {
	byNum := map[int]*feat{}
	longest := 0.0
	for _, f := range feats {
		byNum[f.c.PlaylistNum] = f
		if f.eligible {
			longest = max(longest, f.c.Distinct)
		}
	}
	minDur := math.Min(minFeatureSec, longest*0.5)
	if minDur <= 0 {
		minDur = 1
	}
	if d.Index != nil {
		ix := d.Index
		fpBDJ := ix.FirstPlay.Kind == ObjectBDJ
		t1BDJ := len(ix.Titles) > 0 && ix.Titles[0].Object.Kind == ObjectBDJ
		r.BDJ = fpBDJ || t1BDJ
		if !fpBDJ {
			reach := d.FirstPlayReach()
			fp, intros := reachTarget(reach, byNum, minDur)
			r.FirstPlay = fp
			if f := byNum[fp]; f != nil {
				f.c.add(w.FirstPlay, FlagFirstPlay|FlagMenuTarget, "first play leads to it (a static reading of the disc's HDMV program)")
			}
			for _, n := range intros {
				byNum[n].c.add(0, FlagIntro, "first play passes through it on the way")
			}
			if reach.Unknown > 0 {
				r.Warnings = append(r.Warnings, fmt.Sprintf("First play names %s through registers whose value can't be read without running the disc; those were left out.", pluralWord(reach.Unknown, "target", "targets")))
			}
		}
		if len(ix.Titles) > 0 && !t1BDJ {
			t1, _ := reachTarget(d.TitleReach(1), byNum, minDur)
			r.TitleOne = t1
			if f := byNum[t1]; f != nil {
				if f.c.Flags.Has(FlagFirstPlay) {
					f.c.add(0, FlagTitleOne, "title 1 leads to it as well")
				} else {
					f.c.add(w.TitleOne, FlagTitleOne|FlagMenuTarget, "title 1 leads to it (a static reading of the disc's HDMV program)")
				}
			}
		}
		refs := map[int]int{}
		for _, t := range ix.Titles {
			if t.Object.Kind != ObjectHDMV {
				continue
			}
			for _, n := range d.StaticReach(t.Object, false).Playlists {
				refs[n]++
			}
		}
		for n, k := range refs {
			if f := byNum[n]; f != nil && !f.c.Flags.Has(FlagFirstPlay) && !f.c.Flags.Has(FlagTitleOne) {
				f.c.add(w.MenuReference, FlagMenuTarget, "the programs of %s play it", pluralWord(k, "title", "titles"))
			}
		}
		if r.BDJ {
			listed := map[int]bool{}
			for _, o := range []IndexObject{ix.FirstPlay, firstTitle(ix)} {
				if o.Kind != ObjectBDJ {
					continue
				}
				if bo, err := d.BDJObject(o.BDJName); err == nil && len(bo.Playlists) > 0 && len(bo.Playlists) <= maxBDJListed {
					for _, p := range bo.Playlists {
						listed[p] = true
					}
				}
			}
			for n := range listed {
				if f := byNum[n]; f != nil {
					f.c.add(w.BDJListed, FlagMenuTarget, "the BD-J object of first play or title 1 lists it among the few playlists its Java program may play")
				}
			}
			r.Warnings = append(r.Warnings, "First play or title 1 is a BD-J (Java) program, which this engine doesn't run: the playlists were ranked by their structure alone.")
		}
	}
	// Stream counts and sizes, among the playlists long enough to matter.
	var long []*feat
	for _, f := range feats {
		if f.eligible && f.video && f.c.Distinct >= longest*0.5 {
			long = append(long, f)
		}
	}
	if len(long) < 2 {
		return
	}
	maxA, minA, maxS, minS := -1, 1<<30, -1, 1<<30
	var big, small int64 = -1, 1 << 62
	for _, f := range long {
		maxA, minA = max(maxA, f.c.Audio), min(minA, f.c.Audio)
		maxS, minS = max(maxS, f.c.Subtitles), min(minS, f.c.Subtitles)
		big, small = max(big, f.c.Size), min(small, f.c.Size)
	}
	for _, f := range long {
		c := f.c
		if maxA != minA && c.Audio == maxA {
			c.add(w.AudioMatch, 0, "has the most audio streams of the long playlists (%d: %s)", maxA, strings.Join(c.Languages, ", "))
		}
		if maxS != minS && c.Subtitles == maxS {
			c.add(w.SubtitleMatch, 0, "has the most subtitle streams of the long playlists (%d)", maxS)
		}
		if big != small && big > 0 && c.Size == big {
			c.add(w.LargestSize, 0, "its clips hold the most video (%s)", sizeText(big))
		}
	}
}

func sizeText(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

func firstTitle(ix *Index) IndexObject {
	if len(ix.Titles) == 0 {
		return IndexObject{}
	}
	return ix.Titles[0].Object
}

// isDecoy says the structure gives the playlist away as a decoy.
func isDecoy(f *feat) bool {
	c := f.c
	if c.Flags&(FlagShuffled|FlagSegmentSpam|FlagMissingClip|FlagZeroSize) != 0 {
		return true
	}
	if f.pastEnd > 0 {
		return true
	}
	if f.valid > 0 && 1-c.Distinct/f.valid >= 0.5 {
		return true
	}
	return false
}
