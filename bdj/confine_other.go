// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !linux

package bdj

import "os/exec"

// confine has nothing more to add outside Linux: DiscLoader's list is the wall there.
func confine(*exec.Cmd) bool { return false }
