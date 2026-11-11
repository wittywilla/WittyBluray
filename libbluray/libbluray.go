// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package libbluray optionally cross-checks WittyBluray's pure-Go reading of
// a disc against libbluray's.
//
// It is only compiled for real with the build tag `libbluray` (and cgo, and
// libbluray found through pkg-config); every other build gets the stub in
// libbluray_stub.go, where Available reports false and Read returns
// ErrUnavailable. WittyBluray itself never needs it.
//
// Policy, which this package enforces rather than merely documents:
//
//   - Only unencrypted navigation and metadata are read: the title
//     (playlist) list with durations, clips, chapters and angles
//     (bd_get_titles, bd_get_title_info), the disc flags (bd_get_disc_info)
//     and the disc library metadata (bd_get_meta).
//   - bd_read is never called: no stream content is read through libbluray.
//   - libaacs and libbdplus are never loaded. Before libbluray opens a disc,
//     LIBAACS_PATH and LIBBDPLUS_PATH are pointed at a file that does not
//     exist, which is the only library the loader then tries; no key file is
//     passed. If libbluray still reports either library as loaded, the disc
//     is closed and ErrDecryptionLibraryLoaded returned.
//   - A disc that is encrypted is refused, as WittyBluray refuses it: Read
//     refuses when libbluray reports AACS or BD+ unless the caller has found
//     the streams clear itself (a decrypted copy usually keeps its AACS
//     folder, which is all libbluray's flag looks at), and CrossCheck
//     refuses whatever WittyBluray's own stream sampling calls encrypted
//     without calling libbluray at all.
package libbluray

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/wittywilla/wittybluray"
)

// Errors.
var (
	// ErrUnavailable: built without the libbluray tag, or without cgo.
	ErrUnavailable = errors.New("libbluray: not built in (build with -tags libbluray and cgo)")
	// ErrEncrypted: the disc is encrypted, and is refused.
	ErrEncrypted = errors.New("libbluray: the disc is encrypted; nothing is decrypted, so it is refused")
	// ErrDecryptionLibraryLoaded: libbluray loaded libaacs or libbdplus
	// despite the environment pointing it nowhere. Nothing more is read.
	ErrDecryptionLibraryLoaded = errors.New("libbluray: a decryption library was loaded against policy; the disc was closed")
)

// NoLibraryPath is where LIBAACS_PATH and LIBBDPLUS_PATH point while
// libbluray is in use: a path that does not exist.
const NoLibraryPath = "/nonexistent/wittybluray-loads-no-decryption-library"

// ReadOptions tune Read.
type ReadOptions struct {
	// KnownClear: the caller sampled the streams and found them clear (a
	// decrypted copy that keeps its AACS folder). Without it, any AACS or
	// BD+ that libbluray detects refuses the disc.
	KnownClear bool
}

// DiscFlags are libbluray's bd_get_disc_info flags.
type DiscFlags struct {
	BlurayDetected    bool `json:"blurayDetected"`
	AACSDetected      bool `json:"aacsDetected"`
	LibAACSDetected   bool `json:"libaacsDetected"`
	AACSHandled       bool `json:"aacsHandled"`
	BDPlusDetected    bool `json:"bdplusDetected"`
	LibBDPlusDetected bool `json:"libbdplusDetected"`
	BDPlusHandled     bool `json:"bdplusHandled"`
	BDJDetected       bool `json:"bdjDetected"`
	FirstPlay         bool `json:"firstPlaySupported"`
	TopMenu           bool `json:"topMenuSupported"`
	HDMVTitles        int  `json:"hdmvTitles"`
	BDJTitles         int  `json:"bdjTitles"`
}

// TitleInfo is one of libbluray's titles (a playlist).
type TitleInfo struct {
	Playlist int      `json:"playlist"`
	Duration float64  `json:"durationSec"`
	Clips    []string `json:"clips"` // one per play item
	Chapters int      `json:"chapters"`
	Marks    int      `json:"marks"`
	Angles   int      `json:"angles"`
}

// Info is what libbluray says about a disc.
type Info struct {
	Version  string      `json:"version"`
	Flags    DiscFlags   `json:"flags"`
	Name     string      `json:"name,omitempty"`     // the disc library's title (META/DL)
	Language string      `json:"language,omitempty"` // of that metadata
	Library  []string    `json:"libraryTitles,omitempty"`
	Titles   []TitleInfo `json:"titles"`
}

// Discrepancy is one difference between libbluray's reading and WittyBluray's.
type Discrepancy struct {
	Playlist int    `json:"playlist"` // -1 for the disc as a whole
	Field    string `json:"field"`
	Pure     string `json:"pure"`      // what WittyBluray says
	Lib      string `json:"libbluray"` // what libbluray says
	Note     string `json:"note,omitempty"`
}

func (x Discrepancy) String() string {
	where := "disc"
	if x.Playlist >= 0 {
		where = fmt.Sprintf("playlist %05d", x.Playlist)
	}
	s := fmt.Sprintf("%s %s: WittyBluray %s, libbluray %s", where, x.Field, x.Pure, x.Lib)
	if x.Note != "" {
		s += " (" + x.Note + ")"
	}
	return s
}

// CrossCheck reads the disc with libbluray and lists where it differs from
// WittyBluray. An encrypted disc (by WittyBluray's own stream sampling) is
// refused without libbluray opening it; a build without libbluray, or a
// failure to read, comes back as a single disc-wide entry.
func CrossCheck(d *wittybluray.Disc) []Discrepancy {
	if d == nil {
		return nil
	}
	if d.Encrypted {
		return []Discrepancy{{Playlist: -1, Field: "refused", Pure: "encrypted", Lib: "not opened", Note: ErrEncrypted.Error()}}
	}
	if !Available() {
		return []Discrepancy{{Playlist: -1, Field: "unavailable", Pure: "-", Lib: "-", Note: ErrUnavailable.Error()}}
	}
	path := ""
	if d.Source != nil {
		path = d.Source.Describe()
	}
	info, err := Read(path, ReadOptions{KnownClear: !d.Encrypted})
	if err != nil {
		return []Discrepancy{{Playlist: -1, Field: "error", Pure: "-", Lib: "-", Note: err.Error()}}
	}
	return Compare(d, info)
}

// Compare lists where libbluray's reading differs from the disc's: playlists
// only one of them has, and per playlist the duration (more than a second
// apart), the play items' clips, the chapter count and the angle count; and
// the BD-J flag.
func Compare(d *wittybluray.Disc, info *Info) []Discrepancy {
	var out []Discrepancy
	if d == nil || info == nil {
		return out
	}
	if info.Flags.BDJDetected != d.BDJ {
		out = append(out, Discrepancy{Playlist: -1, Field: "bdj", Pure: fmt.Sprint(d.BDJ), Lib: fmt.Sprint(info.Flags.BDJDetected)})
	}
	lib := map[int]*TitleInfo{}
	for i := range info.Titles {
		lib[info.Titles[i].Playlist] = &info.Titles[i]
	}
	nums := map[int]bool{}
	for n := range d.Playlists {
		nums[n] = true
	}
	for n := range lib {
		nums[n] = true
	}
	order := make([]int, 0, len(nums))
	for n := range nums {
		order = append(order, n)
	}
	sort.Ints(order)
	for _, n := range order {
		pl, t := d.Playlists[n], lib[n]
		switch {
		case pl == nil:
			out = append(out, Discrepancy{Playlist: n, Field: "present", Pure: "no", Lib: "yes", Note: "WittyBluray couldn't read this playlist"})
			continue
		case t == nil:
			out = append(out, Discrepancy{Playlist: n, Field: "present", Pure: "yes", Lib: "no", Note: "libbluray left it out (it drops playlists it can't use)"})
			continue
		}
		if dur := d.PlaylistDuration(pl); math.Abs(dur-t.Duration) > 1 {
			out = append(out, Discrepancy{Playlist: n, Field: "duration", Pure: fmt.Sprintf("%.1fs", dur), Lib: fmt.Sprintf("%.1fs", t.Duration)})
		}
		clips := make([]string, len(pl.Items))
		angles := 1
		for i, it := range pl.Items {
			clips[i] = it.Clip
			angles = max(angles, len(it.Angles))
		}
		if len(t.Clips) > 0 && strings.Join(clips, ",") != strings.Join(t.Clips, ",") {
			out = append(out, Discrepancy{Playlist: n, Field: "clips", Pure: summary(clips), Lib: summary(t.Clips)})
		}
		chapters := 0
		for _, m := range pl.Marks {
			if m.Type == 1 {
				chapters++
			}
		}
		if chapters != t.Chapters {
			out = append(out, Discrepancy{Playlist: n, Field: "chapters", Pure: fmt.Sprint(chapters), Lib: fmt.Sprint(t.Chapters)})
		}
		if t.Angles > 0 && angles != t.Angles {
			out = append(out, Discrepancy{Playlist: n, Field: "angles", Pure: fmt.Sprint(angles), Lib: fmt.Sprint(t.Angles)})
		}
	}
	return out
}

func summary(s []string) string {
	if len(s) <= 6 {
		return strings.Join(s, ",")
	}
	return fmt.Sprintf("%s,... (%d items)", strings.Join(s[:6], ","), len(s))
}
