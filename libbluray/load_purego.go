// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build (darwin || windows || (linux && libbluray)) && (amd64 || arm64) && !nolibbluray

package libbluray

import (
	"fmt"
	"strings"

	"github.com/ebitengine/purego"
)

// The loader: finds a libbluray shared library the user has installed and
// binds the eight functions this package calls. Nothing else of the library
// is reachable from here.
//
// It is built for macOS and Windows always, and for Linux only with the
// `libbluray` build tag: on Linux a Go program built without cgo is a
// static binary that runs on any distribution, and a loader would make it
// depend on the machine's C library. Whoever builds for one distribution
// can add the tag; nothing needs cgo either way.

func load(only string) (*api, string) {
	names := libraryNames()
	if only != "" {
		names = []string{only}
	}
	var errs []string
	for _, n := range names {
		h, err := openLibrary(n)
		if err != nil {
			// The system's own account of where it looked runs to a
			// paragraph; the name is what a person needs.
			errs = append(errs, n+" could not be loaded")
			continue
		}
		a := &api{path: n}
		if err := bind(a, h); err != nil {
			errs = append(errs, n+": "+err.Error())
			continue
		}
		a.setenv = cSetenv()
		return a, ""
	}
	if len(errs) > 3 {
		errs = append(errs[:3], "...")
	}
	return nil, "no usable libbluray was found on this machine (" + strings.Join(errs, "; ") + ")"
}

// bind looks up each function; a library that lacks one is not used.
func bind(a *api, h uintptr) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("not a usable libbluray: %v", r)
		}
	}()
	purego.RegisterLibFunc(&a.version, h, "bd_get_version")
	purego.RegisterLibFunc(&a.init, h, "bd_init")
	purego.RegisterLibFunc(&a.openDisc, h, "bd_open_disc")
	purego.RegisterLibFunc(&a.getTitles, h, "bd_get_titles")
	purego.RegisterLibFunc(&a.mainTitle, h, "bd_get_main_title")
	purego.RegisterLibFunc(&a.titleInfo, h, "bd_get_title_info")
	purego.RegisterLibFunc(&a.freeTitle, h, "bd_free_title_info")
	purego.RegisterLibFunc(&a.closeDisc, h, "bd_close")
	return nil
}
