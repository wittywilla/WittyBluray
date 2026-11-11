// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package libbluray

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/wittywilla/wittybluray"
)

// The tests put a fake library behind the same seam the loader fills: Go
// functions that hand back title records laid out as libbluray's are, so
// the code that reads the library's memory is the code under test.

type fakeTitle struct {
	playlist int
	seconds  float64
	chapters []float64 // start times
	angles   int
}

type fakeLib struct {
	titles  []fakeTitle
	main    int // index into titles; -1 none
	failAt  string
	calls   []string
	opened  string
	keyfile uintptr
	env     map[string]string
	live    map[*cTitleInfo][]cChapter // records handed out and not yet freed
	freed   int
}

func (f *fakeLib) api() *api {
	f.env, f.live = map[string]string{}, map[*cTitleInfo][]cChapter{}
	note := func(s string) { f.calls = append(f.calls, s) }
	return &api{
		path: "fake",
		version: func(major, minor, micro *int32) {
			*major, *minor, *micro = 1, 3, 4
		},
		init: func() uintptr {
			note("init")
			if f.failAt == "init" {
				return 0
			}
			return 0xbd
		},
		openDisc: func(bd uintptr, device string, keyfile uintptr) int32 {
			note("open")
			f.opened, f.keyfile = device, keyfile
			if f.failAt == "open" {
				return 0
			}
			return 1
		},
		getTitles: func(bd uintptr, flags uint8, minLength uint32) uint32 {
			note("titles")
			if flags != 0 || minLength != 0 {
				note("FILTERED")
			}
			return uint32(len(f.titles))
		},
		mainTitle: func(bd uintptr) int32 { note("main"); return int32(f.main) },
		titleInfo: func(bd uintptr, index, angle uint32) *cTitleInfo {
			if f.failAt == "info" && index == 1 {
				return nil
			}
			t := f.titles[index]
			ti := &cTitleInfo{idx: index, playlist: uint32(t.playlist), duration: uint64(t.seconds * 90000), clipCount: 1, angleCount: uint8(t.angles), chapterCount: uint32(len(t.chapters))}
			var chs []cChapter
			for i, s := range t.chapters {
				chs = append(chs, cChapter{idx: uint32(i), start: uint64(s * 90000)})
			}
			if len(chs) > 0 {
				ti.chapters = &chs[0]
			}
			f.live[ti] = chs
			return ti
		},
		freeTitle: func(ti *cTitleInfo) { delete(f.live, ti); f.freed++ },
		closeDisc: func(bd uintptr) { note("close") },
		setenv:    func(name, value string) { f.env[name] = value },
	}
}

// folder is a Source that is nothing but a path: enough for a disc built
// in memory to be handed to a library by name.
type folder struct{ dir string }

func (s folder) ReadFile(string, int64) ([]byte, error)           { return nil, errors.New("no files") }
func (s folder) Extents(string) ([]wittybluray.FileExtent, error) { return nil, errors.New("no files") }
func (s folder) List(string) ([]string, error)                    { return nil, nil }
func (s folder) Describe() string                                 { return s.dir }
func (s folder) Close() error                                     { return nil }
func item(clip string, secs uint32, angles int) wittybluray.PlayItem {
	it := wittybluray.PlayItem{Clip: clip, In: 45000, Out: 45000 + secs*45000, Video: []wittybluray.Stream{{Kind: "video"}}}
	for i := 0; i < angles; i++ {
		it.Angles = append(it.Angles, wittybluray.AngleClip{Clip: clip})
	}
	return it
}

func testDisc(t *testing.T) *wittybluray.Disc {
	return &wittybluray.Disc{Source: folder{t.TempDir()}, Clips: map[string]*wittybluray.Clip{}, Playlists: map[int]*wittybluray.Playlist{
		1: {Number: 1, Items: []wittybluray.PlayItem{item("00001", 3000, 1), item("00002", 3000, 1)},
			Marks: []wittybluray.Mark{{Type: 1, Time: 45000}, {Type: 1, Item: 1, Time: 45000 + 600*45000}, {Type: 2}}},
		2: {Number: 2, Items: []wittybluray.PlayItem{item("00003", 60, 2)}},
		3: {Number: 3, Items: []wittybluray.PlayItem{item("00004", 10, 1)}},
	}}
}

func agreeing() *fakeLib {
	return &fakeLib{main: 0, titles: []fakeTitle{
		{playlist: 1, seconds: 6000, chapters: []float64{0, 3600}, angles: 1},
		{playlist: 2, seconds: 60.4, angles: 2},
		{playlist: 3, seconds: 10, angles: 1},
	}}
}

func use(t *testing.T, f *fakeLib) {
	t.Helper()
	override = f.api()
	t.Cleanup(func() { override = nil })
}

func TestReadThroughTheLibrarySeam(t *testing.T) {
	f := agreeing()
	use(t, f)
	d := testDisc(t)
	if c := Capability(); !c.Available || c.Version != "1.3.4" || c.Path != "fake" {
		t.Fatalf("capability: %+v", c)
	}
	info, err := Read(d)
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != "1.3.4" || info.Main != 1 || len(info.Titles) != 3 {
		t.Fatalf("%+v", info)
	}
	if tt := info.Titles[0]; tt.Playlist != 1 || tt.Duration != 6000 || tt.Chapters != 2 || len(tt.ChapterStarts) != 2 || tt.ChapterStarts[1] != 3600 || tt.Angles != 1 {
		t.Errorf("title: %+v", tt)
	}
	if got := strings.Join(f.calls, " "); got != "init open titles main close" {
		t.Errorf("calls: %s", got)
	}
	if f.opened != d.Source.Describe() || f.keyfile != 0 {
		t.Errorf("opened %q with key file %#x: a key file must never be passed", f.opened, f.keyfile)
	}
	if len(f.live) != 0 || f.freed != 3 {
		t.Errorf("%d title records not freed, %d freed", len(f.live), f.freed)
	}
	for _, k := range []string{"LIBAACS_PATH", "LIBBDPLUS_PATH"} {
		if f.env[k] != NoLibraryPath || os.Getenv(k) != NoLibraryPath {
			t.Errorf("%s = %q in the library's environment, %q in the program's", k, f.env[k], os.Getenv(k))
		}
	}
	if got := CrossCheck(d); len(got) != 0 {
		t.Errorf("an agreeing library gave %v", got)
	}
	r, note := Analyze(d, wittybluray.RankOptions{})
	if note != "" || r.SecondOpinion == nil || !r.SecondOpinion.AgreesOnMain || r.SecondOpinion.Source != "libbluray 1.3.4" || len(r.SecondOpinion.Disagreements) != 0 {
		t.Fatalf("note %q, second opinion %+v", note, r.SecondOpinion)
	}
	if alone := d.Analyze(wittybluray.RankOptions{}); r.Confidence <= alone.Confidence || r.Main != alone.Main {
		t.Errorf("agreement should raise confidence: %.2f with, %.2f without", r.Confidence, alone.Confidence)
	}
	if !strings.Contains(strings.Join(r.Why, " "), "libbluray 1.3.4 picks the same feature") {
		t.Errorf("why: %v", r.Why)
	}
}

func TestDisagreementsAreSurfaced(t *testing.T) {
	f := &fakeLib{main: 1, titles: []fakeTitle{
		{playlist: 1, seconds: 5900, chapters: []float64{0, 3000, 4000}, angles: 1},
		{playlist: 2, seconds: 60, angles: 1},
		{playlist: 9, seconds: 5},
	}}
	use(t, f)
	d := testDisc(t)
	var lines []string
	for _, x := range CrossCheck(d) {
		lines = append(lines, x.String())
	}
	all := strings.Join(lines, "\n")
	for _, want := range []string{
		"disc main: WittyBluray 00001, libbluray 00002",
		"playlist 00001 duration: WittyBluray 6000.0s, libbluray 5900.0s",
		"playlist 00001 chapters: WittyBluray 2, libbluray 3",
		"playlist 00002 angles: WittyBluray 2, libbluray 1",
		"playlist 00009 present: WittyBluray no, libbluray yes",
		"libbluray lists fewer playlists",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	r, _ := Analyze(d, wittybluray.RankOptions{})
	alone := d.Analyze(wittybluray.RankOptions{})
	if r.Main != 1 || r.Confidence >= alone.Confidence || r.SecondOpinion.AgreesOnMain {
		t.Errorf("main %d, confidence %.2f against %.2f alone", r.Main, r.Confidence, alone.Confidence)
	}
	if !strings.Contains(strings.Join(r.Warnings, " "), "libbluray 1.3.4 reads this disc differently") {
		t.Errorf("warnings: %v", r.Warnings)
	}
	// Chapter times that differ with the same count.
	f.titles[0] = fakeTitle{playlist: 1, seconds: 6000, chapters: []float64{0, 3000}, angles: 1}
	f.main = 0
	if got := CrossCheck(d); len(got) < 1 || got[0].Field != "chapterTimes" {
		t.Errorf("chapter times: %v", got)
	}
}

func TestEncryptedDiscsNeverReachTheLibrary(t *testing.T) {
	for name, mark := range map[string]func(d *wittybluray.Disc){
		"encrypted streams": func(d *wittybluray.Disc) { d.Encrypted = true },
		"an AACS folder":    func(d *wittybluray.Disc) { d.AACSFolder = true },
		"a BD+ folder":      func(d *wittybluray.Disc) { d.BDPlus = true },
	} {
		f := agreeing()
		use(t, f)
		d := testDisc(t)
		mark(d)
		if _, err := Read(d); !errors.Is(err, ErrEncrypted) {
			t.Errorf("%s: Read gave %v", name, err)
		}
		if got := CrossCheck(d); len(got) != 1 || got[0].Field != "refused" {
			t.Errorf("%s: CrossCheck gave %v", name, got)
		}
		if r, note := Analyze(d, wittybluray.RankOptions{}); r == nil || r.SecondOpinion != nil || note == "" {
			t.Errorf("%s: Analyze gave a second opinion", name)
		}
		if len(f.calls) != 0 {
			t.Errorf("%s: the library was called: %v", name, f.calls)
		}
	}
	if CrossCheck(nil) != nil || !errors.Is(Allowed(nil), ErrEncrypted) {
		t.Error("no disc")
	}
}

func TestLibraryFailuresAreAnAnswer(t *testing.T) {
	d := testDisc(t)
	for _, at := range []string{"init", "open"} {
		f := agreeing()
		f.failAt = at
		use(t, f)
		if _, err := Read(d); err == nil {
			t.Errorf("%s failing gave no error", at)
		}
		if got := CrossCheck(d); len(got) != 1 || got[0].Field != "error" {
			t.Errorf("%s: %v", at, got)
		}
		if r, note := Analyze(d, wittybluray.RankOptions{}); r.Main != 1 || note == "" || r.SecondOpinion != nil {
			t.Errorf("%s: ranking without the library: main %d, note %q", at, r.Main, note)
		}
	}
	// A title the library gives no record for is left out, the rest kept.
	f := agreeing()
	f.failAt = "info"
	use(t, f)
	info, err := Read(d)
	if err != nil || len(info.Titles) != 2 || len(f.live) != 0 {
		t.Errorf("%v %+v", err, info)
	}
	// A disc that is not a path libbluray could open.
	gone := testDisc(t)
	gone.Source = folder{"/nonexistent/disc"}
	if _, err := Read(gone); err == nil || len(f.calls) > 5 {
		t.Errorf("a path that is not there: %v", err)
	}
	// Absurd counts from the library are not followed.
	big := agreeing()
	use(t, big)
	a := override
	inner := a.titleInfo
	a.titleInfo = func(bd uintptr, index, angle uint32) *cTitleInfo {
		ti := inner(bd, index, angle)
		ti.chapterCount = 1 << 30
		return ti
	}
	if info, err := Read(d); err != nil || len(info.Titles[0].ChapterStarts) != 0 {
		t.Errorf("a billion chapters: %v", err)
	}
}

// Without a library the package says so and everything still works. On a
// machine that has libbluray installed this is skipped.
func TestNoLibraryIsGraceful(t *testing.T) {
	c := Capability()
	if c.Available {
		t.Skipf("libbluray %s is installed here (%s)", c.Version, c.Path)
	}
	if c.Reason == "" || Available() {
		t.Errorf("capability: %+v", c)
	}
	d := testDisc(t)
	if _, err := Read(d); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Read: %v", err)
	}
	if got := CrossCheck(d); len(got) != 1 || got[0].Field != "unavailable" || got[0].Note == "" {
		t.Errorf("CrossCheck: %v", got)
	}
	if r, note := Analyze(d, wittybluray.RankOptions{}); r.Main != 1 || note == "" {
		t.Errorf("Analyze: main %d, note %q", r.Main, note)
	}
}

func TestLoadingCanBeTurnedOffOrPointedElsewhere(t *testing.T) {
	reset := func() { loadOnce, loaded, loadWhy = sync.Once{}, nil, "" }
	t.Cleanup(reset)
	t.Setenv(EnvVar, "off")
	reset()
	if c := Capability(); c.Available || !strings.Contains(c.Reason, "turned off") {
		t.Errorf("off: %+v", c)
	}
	t.Setenv(EnvVar, "/nonexistent/libbluray.so.2")
	reset()
	if c := Capability(); c.Available || c.Reason == "" {
		t.Errorf("a file that is not there: %+v", c)
	}
	// A library that is not libbluray is turned down, not crashed on.
	if runtime.GOOS == "darwin" {
		t.Setenv(EnvVar, "/usr/lib/libSystem.B.dylib")
		reset()
		if c := Capability(); c.Available || !strings.Contains(c.Reason, "not a usable libbluray") {
			t.Errorf("another library: %+v", c)
		}
	}
}
