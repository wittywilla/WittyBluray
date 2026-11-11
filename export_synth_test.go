// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

// Synthetic discs for other projects' tests. A server or a player that uses
// this engine through its helper can't import the generator: it is test code
// of this package, and the engine is no dependency of theirs. So some of the
// scenarios are written out, on request, as discs on disk with what the
// right answer for each is:
//
//	WITTYBLURAY_SYNTH_OUT=<folder> go test -run '^TestExportSyntheticDiscs$' -count=1 .
//
// <folder>/expected.json lists them (SynthDisc). Every disc is opened again
// from where it was written and has to pass its scenario's own checks before
// it is listed, so a list that exists is a list of discs this engine gets
// right. Without the variable the test is skipped.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// EnvSynthOut names the folder the synthetic discs are written to.
const EnvSynthOut = "WITTYBLURAY_SYNTH_OUT"

// SynthDisc is one exported disc and its right answer.
type SynthDisc struct {
	Name string `json:"name"`
	// Path is the disc, relative to the list: a folder holding BDMV, or an
	// image file.
	Path  string `json:"path"`
	Image bool   `json:"image,omitempty"`
	Main  int    `json:"main"`
	// Film names the clips of the feature in the order they play.
	Film       []string `json:"film,omitempty"`
	FeatureSec float64  `json:"featureSec"`
	Chapters   int      `json:"chapters"`
	// Editions are the playlists of the film's cuts, longest first; Episodes
	// the playlists of a TV disc's episodes in viewing order.
	Editions []int `json:"editions,omitempty"`
	Episodes []int `json:"episodes,omitempty"`
	PlayAll  int   `json:"playAll"` // -1 when none
	// PlayAllStarts is where each part of the play-all playlist starts.
	PlayAllStarts []float64 `json:"playAllStartsSec,omitempty"`
	Decoys        int       `json:"decoys"`
	Playlists     int       `json:"playlists"`
}

// exported are the scenarios written out, by name: a plain disc, and the
// obfuscated ones a caller most needs to get right. A scenario whose evidence
// is where its clips lie on the disc is written as an image, since a folder
// has no such order; the rest as folders, and one both ways.
var exported = map[string]struct {
	slug          string
	folder, image bool
}{
	"a plain film with extras":                                                                          {"simple", true, false},
	"1000 permuted decoys, the clock runs through the film":                                             {"decoys1000", true, true},
	"seamless branching: theatrical and extended cuts sharing clips, each with permuted decoys":         {"editions", false, true},
	"a TV disc: episodes in playlists out of order, a play-all, and decoys of it":                       {"tv", true, false},
	"a TV disc with only a play-all playlist":                                                           {"playall", true, false},
	"500 permuted decoys, only the HDMV program tells (a register chosen in first play)":                {"program", true, false},
	"everything at once: two cuts, 1000 decoys of every kind, forced trailers, a register-chosen title": {"everything", false, true},
}

func TestExportSyntheticDiscs(t *testing.T) {
	out := os.Getenv(EnvSynthOut)
	if out == "" {
		t.Skipf("set %s to a folder to write the synthetic discs there", EnvSynthOut)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	var list []SynthDisc
	verify := func(sc scenario, path string, film []string) SynthDisc {
		t.Helper()
		d, err := Open(path)
		if err != nil {
			t.Fatalf("%s: %v", sc.name, err)
		}
		defer d.Close()
		r := d.Analyze(RankOptions{})
		sc.check(t, d, r, film)
		if t.Failed() {
			t.Fatalf("%s, written to %s, is not ranked as its scenario says:\n%s", sc.name, path, r.Explain())
		}
		e := SynthDisc{Name: sc.name, Main: r.Main, Film: film, PlayAll: r.PlayAll, Decoys: len(r.Decoys), Playlists: len(d.Playlists), Chapters: len(r.Chapters)}
		if pl := d.Playlist(r.Main); pl != nil {
			e.FeatureSec = d.PlaylistDuration(pl)
		}
		for _, ed := range r.Editions {
			e.Editions = append(e.Editions, ed.Playlist)
		}
		for _, ep := range r.Episodes {
			e.Episodes = append(e.Episodes, ep.Playlist)
		}
		for _, p := range r.PlayAllParts {
			e.PlayAllStarts = append(e.PlayAllStarts, p.Start)
		}
		return e
	}
	found := 0
	for _, sc := range scenarios() {
		how, ok := exported[sc.name]
		if !ok {
			continue
		}
		found++
		s, film := sc.build(t)
		s.shrink()
		if how.folder {
			dir := filepath.Join(out, how.slug)
			if err := os.RemoveAll(dir); err != nil {
				t.Fatal(err)
			}
			s.writeFolderAt(t, dir)
			entry := verify(sc, dir, film)
			entry.Path = how.slug
			list = append(list, entry)
		}
		if how.image {
			img := filepath.Join(out, how.slug+".iso")
			writeUDFImage(t, img, "SYNTHETIC", s.udfFiles(false), udfOptions{})
			entry := verify(sc, img, film)
			entry.Name, entry.Path, entry.Image = sc.name+", as an image", how.slug+".iso", true
			list = append(list, entry)
		}
	}
	if found < len(exported) {
		t.Fatalf("%d of %d scenarios were found by name", found, len(exported))
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "expected.json"), append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
