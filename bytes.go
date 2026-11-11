// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"encoding/binary"
	"fmt"
)

// rd reads big-endian fields out of a disc structure with bounds checks.
// Every read past the end sets err and returns zero, so a parser can read a
// whole structure and check once; nothing in a damaged file can make it read
// outside the buffer.
type rd struct {
	b   []byte
	err error
}

func (r *rd) fail(what string, off, n int) {
	if r.err == nil {
		r.err = fmt.Errorf("%s: %d bytes at %d are outside the %d-byte structure", what, n, off, len(r.b))
	}
}

func (r *rd) ok(off, n int) bool {
	// off+n is not formed: on a 32-bit build an offset near the top of the
	// range would wrap it round to a small number and pass.
	if off < 0 || n < 0 || off > len(r.b) || n > len(r.b)-off {
		r.fail("read", off, n)
		return false
	}
	return true
}

func (r *rd) u8(off int) uint8 {
	if !r.ok(off, 1) {
		return 0
	}
	return r.b[off]
}

func (r *rd) u16(off int) uint16 {
	if !r.ok(off, 2) {
		return 0
	}
	return binary.BigEndian.Uint16(r.b[off:])
}

func (r *rd) u32(off int) uint32 {
	if !r.ok(off, 4) {
		return 0
	}
	return binary.BigEndian.Uint32(r.b[off:])
}

func (r *rd) u64(off int) uint64 {
	if !r.ok(off, 8) {
		return 0
	}
	return binary.BigEndian.Uint64(r.b[off:])
}

// str returns n bytes as a string with trailing NULs removed.
func (r *rd) str(off, n int) string {
	if !r.ok(off, n) {
		return ""
	}
	s := r.b[off : off+n]
	for len(s) > 0 && s[len(s)-1] == 0 {
		s = s[:len(s)-1]
	}
	return string(s)
}

// ticks45 formats a 45 kHz timestamp as seconds.
func ticks45(t uint32) float64 { return float64(t) / 45000 }

// at returns a reader of its own over n bytes at off, so that a damaged part
// of a file (a stream table, an entry-point map) fails by itself and leaves
// the rest readable. A range outside the structure gives an empty reader
// that has already failed.
func (r *rd) at(off, n int) *rd {
	if off < 0 || n < 0 || off > len(r.b) || n > len(r.b)-off {
		s := &rd{}
		s.fail("section", off, n)
		return s
	}
	return &rd{b: r.b[off : off+n]}
}

// rest is at() to the end of the structure.
func (r *rd) rest(off int) *rd {
	if off < 0 || off > len(r.b) {
		s := &rd{}
		s.fail("section", off, 0)
		return s
	}
	return &rd{b: r.b[off:]}
}
