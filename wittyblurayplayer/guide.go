// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyblurayplayer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittydisc/session"
)

// What a player can show about the engine's ranking of a disc, and the choices it can
// offer besides what the disc's own menus lead to. WittyDVD's player module answers the
// same two calls with the same shape, so a window draws one chooser for both.

// The calls this module answers on top of the session contract.
const (
	// MethodGuide returns a *Guide for the open disc. No parameters.
	MethodGuide = "guide"
	// MethodChoose plays one of the guide's choices: {"id": "..."}. It answers with a
	// presentation, like "title".
	MethodChoose = "choose"
)

// Limits on what a guide lists. A disc with a thousand playlists isn't a menu.
const (
	maxOthers   = 24
	maxEpisodes = 64
	maxVersions = 16
	maxNotes    = 8
	maxRepairs  = 12
)

// Guide is the engine's reading of a disc, in a form for a window.
type Guide struct {
	Kind string `json:"kind"` // bluray
	// Feature is what the engine picked as the film, nil when no playlist can be it.
	Feature *Choice `json:"feature,omitempty"`
	// Why is one sentence on why that pick, and Reasons the engine's own reasons for
	// the pick and for how sure it is, the weightiest first.
	Why     string   `json:"why,omitempty"`
	Reasons []string `json:"reasons,omitempty"`
	// Confidence is how sure the pick is, from 0 to 1, and Level the same as high,
	// medium or low.
	Confidence float64 `json:"confidence"`
	Level      string  `json:"level"`
	// Versions are the cuts of a film that has several (theatrical and extended),
	// the picked one included, longest first.
	Versions []Choice `json:"versions,omitempty"`
	// Episodes are a TV disc's episodes in viewing order, and PlayAll the playlist
	// that plays them one after another. A disc with only a play-all playlist has
	// its parts here instead, each starting inside that playlist.
	Episodes []Choice `json:"episodes,omitempty"`
	PlayAll  *Choice  `json:"playAll,omitempty"`
	// Others are the playlists a viewer might want instead, best first. Decoys are
	// left out; Decoys counts them, and Hidden everything that isn't listed.
	Others []Choice `json:"others,omitempty"`
	Decoys int      `json:"decoys,omitempty"`
	Hidden int      `json:"hidden,omitempty"`
	// Warnings are the engine's notes for people. Repairs says where the disc's
	// playlists, clip information and stream files disagree, the most serious
	// first, and RepairCount how many such places there are in all.
	Warnings    []string `json:"warnings,omitempty"`
	Repairs     []string `json:"repairs,omitempty"`
	RepairCount int      `json:"repairCount,omitempty"`
	// PaddedSectors and ReadRetries are part of the shape WittyDVD's module shares.
	// This engine reads structure only and pads nothing, so they stay 0.
	PaddedSectors int64 `json:"paddedSectors,omitempty"`
	ReadRetries   int64 `json:"readRetries,omitempty"`
}

// Choice is one thing a viewer can ask for by its ID.
type Choice struct {
	ID string `json:"id"`
	// Kind: feature, version, episode, playAll or title.
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	// Title is the playlist that plays it.
	Title    int     `json:"title"`
	Titles   []int   `json:"titles,omitempty"`
	Duration float64 `json:"durationSec"`
	Chapters int     `json:"chapters,omitempty"`
	// StartSec says where it begins inside Title: a part of a play-all playlist.
	StartSec float64 `json:"startSec,omitempty"`
	Chapter  int     `json:"chapter,omitempty"`
	// Angle and ParentalLevel belong to WittyDVD's versions; unused here.
	Angle         int `json:"angle,omitempty"`
	ParentalLevel int `json:"parentalLevel,omitempty"`
	// Main marks the version that is the feature as picked.
	Main      bool   `json:"main,omitempty"`
	Source    string `json:"source,omitempty"`
	OneStream bool   `json:"oneStream,omitempty"`

	item int // the play item a part starts at
}

func clock(sec float64) string {
	n := int(sec + 0.5)
	return fmt.Sprintf("%d:%02d:%02d", n/3600, n/60%60, n%60)
}

// classWords says a playlist's class in words for a menu.
var classWords = map[string]string{
	wittybluray.ClassPlayAll:    "plays all the episodes",
	wittybluray.ClassEpisode:    "episode",
	wittybluray.ClassDuplicate:  "the same as another playlist",
	wittybluray.ClassAlternate:  "another cut of the feature",
	wittybluray.ClassIntro:      "intro",
	wittybluray.ClassStub:       "a few seconds",
	wittybluray.ClassTrailer:    "trailer",
	wittybluray.ClassFeaturette: "featurette",
	wittybluray.ClassExtra:      "extra",
}

// rank is the engine's ranking of this disc, worked out once for the session. No second
// opinion from another reader is asked for.
func (s *bluraySession) rank() *wittybluray.Ranking {
	if s.ranking == nil {
		s.ranking = s.disc.Analyze(wittybluray.RankOptions{})
	}
	return s.ranking
}

// whyLine says in one sentence why a playlist was picked, from what the engine noted
// about it.
func whyLine(c *wittybluray.Candidate) string {
	var parts []string
	switch {
	case c.Flags.Has(wittybluray.FlagFirstPlay):
		parts = append(parts, "the disc starts it by itself")
	case c.Flags.Has(wittybluray.FlagTitleOne):
		parts = append(parts, "the disc's first title plays it")
	case c.Flags.Has(wittybluray.FlagMenuTarget):
		parts = append(parts, "one of the disc's titles plays it")
	case c.Flags.Has(wittybluray.FlagBDJTarget):
		parts = append(parts, "the disc's Java program names it")
	}
	parts = append(parts, fmt.Sprintf("it has %s of video of its own", clock(c.Distinct)))
	if c.ClusterSize > 1 {
		parts = append(parts, fmt.Sprintf("of %d playlists with the same clips it is the one in playing order", c.ClusterSize))
	}
	return fmt.Sprintf("Playlist %05d is the main feature because %s.", c.PlaylistNum, strings.Join(parts, ", "))
}

// Guide puts the ranking in a form for a window. It is built once.
func (s *bluraySession) Guide() *Guide {
	if s.guide == nil {
		s.guide = s.buildGuide()
	}
	g := *s.guide
	return &g
}

func (s *bluraySession) buildGuide() *Guide {
	r := s.rank()
	g := &Guide{Kind: "bluray", Confidence: r.Confidence, Level: r.ConfidenceLabel}
	if g.Level == "" {
		g.Level = "low"
	}
	if main := r.Candidate(r.Main); main != nil && r.Main >= 0 {
		g.Feature = &Choice{ID: "feature", Kind: "feature", Label: fmt.Sprintf("Playlist %05d", main.PlaylistNum), Detail: main.Video, Title: main.PlaylistNum, Duration: main.Duration, Chapters: len(r.Chapters)}
		g.Why = whyLine(main)
	}
	for _, w := range r.Why {
		if len(g.Reasons) < maxNotes {
			g.Reasons = append(g.Reasons, w)
		}
	}
	for _, e := range r.Editions {
		if len(g.Versions) >= maxVersions {
			break
		}
		label := e.Label
		if label != "" {
			label = strings.ToUpper(label[:1]) + label[1:]
		}
		c := Choice{ID: fmt.Sprintf("edition:%d", e.Playlist), Kind: "version", Label: label, Detail: fmt.Sprintf("playlist %05d", e.Playlist), Title: e.Playlist, Duration: e.Duration, Chapters: e.Chapters, Main: e.Main}
		if e.Own >= 1 {
			c.Detail += fmt.Sprintf(", %s only in this cut", clock(e.Own))
		}
		g.Versions = append(g.Versions, c)
	}
	for _, e := range r.Episodes {
		if len(g.Episodes) >= maxEpisodes {
			break
		}
		g.Episodes = append(g.Episodes, Choice{ID: fmt.Sprintf("episode:%d", e.Index), Kind: "episode", Label: fmt.Sprintf("Episode %d", e.Index), Detail: fmt.Sprintf("playlist %05d", e.Playlist), Title: e.Playlist, Duration: e.Duration, Chapters: e.Chapters})
	}
	// Parts of a play-all playlist with no playlists for its episodes are the
	// engine's guess at where the episodes begin, and say so.
	whole := r.PlayAll
	if whole < 0 {
		whole = r.Main
	}
	if len(r.Episodes) == 0 && whole >= 0 {
		for _, p := range r.PlayAllParts {
			if len(g.Episodes) >= maxEpisodes {
				break
			}
			g.Episodes = append(g.Episodes, Choice{ID: fmt.Sprintf("part:%d", p.Index), Kind: "episode", Label: fmt.Sprintf("Part %d", p.Index), Detail: fmt.Sprintf("in playlist %05d, from %s", whole, clock(p.Start)),
				Title: whole, Duration: p.Duration, StartSec: p.Start, item: p.FirstItem})
		}
	}
	if c := r.Candidate(r.PlayAll); c != nil && r.PlayAll >= 0 {
		g.PlayAll = &Choice{ID: "playall", Kind: "playAll", Label: "Play all", Detail: fmt.Sprintf("playlist %05d", c.PlaylistNum), Title: c.PlaylistNum, Duration: c.Duration, Chapters: c.Chapters}
	}
	g.Decoys = len(r.Decoys)
	for _, c := range r.Candidates {
		switch {
		case c.PlaylistNum == r.Main:
			continue
		case c.Class == wittybluray.ClassDecoy, len(g.Others) >= maxOthers:
			g.Hidden++
			continue
		}
		detail := classWords[c.Class]
		if c.Class == wittybluray.ClassMain {
			detail = ""
		}
		g.Others = append(g.Others, Choice{ID: fmt.Sprintf("title:%d", c.PlaylistNum), Kind: "title", Label: fmt.Sprintf("Playlist %05d", c.PlaylistNum), Detail: detail, Title: c.PlaylistNum, Duration: c.Duration, Chapters: c.Chapters})
	}
	for _, w := range r.Warnings {
		if len(g.Warnings) < maxNotes {
			g.Warnings = append(g.Warnings, w)
		}
	}
	// What can't play as written, and what plays otherwise than the disc says. The
	// oddities that change nothing are left to the engine's own diagnosis.
	for _, in := range s.disc.Consistency() {
		if in.Severity == "info" {
			continue
		}
		g.RepairCount++
		if len(g.Repairs) < maxRepairs {
			g.Repairs = append(g.Repairs, in.What)
		}
	}
	return g
}

// Choose plays one of the guide's choices.
func (s *bluraySession) Choose(id string) (*Presentation, error) {
	g := s.Guide()
	var c *Choice
	pick := func(list []Choice) {
		for i := range list {
			if list[i].ID == id {
				c = &list[i]
			}
		}
	}
	if g.Feature != nil && g.Feature.ID == id {
		c = g.Feature
	}
	if g.PlayAll != nil && g.PlayAll.ID == id {
		c = g.PlayAll
	}
	pick(g.Versions)
	pick(g.Episodes)
	pick(g.Others)
	if c == nil {
		return nil, &RefusalError{Code: session.CodeMissing, Message: "This disc has no such choice."}
	}
	pl := s.disc.Playlist(c.Title)
	if pl == nil {
		return nil, &RefusalError{Code: session.CodeMissing, Message: "This disc has no such choice."}
	}
	if c.StartSec > 0 && c.item >= 0 && c.item < len(pl.Items) {
		// A part of a play-all playlist begins with one of its play items.
		return s.play(pl, c.item, pl.Items[c.item].In), nil
	}
	return s.play(pl, 0, 0), nil
}

// ---------------------------------------------------------------------------
// The calls
// ---------------------------------------------------------------------------

// Host answers a player's calls about one disc at a time: the session contract's, and
// this module's own two. A player's built-in copy and the helper program both use it,
// so they answer alike.
type Host struct {
	base session.Host
	cur  *bluraySession
}

// NewHost makes a host that opens discs with Open.
func NewHost() *Host {
	h := &Host{}
	h.base.Open = func(path string, drive bool, prefs Prefs) (session.Session, error) {
		h.cur = nil
		s, err := Open(path, drive, prefs)
		if err != nil {
			return nil, err
		}
		h.cur, _ = s.(*bluraySession)
		return s, nil
	}
	return h
}

// Close ends the session in progress.
func (h *Host) Close() {
	h.cur = nil
	h.base.Close()
}

// Handle answers one call with a result to send as JSON, and an optional blob.
func (h *Host) Handle(method string, raw json.RawMessage) (any, []byte, error) {
	switch method {
	case "close":
		h.cur = nil
	case MethodGuide, MethodChoose:
		if h.cur == nil {
			return nil, nil, fmt.Errorf("no disc is open")
		}
		if method == MethodGuide {
			return h.cur.Guide(), nil, nil
		}
		var in struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		p, err := h.cur.Choose(in.ID)
		if err != nil {
			return nil, nil, err
		}
		return p, nil, nil
	}
	return h.base.Handle(method, raw)
}
