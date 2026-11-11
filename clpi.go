// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"errors"
	"fmt"
	"sort"
)

// BDMV/CLIPINF/xxxxx.clpi describes one stream file (an .m2ts clip): its
// programs and their streams, its time bases, and the entry-point map that
// turns a time into a byte position, which is how a player starts a play
// item at its IN time or seeks to a chapter.

// Clip is one .clpi file.
type Clip struct {
	Name    string // five digits
	Version string
	// StreamType: 1 = a normal transport stream. ApplicationType: 1 = main
	// movie, 2 = time-based slideshow, 3 = browsable slideshow pictures, 4 =
	// browsable slideshow audio, 5 = IG menu, 6 = text subtitles, 7 = other
	// sub-path streams (secondary audio or video).
	StreamType, ApplicationType int
	TSRate                      int    // bytes per second of the transport stream
	Packets                     uint32 // source packets (192 bytes each)
	STC                         []STCSequence
	Programs                    []Program
	EPMaps                      []EPMap
	// ATC lists the arrival-time sequences: stretches of the file recorded
	// in one go. A clip mastered for a disc has one; a second one means the
	// packet clock restarts mid-file.
	ATC []ATCSequence
	// Warnings names the parts of the file that were damaged and done
	// without (the program table, the entry-point map). The clip's length
	// and time bases were still read.
	Warnings []string
}

// ATCSequence is one arrival-time sequence and the time bases inside it.
type ATCSequence struct {
	StartSPN uint32
	FirstSTC int // the ID of its first time base
	STCs     int // how many time bases it holds
}

// STCSequence is a stretch of the clip with one continuous clock.
type STCSequence struct {
	ID         int // the number play items name it by
	ATC        int // index into Clip.ATC
	PCRPID     uint16
	StartSPN   uint32
	Start, End uint32 // presentation times, 45 kHz
}

// Program is one program of the transport stream and its streams.
type Program struct {
	StartSPN uint32
	PMTPID   uint16
	Streams  []ClipStream
}

// ClipStream is one elementary stream as the clip describes it.
type ClipStream struct {
	PID    uint16
	Coding byte
	Format int
	Rate   int
	Aspect int // video: 2 = 4:3, 3 = 16:9
	Lang   string
	// HEVC video: DynamicRange 0 = SDR, 1 = HDR10, 2 = Dolby Vision; ColorSpace 1 = BT.709, 2 = BT.2020.
	DynamicRange, ColorSpace int
}

// EPMap maps presentation times to source packet numbers for one stream.
type EPMap struct {
	PID     uint16
	Entries []EPEntry // in time order
}

// EPEntry is one entry point: a packet at which decoding can start.
type EPEntry struct {
	PTS uint32 // 45 kHz
	SPN uint32 // source packet number; byte offset = SPN * 192
	// AngleChange marks the packet as a point where a seamless angle change
	// may happen.
	AngleChange bool
}

// Size is the clip's length in bytes.
func (c *Clip) Size() int64 { return int64(c.Packets) * 192 }

// Stream finds a stream by PID.
func (c *Clip) Stream(pid uint16) *ClipStream {
	for i := range c.Programs {
		for j := range c.Programs[i].Streams {
			if c.Programs[i].Streams[j].PID == pid {
				return &c.Programs[i].Streams[j]
			}
		}
	}
	return nil
}

// VideoPID is the clip's main video stream, or 0.
func (c *Clip) VideoPID() uint16 {
	for _, p := range c.Programs {
		for _, s := range p.Streams {
			switch s.Coding {
			case 0x01, 0x02, 0x1b, 0x24, 0xea:
				return s.PID
			}
		}
	}
	return 0
}

// Duration is the clip's length in seconds from its time bases.
func (c *Clip) Duration() float64 {
	var d float64
	for _, s := range c.STC {
		if s.End > s.Start {
			d += ticks45(s.End - s.Start)
		}
	}
	return d
}

// SPNAt returns the entry point at or before the time (45 kHz) in the
// stream, so a player can start reading there. ok is false when the clip has
// no entry-point map for that stream.
func (c *Clip) SPNAt(pid uint16, pts uint32) (e EPEntry, ok bool) {
	return c.EntryAt(pid, -1, pts)
}

// EntryAt is SPNAt within one time base. A clip with several (the clock
// restarts mid-file, as when studios join separately encoded reels) has the
// same times more than once, and its entries are not in time order overall,
// so a play item's time only means something inside its own time base. stc
// -1 searches the whole map. A stream with no map of its own uses the clip's
// first one (audio-only clips map their audio).
func (c *Clip) EntryAt(pid uint16, stc int, pts uint32) (e EPEntry, ok bool) {
	entries := c.epEntries(pid, stc)
	if len(entries) == 0 {
		return EPEntry{}, false
	}
	k := sort.Search(len(entries), func(j int) bool { return entries[j].PTS > pts })
	if k == 0 {
		return entries[0], true
	}
	return entries[k-1], true
}

// epEntries is a stream's entry points within one time base.
func (c *Clip) epEntries(pid uint16, stc int) []EPEntry {
	var m *EPMap
	for i := range c.EPMaps {
		if c.EPMaps[i].PID == pid && len(c.EPMaps[i].Entries) > 0 {
			m = &c.EPMaps[i]
			break
		}
	}
	if m == nil {
		for i := range c.EPMaps {
			if len(c.EPMaps[i].Entries) > 0 {
				m = &c.EPMaps[i]
				break
			}
		}
	}
	if m == nil {
		return nil
	}
	entries := m.Entries
	if from, to, found := c.STCRange(stc); found {
		lo := sort.Search(len(entries), func(j int) bool { return entries[j].SPN >= from })
		hi := sort.Search(len(entries), func(j int) bool { return entries[j].SPN >= to })
		if lo < hi {
			entries = entries[lo:hi]
		}
	}
	return entries
}

// EntryAfter is the first entry point after a time on a video stream, within
// that time base; ok is false when there is none.
func (c *Clip) EntryAfter(pid uint16, stc int, pts uint32) (e EPEntry, ok bool) {
	entries := c.epEntries(pid, stc)
	k := sort.Search(len(entries), func(j int) bool { return entries[j].PTS > pts })
	if k >= len(entries) {
		return EPEntry{}, false
	}
	return entries[k], true
}

// STCRange is the source packets of a time base; found is false when the
// clip has only one, or no such number.
func (c *Clip) STCRange(stc int) (from, to uint32, found bool) {
	if stc < 0 || len(c.STC) < 2 {
		return 0, 0, false
	}
	for i := range c.STC {
		if c.STC[i].ID == stc {
			to = c.Packets
			if i+1 < len(c.STC) {
				to = c.STC[i+1].StartSPN
			}
			return c.STC[i].StartSPN, to, true
		}
	}
	return 0, 0, false
}

// EndPTS is where a time base ends (45 kHz), or 0 when the clip does not say.
func (c *Clip) EndPTS(stc int) uint32 {
	for i := range c.STC {
		if c.STC[i].ID == stc {
			return c.STC[i].End
		}
	}
	if n := len(c.STC); n > 0 {
		return c.STC[n-1].End
	}
	return 0
}

// Limits on what one clip information file may make the parser keep. A
// three-hour film has some twenty thousand entry points.
const (
	maxEPEntriesPerClip = 1 << 19
	maxSTCPerClip       = 1024
)

// ParseClip parses one .clpi file. The header, the clip information and the
// time bases must be readable; a damaged program table or entry-point map
// is left out and named in Warnings, because the clip's length and time
// bases are enough to describe and rank what plays it.
func ParseClip(name string, b []byte) (*Clip, error) {
	r := &rd{b: b}
	if r.str(0, 4) != "HDMV" {
		return nil, errors.New("not a clip information file")
	}
	c := &Clip{Name: name, Version: r.str(4, 4)}
	seqStart, progStart, cpiStart := int(r.u32(8)), int(r.u32(12)), int(r.u32(16))
	// ClipInfo
	c.StreamType = int(r.u8(46))
	c.ApplicationType = int(r.u8(47))
	c.TSRate = int(r.u32(52) & 0x7fffffff)
	c.Packets = r.u32(56)
	// SequenceInfo: ATC sequences, each with STC sequences.
	o := seqStart + 5
	nATC := int(r.u8(o))
	o++
	for i := 0; i < nATC && i < 32 && r.err == nil; i++ {
		atcStart := r.u32(o)
		nSTC, first := int(r.u8(o+4)), int(r.u8(o+5))
		o += 6
		c.ATC = append(c.ATC, ATCSequence{StartSPN: atcStart, FirstSTC: first, STCs: nSTC})
		for j := 0; j < nSTC && r.err == nil; j++ {
			if len(c.STC) < maxSTCPerClip {
				c.STC = append(c.STC, STCSequence{ID: first + j, ATC: i, PCRPID: r.u16(o), StartSPN: r.u32(o + 2), Start: r.u32(o + 6), End: r.u32(o + 10)})
			}
			o += 14
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	if err := parseClipPrograms(c, b, progStart); err != nil {
		c.Programs = nil
		c.Warnings = append(c.Warnings, "program table: "+err.Error())
	}
	if cpiStart > 0 {
		if err := parseClipCPI(c, b, cpiStart); err != nil {
			c.Warnings = append(c.Warnings, "entry-point map: "+err.Error())
		}
	}
	return c, nil
}

func parseClipPrograms(c *Clip, b []byte, progStart int) error {
	r := &rd{b: b}
	o := progStart + 5
	nProg := int(r.u8(o))
	o++
	for i := 0; i < nProg && i < 255 && r.err == nil; i++ {
		p := Program{StartSPN: r.u32(o), PMTPID: r.u16(o + 4)}
		ns := int(r.u8(o + 6))
		o += 8
		for j := 0; j < ns && j < 255 && r.err == nil; j++ {
			s := ClipStream{PID: r.u16(o)}
			l := int(r.u8(o + 2))
			s.Coding = r.u8(o + 3)
			a := o + 4
			switch s.Coding {
			case 0x01, 0x02, 0x1b, 0x20, 0x24, 0xea:
				s.Format, s.Rate = int(r.u8(a)>>4), int(r.u8(a)&15)
				s.Aspect = int(r.u8(a+1) >> 4)
				if s.Coding == 0x24 && l >= 5 {
					s.DynamicRange, s.ColorSpace = int(r.u8(a+2)>>4), int(r.u8(a+2)&15)
				}
			case 0x03, 0x04, 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0xa1, 0xa2:
				s.Format, s.Rate = int(r.u8(a)>>4), int(r.u8(a)&15)
				s.Lang = r.str(a+1, 3)
			case 0x90, 0x91:
				s.Lang = r.str(a, 3)
			case 0x92:
				s.Lang = r.str(a+1, 3)
			}
			if r.err == nil {
				p.Streams = append(p.Streams, s)
			}
			o += 3 + l
		}
		if r.err == nil {
			c.Programs = append(c.Programs, p)
		}
	}
	return r.err
}

// parseClipCPI reads the entry-point maps. A map that is damaged part of
// the way keeps the entries read so far when they are in order, and is
// dropped otherwise; the maps before it stay.
func parseClipCPI(c *Clip, b []byte, cpiStart int) error {
	r := &rd{b: b}
	if r.u32(cpiStart) == 0 || r.u16(cpiStart+4)&15 != 1 {
		return r.err // no map, or not an EP_map
	}
	base := cpiStart + 6 // EP_map starts after the length and type
	n := int(r.u8(base + 1))
	o := base + 2
	total := 0
	for i := 0; i < n && i < 32 && r.err == nil; i++ {
		m := EPMap{PID: r.u16(o)}
		// 48 bits: 10 reserved, 4 stream type, 16 coarse entries, 18 fine entries.
		v := uint64(r.u16(o+2))<<32 | uint64(r.u32(o+4))
		nCoarse := int((v >> 18) & 0xffff)
		nFine := int(v & 0x3ffff)
		start := base + int(r.u32(o+8))
		o += 12
		if r.err != nil {
			break
		}
		e := &rd{b: b}
		fineStart := start + int(e.u32(start))
		coarse := start + 4
		// What the file is long enough to hold, whatever it claims.
		if coarse >= 0 && coarse <= len(b) {
			nCoarse = min(nCoarse, (len(b)-coarse)/8)
		} else {
			nCoarse = 0
		}
		if fineStart >= 0 && fineStart <= len(b) {
			nFine = min(nFine, (len(b)-fineStart)/4)
		} else {
			nFine = 0
		}
		if room := maxEPEntriesPerClip - total; nFine > room {
			nFine = room
			c.Warnings = append(c.Warnings, fmt.Sprintf("entry-point map: more than %d entries; the rest were left out", maxEPEntriesPerClip))
		}
		m.Entries = make([]EPEntry, 0, nFine)
		next := 0 // the fine entry the next coarse entry must start at or after
		for k := 0; k < nCoarse && e.err == nil; k++ {
			w := e.u32(coarse + k*8)
			first, ptsCoarse, spnCoarse := int(w>>14), uint64(w&0x3fff), e.u32(coarse+k*8+4)
			end := nFine
			if k+1 < nCoarse {
				end = min(nFine, int(e.u32(coarse+(k+1)*8)>>14))
			}
			// Coarse entries name their fine entries in rising order. One
			// that goes back would read the same entries again (and a file
			// of them could ask for billions), so it is skipped.
			if first < next {
				continue
			}
			hi, prev := spnCoarse&^0x1ffff, uint32(0)
			for f := first; f < end && e.err == nil; f++ {
				w := e.u32(fineStart + f*4)
				if low := w & 0x1ffff; low < prev {
					hi += 0x20000 // packet numbers only rise: the low 17 bits wrapped inside this group
				}
				prev = w & 0x1ffff
				// The 33-bit, 90 kHz time is split so that bit 19 is in both
				// halves: the coarse entry holds bits 32..19 as they were at
				// its first fine entry, each fine entry bits 19..9. The fine
				// entry's bit 19 is the one that counts. Likewise the packet
				// number: the coarse entry has all 32 bits, the fine one the
				// low 17.
				pts90 := (ptsCoarse&^1)<<19 + uint64((w>>17)&0x7ff)<<9
				spn := hi + w&0x1ffff
				m.Entries = append(m.Entries, EPEntry{PTS: uint32(pts90 / 2), SPN: spn, AngleChange: w>>31 != 0})
			}
			next = max(next, end)
		}
		total += len(m.Entries)
		if e.err != nil && len(m.Entries) == 0 {
			c.Warnings = append(c.Warnings, fmt.Sprintf("entry-point map for PID 0x%04x: %v", m.PID, e.err))
			continue
		}
		c.EPMaps = append(c.EPMaps, m)
	}
	return r.err
}
