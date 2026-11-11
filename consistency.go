// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"fmt"
	"sort"
)

// Cross-checks between the three things that describe the same video: a
// playlist (which clip, from when to when), the clip information (how long
// the clip is, how many packets, where its entry points are) and the stream
// file itself (how many bytes are really there). A 1:1 copy agrees on all
// three. A copy that was cut short, a disc with decoy playlists and a
// damaged structure each disagree in their own way, and the ranking and
// the diagnoser both want to know which.

// Inconsistency is one disagreement between a disc's structures.
type Inconsistency struct {
	// Severity: "error" when something cannot play as written, "warn" when
	// it plays but not as the disc describes, "info" for oddities.
	Severity string `json:"severity"`
	// Kind names the check: clipMissing, streamMissing, streamEmpty,
	// streamShort, streamLong, streamUnaligned, timesOutside, emptyItem,
	// stcOrder, entryPastEnd, bitrate, markNoItem, markOutside,
	// angleMissing, subClipMissing.
	Kind     string `json:"kind"`
	Playlist int    `json:"playlist"` // -1 when it is about a clip alone
	Clip     string `json:"clip,omitempty"`
	What     string `json:"what"`
	// PlayableSec, for streamShort: how much of the clip the bytes that are
	// there can play, by the clip's entry-point map. 0 when it cannot be told.
	PlayableSec float64 `json:"playableSec,omitempty"`
}

const maxInconsistencies = 1000

// clipState is what the stream file says about a clip, against its
// information file.
type clipState int

const (
	clipUnknown clipState = iota // no size known
	clipWhole
	clipFileMissing
	clipFileEmpty
	clipFileShort // under 98% of the bytes its information counts
)

func clipFileState(c *Clip, size int64, known bool) clipState {
	switch {
	case !known:
		return clipUnknown
	case size < 0:
		return clipFileMissing
	case size == 0:
		return clipFileEmpty
	case c != nil && c.Packets > 0 && size < c.Size()/100*98:
		return clipFileShort
	}
	return clipWhole
}

// PlayableSeconds says how much of a clip a stream file of the given size
// can play: the time of the last entry point that lies inside the file,
// measured from the clip's start. ok is false when the clip has no
// entry-point map to tell by.
func (c *Clip) PlayableSeconds(size int64) (sec float64, ok bool) {
	if c == nil || len(c.EPMaps) == 0 || len(c.STC) == 0 {
		return 0, false
	}
	entries := c.EPMaps[0].Entries
	if pid := c.VideoPID(); pid != 0 {
		for i := range c.EPMaps {
			if c.EPMaps[i].PID == pid {
				entries = c.EPMaps[i].Entries
			}
		}
	}
	if len(entries) == 0 {
		return 0, false
	}
	last := sort.Search(len(entries), func(i int) bool { return int64(entries[i].SPN)*192 >= size })
	if last == 0 {
		return 0, true
	}
	// Time played before the time base the last entry lies in, plus the
	// stretch inside it.
	e := entries[last-1]
	for i := range c.STC {
		s := &c.STC[i]
		to := c.Packets
		if i+1 < len(c.STC) {
			to = c.STC[i+1].StartSPN
		}
		if e.SPN >= s.StartSPN && e.SPN < to || i == len(c.STC)-1 {
			if e.PTS > s.Start {
				sec += ticks45(min(e.PTS, s.End) - s.Start)
			}
			break
		}
		if s.End > s.Start {
			sec += ticks45(s.End - s.Start)
		}
	}
	return sec, true
}

// Consistency lists where the disc's playlists, clip information and stream
// files disagree, the most serious first. It reads structure and file sizes
// only, never stream content. Worked out once per opened disc; at most a
// thousand entries are kept.
func (d *Disc) Consistency() []Inconsistency {
	if d == nil {
		return nil
	}
	d.consOnce.Do(func() { d.cons = d.consistency(d.clipFileSizes()) })
	return d.cons
}

func (d *Disc) consistency(sizes map[string]int64) []Inconsistency {
	var out []Inconsistency
	dropped := 0
	add := func(sev, kind string, pl int, clip, format string, args ...any) *Inconsistency {
		if len(out) >= maxInconsistencies {
			dropped++
			return &Inconsistency{}
		}
		out = append(out, Inconsistency{Severity: sev, Kind: kind, Playlist: pl, Clip: clip, What: fmt.Sprintf(format, args...)})
		return &out[len(out)-1]
	}
	names := make([]string, 0, len(d.Clips))
	for n := range d.Clips {
		names = append(names, n)
	}
	sort.Strings(names)
	// Which clips anything plays: a stray clip nothing names matters less.
	used := map[string]bool{}
	for _, pl := range d.Playlists {
		for i := range pl.Items {
			for _, a := range pl.Items[i].Angles {
				used[a.Clip] = true
			}
		}
	}
	for _, n := range names {
		c := d.Clips[n]
		size, known := sizes[n]
		sev := "warn"
		if !used[n] {
			sev = "info"
		}
		switch clipFileState(c, size, known) {
		case clipFileMissing:
			add(sev, "streamMissing", -1, n, "clip %s has information but no stream file", n)
		case clipFileEmpty:
			add(sev, "streamEmpty", -1, n, "clip %s's stream file is empty", n)
		case clipFileShort:
			x := add(sev, "streamShort", -1, n, "clip %s's stream file has %s of the %s its information counts", n, sizeText(size), sizeText(c.Size()))
			if sec, ok := c.PlayableSeconds(size); ok {
				x.PlayableSec = sec
				x.What += fmt.Sprintf(": about %s of its %s can play", fmtDur(sec), fmtDur(c.Duration()))
			}
		case clipWhole:
			if c.Packets > 0 && size > c.Size()+192*32 {
				add("info", "streamLong", -1, n, "clip %s's stream file is %s, more than the %s its information counts", n, sizeText(size), sizeText(c.Size()))
			}
			if size%192 != 0 {
				add("info", "streamUnaligned", -1, n, "clip %s's stream file is not a whole number of 192-byte packets", n)
			}
		}
		for i := range c.STC {
			s := &c.STC[i]
			if s.End <= s.Start {
				add("warn", "stcOrder", -1, n, "clip %s's time base %d ends before it starts", n, s.ID)
			}
			if i > 0 && s.StartSPN < c.STC[i-1].StartSPN || c.Packets > 0 && s.StartSPN > c.Packets {
				add("warn", "stcOrder", -1, n, "clip %s's time base %d starts at a packet out of order or past the clip's end", n, s.ID)
			}
		}
		for i := range c.EPMaps {
			if e := c.EPMaps[i].Entries; len(e) > 0 && c.Packets > 0 && e[len(e)-1].SPN >= c.Packets {
				add("warn", "entryPastEnd", -1, n, "clip %s's entry-point map for PID 0x%04x runs past the clip's last packet", n, c.EPMaps[i].PID)
				break
			}
		}
		// A Blu-ray stream is at most 48 Mbit/s, an Ultra HD one about 128;
		// video under 50 kbit/s is no video.
		if dur := c.Duration(); dur >= 10 && c.Packets > 0 && c.ApplicationType <= 1 {
			if mbit := float64(c.Size()) * 8 / dur / 1e6; mbit > 160 || mbit < 0.05 {
				add("info", "bitrate", -1, n, "clip %s would average %.2f Mbit/s over its %s, which no disc's video does", n, mbit, fmtDur(dur))
			}
		}
	}
	for _, pl := range d.sortedPlaylists() {
		perList := 0
		note := func(sev, kind, clip, format string, args ...any) {
			if perList++; perList <= 4 { // a decoy names hundreds; four say it
				add(sev, kind, pl.Number, clip, format, args...)
			}
		}
		for i := range pl.Items {
			it := &pl.Items[i]
			c := d.Clips[it.Clip]
			switch {
			case c == nil && len(d.Clips) > 0:
				note("error", "clipMissing", it.Clip, "playlist %05d, item %d: clip %s has no information file", pl.Number, i, it.Clip)
			case it.Out <= it.In:
				note("warn", "emptyItem", it.Clip, "playlist %05d, item %d ends before it starts", pl.Number, i)
			case c != nil:
				end := c.EndPTS(it.STCID)
				st, ok := clipStart(c, it.STCID)
				if end > 0 && it.Out > end+45000 {
					note("error", "timesOutside", it.Clip, "playlist %05d, item %d plays clip %s to %s, past its end at %s", pl.Number, i, it.Clip, fmtDur(ticks45(it.Out)), fmtDur(ticks45(end)))
				} else if ok && uint64(it.In)+45000 < uint64(st) {
					note("warn", "timesOutside", it.Clip, "playlist %05d, item %d starts clip %s before its time base begins", pl.Number, i, it.Clip)
				}
			}
			for k := 1; k < len(it.Angles); k++ {
				if len(d.Clips) > 0 && d.Clips[it.Angles[k].Clip] == nil {
					note("warn", "angleMissing", it.Angles[k].Clip, "playlist %05d, item %d: angle %d's clip %s has no information file", pl.Number, i, k+1, it.Angles[k].Clip)
				}
			}
		}
		for _, sps := range [][]SubPath{pl.SubPaths, pl.ExtSubPaths} {
			for _, sp := range sps {
				for _, si := range sp.Items {
					if len(d.Clips) > 0 && si.Clip != "" && d.Clips[si.Clip] == nil {
						note("warn", "subClipMissing", si.Clip, "playlist %05d: a sub-path names clip %s, which has no information file", pl.Number, si.Clip)
					}
				}
			}
		}
		if noItem, outside := pl.chapterFaults(); noItem > 0 {
			note("warn", "markNoItem", "", "playlist %05d: %d chapter marks name play items it does not have", pl.Number, noItem)
		} else if outside > 0 {
			note("info", "markOutside", "", "playlist %05d: %d chapter marks lie outside their play item and were moved to its edge", pl.Number, outside)
		}
	}
	rank := map[string]int{"error": 0, "warn": 1, "info": 2}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	if dropped > 0 {
		out = append(out, Inconsistency{Severity: "info", Kind: "more", Playlist: -1, What: fmt.Sprintf("%d more inconsistencies were not listed", dropped)})
	}
	return out
}
