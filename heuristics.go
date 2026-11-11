// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

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
	FlagTruncated                    // names a clip whose .m2ts is much shorter than its information says
	FlagAmbiguous                    // one of several playlists of the same video that nothing tells apart
	FlagEdition                      // one cut of a film that has several (seamless branching)
	FlagBDJTarget                    // the disc's Java program names it (static reading of its JAR files)
	FlagUHD                          // 2160p video
	FlagHDR                          // HDR10, HDR10+ or Dolby Vision video
)

var flagNames = []string{"main", "decoy", "menuTarget", "firstPlay", "titleOne", "intro", "episode", "playAll", "duplicate", "shuffled", "outOfRange", "repeats", "segmentSpam", "missingClip", "zeroSize", "badChapters", "alternate", "stereo3D", "multiAngle", "audioOnly", "truncated", "ambiguous", "edition", "bdjTarget", "uhd", "hdr"}

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
	// Evidence is Reasons in a form a program can use: one entry per signal
	// with its name, the points it gave or took, and the sentence.
	Evidence []Evidence `json:"evidence,omitempty"`
	// Video describes the main video stream: "1080p H.264", "2160p HEVC
	// HDR10", "1080p H.264 3D".
	Video             string   `json:"video,omitempty"`
	SubtitleLanguages []string `json:"subtitleLanguages,omitempty"`
	// Continuity is how well the play items follow on from each other, from
	// -1 (every join goes back) to 1 (every join continues in time): the
	// evidence that tells a film from a shuffled copy of it. 1 for a single
	// item.
	Continuity float64 `json:"continuity"`
	// Cluster groups playlists that play the same set of segments (decoys
	// are permutations and copies of one another): the lowest playlist
	// number in the group, and how many it holds. ClusterSize is 1 for a
	// playlist like no other.
	Cluster     int `json:"cluster"`
	ClusterSize int `json:"clusterSize"`

	pl *Playlist
}

// Evidence is one signal's contribution to a candidate's score.
type Evidence struct {
	Signal string  `json:"signal"` // distinct, continuity, backJumps, firstPlay, shuffled, ...
	Points float64 `json:"points"`
	Detail string  `json:"detail"`
}

// Playlist is the playlist the candidate was scored from.
func (c Candidate) Playlist() *Playlist { return c.pl }

// Episode is one episode of a TV disc, in viewing order.
type Episode struct {
	Index    int     `json:"index"` // 1-based viewing order
	Playlist int     `json:"playlist"`
	Duration float64 `json:"durationSec"`
	Chapters int     `json:"chapters"`
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
	// engine doesn't run; the ranking falls back to structure, helped by a
	// static reading of the program's constants.
	BDJ      bool     `json:"bdj,omitempty"`
	Warnings []string `json:"warnings,omitempty"`

	// Confidence says how sure the pick is, from 0 to 1, and
	// ConfidenceLabel the same as "high", "medium" or "low". Why gives the
	// reasons for the pick and for the confidence in short sentences, the
	// weightiest first: what a server shows as "why this playlist".
	Confidence      float64  `json:"confidence"`
	ConfidenceLabel string   `json:"confidenceLabel"`
	Why             []string `json:"why,omitempty"`
	// Editions lists the cuts of a film that shares its video between
	// several playlists (seamless branching: theatrical and extended), the
	// main one included, longest first. Empty when there is one cut.
	Editions []Edition `json:"editions,omitempty"`
	// Clusters summarises the groups of playlists that play the same
	// segments: the decoys of an obfuscated disc show as a few large ones.
	Clusters []Cluster `json:"clusters,omitempty"`
	// PlayAllParts splits a play-all playlist where its episodes begin.
	// With per-episode playlists these are the Episodes mapped onto it;
	// without, they are a guess from play items of even length, and
	// Episodes stays empty.
	PlayAllParts []Part `json:"playAllParts,omitempty"`
	// Chapters are the main playlist's chapters, mapped onto its playing time.
	Chapters []Chapter `json:"chapters,omitempty"`
	// Stereo3D, UHD and HDR describe the main playlist's video. HDR is "",
	// "HDR10", "HDR10+" or "Dolby Vision".
	Stereo3D bool   `json:"stereo3d,omitempty"`
	UHD      bool   `json:"uhd,omitempty"`
	HDR      string `json:"hdr,omitempty"`
	// MovieOnly: the copy has no disc program at all.
	MovieOnly bool `json:"movieOnly,omitempty"`
	// Program says how the disc's own program was read for this ranking.
	Program ProgramReading `json:"program"`
	// Inconsistencies counts the disagreements between playlists, clip
	// information and stream files by kind (see Disc.Consistency).
	Inconsistencies map[string]int `json:"inconsistencies,omitempty"`
	// SecondOpinion is the comparison with another reader of the disc
	// (libbluray, when the user has it installed and the caller passed its
	// answer in). nil when there was none.
	SecondOpinion *SecondOpinionReport `json:"secondOpinion,omitempty"`
}

// ProgramReading says what the static reading of the disc's program gave.
type ProgramReading struct {
	// Kind: "hdmv" (movie objects were read), "bdj" (first play or title 1
	// is Java), "none" (a copy without a program).
	Kind string `json:"kind"`
	// Certain: the playlists first play is sure to play, whatever the
	// player's state.
	Certain []int `json:"certain,omitempty"`
	// Unknown counts playlist, object and jump operands that were registers
	// of unknown value. Incomplete: the reading ran out of budget.
	Unknown    int  `json:"unknown,omitempty"`
	Incomplete bool `json:"incomplete,omitempty"`
	// BDJClasses is how many Java class files were read for constants, and
	// BDJNamed how many of the disc's playlists they name.
	BDJClasses int `json:"bdjClasses,omitempty"`
	BDJNamed   int `json:"bdjNamed,omitempty"`
}

// Edition is one cut of a film with several.
type Edition struct {
	Playlist int     `json:"playlist"`
	Duration float64 `json:"durationSec"`
	// Shared is the playing time it has in common with the main playlist,
	// Own the time of video that only this cut has among the editions.
	Shared float64 `json:"sharedSec"`
	Own    float64 `json:"ownSec"`
	// Label: "longest cut", "shortest cut" or "cut" for those between.
	Label    string `json:"label"`
	Chapters int    `json:"chapters"`
	Main     bool   `json:"main,omitempty"`
	// ByProgram: the disc's program is seen to play it.
	ByProgram bool `json:"byProgram,omitempty"`
}

// Cluster is a group of playlists that play the same set of segments.
type Cluster struct {
	// Kept is the member that stands for the group: the one in the most
	// natural order. Size counts all members; Shuffled those in a less
	// natural order, Duplicates those in exactly Kept's order.
	Kept       int     `json:"kept"`
	Size       int     `json:"size"`
	Shuffled   int     `json:"shuffled"`
	Duplicates int     `json:"duplicates"`
	Duration   float64 `json:"durationSec"`
	// Ambiguous: more than one member is in an equally natural order and
	// nothing else tells them apart.
	Ambiguous bool `json:"ambiguous,omitempty"`
}

// Part is one stretch of a play-all playlist.
type Part struct {
	Index     int     `json:"index"`     // 1-based
	FirstItem int     `json:"firstItem"` // play items, inclusive
	LastItem  int     `json:"lastItem"`
	Start     float64 `json:"startSec"`
	Duration  float64 `json:"durationSec"`
	// Episode is the per-episode playlist this part matches, or -1.
	Episode int `json:"episode"`
}

// SecondOpinion is another reader's account of the disc, to compare with.
// The libbluray package fills one in from a user-installed libbluray.
type SecondOpinion struct {
	Source string               `json:"source"` // "libbluray 1.3.4"
	Main   int                  `json:"main"`   // the playlist it would play as the feature; -1 none
	Titles []SecondOpinionTitle `json:"titles"`
}

// SecondOpinionTitle is one playlist as the other reader sees it.
type SecondOpinionTitle struct {
	Playlist int     `json:"playlist"`
	Duration float64 `json:"durationSec"`
	Chapters int     `json:"chapters"`
	Angles   int     `json:"angles"`
	// ChapterStarts in seconds of playing time, when the reader gives them.
	ChapterStarts []float64 `json:"chapterStarts,omitempty"`
}

// SecondOpinionReport is how the second opinion compares.
type SecondOpinionReport struct {
	Source string `json:"source"`
	Main   int    `json:"main"`
	// AgreesOnMain: it picks the same playlist, or one that plays the same
	// segments in the same order.
	AgreesOnMain  bool           `json:"agreesOnMain"`
	Disagreements []Disagreement `json:"disagreements,omitempty"`
	// More counts disagreements past the first fifty.
	More int `json:"more,omitempty"`
}

// Disagreement is one difference between this engine and a second opinion.
type Disagreement struct {
	Playlist int    `json:"playlist"` // -1 for the disc as a whole
	Field    string `json:"field"`    // main, present, duration, chapters, chapterTimes, angles
	Ours     string `json:"ours"`
	Theirs   string `json:"theirs"`
	Note     string `json:"note,omitempty"`
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
	Truncated         float64 // times the share of play time on clips whose stream file is cut short
	Forward           float64 // times the share of joins that only go forward on the disc
	BadConnection     float64 // an item claims to continue its clock from the last one and does not
	BDJLocator        float64 // the Java program has a locator or file name for it
	BDJName           float64 // the Java program has its five digits as a string
	BDJNumber         float64 // the Java program pushes its number, and few other long playlists'
	Ambiguous         float64 // nothing tells it from a sibling of the same video
	Rewind            float64 // times the share of joins that go back inside one clip
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
	Truncated:         -30,
	Forward:           2,
	BadConnection:     -3,
	BDJLocator:        8,
	BDJName:           5,
	BDJNumber:         2,
	Ambiguous:         0,
	Rewind:            -40,
}

// RankOptions add what the caller knows beyond the disc's structures.
type RankOptions struct {
	// ClipSizes is the size in bytes of each clip's .m2ts file (-1 when the
	// file is missing). nil reads them from the disc's Source when it has
	// one; a disc without a Source is ranked without them.
	ClipSizes map[string]int64
	Weights   *Weights
	// ClipOffsets is where each clip's stream file begins on the disc, in
	// bytes from the start of the image. A film's clips lie in playing
	// order (a player cannot seek back between them without a pause), so
	// the order is evidence. nil reads them from an image's Source; a
	// folder of files has none.
	ClipOffsets map[string]int64
	// SecondOpinion is another reader's account of the disc to compare
	// with; nil for none.
	SecondOpinion *SecondOpinion
	// NoBDJ skips the static reading of a BD-J disc's JAR files.
	NoBDJ bool
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

func (k segKey) less(o segKey) bool {
	switch {
	case k.clip != o.clip:
		return k.clip < o.clip
	case k.stc != o.stc:
		return k.stc < o.stc
	case k.in != o.in:
		return k.in < o.in
	}
	return k.out < o.out
}

func (k segKey) dur() float64 {
	if k.out <= k.in {
		return 0
	}
	return ticks45(k.out - k.in)
}

// feat is what one playlist's structure says.
type feat struct {
	c         *Candidate
	pl        *Playlist
	seq       []segKey // readable segments in play order
	set       map[segKey]bool
	keys      []segKey // the set, sorted
	setHash   uint64
	seqHash   uint64
	total     float64
	valid     float64 // play time of readable segments
	missing   int
	zero      int
	pastEnd   int
	before    int
	invalid   int // items that can't be read (no clip information, or past the clip's end)
	joins     int
	carry     int     // joins where time or the packet map carries straight on
	cont      int     // those, and joins onto the very next clip on the disc
	fwd       int     // joins that only go forward on the disc
	back      int     // joins that go back
	rewinds   int     // those that go back inside one clip: no film does that
	order     float64 // the sum of every join's evidence, each from -1 to 1
	badCC     int
	shortTime float64 // play time on clips whose stream file is cut short
	tiny      int
	short     int
	video     bool
	audio     bool
	eligible  bool
	partial   bool // some of it reads: a last resort when nothing is eligible
	marks     []float64
	badMarks  bool
	nav       int // how strongly the disc's program points at it: 3 first play, 2 title 1, 1 a title or the Java program
}

// rankCtx is what ranking knows about the disc besides its playlists.
type rankCtx struct {
	d       *Disc
	sizes   map[string]int64
	rankOf  map[string]int // clips in the order they lie on the disc; nil when unknown
	hasSize bool
}

func fmtDur(sec float64) string {
	if sec < 0 || math.IsNaN(sec) || math.IsInf(sec, 0) {
		sec = 0
	}
	s := int(sec + 0.5)
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
}

func (c *Candidate) add(points float64, flag Flag, format string, args ...any) {
	c.note("", points, flag, format, args...)
}

// note records one signal: its points, its flag, and its sentence, both as
// a reason for people and as evidence for programs.
func (c *Candidate) note(signal string, points float64, flag Flag, format string, args ...any) {
	if math.IsNaN(points) || math.IsInf(points, 0) {
		points = 0
	}
	c.Score += points
	c.Flags |= flag
	detail := fmt.Sprintf(format, args...)
	msg := detail
	if points != 0 {
		msg = fmt.Sprintf("%+.1f %s", points, msg)
	}
	c.Reasons = append(c.Reasons, msg)
	if signal == "" {
		signal = "note"
	}
	c.Evidence = append(c.Evidence, Evidence{Signal: signal, Points: round1(points), Detail: detail})
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

// clipFileSizes reads the size of every clip's stream file once, and where
// each begins when they all lie in one image.
func (d *Disc) clipFileSizes() map[string]int64 {
	if d.Source == nil {
		return nil
	}
	d.sizesOnce.Do(func() {
		m := make(map[string]int64, len(d.Clips))
		at := make(map[string]int64, len(d.Clips))
		file, same := "", true
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
			if len(ext) > 0 {
				if file == "" {
					file = ext[0].Path
				}
				if ext[0].Path != file || file == "" {
					same = false
				}
				at[n] = ext[0].Offset
			}
		}
		d.sizes = m
		if same && len(at) > 1 {
			d.offsets, d.sameFile = at, true
		}
	})
	return d.sizes
}

// clipOrder ranks clips by where they lie on the disc. nil when fewer than
// two positions are known or they are all the same.
func clipOrder(at map[string]int64) map[string]int {
	if len(at) < 2 {
		return nil
	}
	names := make([]string, 0, len(at))
	for n := range at {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if at[names[i]] != at[names[j]] {
			return at[names[i]] < at[names[j]]
		}
		return names[i] < names[j]
	})
	if at[names[0]] == at[names[len(names)-1]] {
		return nil
	}
	out := make(map[string]int, len(names))
	for i, n := range names {
		out[n] = i
	}
	return out
}

// Analyze scores and classifies every playlist. Its cost grows with the
// number of play items on the disc, not with the square of the number of
// playlists: playlists are compared through hashes of their segments, and
// against a fixed number of reference playlists.
func (d *Disc) Analyze(opt RankOptions) *Ranking {
	r := &Ranking{Main: -1, PlayAll: -1, FirstPlay: -1, TitleOne: -1, ConfidenceLabel: "low"}
	if d == nil || len(d.Playlists) == 0 {
		r.Warnings = append(r.Warnings, "The disc has no playlist.")
		return r
	}
	w := DefaultWeights
	if opt.Weights != nil {
		w = *opt.Weights
	}
	ctx := &rankCtx{d: d, sizes: opt.ClipSizes}
	if ctx.sizes == nil {
		ctx.sizes = d.clipFileSizes()
	}
	ctx.hasSize = len(ctx.sizes) > 0
	switch {
	case opt.ClipOffsets != nil:
		ctx.rankOf = clipOrder(opt.ClipOffsets)
	case opt.ClipSizes == nil && d.Source != nil && d.sameFile:
		ctx.rankOf = clipOrder(d.offsets)
	}
	var pls []*Playlist
	for _, pl := range d.sortedPlaylists() {
		if pl != nil {
			pls = append(pls, pl)
		}
	}
	r.Candidates = make([]Candidate, len(pls))
	feats := make([]*feat, len(pls))
	byNum := make(map[int]*feat, len(pls))
	for i, pl := range pls {
		feats[i] = ctx.measure(&r.Candidates[i], pl)
		byNum[pl.Number] = feats[i]
	}
	for _, f := range feats {
		scorePlaylist(f, w)
	}
	d.programSignals(r, feats, byNum, w, opt)
	crossPlaylist(r, feats, w)
	streamSignals(feats, w)

	tier := func(f *feat) int {
		t := 0
		switch {
		case !f.eligible && f.partial:
			return 8
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
	tiers := make(map[int]int, len(feats))
	for _, f := range feats {
		tiers[f.c.PlaylistNum] = tier(f)
	}
	sort.SliceStable(r.Candidates, func(i, j int) bool {
		a, b := &r.Candidates[i], &r.Candidates[j]
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
	preferProgramCut(r, byNum, tiers)
	if t := tiers[r.Candidates[0].PlaylistNum]; t < 9 {
		r.Candidates[0].Flags |= FlagMain
		r.Main = r.Candidates[0].PlaylistNum
		if t == 8 {
			r.Warnings = append(r.Warnings, fmt.Sprintf("No playlist reads as a whole: playlist %05d was picked as the one with the most video that does read, and parts of it will not play.", r.Main))
		}
	}
	detectEpisodes(r, byNum)
	detectEditions(r, byNum)
	classify(r, byNum)
	splitPlayAll(r, byNum)
	if m := byNum[r.Main]; m != nil && r.Main >= 0 {
		r.Chapters = m.pl.Chapters()
		r.Stereo3D = m.c.Flags.Has(FlagStereo3D)
		r.UHD = m.c.Flags.Has(FlagUHD)
		r.HDR = hdrName(m.pl)
	}
	r.MovieOnly = d.MovieOnly
	if opt.ClipSizes == nil && d.Source != nil {
		for _, x := range d.Consistency() {
			if x.Kind == "more" {
				continue
			}
			if r.Inconsistencies == nil {
				r.Inconsistencies = map[string]int{}
			}
			r.Inconsistencies[x.Kind]++
		}
	}
	compareSecondOpinion(r, d, byNum, opt.SecondOpinion)
	r.warn(byNum)
	r.confidence(byNum)
	return r
}

// preferProgramCut moves to the front the cut of the top playlist's film
// that the disc's own program plays, when the top one is another cut of it
// that only won on length. A film with a theatrical and an extended cut has
// both as clean playlists sharing most clips; the one first play or title 1
// leads to is the disc's own choice, and the other is listed as an edition.
func preferProgramCut(r *Ranking, feats map[int]*feat, tiers map[int]int) {
	top := feats[r.Candidates[0].PlaylistNum]
	if top == nil || tiers[top.c.PlaylistNum] != 0 || top.nav >= 2 {
		return
	}
	topTimes := clipTimes(top)
	for i := 1; i < len(r.Candidates) && i < 64; i++ {
		f := feats[r.Candidates[i].PlaylistNum]
		if f == nil || tiers[f.c.PlaylistNum] != 0 || f.nav < 2 || f.back > 0 || f.c.Distinct < top.c.Distinct*0.5 {
			continue
		}
		shared := 0.0
		for clip, t := range clipTimes(f) {
			shared += math.Min(t, topTimes[clip])
		}
		if shared < 0.5*math.Min(f.c.Distinct, top.c.Distinct) {
			continue
		}
		f.c.note("programCut", 0, 0, "a cut of the same film as playlist %05d, which is longer; this is the one the disc's program plays", top.c.PlaylistNum)
		picked := r.Candidates[i]
		copy(r.Candidates[1:i+1], r.Candidates[:i])
		r.Candidates[0] = picked
		for k := 0; k <= i; k++ {
			feats[r.Candidates[k].PlaylistNum].c = &r.Candidates[k]
		}
		return
	}
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

func hashKey(h uint64, k segKey) uint64 {
	const prime = 1099511628211
	for i := 0; i < len(k.clip); i++ {
		h = (h ^ uint64(k.clip[i])) * prime
	}
	for _, v := range [3]uint32{uint32(k.stc), k.in, k.out} {
		for s := 0; s < 32; s += 8 {
			h = (h ^ uint64(byte(v>>s))) * prime
		}
	}
	return (h ^ 0xff) * prime
}

var videoFormats = map[int]string{1: "480i", 2: "576i", 3: "480p", 4: "1080i", 5: "720p", 6: "1080p", 7: "576p", 8: "2160p"}

// hdrName names the dynamic range of a playlist's main video.
func hdrName(pl *Playlist) string {
	if pl == nil || len(pl.Items) == 0 || len(pl.Items[0].Video) == 0 {
		return ""
	}
	it := &pl.Items[0]
	v := it.Video[0]
	switch {
	case len(it.DV) > 0 || v.DynamicRange == 2:
		return "Dolby Vision"
	case v.HDR10Plus:
		return "HDR10+"
	case v.DynamicRange == 1:
		return "HDR10"
	}
	return ""
}

// measure reads one playlist's structure.
func (x *rankCtx) measure(c *Candidate, pl *Playlist) *feat {
	d := x.d
	f := &feat{c: c, pl: pl, set: make(map[segKey]bool, len(pl.Items))}
	*c = Candidate{PlaylistNum: pl.Number, Items: len(pl.Items), pl: pl, Cluster: pl.Number, ClusterSize: 1, Continuity: 1}
	clips := map[string]bool{}
	starts := make([]float64, len(pl.Items))
	f.seqHash = 14695981039346656037
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
		cl := d.Clips[it.Clip]
		if len(d.Clips) > 0 {
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
		if s, ok := x.sizes[it.Clip]; ok && !bad {
			switch clipFileState(cl, s, true) {
			case clipFileMissing:
				f.missing++
			case clipFileEmpty:
				f.zero++
			case clipFileShort:
				f.shortTime += dur
			}
		}
		if bad {
			f.invalid++
		} else {
			k := segKey{it.Clip, it.STCID, it.In, it.Out}
			f.seq = append(f.seq, k)
			f.seqHash = hashKey(f.seqHash, k)
			f.valid += dur
			if !f.set[k] {
				f.set[k] = true
				c.Distinct += dur
			}
			if !clips[it.Clip] {
				clips[it.Clip] = true
				if s := x.sizes[it.Clip]; s > 0 {
					c.Size += s
				} else if cl != nil {
					c.Size += cl.Size()
				}
			}
		}
		c.Angles = max(c.Angles, len(it.Angles))
		if i > 0 {
			f.joins++
			e, badCC := x.join(&pl.Items[i-1], it)
			f.order += e
			if e >= 0.99 {
				f.carry++
			}
			switch {
			case e >= 0.75:
				f.cont++
			case e > 0:
				f.fwd++
			case e < 0:
				f.back++
				if e <= -0.99 {
					f.rewinds++
				}
			}
			if badCC {
				f.badCC++
			}
		}
	}
	if f.joins > 0 {
		c.Continuity = math.Round(f.order/float64(f.joins)*1000) / 1000
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
		seen = map[string]bool{}
		for _, a := range first.PG {
			if a.Lang != "" && !seen[a.Lang] {
				seen[a.Lang] = true
				c.SubtitleLanguages = append(c.SubtitleLanguages, a.Lang)
			}
		}
		if f.video {
			v := first.Video[0]
			c.Video = strings.TrimSpace(videoFormats[v.Format] + " " + v.CodingName())
			if h := hdrName(pl); h != "" {
				c.Video += " " + h
				c.Flags |= FlagHDR
			}
			if v.Format == 8 {
				c.Flags |= FlagUHD
			}
			if pl.Stereo3D {
				c.Video += " 3D"
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
	f.keys = make([]segKey, 0, len(f.set))
	for k := range f.set {
		f.keys = append(f.keys, k)
	}
	sort.Slice(f.keys, func(i, j int) bool { return f.keys[i].less(f.keys[j]) })
	f.setHash = 14695981039346656037
	for _, k := range f.keys {
		f.setHash = hashKey(f.setHash, k)
	}
	f.eligible = len(pl.Items) > 0 && f.invalid == 0 && c.Distinct > 0 && (f.video || f.audio)
	f.partial = !f.eligible && c.Distinct > 0 && (f.video || f.audio)
	return f
}

// join weighs the evidence at the seam between two play items, from -1 (the
// second goes back on the disc) to 1 (it carries on exactly where the
// first stopped). In falling order of weight:
//
//   - the same clip and time base, the second item starting where the
//     first ended in time, or at the same entry point of the clip's packet
//     map: 1;
//   - another clip whose time carries on from the first's (studios cut one
//     encode into clips, and the clock runs through): 1;
//   - another clip whose times lie wholly after the first's 0.5, wholly
//     before -0.8 (the same clock, with a gap: a cut that leaves scenes
//     out, or a shuffle);
//   - on an image, where the clips lie: the very next clip on the disc 0.8,
//     any later one 0.5, an earlier one -0.8. A film's clips are laid out
//     in playing order, because a player cannot seek back between two
//     seamless items;
//   - with nothing better, the clip numbers: a whole clip following a whole
//     clip of a higher number 0.5, a higher number 0.25, a lower one -0.5.
//
// badCC reports an item that says it continues the last one's clock
// (connection condition 6) while its time does not follow on.
func (x *rankCtx) join(a, b *PlayItem) (e float64, badCC bool) {
	d := x.d
	pts := near(a.Out, b.In)
	ca, cb := d.Clips[a.Clip], d.Clips[b.Clip]
	if a.Clip == b.Clip {
		if pts && a.STCID == b.STCID {
			return 1, false
		}
		if ca != nil && len(ca.EPMaps) > 0 && a.STCID == b.STCID && b.In >= a.In {
			pid := ca.VideoPID()
			ea, oka := ca.EntryAt(pid, a.STCID, a.Out)
			eb, okb := ca.EntryAt(pid, b.STCID, b.In)
			if oka && okb && ea.SPN == eb.SPN {
				return 1, false
			}
		}
		switch {
		case a.STCID != b.STCID:
			if b.STCID > a.STCID {
				return 0.5, false
			}
			return -1, false
		case uint64(b.In)+45000 >= uint64(a.Out):
			return 0.3, false // skips ahead in the same clip
		}
		return -1, false
	}
	// Different clips. A clock that runs through both is the best evidence,
	// but two clips that are each under two seconds long would "continue"
	// from anything.
	if pts && a.Duration() >= 2 && b.Duration() >= 2 {
		return 1, false
	}
	badCC = b.ConnectionCondition == 6 && !pts
	// Clips cut from one encode keep its clock, so their times do not
	// overlap, and which comes later is plain. Clips that each start their
	// clock afresh overlap, and say nothing.
	if a.Duration() >= 2 && b.Duration() >= 2 {
		switch {
		case b.In >= a.Out:
			return 0.5, badCC
		case b.Out <= a.In:
			return -0.8, badCC
		}
	}
	if ra, oka := x.rankOf[a.Clip]; oka {
		if rb, okb := x.rankOf[b.Clip]; okb {
			switch {
			case rb == ra+1:
				return 0.8, badCC
			case rb > ra:
				return 0.5, badCC
			}
			return -0.8, badCC
		}
	}
	ascending := b.Clip > a.Clip
	if na, oka := clipNumber(a.Clip); oka {
		if nb, okb := clipNumber(b.Clip); okb {
			ascending = nb > na
		}
	}
	if !ascending {
		return -0.5, badCC
	}
	if ca != nil && cb != nil {
		endA := ca.EndPTS(a.STCID)
		stB, okB := clipStart(cb, b.STCID)
		if endA > 0 && near(a.Out, endA) && okB && near(b.In, stB) {
			return 0.5, badCC
		}
	}
	return 0.25, badCC
}

// scorePlaylist applies the signals one playlist carries by itself.
func scorePlaylist(f *feat, w Weights) {
	c := f.c
	n := len(f.pl.Items)
	c.note("distinct", w.DistinctPerMinute*c.Distinct/60, 0, "distinct video: %s of unique, readable segments (plays %s)", fmtDur(c.Distinct), fmtDur(c.Duration))
	if f.missing > 0 {
		c.note("missingClip", w.MissingClip, FlagMissingClip, "%d of %d play items name a clip whose information or stream file is missing", f.missing, n)
	}
	if f.zero > 0 {
		c.note("zeroSize", w.ZeroSize, FlagZeroSize, "%d of %d play items name a clip whose stream file is empty", f.zero, n)
	}
	if f.shortTime > 0 && f.total > 0 {
		share := f.shortTime / f.total
		c.note("truncated", w.Truncated*share, FlagTruncated, "%.0f%% of its play time is on clips whose stream file is shorter than the clip information says (a copy cut short)", share*100)
	}
	if bad := f.pastEnd + f.before; bad > 0 {
		share := float64(bad) / float64(max(n, 1))
		c.note("outOfRange", w.OutOfRangeAny+w.OutOfRangeShare*share, FlagOutOfRange, "%d of %d play items start before or end after their clip's time base", bad, n)
	}
	if f.valid > 0 && len(f.seq) > len(f.set) {
		if rep := 1 - c.Distinct/f.valid; rep >= 0.2 {
			c.note("repeats", w.Repeats*rep, FlagRepeats, "%.0f%% of its play time replays segments it already played (%d repeats)", rep*100, len(f.seq)-len(f.set))
		}
	}
	if f.back > 0 {
		share := float64(f.back) / float64(max(f.joins, 1))
		c.note("backJumps", w.BackJumpAny+w.BackJumpShare*share, 0, "plays its clips out of order (%d of %d joins go back)", f.back, f.joins)
	}
	if f.rewinds > 0 {
		share := float64(f.rewinds) / float64(max(f.joins, 1))
		c.note("rewinds", w.Rewind*share, 0, "%d of %d joins go back to an earlier part of the clip just played, which a film never does", f.rewinds, f.joins)
	}
	if f.joins > 0 && f.cont > 0 {
		c.note("continuity", w.Continuity*float64(f.cont)/float64(f.joins), 0, "%d of %d joins continue where the last segment ended (presentation time, clip packet map, or the next clip on the disc)", f.cont, f.joins)
	}
	if f.joins > 0 && f.fwd > 0 {
		c.note("forward", w.Forward*float64(f.fwd)/float64(f.joins), 0, "%d of %d joins go forward on the disc without continuing in time", f.fwd, f.joins)
	}
	if f.badCC > 0 {
		c.note("badConnection", w.BadConnection, 0, "%d play items say they continue the last one's clock, and their times do not follow on", f.badCC)
	}
	if (f.tiny >= 10 && f.tiny*2 >= n) || (n >= 20 && (f.tiny+f.short)*2 >= n) {
		c.note("segmentSpam", w.SegmentSpam, FlagSegmentSpam, "%d of %d play items are under 10 seconds long (%d under a second)", f.tiny+f.short, n, f.tiny)
	}
	if f.badMarks {
		c.note("badChapters", w.BadChapters, FlagBadChapters, "its chapter marks name play items that don't exist")
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
				c.note("regularChapters", w.RegularChapters, 0, "%d chapters of even length (about %s each)", k, fmtDur(mean))
			}
		}
	}
	if f.pl.Stereo3D {
		c.note("stereo3D", w.Stereo3D, FlagStereo3D, "has a 3D (MVC) dependent view; it plays here as 2D")
	}
	if c.Angles > 1 {
		c.note("multiAngle", 0, FlagMultiAngle, "has %d angles", c.Angles)
	}
	if !f.video && f.audio {
		c.note("audioOnly", 0, FlagAudioOnly, "has sound and no video")
	}
}

// natural is how well the joins follow on, from 0 to 1; 1 for a single item.
func (f *feat) natural() float64 {
	if f.joins == 0 {
		return 1
	}
	return (f.order/float64(f.joins) + 1) / 2
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

// overlapTime is the playing time of the segments two playlists share.
func overlapTime(a, b *feat) float64 {
	if len(a.keys) > len(b.keys) {
		a, b = b, a
	}
	t := 0.0
	for _, k := range a.keys {
		if b.set[k] {
			t += k.dur()
		}
	}
	return t
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

// maxReferences is how many playlists every other one is compared with
// when looking for scrambled copies. It is what keeps a disc of thousands
// of playlists from costing millions of comparisons.
const maxReferences = 24

// crossPlaylist compares playlists: duplicates, shuffled copies, and the
// clusters they form. Feats are in playlist number order.
func crossPlaylist(r *Ranking, feats []*feat, w Weights) {
	// Siblings that play the same set of segments: the one in the most
	// natural order stands (the disc's program breaking ties); the others
	// are shuffled copies, or duplicates.
	groups := map[uint64][]*feat{}
	var order []uint64
	for _, f := range feats {
		if len(f.set) == 0 {
			continue
		}
		h := f.setHash
		// Two different sets under one hash would be told apart here; the
		// hash is then stepped on until it finds its own group.
		for {
			g := groups[h]
			if len(g) == 0 || sameSeq(g[0].keys, f.keys) {
				break
			}
			h++
		}
		if len(groups[h]) == 0 {
			order = append(order, h)
		}
		groups[h] = append(groups[h], f)
	}
	for _, h := range order {
		g := groups[h]
		if len(g) < 2 {
			continue
		}
		// Joins that carry on (time, packet map, the next clip on the disc)
		// are strong evidence and weigh most; then what the disc's program
		// points at; clip numbers and "later on the disc" weigh least, so
		// they only decide when nothing else speaks.
		rate := func(f *feat) float64 {
			strong := 0.0
			if f.joins > 0 {
				strong = float64(f.cont) / float64(f.joins)
			}
			return strong*10 + float64(f.nav)*2 + f.natural()
		}
		keep := g[0]
		for _, f := range g[1:] {
			if rate(f) > rate(keep)+1e-9 {
				keep = f
			}
		}
		cl := Cluster{Kept: keep.c.PlaylistNum, Size: len(g), Duration: keep.c.Distinct}
		first := map[uint64]*feat{} // by sequence: the lowest-numbered playlist in each order
		var tied []*feat
		for _, f := range g {
			f.c.Cluster, f.c.ClusterSize = g[0].c.PlaylistNum, len(g)
			if e := first[f.seqHash]; e != nil && sameSeq(e.seq, f.seq) {
				f.c.note("duplicate", w.Duplicate, FlagDuplicate, "plays the same segments in the same order as playlist %05d", e.c.PlaylistNum)
				if sameSeq(f.seq, keep.seq) {
					cl.Duplicates++
				} else if rate(f) < rate(keep)-1e-9 {
					cl.Shuffled++
				}
				continue
			} else if e == nil {
				first[f.seqHash] = f
			}
			switch {
			case f == keep:
			case rate(f) < rate(keep)-1e-9:
				f.c.note("shuffled", w.Shuffled, FlagShuffled, "plays the same segments as playlist %05d in a less natural order", keep.c.PlaylistNum)
				cl.Shuffled++
			default:
				tied = append(tied, f)
			}
		}
		if len(tied) > 0 {
			cl.Ambiguous = true
			for _, f := range append(tied, keep) {
				f.c.note("ambiguous", w.Ambiguous, FlagAmbiguous, "%s of the same segments in other orders follow on just as well, and the disc's program does not single one out", pluralWord(len(tied), "playlist", "playlists"))
			}
		}
		if len(r.Clusters) < 64 {
			r.Clusters = append(r.Clusters, cl)
		}
	}
	sort.SliceStable(r.Clusters, func(i, j int) bool { return r.Clusters[i].Size > r.Clusters[j].Size })
	// A playlist that jumps about more than another and mostly plays its
	// segments is a scrambled copy of it. Every playlist is compared with a
	// few references only: the long ones that jump about least.
	var refs []*feat
	for _, f := range feats {
		if f.eligible && len(f.set) > 0 && !f.c.Flags.Has(FlagShuffled) && !f.c.Flags.Has(FlagDuplicate) {
			refs = append(refs, f)
		}
	}
	sort.SliceStable(refs, func(i, j int) bool {
		a, b := refs[i], refs[j]
		if a.back != b.back {
			return a.back < b.back
		}
		if a.c.Distinct != b.c.Distinct {
			return a.c.Distinct > b.c.Distinct
		}
		return a.natural() > b.natural()
	})
	if len(refs) > maxReferences {
		refs = refs[:maxReferences]
	}
	for _, a := range feats {
		if a.back == 0 || len(a.set) == 0 || a.c.Flags.Has(FlagShuffled) {
			continue
		}
		for _, b := range refs {
			if a == b || a.back <= b.back || a.setHash == b.setHash {
				continue
			}
			// By playing time, not by count: a copy padded with fake clips
			// of half a second each is still a copy.
			n := overlap(a.set, b.set)
			if ov := overlapTime(a, b); (ov >= 0.8*a.c.Distinct && ov >= 0.5*b.c.Distinct) || (n*5 >= len(a.set)*4 && n*2 >= len(b.set)) {
				a.c.note("shuffled", w.Shuffled, FlagShuffled, "plays playlist %05d's segments in a scrambled order", b.c.PlaylistNum)
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

func hasInt(list []int, n int) bool {
	for _, v := range list {
		if v == n {
			return true
		}
	}
	return false
}

// maxTitleChoice is how many playlists one title's program may lead to and
// still count as pointing at them. A program that could play any of
// hundreds, picked by a register, points at none.
const maxTitleChoice = 8

// programSignals applies what the disc's own program says: the static
// reading of its movie objects, and of its Java program's constants.
func (d *Disc) programSignals(r *Ranking, feats []*feat, byNum map[int]*feat, w Weights, opt RankOptions) {
	longest := 0.0
	for _, f := range feats {
		if f.eligible {
			longest = max(longest, f.c.Distinct)
		}
	}
	minDur := math.Min(minFeatureSec, longest*0.5)
	if minDur <= 0 {
		minDur = 1
	}
	r.Program.Kind = "none"
	if d.Index == nil {
		return
	}
	ix := d.Index
	fpBDJ := ix.FirstPlay.Kind == ObjectBDJ
	t1BDJ := len(ix.Titles) > 0 && ix.Titles[0].Object.Kind == ObjectBDJ
	r.BDJ = fpBDJ || t1BDJ
	switch {
	case r.BDJ:
		r.Program.Kind = "bdj"
	case len(d.MovieObjects) > 0:
		r.Program.Kind = "hdmv"
	}
	if !fpBDJ {
		reach := d.FirstPlayReach()
		r.Program.Unknown, r.Program.Incomplete = reach.Unknown, reach.Incomplete
		r.Program.Certain = reach.Certain
		fp, intros := reachTarget(reach, byNum, minDur)
		r.FirstPlay = fp
		if f := byNum[fp]; f != nil {
			how := "one of the ways through the program"
			if hasInt(reach.Certain, fp) {
				how = "whatever the player's state"
			}
			f.c.note("firstPlay", w.FirstPlay, FlagFirstPlay|FlagMenuTarget, "first play leads to it, %s (a static reading of the disc's HDMV program)", how)
			f.nav = max(f.nav, 3)
		}
		for _, n := range intros {
			byNum[n].c.note("intro", 0, FlagIntro, "first play passes through it on the way")
		}
		if reach.Unknown > 0 {
			r.Warnings = append(r.Warnings, fmt.Sprintf("First play names %s through registers whose value can't be read without running the disc; those were left out.", pluralWord(reach.Unknown, "target", "targets")))
		}
		if reach.Incomplete {
			r.Warnings = append(r.Warnings, "The disc's program is larger than the static reading follows; what it leads to past that point was left out.")
		}
	}
	if len(ix.Titles) > 0 && !t1BDJ {
		reach := d.TitleReach(1)
		if len(reach.Playlists) <= maxTitleChoice {
			t1, _ := reachTarget(reach, byNum, minDur)
			r.TitleOne = t1
			if f := byNum[t1]; f != nil {
				if f.c.Flags.Has(FlagFirstPlay) {
					f.c.note("titleOne", 0, FlagTitleOne, "title 1 leads to it as well")
				} else {
					f.c.note("titleOne", w.TitleOne, FlagTitleOne|FlagMenuTarget, "title 1 leads to it (a static reading of the disc's HDMV program)")
					f.nav = max(f.nav, 2)
				}
			}
		}
	}
	refs := map[int]int{}
	wide := 0
	for i, t := range ix.Titles {
		if t.Object.Kind != ObjectHDMV || i >= 1000 {
			continue
		}
		reach := d.StaticReach(t.Object, false)
		if len(reach.Playlists) > maxTitleChoice {
			wide++
			continue
		}
		for _, n := range reach.Playlists {
			refs[n]++
		}
	}
	if wide > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("The programs of %s could each play more than %d playlists, chosen by a register: that points at none of them, and they were not counted.", pluralWord(wide, "title", "titles"), maxTitleChoice))
	}
	nums := make([]int, 0, len(refs))
	for n := range refs {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	for _, n := range nums {
		if f := byNum[n]; f != nil && !f.c.Flags.Has(FlagFirstPlay) && !f.c.Flags.Has(FlagTitleOne) {
			f.c.note("menuReference", w.MenuReference, FlagMenuTarget, "the programs of %s play it", pluralWord(refs[n], "title", "titles"))
			f.nav = max(f.nav, 1)
		}
	}
	if !r.BDJ {
		return
	}
	listed := map[int]bool{}
	for _, o := range []IndexObject{ix.FirstPlay, firstTitle(ix)} {
		if o.Kind != ObjectBDJ || d.Source == nil {
			continue
		}
		if bo, err := d.BDJObject(o.BDJName); err == nil && len(bo.Playlists) > 0 && len(bo.Playlists) <= maxBDJListed {
			for _, p := range bo.Playlists {
				listed[p] = true
			}
		}
	}
	nums = nums[:0]
	for n := range listed {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	for _, n := range nums {
		if f := byNum[n]; f != nil {
			f.c.note("bdjListed", w.BDJListed, FlagMenuTarget, "the BD-J object of first play or title 1 lists it among the few playlists its Java program may play")
			f.nav = max(f.nav, 1)
		}
	}
	named := 0
	if !opt.NoBDJ {
		if h := d.BDJStaticHints(); h != nil {
			r.Program.BDJClasses = h.Classes
			// A number is only evidence when the program pushes few long
			// playlists' numbers: on a disc of decoys numbered 1 to 500,
			// every small constant "names" one.
			longNumbered := 0
			for _, x := range h.Hints {
				if f := byNum[x.Playlist]; f != nil && x.Numbers > 0 && x.Locators == 0 && x.Names == 0 && f.eligible && f.c.Distinct >= minDur {
					longNumbered++
				}
			}
			for _, x := range h.Hints {
				f := byNum[x.Playlist]
				if f == nil {
					continue
				}
				switch {
				case x.Locators > 0:
					f.c.note("bdjLocator", w.BDJLocator, FlagBDJTarget|FlagMenuTarget, "the disc's Java program has a locator or file name for it (read from its class files, not run)")
					f.nav = max(f.nav, 2)
					named++
				case x.Names > 0:
					f.c.note("bdjName", w.BDJName, FlagBDJTarget|FlagMenuTarget, "the disc's Java program has its number as a five-digit string (read from its class files, not run)")
					f.nav = max(f.nav, 2)
					named++
				case x.Numbers > 0 && x.Playlist >= 10 && longNumbered <= 3 && f.eligible && f.c.Distinct >= minDur:
					f.c.note("bdjNumber", w.BDJNumber, FlagBDJTarget, "the disc's Java program pushes its number as a constant, and few other long playlists' (read from its class files, not run)")
					f.nav = max(f.nav, 1)
					named++
				}
			}
			if h.Incomplete {
				r.Warnings = append(r.Warnings, "The disc's Java program is larger than the static reading goes through; some of it was not read.")
			}
		}
	}
	r.Program.BDJNamed = named
	if named > 0 {
		r.Warnings = append(r.Warnings, fmt.Sprintf("First play or title 1 is a BD-J (Java) program, which this engine doesn't run for ranking: its class files were read for the playlists they name (%d found), and the playlists' structure decided the rest.", named))
	} else {
		r.Warnings = append(r.Warnings, "First play or title 1 is a BD-J (Java) program, which this engine doesn't run: the playlists were ranked by their structure alone.")
	}
}

// streamSignals compares stream counts and sizes among the playlists long
// enough to matter.
func streamSignals(feats []*feat, w Weights) {
	longest := 0.0
	for _, f := range feats {
		if f.eligible {
			longest = max(longest, f.c.Distinct)
		}
	}
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
			c.note("audioStreams", w.AudioMatch, 0, "has the most audio streams of the long playlists (%d: %s)", maxA, strings.Join(c.Languages, ", "))
		}
		if maxS != minS && c.Subtitles == maxS {
			c.note("subtitleStreams", w.SubtitleMatch, 0, "has the most subtitle streams of the long playlists (%d)", maxS)
		}
		if big != small && big > 0 && c.Size == big {
			c.note("largestSize", w.LargestSize, 0, "its clips hold the most video (%s)", sizeText(big))
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
	// Going back inside a clip at two joins or more, and at a quarter of
	// them: the clip's parts are being played out of their own order.
	if f.rewinds >= 2 && f.rewinds*4 >= f.joins {
		return true
	}
	return false
}
