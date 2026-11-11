// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"sort"
	"strings"
)

// A static look at a disc's Java (BD-J) programs for the playlists they
// name. Nothing is run: the JAR files are opened as the ZIP archives they
// are and each class file's constants and bytecode are read for playlist
// numbers. A program names a playlist as a locator string
// ("bd://0.PLAYLIST:00800"), as a five-digit name, or as a plain number it
// pushes; the first is strong evidence, the last weak, since any number can
// be a coincidence. A program that computes its playlist, or keeps its
// strings scrambled, shows nothing here, and then the structure decides.

// BDJHint is one playlist a disc's Java program names.
type BDJHint struct {
	Playlist int `json:"playlist"`
	// Locators counts string constants that name the playlist in a locator
	// or as an .mpls file; Names, constants that are just its five digits;
	// Numbers, integer constants in bytecode equal to its number.
	Locators int `json:"locators,omitempty"`
	Names    int `json:"names,omitempty"`
	Numbers  int `json:"numbers,omitempty"`
	// Listed: a BD-J object's table of playable playlists has it.
	Listed bool `json:"listed,omitempty"`
}

// BDJHints is what the static look found.
type BDJHints struct {
	Hints   []BDJHint `json:"hints,omitempty"` // by playlist number
	Objects int       `json:"objects"`         // BD-J objects read
	Jars    int       `json:"jars"`            // JAR files read
	Classes int       `json:"classes"`         // class files read
	// Incomplete: a limit was reached (too many or too large JARs or
	// classes) before everything had been read.
	Incomplete bool `json:"incomplete,omitempty"`
}

// Hint returns what was found for a playlist, or nil.
func (h *BDJHints) Hint(playlist int) *BDJHint {
	if h == nil {
		return nil
	}
	i := sort.Search(len(h.Hints), func(i int) bool { return h.Hints[i].Playlist >= playlist })
	if i < len(h.Hints) && h.Hints[i].Playlist == playlist {
		return &h.Hints[i]
	}
	return nil
}

// Limits on the static look: it runs when a BD-J disc is ranked, possibly
// on a small machine.
const (
	maxBDJJars        = 32
	maxBDJJarBytes    = 16 << 20
	maxBDJTotalBytes  = 48 << 20 // compressed, over all JARs
	maxBDJClassBytes  = 4 << 20
	maxBDJInflated    = 96 << 20
	maxBDJClasses     = 20000
	maxBDJObjectFiles = 256
)

// BDJStaticHints reads the disc's BD-J objects and JAR files for the
// playlists they name, without running anything. Only playlists the disc
// has are reported. Worked out once per opened disc; nil when the disc has
// no BD-J object.
func (d *Disc) BDJStaticHints() *BDJHints {
	if d == nil || d.Source == nil {
		return nil
	}
	d.bdjOnce.Do(func() { d.bdjHints = d.bdjStaticHints() })
	return d.bdjHints
}

func (d *Disc) bdjStaticHints() *BDJHints {
	objs, _ := d.Source.List(d.layout.root + "/BDJO")
	if len(objs) == 0 {
		objs, _ = d.Source.List(d.layout.root + "/BACKUP/BDJO")
	}
	jars, _ := d.Source.List(d.layout.root + "/JAR")
	if len(objs) == 0 && len(jars) == 0 {
		return nil
	}
	h := &BDJHints{}
	found := map[int]*BDJHint{}
	hint := func(n int) *BDJHint {
		if d.Playlists[n] == nil {
			return &BDJHint{} // not a playlist of this disc: counted nowhere
		}
		if found[n] == nil {
			found[n] = &BDJHint{Playlist: n}
		}
		return found[n]
	}
	for i, n := range objs {
		if i >= maxBDJObjectFiles {
			h.Incomplete = true
			break
		}
		name := strings.TrimSuffix(n, path4(n))
		if !strings.EqualFold(path4(n), ".bdjo") || !safeName(name) {
			continue
		}
		o, err := d.BDJObject(name)
		if err != nil {
			continue
		}
		h.Objects++
		for _, p := range o.Playlists {
			hint(p).Listed = true
		}
	}
	var total, inflated int64
	for _, n := range jars {
		if !strings.EqualFold(path4(n), ".jar") {
			continue
		}
		if h.Jars >= maxBDJJars || total >= maxBDJTotalBytes {
			h.Incomplete = true
			break
		}
		b, err := d.Source.ReadFile(d.layout.root+"/JAR/"+n, maxBDJJarBytes+1)
		if err != nil {
			continue
		}
		if len(b) > maxBDJJarBytes {
			h.Incomplete = true // a ZIP cut short has no directory to read
			continue
		}
		total += int64(len(b))
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			continue
		}
		h.Jars++
		for _, f := range zr.File {
			if !strings.HasSuffix(f.Name, ".class") {
				continue
			}
			if h.Classes >= maxBDJClasses || inflated >= maxBDJInflated {
				h.Incomplete = true
				break
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			// What the archive says a file's size is decides nothing: at
			// most maxBDJClassBytes are inflated.
			cb, _ := io.ReadAll(io.LimitReader(rc, maxBDJClassBytes))
			rc.Close()
			inflated += int64(len(cb))
			h.Classes++
			strs, ints := ClassConstants(cb)
			for _, s := range strs {
				for _, n := range playlistNumbersIn(s) {
					if len(s) == 5 {
						hint(n).Names++
					} else {
						hint(n).Locators++
					}
				}
			}
			for _, v := range ints {
				if v >= 0 && v <= 99999 {
					hint(int(v)).Numbers++
				}
			}
		}
	}
	for _, x := range found {
		h.Hints = append(h.Hints, *x)
	}
	sort.Slice(h.Hints, func(i, j int) bool { return h.Hints[i].Playlist < h.Hints[j].Playlist })
	return h
}

// path4 is a file name's extension, when it has one of up to four letters.
func path4(n string) string {
	if i := strings.LastIndexByte(n, '.'); i >= 0 && len(n)-i <= 5 {
		return n[i:]
	}
	return ""
}

// playlistNumbersIn finds the playlists a string constant names: the whole
// string when it is five digits, or each five-digit run in a string that
// reads as a locator or a playlist file ("PLAYLIST:00800", "00800.mpls").
func playlistNumbersIn(s string) []int {
	if len(s) < 5 || len(s) > 512 {
		return nil
	}
	up := strings.ToUpper(s)
	whole := len(s) == 5
	if !whole && !strings.Contains(up, "PLAYLIST") && !strings.Contains(up, ".MPLS") && !strings.Contains(up, "BD://") {
		return nil
	}
	var out []int
	for i := 0; i < len(s); {
		if s[i] < '0' || s[i] > '9' {
			i++
			continue
		}
		j, v := i, 0
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			if j-i < 9 {
				v = v*10 + int(s[j]-'0')
			}
			j++
		}
		if j-i == 5 {
			out = append(out, v)
		}
		i = j
	}
	if whole && len(out) != 1 {
		return nil
	}
	return out
}

// ClassConstants reads a Java class file without loading it: the string
// constants its code uses, and the integer constants its bytecode pushes or
// switches on (bipush, sipush, ldc of an integer, lookupswitch keys). A
// file that is not a class, or is cut short, gives what was read up to
// there. Nothing in the file is executed.
func ClassConstants(b []byte) (strs []string, ints []int32) {
	r := &rd{b: b}
	if r.u32(0) != 0xcafebabe {
		return nil, nil
	}
	n := int(r.u16(8))
	type cpEntry struct {
		tag    byte
		off, l int   // Utf8: the bytes
		ref    int   // String: the Utf8 index
		val    int32 // Integer
	}
	cp := make([]cpEntry, n)
	o := 10
	for i := 1; i < n && r.err == nil; i++ {
		tag := r.u8(o)
		cp[i].tag = tag
		switch tag {
		case 1:
			l := int(r.u16(o + 1))
			if !r.ok(o+3, l) {
				break
			}
			cp[i].off, cp[i].l = o+3, l
			o += 3 + l
		case 3:
			cp[i].val = int32(r.u32(o + 1))
			o += 5
		case 4:
			o += 5
		case 5, 6:
			o += 9
			i++ // takes two slots
		case 7, 16, 19, 20:
			o += 3
		case 8:
			cp[i].ref = int(r.u16(o + 1))
			o += 3
		case 9, 10, 11, 12, 17, 18:
			o += 5
		case 15:
			o += 4
		default:
			r.fail("constant", o, 1)
		}
	}
	if r.err != nil {
		return nil, nil
	}
	utf := func(i int) (string, bool) {
		if i <= 0 || i >= n || cp[i].tag != 1 {
			return "", false
		}
		return string(b[cp[i].off : cp[i].off+cp[i].l]), true
	}
	seen := map[int]bool{}
	for i := 1; i < n; i++ {
		if cp[i].tag == 8 && !seen[cp[i].ref] {
			seen[cp[i].ref] = true
			if s, ok := utf(cp[i].ref); ok && len(strs) < 1<<16 {
				strs = append(strs, s)
			}
		}
	}
	addInt := func(v int32) {
		if len(ints) < 1<<18 {
			ints = append(ints, v)
		}
	}
	// access, this, super, interfaces
	o += 6
	o += 2 + 2*int(r.u16(o))
	skipAttrs := func(code func(at, l int)) {
		na := int(r.u16(o))
		o += 2
		for a := 0; a < na && r.err == nil; a++ {
			name, _ := utf(int(r.u16(o)))
			l := int(r.u32(o + 2))
			if l < 0 || !r.ok(o+6, l) {
				r.fail("attribute", o, l)
				return
			}
			if name == "Code" && code != nil {
				code(o+6, l)
			}
			o += 6 + l
		}
	}
	members := func(code func(at, l int)) {
		nm := int(r.u16(o))
		o += 2
		for m := 0; m < nm && r.err == nil; m++ {
			o += 6
			skipAttrs(code)
		}
	}
	members(nil) // fields
	members(func(at, l int) {
		c := &rd{b: b[at : at+l]}
		cl := int(c.u32(4))
		if c.err != nil || !c.ok(8, cl) {
			return
		}
		code := c.b[8 : 8+cl]
		for pc := 0; pc < len(code); {
			op := code[pc]
			need := func(k int) bool { return pc+k <= len(code) }
			switch {
			case op == 0x10 && need(2): // bipush
				addInt(int32(int8(code[pc+1])))
				pc += 2
			case op == 0x11 && need(3): // sipush
				addInt(int32(int16(binary.BigEndian.Uint16(code[pc+1:]))))
				pc += 3
			case op == 0x12 && need(2): // ldc
				if i := int(code[pc+1]); i < n && cp[i].tag == 3 {
					addInt(cp[i].val)
				}
				pc += 2
			case op == 0x13 && need(3): // ldc_w
				if i := int(binary.BigEndian.Uint16(code[pc+1:])); i < n && cp[i].tag == 3 {
					addInt(cp[i].val)
				}
				pc += 3
			case op == 0xaa: // tableswitch
				p := (pc + 4) &^ 3
				if p+12 > len(code) {
					return
				}
				lo, hi := int64(int32(binary.BigEndian.Uint32(code[p+4:]))), int64(int32(binary.BigEndian.Uint32(code[p+8:])))
				if hi < lo || hi-lo > 1<<16 {
					return
				}
				pc = p + 12 + int(hi-lo+1)*4
			case op == 0xab: // lookupswitch
				p := (pc + 4) &^ 3
				if p+8 > len(code) {
					return
				}
				np := int64(int32(binary.BigEndian.Uint32(code[p+4:])))
				if np < 0 || np > 1<<16 || p+8+int(np)*8 > len(code) {
					return
				}
				for k := 0; k < int(np); k++ {
					addInt(int32(binary.BigEndian.Uint32(code[p+8+k*8:])))
				}
				pc = p + 8 + int(np)*8
			case op == 0xc4: // wide
				if need(2) && code[pc+1] == 0x84 {
					pc += 6
				} else {
					pc += 4
				}
			default:
				pc += jvmLength(op)
			}
		}
	})
	return strs, ints
}

// jvmLength is the length in bytes of a JVM instruction that has no
// variable part.
func jvmLength(op byte) int {
	switch {
	case op == 0x10, op == 0x12, op >= 0x15 && op <= 0x19, op >= 0x36 && op <= 0x3a, op == 0xa9, op == 0xbc:
		return 2
	case op == 0x11, op == 0x13, op == 0x14, op == 0x84, op >= 0x99 && op <= 0xa8, op >= 0xb2 && op <= 0xb8, op == 0xbb, op == 0xbd, op == 0xc0, op == 0xc1, op == 0xc6, op == 0xc7:
		return 3
	case op == 0xc5:
		return 4
	case op == 0xb9, op == 0xba, op == 0xc8, op == 0xc9:
		return 5
	}
	return 1
}
