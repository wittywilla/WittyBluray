// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !((darwin || windows || (linux && libbluray)) && (amd64 || arm64) && !nolibbluray)

package libbluray

// This build has no loader: it is for a platform the loader is not built
// for, or was made with the `nolibbluray` tag, or is a Linux build without
// the `libbluray` tag (which keeps the program a static binary).
func load(only string) (*api, string) {
	return nil, "this build cannot load libbluray (on Linux, build with -tags libbluray; 64-bit macOS, Windows and Linux only)"
}
