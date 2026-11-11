// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package libbluray asks a libbluray that the user has installed for a
// second opinion on a disc's navigation, and compares it with WittyBluray's
// own reading.
//
// WittyBluray is an original implementation and contains no libbluray code;
// none is bundled, linked or copied here either. When a libbluray shared
// library is found on the machine at run time it is loaded (dlopen or
// LoadLibrary, through purego: no cgo, no build-time dependency) and asked
// only for what it reads from the disc's unencrypted navigation files: the
// list of playlists with their lengths, chapters and angles, and which one
// it would play as the feature. Where it disagrees with WittyBluray, the
// disagreement is reported; WittyBluray's answer is never replaced by it.
//
// It is never required. Without the library, or on a platform the loader
// is not built for, Available reports false, Capability says why, and
// everything else works as before.
//
// Policy, which this package enforces rather than merely documents:
//
//   - Nothing is decrypted, and no decryption library is involved. A disc
//     is only handed to libbluray when WittyBluray has found its streams
//     clear AND it has no AACS and no BDSVM folder at all. libbluray loads
//     libaacs or libbdplus only for a disc that has those folders, so with
//     this rule it never has a reason to; a decrypted copy that kept its
//     AACS folder is refused here too, and is simply not cross-checked.
//     LIBAACS_PATH and LIBBDPLUS_PATH are also pointed at a path that does
//     not exist while libbluray is in use.
//   - No key file is ever passed, and no key database is read or shipped.
//   - bd_read and the other stream-reading calls are not bound at all: the
//     only functions this package can call are bd_get_version, bd_init,
//     bd_open_disc, bd_get_titles, bd_get_main_title, bd_get_title_info,
//     bd_free_title_info and bd_close.
//   - Loading can be turned off (WITTYBLURAY_LIBBLURAY=off) or pointed at
//     one file (WITTYBLURAY_LIBBLURAY=/path/to/libbluray.so.2).
package libbluray

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"sync"

	"github.com/wittywilla/wittybluray"
)

// Errors.
var (
	// ErrUnavailable: no libbluray could be loaded (none installed, loading
	// turned off, or a platform the loader is not built for).
	ErrUnavailable = errors.New("libbluray: not available")
	// ErrEncrypted: the disc is encrypted, or has an AACS or BD+ folder,
	// and is not handed to libbluray.
	ErrEncrypted = errors.New("libbluray: the disc is encrypted or has an AACS or BD+ folder; nothing is decrypted, so it is not handed to libbluray")
)

// EnvVar turns loading off ("off", "0", "no") or names the one library file
// to load.
const EnvVar = "WITTYBLURAY_LIBBLURAY"

// NoLibraryPath is where LIBAACS_PATH and LIBBDPLUS_PATH point while
// libbluray is in use: a path that does not exist.
const NoLibraryPath = "/nonexistent/wittybluray-loads-no-decryption-library"

// Limits on what is taken from the library's answers.
const (
	maxTitles   = 20000
	maxChapters = 10000
)

// Cap says whether a libbluray is there to ask.
type Cap struct {
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Path      string `json:"path,omitempty"`   // the library that was loaded
	Reason    string `json:"reason,omitempty"` // why not, when not available
}

// TitleInfo is one of libbluray's titles (a playlist).
type TitleInfo struct {
	Playlist int     `json:"playlist"`
	Duration float64 `json:"durationSec"`
	Clips    int     `json:"clips"` // play items
	Chapters int     `json:"chapters"`
	Marks    int     `json:"marks"`
	Angles   int     `json:"angles"`
	// ChapterStarts in seconds of the playlist's playing time.
	ChapterStarts []float64 `json:"chapterStarts,omitempty"`
}

// Info is what libbluray says about a disc.
type Info struct {
	Version string      `json:"version"`
	Titles  []TitleInfo `json:"titles"`
	// Main is the playlist libbluray would play as the feature (its
	// bd_get_main_title), or -1.
	Main int `json:"main"`
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

// api is the handful of libbluray functions this package may call. The
// loader fills it from the shared library; tests fill it with a fake.
type api struct {
	path         string
	version      func(major, minor, micro *int32)
	init         func() uintptr
	openDisc     func(bd uintptr, device string, keyfile uintptr) int32
	getTitles    func(bd uintptr, flags uint8, minLength uint32) uint32
	mainTitle    func(bd uintptr) int32
	titleInfo    func(bd uintptr, index, angle uint32) *cTitleInfo
	freeTitle    func(ti *cTitleInfo)
	closeDisc    func(bd uintptr)
	setenv       func(name, value string) // in the C library's environment, where libbluray looks
	versionCache string
}

// cTitleInfo and cChapter are the leading fields of libbluray's
// BLURAY_TITLE_INFO and BLURAY_TITLE_CHAPTER as its public header declares
// them, laid out as a 64-bit C compiler does (which is why the loader is
// only built for 64-bit targets). Only these fields are read.
type cTitleInfo struct {
	idx          uint32
	playlist     uint32
	duration     uint64 // 90 kHz
	clipCount    uint32
	angleCount   uint8
	_            [3]byte
	chapterCount uint32
	markCount    uint32
	clips        uintptr
	chapters     *cChapter
	marks        uintptr
}

type cChapter struct {
	idx      uint32
	_        uint32
	start    uint64 // 90 kHz, playlist time
	duration uint64
	offset   uint64
	clipRef  uint32
	_        uint32
}

var (
	loadOnce sync.Once
	loaded   *api
	loadWhy  string
	// mu serialises use of the library: libbluray reads the environment
	// when it opens a disc.
	mu sync.Mutex
	// override replaces the loaded library in tests.
	override *api
)

func current() (*api, string) {
	if override != nil {
		return override, ""
	}
	loadOnce.Do(func() {
		switch v := os.Getenv(EnvVar); v {
		case "off", "0", "no", "false":
			loadWhy = "turned off by " + EnvVar
		default:
			loaded, loadWhy = load(v)
		}
	})
	return loaded, loadWhy
}

// Available reports whether a libbluray was found and loaded.
func Available() bool {
	a, _ := current()
	return a != nil
}

// Capability says whether a libbluray is there to ask, which one, and if
// not, why not.
func Capability() Cap {
	a, why := current()
	if a == nil {
		return Cap{Reason: why}
	}
	return Cap{Available: true, Version: a.libVersion(), Path: a.path}
}

func (a *api) libVersion() string {
	mu.Lock()
	defer mu.Unlock()
	if a.versionCache == "" && a.version != nil {
		var major, minor, micro int32
		a.version(&major, &minor, &micro)
		a.versionCache = fmt.Sprintf("%d.%d.%d", major, minor, micro)
	}
	return a.versionCache
}

// Allowed says whether policy lets libbluray see the disc: its streams are
// clear and it has neither an AACS nor a BD+ folder.
func Allowed(d *wittybluray.Disc) error {
	if d == nil || d.Encrypted || d.AACSFolder || d.BDPlus {
		return ErrEncrypted
	}
	return nil
}

// Read asks libbluray for the disc's titles. The disc must be one that
// policy allows (see Allowed); its Source must be an image file or a folder
// libbluray can open by path.
func Read(d *wittybluray.Disc) (*Info, error) {
	if err := Allowed(d); err != nil {
		return nil, err
	}
	a, why := current()
	if a == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, why)
	}
	if d.Source == nil || d.Source.Describe() == "" {
		return nil, errors.New("libbluray: the disc has no path to open")
	}
	path := d.Source.Describe()
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("libbluray: the disc is not a file or folder it can open: %w", err)
	}
	version := a.libVersion()
	mu.Lock()
	defer mu.Unlock()
	return a.read(path, version)
}

// read opens the path, lists the titles and closes it again.
func (a *api) read(path, version string) (*Info, error) {
	for _, k := range []string{"LIBAACS_PATH", "LIBBDPLUS_PATH"} {
		_ = os.Setenv(k, NoLibraryPath)
		if a.setenv != nil {
			a.setenv(k, NoLibraryPath)
		}
	}
	bd := a.init()
	if bd == 0 {
		return nil, errors.New("libbluray: bd_init failed")
	}
	defer a.closeDisc(bd)
	// No key file: nothing is decrypted.
	if a.openDisc(bd, path, 0) == 0 {
		return nil, fmt.Errorf("libbluray: can't open %s", path)
	}
	info := &Info{Version: version, Main: -1}
	n := a.getTitles(bd, 0, 0) // every title, none filtered out
	main := a.mainTitle(bd)
	for i := uint32(0); i < n && i < maxTitles; i++ {
		ti := a.titleInfo(bd, i, 0)
		if ti == nil {
			continue
		}
		t := TitleInfo{Playlist: int(ti.playlist), Duration: float64(ti.duration) / 90000, Clips: int(ti.clipCount), Chapters: int(ti.chapterCount), Marks: int(ti.markCount), Angles: int(ti.angleCount)}
		if ti.chapters != nil && ti.chapterCount > 0 && ti.chapterCount <= maxChapters {
			for _, c := range chapterSlice(ti.chapters, int(ti.chapterCount)) {
				t.ChapterStarts = append(t.ChapterStarts, float64(c.start)/90000)
			}
		}
		if int64(i) == int64(main) {
			info.Main = t.Playlist
		}
		a.freeTitle(ti)
		if math.IsNaN(t.Duration) || t.Playlist < 0 || t.Playlist > 99999 {
			continue
		}
		info.Titles = append(info.Titles, t)
	}
	sort.SliceStable(info.Titles, func(i, j int) bool { return info.Titles[i].Playlist < info.Titles[j].Playlist })
	return info, nil
}

// SecondOpinion puts libbluray's answer in the form the ranking compares
// with (wittybluray.RankOptions.SecondOpinion).
func (info *Info) SecondOpinion() *wittybluray.SecondOpinion {
	if info == nil {
		return nil
	}
	so := &wittybluray.SecondOpinion{Source: "libbluray " + info.Version, Main: info.Main}
	for _, t := range info.Titles {
		so.Titles = append(so.Titles, wittybluray.SecondOpinionTitle{Playlist: t.Playlist, Duration: t.Duration, Chapters: t.Chapters, Angles: t.Angles, ChapterStarts: t.ChapterStarts})
	}
	return so
}

// Analyze ranks the disc's playlists with libbluray's second opinion when
// there is one to be had, and without it otherwise. The ranking's
// SecondOpinion field says what was compared; note says why no comparison
// was made, when none was.
func Analyze(d *wittybluray.Disc, opt wittybluray.RankOptions) (r *wittybluray.Ranking, note string) {
	info, err := Read(d)
	if err != nil {
		return d.Analyze(opt), err.Error()
	}
	opt.SecondOpinion = info.SecondOpinion()
	return d.Analyze(opt), ""
}

// CrossCheck reads the disc with libbluray and lists where it differs from
// WittyBluray. A disc policy does not allow is refused without libbluray
// opening it; no library, or a failure to read, comes back as a single
// disc-wide entry.
func CrossCheck(d *wittybluray.Disc) []Discrepancy {
	if d == nil {
		return nil
	}
	if err := Allowed(d); err != nil {
		return []Discrepancy{{Playlist: -1, Field: "refused", Pure: "encrypted or AACS/BD+ folder", Lib: "not opened", Note: err.Error()}}
	}
	if !Available() {
		return []Discrepancy{{Playlist: -1, Field: "unavailable", Pure: "-", Lib: "-", Note: Capability().Reason}}
	}
	info, err := Read(d)
	if err != nil {
		return []Discrepancy{{Playlist: -1, Field: "error", Pure: "-", Lib: "-", Note: err.Error()}}
	}
	return Compare(d, info)
}

// Compare lists where libbluray's reading differs from the disc's: the
// feature it would pick, playlists only one of them has, and per playlist
// the duration (more than a second apart), the chapter count and times,
// and the angle count.
func Compare(d *wittybluray.Disc, info *Info) []Discrepancy {
	var out []Discrepancy
	if d == nil || info == nil {
		return out
	}
	r := d.Analyze(wittybluray.RankOptions{SecondOpinion: info.SecondOpinion()})
	if r.SecondOpinion != nil {
		for _, x := range r.SecondOpinion.Disagreements {
			out = append(out, Discrepancy{Playlist: x.Playlist, Field: x.Field, Pure: x.Ours, Lib: x.Theirs, Note: x.Note})
		}
		if r.SecondOpinion.More > 0 {
			out = append(out, Discrepancy{Playlist: -1, Field: "more", Pure: "-", Lib: "-", Note: fmt.Sprintf("%d more differences were not listed", r.SecondOpinion.More)})
		}
	}
	// Playlists libbluray does not list at all, beyond the main one that
	// the ranking already reports: it drops what it finds unusable.
	lib := map[int]bool{}
	for _, t := range info.Titles {
		lib[t.Playlist] = true
	}
	var missing []int
	for n := range d.Playlists {
		if !lib[n] && n != r.Main {
			missing = append(missing, n)
		}
	}
	sort.Ints(missing)
	if len(info.Titles) > 0 && len(missing) > 0 {
		note := fmt.Sprintf("%d playlists", len(missing))
		if len(missing) <= 8 {
			note = fmt.Sprint(missing)
		}
		out = append(out, Discrepancy{Playlist: -1, Field: "present", Pure: "yes", Lib: "no", Note: "libbluray lists fewer playlists (it drops those it can't use): " + note})
	}
	return out
}
