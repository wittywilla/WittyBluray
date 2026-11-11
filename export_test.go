// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"path/filepath"
	"testing"
)

// What tests outside the package (those that also import packages which
// import this one) may use of the synthetic generator.

// SynthFolder writes a small synthetic disc (a film of six clips as
// playlist 800 that first play leads to, twelve decoys, two extras) as a
// BDMV folder and returns the folder and the feature's playlist number.
func SynthFolder(t testing.TB, seed int64) (dir string, feature int) {
	s, _ := smallDisc(seed)
	return s.writeFolder(t), 800
}

// SynthImage writes the same disc as a UDF 2.50 image.
func SynthImage(t testing.TB, seed int64) (path string, feature int) {
	s, _ := smallDisc(seed)
	path = filepath.Join(t.TempDir(), "synthetic.iso")
	writeUDFImage(t, path, "SYNTH_DISC", s.udfFiles(false), udfOptions{})
	return path, 800
}
