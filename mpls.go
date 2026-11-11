// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"errors"
	"fmt"
)

// BDMV/PLAYLIST/xxxxx.mpls: a playlist is what a title or a menu plays: a
// sequence of play items, each a stretch of one clip (an .m2ts stream file)
// with the streams the user may choose from; sub-paths for streams kept in
// other clips (menu graphics, secondary audio, 3D dependent views); and
// marks, which are the chapters.

// Playlist is one .mpls file.
type Playlist struct {
	Number       int
	Version      string
	PlaybackType int // 1 = sequential, 2 = random, 3 = shuffle
	// PlaybackCount is how many items a random or shuffle playlist plays.
	PlaybackCount int
	UOMask        uint64 // user operations the disc forbids (see UO constants)
	RandomAccess  bool   // the user may not jump around (chapter search, time search) when false
	// AudioMix: the disc mixes button sounds and secondary audio into the
	// primary audio. LosslessBypass: it allows lossless output unmixed.
	// RightEyeBase: on a 3D playlist the base (2D) view is the right eye.
	AudioMix, LosslessBypass, RightEyeBase bool
	Items                                  []PlayItem
	SubPaths                               []SubPath
	Marks                                  []Mark
	// ExtSubPaths are the sub-paths kept in the file's extension data, where
	// 3D discs put the dependent view and Ultra HD discs the Dolby Vision
	// layer. Stream entries never index them, so they are apart from SubPaths.
	ExtSubPaths []SubPath
	// Stereo3D: the playlist has a 3D stream table or a dependent-view
	// sub-path (in SubPaths or in the extension data).
	Stereo3D bool
	// Extensions lists the blocks of extension data the file carries.
	Extensions []Extension
	// Warnings names what was damaged in the file and done without: a stream
	// table cut short, chapter marks that could not be read. The play items
	// themselves were read, or parsing would have failed.
	Warnings []string
}

// Extension is one block of a playlist's extension data. ID1 2 with ID2 1 is
// the 3D stream table, ID1 2 with ID2 2 more sub-paths, ID1 1 with ID2 1
// picture-in-picture placement.
type Extension struct {
	ID1, ID2 int
	Length   int
}

// PlayItem is one stretch of a clip.
type PlayItem struct {
	Clip  string // the clip's five-digit name, e.g. "00005"
	Codec string // "M2TS"
	// ConnectionCondition says how this item joins the previous one: 1 =
	// seamless (same clip continues), 5 = seamless across clips, 6 = a break
	// (the player may pause briefly).
	ConnectionCondition int
	STCID               int
	In, Out             uint32 // 45 kHz ticks into the clip
	UOMask              uint64
	RandomAccess        bool
	// StillMode: 0 = none, 1 = a still of StillTime seconds after the item,
	// 2 = a still until the user acts.
	StillMode int
	StillTime int
	// Angles lists the clips of a multi-angle item; the first is the main
	// angle (the same as Clip).
	Angles           []AngleClip
	SeamlessAngles   bool
	DifferentAudios  bool
	Video, Audio, PG []Stream
	IG, SecAudio     []Stream
	SecVideo, PiPPG  []Stream
	DV               []Stream // Dolby Vision enhancement layers
}

// Duration is the item's length in seconds.
func (p *PlayItem) Duration() float64 {
	if p.Out <= p.In {
		return 0
	}
	return ticks45(p.Out - p.In)
}

// AngleClip is one angle's clip.
type AngleClip struct {
	Clip  string
	Codec string
	STCID int
}

// Stream is one selectable stream of a play item.
type Stream struct {
	Kind string // video, audio, pg (subtitles), ig (menu), sec-audio, sec-video, pip-pg
	// EntryType says where the stream is: 1 = in this item's clip, 2 = in a
	// sub-path's clip, 3 = in a sub-path (text subtitles), 4 = in a sub-path
	// clip with its own PID.
	EntryType int
	PID       uint16
	SubPath   int
	SubClip   int
	Coding    byte // MPEG-TS stream type: 0x1b H.264, 0x24 HEVC, 0xea VC-1, 0x02 MPEG-2, 0x20 MVC; 0x80 LPCM, 0x81 AC-3, 0x82 DTS, 0x83 TrueHD, 0x84 E-AC-3, 0x85 DTS-HD, 0x86 DTS-HD MA; 0x90 PG, 0x91 IG, 0x92 text
	Format    int  // video: 1 480i 2 576i 3 480p 4 1080i 5 720p 6 1080p 7 576p 8 2160p; audio: 1 mono 3 stereo 6 multi-channel 12 stereo+multi
	Rate      int  // video frame rate (see Index.FrameRate); audio: 1 48 kHz, 4 96 kHz, 5 192 kHz, 12 48/192, 14 48/96
	Lang      string
	CharCode  int // text subtitles: character coding
	// UHD (HEVC) video: DynamicRange 0 = SDR, 1 = HDR10, 2 = Dolby Vision; ColorSpace 1 = BT.709, 2 = BT.2020; HDR10Plus when the stream carries HDR10+ metadata.
	DynamicRange int
	ColorSpace   int
	HDR10Plus    bool
}

// CodingName is a readable name for the stream's coding.
func (s Stream) CodingName() string {
	switch s.Coding {
	case 0x01:
		return "MPEG-1 video"
	case 0x02:
		return "MPEG-2 video"
	case 0x1b:
		return "H.264"
	case 0x20:
		return "H.264 MVC (3D)"
	case 0x24:
		return "HEVC"
	case 0xea:
		return "VC-1"
	case 0x03, 0x04:
		return "MPEG audio"
	case 0x80:
		return "LPCM"
	case 0x81:
		return "AC-3"
	case 0x82:
		return "DTS"
	case 0x83:
		return "TrueHD"
	case 0x84:
		return "E-AC-3"
	case 0x85:
		return "DTS-HD"
	case 0x86:
		return "DTS-HD Master Audio"
	case 0xa1:
		return "E-AC-3 (secondary)"
	case 0xa2:
		return "DTS-HD (secondary)"
	case 0x90:
		return "subtitles (PG)"
	case 0x91:
		return "menu graphics (IG)"
	case 0x92:
		return "text subtitles"
	}
	return fmt.Sprintf("type 0x%02x", s.Coding)
}

// SubPath is a sequence of sub play items with streams that play alongside
// the main path: out-of-mux menu graphics, secondary audio or video,
// picture-in-picture, text subtitles, or a 3D dependent view.
type SubPath struct {
	// Type: 2 = primary audio of a browsable slideshow, 3 = IG menu, 4 = text
	// subtitles, 5 = out-of-mux synchronous (secondary audio/video), 6 =
	// out-of-mux asynchronous PiP, 7 = in-mux synchronous PiP, 8 = 3D
	// dependent view (MVC), 9 = 3D dependent view with PiP.
	Type   int
	Repeat bool
	Items  []SubPlayItem
}

// SubPlayItem is one stretch of a clip in a sub-path.
type SubPlayItem struct {
	Clip                string
	Codec               string
	ConnectionCondition int
	STCID               int
	In, Out             uint32
	SyncItem            int    // the main path's play item this one is synchronised with
	SyncStart           uint32 // and the time in it (45 kHz)
	MultiClips          []AngleClip
}

// Mark is a chapter (Type 1, "entry mark") or a link point (Type 2).
type Mark struct {
	Type     int
	Item     int    // index into Items
	Time     uint32 // 45 kHz ticks into the clip, in the item's time base
	PID      uint16 // the stream it applies to, 0xffff for all
	Duration uint32
}

// User operation mask bits (UOMask). A set bit means the operation is
// forbidden while the playlist or item plays.
const (
	UOMenuCall      = 1 << 63
	UOTitleSearch   = 1 << 62
	UOChapterSearch = 1 << 61
	UOTimeSearch    = 1 << 60
	UOSkipToNext    = 1 << 59
	UOSkipBack      = 1 << 58
	UOPlay          = 1 << 57
	UOStop          = 1 << 56
	UOPauseOn       = 1 << 55
	UOPauseOff      = 1 << 54
	UOStillOff      = 1 << 53
	UOForward       = 1 << 52
	UOBackward      = 1 << 51
	UOResume        = 1 << 50
	UOMoveUp        = 1 << 49
	UOMoveDown      = 1 << 48
	UOMoveLeft      = 1 << 47
	UOMoveRight     = 1 << 46
	UOSelect        = 1 << 45
	UOActivate      = 1 << 44
	UOSelectActive  = 1 << 43
	UOPrimaryAudio  = 1 << 42
	UOAngle         = 1 << 40
	UOPopupOn       = 1 << 39
	UOPopupOff      = 1 << 38
	UOPGEnable      = 1 << 37
	UOPGChange      = 1 << 36
	UOSecVideoOn    = 1 << 35
	UOSecVideo      = 1 << 34
	UOSecAudioOn    = 1 << 33
	UOSecAudio      = 1 << 32
	UOPiPPG         = 1 << 30
)

// SubClip finds the clip a stream outside the main path is in: its sub-path
// and the numbered clip of that sub-path's first item.
func (pl *Playlist) SubClip(s Stream) string {
	if s.SubPath >= len(pl.SubPaths) || len(pl.SubPaths[s.SubPath].Items) == 0 {
		return ""
	}
	it := &pl.SubPaths[s.SubPath].Items[0]
	if s.SubClip >= 1 && s.SubClip <= len(it.MultiClips) {
		return it.MultiClips[s.SubClip-1].Clip
	}
	return it.Clip
}

// What one playlist file may make the parser keep. The format allows 999
// play items; a file is given some slack, not the 16 MB it may be long.
const (
	maxPlayItems       = 1000
	maxMarks           = 10000
	maxStreamsPerItem  = 700 // the format's own limits add up to 640
	maxPlaylistStreams = 1 << 16
)

// stnCache shares the parsed stream tables of play items whose tables are
// byte for byte the same, within a playlist and across a disc. An
// obfuscated disc repeats one table tens of thousands of times; parsed
// once, a thousand decoy playlists cost little more than their play items.
// Shared tables are read-only.
type stnCache struct {
	m       map[string]*stnTable
	streams int // parsed streams held, for the disc's budget
}

type stnTable struct {
	video, audio, pg []Stream
	ig, secAudio     []Stream
	secVideo, pipPG  []Stream
	dv               []Stream
	count            int
	full             bool // cut short by the playlist's budget, so not for sharing
	warn             string
}

func newSTNCache() *stnCache { return &stnCache{m: map[string]*stnTable{}} }

// ParsePlaylist parses one .mpls file. The play items must be readable. A
// stream table that runs past its own length keeps the streams read so far,
// and unreadable chapter marks, sub-paths or extension data are left out;
// each is named in Warnings.
func ParsePlaylist(number int, b []byte) (*Playlist, error) {
	return parsePlaylist(number, b, nil)
}

func parsePlaylist(number int, b []byte, cache *stnCache) (*Playlist, error) {
	r := &rd{b: b}
	if r.str(0, 4) != "MPLS" {
		return nil, errors.New("not a playlist file")
	}
	if cache == nil {
		cache = newSTNCache()
	}
	pl := &Playlist{Number: number, Version: r.str(4, 4)}
	plStart, markStart, extStart := int(r.u32(8)), int(r.u32(12)), int(r.u32(16))
	// AppInfoPlayList
	pl.PlaybackType = int(r.u8(45))
	pl.PlaybackCount = int(r.u16(46))
	pl.UOMask = r.u64(48)
	af := r.u16(56)
	pl.RandomAccess = af&0x8000 == 0
	pl.AudioMix, pl.LosslessBypass, pl.RightEyeBase = af&0x4000 != 0, af&0x2000 != 0, af&0x1000 != 0
	// PlayList
	o := plStart
	nItems, nSub := int(r.u16(o+6)), int(r.u16(o+8))
	o += 10
	if nItems > maxPlayItems || nSub > 255 {
		return nil, fmt.Errorf("%d play items and %d sub-paths is more than a playlist can have", nItems, nSub)
	}
	streams := 0
	for i := 0; i < nItems && r.err == nil; i++ {
		l := int(r.u16(o))
		if !r.ok(o+2, 32) { // the fixed part of a play item
			break
		}
		// The item is read inside its own length when that is believable,
		// so a damaged stream table cannot read into the next item.
		ir := r.rest(o + 2)
		if l >= 32 && r.ok(o+2, l) {
			ir = r.at(o+2, l)
		}
		it, warn := parsePlayItem(ir, cache, maxPlaylistStreams-streams)
		streams += len(it.Video) + len(it.Audio) + len(it.PG) + len(it.IG) + len(it.SecAudio) + len(it.SecVideo) + len(it.PiPPG) + len(it.DV)
		if warn != "" && len(pl.Warnings) < 8 {
			pl.Warnings = append(pl.Warnings, fmt.Sprintf("play item %d: %s", i, warn))
		}
		pl.Items = append(pl.Items, it)
		o += 2 + l
	}
	if r.err != nil {
		return nil, r.err
	}
	sr := &rd{b: b}
	for i := 0; i < nSub && sr.err == nil; i++ {
		l := int(sr.u32(o))
		sp := parseSubPath(sr, o+4)
		if sr.err == nil {
			pl.SubPaths = append(pl.SubPaths, sp)
		}
		o += 4 + l
	}
	if sr.err != nil {
		pl.Warnings = append(pl.Warnings, "sub-paths: "+sr.err.Error())
	}
	// PlayListMark
	if markStart > 0 {
		mr := &rd{b: b}
		o = markStart
		nMarks := int(mr.u16(o + 4))
		o += 6
		if nMarks > maxMarks {
			pl.Warnings = append(pl.Warnings, fmt.Sprintf("%d marks is more than a playlist can have; the first %d were read", nMarks, maxMarks))
			nMarks = maxMarks
		}
		if o >= 0 && o <= len(b) {
			nMarks = min(nMarks, (len(b)-o)/14+1) // one past what fits, so a cut-off table is noticed
		}
		for i := 0; i < nMarks && mr.err == nil; i++ {
			m := Mark{Type: int(mr.u8(o + 1)), Item: int(mr.u16(o + 2)), Time: mr.u32(o + 4), PID: mr.u16(o + 8), Duration: mr.u32(o + 10)}
			if mr.err == nil {
				pl.Marks = append(pl.Marks, m)
			}
			o += 14
		}
		if mr.err != nil {
			pl.Warnings = append(pl.Warnings, "chapter marks: "+mr.err.Error())
		}
	}
	if extStart > 0 {
		parsePlaylistExtensions(pl, b, extStart)
	}
	for _, sp := range pl.SubPaths {
		if sp.Type == 8 || sp.Type == 9 {
			pl.Stereo3D = true
		}
	}
	return pl, nil
}

// parsePlaylistExtensions reads the extension data's table of contents and,
// from it, the extra sub-paths. Damage here costs only what it describes.
func parsePlaylistExtensions(pl *Playlist, b []byte, extStart int) {
	r := &rd{b: b}
	if r.u32(extStart) == 0 {
		return
	}
	n := int(r.u8(extStart + 11))
	o := extStart + 12
	for i := 0; i < n && i < 32 && r.err == nil; i++ {
		id1, id2 := int(r.u16(o)), int(r.u16(o+2))
		at, l := extStart+int(r.u32(o+4)), int(r.u32(o+8))
		o += 12
		if r.err != nil {
			break
		}
		pl.Extensions = append(pl.Extensions, Extension{ID1: id1, ID2: id2, Length: l})
		switch {
		case id1 == 2 && id2 == 1:
			pl.Stereo3D = true
		case id1 == 2 && id2 == 2:
			// length (4), count (2), then sub-paths as in the playlist.
			e := &rd{b: b}
			cnt := int(e.u16(at + 4))
			p := at + 6
			for k := 0; k < cnt && k < 255 && e.err == nil; k++ {
				sl := int(e.u32(p))
				sp := parseSubPath(e, p+4)
				if e.err == nil {
					pl.ExtSubPaths = append(pl.ExtSubPaths, sp)
					if sp.Type == 8 || sp.Type == 9 {
						pl.Stereo3D = true
					}
				}
				p += 4 + sl
			}
		}
	}
	if r.err != nil {
		pl.Warnings = append(pl.Warnings, "extension data: "+r.err.Error())
	}
}

// parsePlayItem reads a play item from a reader that starts at its clip
// name. budget is how many more streams the playlist may keep.
func parsePlayItem(r *rd, cache *stnCache, budget int) (PlayItem, string) {
	o := 0
	it := PlayItem{Clip: r.str(o, 5), Codec: r.str(o+5, 4)}
	flags := r.u16(o + 9)
	multi := flags&0x10 != 0
	it.ConnectionCondition = int(flags & 15)
	it.STCID = int(r.u8(o + 11))
	it.In, it.Out = r.u32(o+12), r.u32(o+16)
	it.UOMask = r.u64(o + 20)
	it.RandomAccess = r.u8(o+28)&0x80 == 0
	it.StillMode = int(r.u8(o + 29))
	it.StillTime = int(r.u16(o + 30))
	it.Angles = []AngleClip{{Clip: it.Clip, Codec: it.Codec, STCID: it.STCID}}
	p := o + 32
	// From here on a short or damaged item keeps what was read.
	t := &rd{b: r.b}
	if multi {
		n := int(t.u8(p))
		af := t.u8(p + 1)
		it.DifferentAudios = af&2 != 0
		it.SeamlessAngles = af&1 != 0
		p += 2
		for k := 1; k < n && t.err == nil; k++ {
			if k < 10 { // the format allows nine angles
				a := AngleClip{Clip: t.str(p, 5), Codec: t.str(p+5, 4), STCID: int(t.u8(p + 9))}
				if t.err == nil {
					it.Angles = append(it.Angles, a)
				}
			}
			p += 10
		}
	}
	if t.err != nil {
		return it, "its angle table is cut short"
	}
	// STN table: its own length says where it ends.
	stnLen := int(t.u16(p))
	if t.err != nil {
		return it, "it has no stream table"
	}
	// A table whose length is believable is read inside it and shared with
	// every other item that has the same bytes.
	var tab *stnTable
	if t.ok(p, 2+stnLen) && stnLen <= 1<<16 {
		raw := t.at(p, 2+stnLen)
		if tab = cache.m[string(raw.b)]; tab == nil || tab.count > budget {
			tab = parseSTN(raw, budget)
			if !tab.full {
				cache.m[string(raw.b)] = tab
				cache.streams += tab.count
			}
		}
	} else {
		tab = parseSTN(t.rest(p), budget)
		if tab.warn == "" {
			tab.warn = "its stream table's length is wrong"
		}
	}
	it.Video, it.Audio, it.PG, it.IG = tab.video, tab.audio, tab.pg, tab.ig
	it.SecAudio, it.SecVideo, it.PiPPG, it.DV = tab.secAudio, tab.secVideo, tab.pipPG, tab.dv
	return it, tab.warn
}

// parseSTN reads a stream table from a reader that starts at its length.
func parseSTN(r *rd, budget int) *stnTable {
	tab := &stnTable{}
	counts := [8]int{}
	for k := range counts {
		counts[k] = int(r.u8(4 + k))
	}
	if r.err != nil {
		tab.warn = "its stream table is cut short"
		return tab
	}
	budget = min(budget, maxStreamsPerItem)
	q := 16
	full := false
	read := func(kind string, n int, after func()) []Stream {
		var out []Stream
		for k := 0; k < n && r.err == nil; k++ {
			s, next := parseStream(r, q, kind)
			q = next
			if after != nil {
				after()
			}
			if r.err != nil {
				break
			}
			if tab.count >= budget {
				full = true
				continue
			}
			tab.count++
			out = append(out, s)
		}
		return out
	}
	refs := func() {
		n := int(r.u8(q))
		q += 2 + n + n&1
	}
	// The table's own order: picture-in-picture subtitles follow the
	// subtitles, and secondary streams carry lists of what they go with.
	tab.video = read("video", counts[0], nil)
	tab.audio = read("audio", counts[1], nil)
	tab.pg = read("pg", counts[2], nil)
	tab.pipPG = read("pip-pg", counts[6], nil)
	tab.ig = read("ig", counts[3], nil)
	tab.secAudio = read("sec-audio", counts[4], refs)
	tab.secVideo = read("sec-video", counts[5], func() { refs(); refs() })
	tab.dv = read("dv", counts[7], nil)
	switch {
	case r.err != nil:
		tab.warn = "its stream table is cut short; the streams before the damage were kept"
	case full:
		tab.full = true
		tab.warn = "it lists more streams than a play item can have; the rest were left out"
	}
	return tab
}

// parseStream reads a stream entry and its attributes; returns the stream
// and the offset after them.
func parseStream(r *rd, o int, kind string) (Stream, int) {
	s := Stream{Kind: kind}
	el := int(r.u8(o))
	s.EntryType = int(r.u8(o + 1))
	switch s.EntryType {
	case 1:
		s.PID = r.u16(o + 2)
	case 2, 4:
		s.SubPath, s.SubClip, s.PID = int(r.u8(o+2)), int(r.u8(o+3)), r.u16(o+4)
	case 3:
		s.SubPath, s.PID = int(r.u8(o+2)), r.u16(o+3)
	}
	o += 1 + el
	al := int(r.u8(o))
	s.Coding = r.u8(o + 1)
	a := o + 2
	switch s.Coding {
	case 0x01, 0x02, 0x1b, 0x20, 0x24, 0xea:
		s.Format, s.Rate = int(r.u8(a)>>4), int(r.u8(a)&15)
		if s.Coding == 0x24 && al >= 4 {
			s.DynamicRange, s.ColorSpace = int(r.u8(a+1)>>4), int(r.u8(a+1)&15)
			s.HDR10Plus = r.u8(a+2)&0x40 != 0
		}
	case 0x03, 0x04, 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0xa1, 0xa2:
		s.Format, s.Rate = int(r.u8(a)>>4), int(r.u8(a)&15)
		s.Lang = r.str(a+1, 3)
	case 0x90, 0x91:
		s.Lang = r.str(a, 3)
	case 0x92:
		s.CharCode = int(r.u8(a))
		s.Lang = r.str(a+1, 3)
	}
	return s, o + 1 + al
}

func parseSubPath(r *rd, o int) SubPath {
	sp := SubPath{Type: int(r.u8(o + 1)), Repeat: r.u16(o+2)&1 != 0}
	n := int(r.u8(o + 5))
	p := o + 6
	for i := 0; i < n && i < 255 && r.err == nil; i++ {
		l := int(r.u16(p))
		q := p + 2
		it := SubPlayItem{Clip: r.str(q, 5), Codec: r.str(q+5, 4)}
		w := r.u32(q + 9)
		it.ConnectionCondition = int(w>>1) & 15
		multi := w&1 != 0
		it.STCID = int(r.u8(q + 13))
		it.In, it.Out = r.u32(q+14), r.u32(q+18)
		it.SyncItem = int(r.u16(q + 22))
		it.SyncStart = r.u32(q + 24)
		if multi {
			m := int(r.u8(q + 28))
			e := q + 30
			for k := 1; k < m && k < 10 && r.err == nil; k++ {
				it.MultiClips = append(it.MultiClips, AngleClip{Clip: r.str(e, 5), Codec: r.str(e+5, 4), STCID: int(r.u8(e + 9))})
				e += 10
			}
		}
		sp.Items = append(sp.Items, it)
		p += 2 + l
	}
	return sp
}
