// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build libbluray && cgo

package diagnose

import (
	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/libbluray"
)

// With the libbluray tag the check also compares libbluray's reading of the
// disc's navigation with WittyBluray's. libbluray never decrypts here (see
// the libbluray package), and an encrypted disc isn't given to it.
func init() { libblurayCrossCheck = crossCheck }

func crossCheck(j *Job, d *wittybluray.Disc) {
	got := libbluray.CrossCheck(d)
	if len(got) == 1 && got[0].Playlist < 0 && (got[0].Field == "refused" || got[0].Field == "unavailable" || got[0].Field == "error") {
		j.Add(Finding{Severity: SevInfo, Title: "libbluray didn't compare this disc", What: got[0].Note})
		return
	}
	if len(got) == 0 {
		j.Add(Finding{Severity: SevOK, Title: "libbluray reads the playlists the same way", What: "libbluray's list of playlists, their lengths, clips, chapters and angles match WittyBluray's."})
		return
	}
	var list []string
	for _, x := range got {
		list = append(list, x.String())
	}
	list, more := clip(list, len(list), 50)
	j.Add(Finding{Severity: SevInfo, Title: "libbluray reads some playlists differently", What: "These are where libbluray's reading of the disc's navigation differs from WittyBluray's. libbluray drops playlists it finds unusable, so some of these are expected on discs with decoys.", Details: list, More: more})
}
