// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows && !linux

package bdj

func usage(pid int) (procUsage, bool) { return usagePS(pid) }
