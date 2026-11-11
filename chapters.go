// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import "sort"

// Chapters of a playlist. A playlist's marks name a play item and a time in
// that item's clip; a viewer wants "chapter 7 starts 41 minutes in". The
// mapping is the sum of the play items before the mark's item plus how far
// into its own item the mark lies.

// Chapter is one chapter of a playlist, in playing order.
type Chapter struct {
	Number int `json:"number"` // 1-based
	// Mark is the index of the entry mark in Playlist.Marks, which is what
	// PLAY_PL_PM and LINK_MK name.
	Mark int    `json:"mark"`
	Item int    `json:"item"` // the play item the chapter starts in
	Clip string `json:"clip"`
	// ClipTime is the start in the clip's own time base (45 kHz), as the
	// mark gives it, moved inside the play item if the mark lay outside.
	ClipTime uint32 `json:"clipTime"`
	// Start and Duration are in seconds of the playlist's playing time.
	Start    float64 `json:"startSec"`
	Duration float64 `json:"durationSec"`
	// Adjusted: the mark's time lay outside its play item and was moved to
	// the item's start or end.
	Adjusted bool `json:"adjusted,omitempty"`
}

// Chapters maps the playlist's entry marks onto its playing time. Link
// points (mark type 2) are not chapters. A mark naming a play item that
// does not exist is left out; a mark whose time lies outside its item is
// moved to the item's nearer end; marks are put in playing order, and two
// at the same instant count once. The last chapter runs to the end of the
// playlist. nil when the playlist has no usable entry mark.
func (pl *Playlist) Chapters() []Chapter {
	if pl == nil || len(pl.Items) == 0 {
		return nil
	}
	starts := make([]float64, len(pl.Items)+1)
	for i := range pl.Items {
		starts[i+1] = starts[i] + pl.Items[i].Duration()
	}
	total := starts[len(pl.Items)]
	var out []Chapter
	for i, m := range pl.Marks {
		if m.Type != 1 || m.Item < 0 || m.Item >= len(pl.Items) {
			continue
		}
		it := &pl.Items[m.Item]
		c := Chapter{Mark: i, Item: m.Item, Clip: it.Clip, ClipTime: m.Time}
		switch {
		case m.Time < it.In:
			c.ClipTime, c.Adjusted = it.In, true
		case it.Out > it.In && m.Time > it.Out:
			c.ClipTime, c.Adjusted = it.Out, true
		}
		c.Start = starts[m.Item]
		if c.ClipTime > it.In {
			c.Start += ticks45(c.ClipTime - it.In)
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	kept := out[:0]
	for _, c := range out {
		// A chapter that would start at the very end plays nothing.
		if len(kept) > 0 && c.Start-kept[len(kept)-1].Start < 0.001 {
			continue
		}
		if total > 0 && c.Start >= total && len(kept) > 0 {
			continue
		}
		kept = append(kept, c)
	}
	for i := range kept {
		kept[i].Number = i + 1
		end := total
		if i+1 < len(kept) {
			end = kept[i+1].Start
		}
		if end > kept[i].Start {
			kept[i].Duration = end - kept[i].Start
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// chapterFaults counts entry marks that name no play item, and those whose
// time lies more than a second outside their item.
func (pl *Playlist) chapterFaults() (noItem, outside int) {
	for _, m := range pl.Marks {
		if m.Type != 1 {
			continue
		}
		if m.Item < 0 || m.Item >= len(pl.Items) {
			noItem++
			continue
		}
		it := &pl.Items[m.Item]
		if uint64(m.Time)+45000 < uint64(it.In) || (it.Out > it.In && uint64(m.Time) > uint64(it.Out)+45000) {
			outside++
		}
	}
	return noItem, outside
}
