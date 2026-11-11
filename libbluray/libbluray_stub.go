// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !libbluray || !cgo

package libbluray

// Available reports whether libbluray is built in. This build has no
// libbluray: it was made without the `libbluray` tag, or without cgo.
func Available() bool { return false }

// Read would read a disc's navigation with libbluray; this build has none.
func Read(path string, o ReadOptions) (*Info, error) { return nil, ErrUnavailable }
