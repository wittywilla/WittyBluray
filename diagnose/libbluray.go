// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package diagnose

import (
	"fmt"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/libbluray"
)

// When the user has libbluray installed, the check also compares its
// reading of the disc's navigation with WittyBluray's. libbluray never
// decrypts here (see the libbluray package): a disc that is encrypted, or
// that has an AACS or BD+ folder at all, isn't given to it.
func libblurayCrossCheck(j *Job, d *wittybluray.Disc) {
	got := libbluray.CrossCheck(d)
	if len(got) == 1 && got[0].Playlist < 0 && (got[0].Field == "refused" || got[0].Field == "unavailable" || got[0].Field == "error") {
		j.Add(Finding{Severity: SevInfo, Title: "libbluray didn't compare this disc", What: got[0].Note})
		return
	}
	version := libbluray.Capability().Version
	if len(got) == 0 {
		j.Add(Finding{Severity: SevOK, Title: "libbluray reads the playlists the same way", What: fmt.Sprintf("libbluray %s is installed on this machine. Its list of playlists, their lengths, chapters and angles, and the feature it would pick all match WittyBluray's.", version)})
		return
	}
	var list []string
	for _, x := range got {
		list = append(list, x.String())
	}
	list, more := clip(list, len(list), 50)
	j.Add(Finding{Severity: SevInfo, Title: "libbluray reads some of this disc differently", What: fmt.Sprintf("libbluray %s is installed on this machine, and these are where its reading of the disc's navigation differs from WittyBluray's. WittyBluray's answer is the one used. libbluray picks the feature by length and drops playlists it finds unusable, so differences are expected on discs with decoys.", version), Details: list, More: more})
}

// blurayConsistency lists where the playlists, the clip information and
// the stream files disagree.
func blurayConsistency(j *Job, d *wittybluray.Disc) {
	var errs, warns []string
	for _, x := range d.Consistency() {
		switch x.Severity {
		case "error":
			errs = append(errs, x.What)
		case "warn":
			warns = append(warns, x.What)
		}
	}
	var notes []string
	for _, n := range d.FromBackup {
		notes = append(notes, n+" was read from the disc's BACKUP folder")
	}
	notes = append(notes, d.Notes...)
	if len(notes) > 0 {
		list, more := clip(notes, len(notes), 30)
		j.Add(Finding{Severity: SevInfo, Title: "How this copy was read", What: "Remarks on reading the disc's structure that aren't damage by themselves.", Details: list, More: more})
	}
	if len(errs)+len(warns) == 0 {
		return
	}
	list, more := clip(append(errs, warns...), len(errs)+len(warns), 40)
	sev := SevInfo
	if main := d.MainPlaylist(); main != nil {
		for _, x := range d.Consistency() {
			if x.Playlist == main.Number && x.Severity != "info" {
				sev = SevWarn
			}
		}
	}
	j.Add(Finding{Severity: sev, Title: "The disc's structures disagree with each other", What: "Playlists, clip information and stream files describe the same video, and on this copy they don't agree everywhere. On a disc with decoy playlists most of these are the decoys; when they name the main feature, the copy is incomplete.", Details: list, More: more})
}
