// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package libbluray

import "unsafe"

// chapterSlice views the library's array of n chapters. The caller has
// checked n against maxChapters, and the array is the library's until the
// title info is freed, so the values are copied out before then.
func chapterSlice(first *cChapter, n int) []cChapter {
	return unsafe.Slice(first, n)
}
