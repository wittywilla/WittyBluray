// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyblurayplayer

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wittywilla/wittydisc/session"
)

// The engine's synthetic discs, written out by the engine's own test (see
// export_synth_test.go beside this folder), played through a session here: the film
// the engine picks is the film the window gets, with its cuts and episodes to choose.

// synthDisc is one exported disc and its right answer, as the engine lists it.
type synthDisc struct {
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	Image         bool      `json:"image"`
	Main          int       `json:"main"`
	FeatureSec    float64   `json:"featureSec"`
	Chapters      int       `json:"chapters"`
	Editions      []int     `json:"editions"`
	Episodes      []int     `json:"episodes"`
	PlayAll       int       `json:"playAll"`
	PlayAllStarts []float64 `json:"playAllStartsSec"`
	Decoys        int       `json:"decoys"`
	Playlists     int       `json:"playlists"`
}

var synth struct {
	once  sync.Once
	dir   string
	list  []synthDisc
	skip  string
	fatal string
}

// TestMain removes the exported discs when the tests are done.
func TestMain(m *testing.M) {
	code := m.Run()
	if synth.dir != "" {
		_ = os.RemoveAll(synth.dir)
	}
	os.Exit(code)
}

// synthDiscs has the engine write its scenario discs out, once for the whole test
// binary. It skips when this folder isn't inside a checkout of the engine.
func synthDiscs(t *testing.T) (string, []synthDisc) {
	t.Helper()
	synth.once.Do(func() {
		engine, err := filepath.Abs("..")
		if err != nil {
			synth.fatal = err.Error()
			return
		}
		if _, err := os.Stat(filepath.Join(engine, "export_synth_test.go")); err != nil {
			synth.skip = "the engine's checkout isn't around this folder (no ../export_synth_test.go), so there are no synthetic discs to play"
			return
		}
		gobin, err := exec.LookPath("go")
		if err != nil {
			synth.skip = "the go command isn't on the PATH, so the engine can't write its synthetic discs out"
			return
		}
		dir, err := os.MkdirTemp("", "wittybluray-synth-")
		if err != nil {
			synth.fatal = err.Error()
			return
		}
		synth.dir = dir
		cmd := exec.Command(gobin, "test", "-run", "^TestExportSyntheticDiscs$", "-count=1", ".")
		cmd.Dir = engine
		cmd.Env = append(os.Environ(), "WITTYBLURAY_SYNTH_OUT="+dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			synth.fatal = "the engine couldn't write its synthetic discs: " + err.Error() + "\n" + string(out)
			return
		}
		b, err := os.ReadFile(filepath.Join(dir, "expected.json"))
		if err == nil {
			err = json.Unmarshal(b, &synth.list)
		}
		if err != nil {
			synth.fatal = "expected.json: " + err.Error()
		}
	})
	if synth.skip != "" {
		t.Skip(synth.skip)
	}
	if synth.fatal != "" {
		t.Fatal(synth.fatal)
	}
	return synth.dir, synth.list
}

// openSynth opens one of the discs by its path in the list.
func openSynth(t *testing.T, name string) (*bluraySession, synthDisc) {
	t.Helper()
	dir, list := synthDiscs(t)
	for _, d := range list {
		if d.Path != name {
			continue
		}
		s, err := Open(filepath.Join(dir, filepath.FromSlash(d.Path)), false, Prefs{})
		if err != nil {
			t.Fatalf("%s: %v", d.Name, err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s.(*bluraySession), d
	}
	t.Fatalf("the engine exported no disc called %s", name)
	return nil, synthDisc{}
}

// near allows for a stream starting at the entry point before a play item's first
// picture: a fraction of a second per item.
func near(a, b float64) bool { return math.Abs(a-b) < 2 }

// clipsOf names the clips a presentation's stream reads, in order.
func clipsOf(p *Presentation) []string {
	var out []string
	for _, e := range p.Media.Extents {
		name := filepath.Base(p.Media.Files[e[0]])
		name = name[:len(name)-len(filepath.Ext(name))]
		if len(out) == 0 || out[len(out)-1] != name {
			out = append(out, name)
		}
	}
	return out
}

// Every exported disc: the guide names the engine's film with its length, cuts and
// episodes, and choosing the feature plays that playlist's clips in the film's order.
func TestGuideAndFeatureOfEverySyntheticDisc(t *testing.T) {
	_, list := synthDiscs(t)
	for _, d := range list {
		t.Run(d.Path, func(t *testing.T) {
			s, _ := openSynth(t, d.Path)
			if in := s.Info(); in.Main != d.Main {
				t.Errorf("info: main %d, want %d", in.Main, d.Main)
			}
			g := s.Guide()
			if g.Feature == nil || g.Feature.Title != d.Main || !near(g.Feature.Duration, d.FeatureSec) || g.Feature.Chapters != d.Chapters {
				t.Fatalf("feature %+v, want playlist %d of %v s with %d chapters", g.Feature, d.Main, d.FeatureSec, d.Chapters)
			}
			if g.Why == "" || len(g.Reasons) == 0 || g.Level == "" || g.Confidence <= 0 || g.Confidence > 1 {
				t.Errorf("why %q, reasons %v, level %q, confidence %v", g.Why, g.Reasons, g.Level, g.Confidence)
			}
			if g.Decoys != d.Decoys || len(g.Others) > maxOthers || len(g.Others)+g.Hidden+1 != d.Playlists {
				t.Errorf("%d decoys, %d others and %d hidden of %d playlists with %d decoys", g.Decoys, len(g.Others), g.Hidden, d.Playlists, d.Decoys)
			}
			if len(g.Versions) != len(d.Editions) {
				t.Fatalf("versions %+v, want %v", g.Versions, d.Editions)
			}
			for i, v := range g.Versions {
				if v.Title != d.Editions[i] || v.Main != (v.Title == d.Main) {
					t.Errorf("version %d is %+v, want playlist %d", i+1, v, d.Editions[i])
				}
			}
			want := len(d.Episodes)
			if want == 0 {
				want = len(d.PlayAllStarts)
			}
			if len(g.Episodes) != want {
				t.Fatalf("%d episodes, want %d", len(g.Episodes), want)
			}
			if (g.PlayAll == nil) != (d.PlayAll < 0) || (g.PlayAll != nil && g.PlayAll.Title != d.PlayAll) {
				t.Errorf("play all %+v, want playlist %d", g.PlayAll, d.PlayAll)
			}

			p, err := s.Choose("feature")
			if err != nil {
				t.Fatal(err)
			}
			if p.Kind != "title" || p.Title == nil || p.Title.TTN != d.Main || p.Media == nil {
				t.Fatalf("the feature presents as %+v", p)
			}
			if !near(p.Title.Duration, d.FeatureSec) || len(p.Title.Chapters) != d.Chapters {
				t.Errorf("the feature plays %v s with %d chapters, want %v s with %d", p.Title.Duration, len(p.Title.Chapters), d.FeatureSec, d.Chapters)
			}
			for i, c := range p.Title.Chapters {
				if c < 0 || c >= p.Title.Duration || (i > 0 && c <= p.Title.Chapters[i-1]) {
					t.Errorf("chapter %d starts at %v in a title of %v s", i+1, c, p.Title.Duration)
				}
			}
			// The stream is the picked playlist's play items, in its order. From a
			// folder the files say which clips those are.
			pl := s.disc.Playlist(d.Main)
			if len(p.Media.Segments) != len(pl.Items) {
				t.Errorf("%d segments for %d play items", len(p.Media.Segments), len(pl.Items))
			}
			if !d.Image {
				var want []string
				for _, it := range pl.Items {
					want = append(want, it.Clip)
				}
				if got := clipsOf(p); !equal(got, want) {
					t.Errorf("the stream reads clips %v, the playlist names %v", got, want)
				}
			}
			// A chapter by number starts where the list says it does.
			if n := len(p.Title.Chapters); n > 2 {
				q := s.PlayTitle(d.Main, n)
				if q.Kind != "title" || !near(q.Title.StartSec, p.Title.Chapters[n-1]) || q.Title.Chapter != n {
					t.Errorf("chapter %d starts at %v (chapter %d), want %v", n, q.Title.StartSec, q.Title.Chapter, p.Title.Chapters[n-1])
				}
			}
			if _, err := s.Choose("title:99999"); err == nil {
				t.Error("a choice the disc doesn't have was played")
			}
		})
	}
}

func equal(a, b []string) bool {
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

// A film with two cuts: each is a version, and each plays as its own playlist.
func TestEditionsPlay(t *testing.T) {
	s, d := openSynth(t, "editions.iso")
	g := s.Guide()
	for _, v := range g.Versions {
		p, err := s.Choose(v.ID)
		if err != nil || p.Kind != "title" || p.Title.TTN != v.Title || !near(p.Title.Duration, v.Duration) {
			t.Fatalf("%s: %+v %v", v.ID, p, err)
		}
	}
	longest, shortest := g.Versions[0], g.Versions[len(g.Versions)-1]
	if longest.Duration <= shortest.Duration || shortest.Title != d.Main {
		t.Errorf("versions %+v", g.Versions)
	}
}

// A TV disc: each episode plays its own playlist, in viewing order, and play all the
// playlist that holds them.
func TestEpisodesPlay(t *testing.T) {
	s, d := openSynth(t, "tv")
	g := s.Guide()
	for i, e := range g.Episodes {
		p, err := s.Choose(e.ID)
		if err != nil || p.Kind != "title" || p.Title.TTN != d.Episodes[i] || p.Title.StartSec != 0 {
			t.Fatalf("episode %d: %+v %v, want playlist %d", i+1, p, err, d.Episodes[i])
		}
	}
	if p, err := s.Choose("playall"); err != nil || p.Title.TTN != d.PlayAll {
		t.Errorf("play all: %+v %v", p, err)
	}
}

// A disc with only a play-all playlist: its parts start inside it, where the engine
// says the episodes begin.
func TestPartsOfAPlayAllPlaylist(t *testing.T) {
	s, d := openSynth(t, "playall")
	g := s.Guide()
	for i, e := range g.Episodes {
		p, err := s.Choose(e.ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.Kind != "title" || p.Title.TTN != d.Main || !near(p.Title.StartSec, d.PlayAllStarts[i]) || !near(p.Title.Duration, d.FeatureSec) {
			t.Fatalf("part %d presents as playlist %d from %v s, want playlist %d from %v s", i+1, p.Title.TTN, p.Title.StartSec, d.Main, d.PlayAllStarts[i])
		}
		if i > 0 && p.Media.StartSec != p.Title.StartSec {
			t.Errorf("part %d: the stream starts at %v s", i+1, p.Media.StartSec)
		}
	}
}

// The calls a window makes: the guide and a choice through the host, as the helper
// program and a player's built-in copy both answer them.
func TestHostAnswersGuideAndChoose(t *testing.T) {
	dir, list := synthDiscs(t)
	h := NewHost()
	defer h.Close()
	if _, _, err := h.Handle(MethodGuide, nil); err == nil {
		t.Error("a guide with no disc open")
	}
	params, _ := json.Marshal(map[string]any{"path": filepath.Join(dir, list[0].Path)})
	if _, _, err := h.Handle("open", params); err != nil {
		t.Fatal(err)
	}
	res, _, err := h.Handle(MethodGuide, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Kind    string `json:"kind"`
		Feature struct {
			ID    string `json:"id"`
			Title int    `json:"title"`
		} `json:"feature"`
		Why string `json:"why"`
	}
	if err := json.Unmarshal(b, &g); err != nil || g.Kind != "bluray" || g.Feature.ID != "feature" || g.Feature.Title != list[0].Main || g.Why == "" {
		t.Fatalf("%s %v", b, err)
	}
	res, _, err = h.Handle(MethodChoose, json.RawMessage(`{"id":"feature"}`))
	if p, ok := res.(*Presentation); err != nil || !ok || p.Kind != "title" {
		t.Fatalf("%+v %v", res, err)
	}
	_, _, err = h.Handle(MethodChoose, json.RawMessage(`{"id":"episode:40"}`))
	var r *session.RefusalError
	if !errors.As(err, &r) || r.Code != session.CodeMissing {
		t.Errorf("a choice that isn't there: %v", err)
	}
	if _, _, err := h.Handle("close", nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.Handle(MethodChoose, json.RawMessage(`{"id":"feature"}`)); err == nil {
		t.Error("a choice after the disc was closed")
	}
}
