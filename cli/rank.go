// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/libbluray"
)

// rank prints how the engine chose the feature: the ranking in words (or as
// JSON), the main playlist's chapters, where the copy's structures
// disagree, and what a libbluray installed on this machine says, when
// there is one and the disc may be shown to it.
func rank(d *wittybluray.Disc, asJSON bool) int {
	r, note := libbluray.Analyze(d, wittybluray.RankOptions{})
	if asJSON {
		out := struct {
			Ranking     *wittybluray.Ranking        `json:"ranking"`
			Consistency []wittybluray.Inconsistency `json:"consistency"`
			FromBackup  []string                    `json:"fromBackup,omitempty"`
			Notes       []string                    `json:"notes,omitempty"`
			Libbluray   libbluray.Cap               `json:"libbluray"`
			Note        string                      `json:"libblurayNote,omitempty"`
		}{r, d.Consistency(), d.FromBackup, d.Notes, libbluray.Capability(), note}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	fmt.Print(r.Explain())
	for _, c := range r.Chapters {
		fmt.Printf("chapter %2d  %s  (%s)  item %d, clip %s\n", c.Number, clock(c.Start), clock(c.Duration), c.Item, c.Clip)
	}
	for _, n := range d.FromBackup {
		fmt.Println("read from BACKUP:", n)
	}
	for _, n := range d.Notes {
		fmt.Println("note:", n)
	}
	for i, x := range d.Consistency() {
		if i >= 40 {
			fmt.Printf("... and %d more inconsistencies\n", len(d.Consistency())-i)
			break
		}
		fmt.Printf("%s: %s\n", x.Severity, x.What)
	}
	if so := r.SecondOpinion; so != nil {
		fmt.Printf("second opinion: %s picks %05d", so.Source, so.Main)
		if so.AgreesOnMain {
			fmt.Print(" (the same feature)")
		}
		fmt.Println()
		for _, x := range so.Disagreements {
			where := "disc"
			if x.Playlist >= 0 {
				where = fmt.Sprintf("playlist %05d", x.Playlist)
			}
			fmt.Printf("  %s %s: here %s, there %s %s\n", where, x.Field, x.Ours, x.Theirs, x.Note)
		}
	} else {
		fmt.Println("second opinion: none:", note)
	}
	return 0
}

func clock(sec float64) string {
	s := int(sec + 0.5)
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
}
