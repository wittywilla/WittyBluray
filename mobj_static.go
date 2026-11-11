// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

// A static reading of the disc's HDMV program: where the commands of a movie
// object lead (the playlists they play, the objects and titles they jump or
// call to) without running them. The VM in vm.go runs the program for real;
// this is what the playlist ranking uses, because it is cheap, needs no
// player state and cannot loop. Commands under a condition are all counted,
// whichever way the condition would go.

// NavTarget is one place a movie object's commands lead.
type NavTarget struct {
	Object int    `json:"object"` // the movie object the command is in
	PC     int    `json:"pc"`     // the command's index in it
	Op     string `json:"op"`     // PLAY_PL, PLAY_PL_PI, PLAY_PL_PM, JUMP_OBJECT, JUMP_TITLE, CALL_OBJECT, CALL_TITLE
	// Value is the playlist, object or title; -1 when the operand is a
	// register whose value can't be told statically.
	Value int `json:"value"`
	// Arg is PLAY_PL_PI's play item or PLAY_PL_PM's mark; -1 otherwise or
	// when unknown.
	Arg int `json:"arg"`
	// FromRegister: the operand is a register, and Value is the constant an
	// earlier MOVE in the same object put there.
	FromRegister bool `json:"fromRegister,omitempty"`
}

// IsPlay says the target starts a playlist.
func (t NavTarget) IsPlay() bool {
	return t.Op == "PLAY_PL" || t.Op == "PLAY_PL_PI" || t.Op == "PLAY_PL_PM"
}

// ObjectTargets decodes a movie object's commands and lists every PLAY_PL,
// PLAY_PL_PI, PLAY_PL_PM, JUMP/CALL_OBJECT and JUMP/CALL_TITLE in program
// order. A register operand is resolved when an earlier MOVE of a constant
// into that register is the last write to it in program order (a simple
// constant propagation that ignores branches; good enough for the common
// "MOVE GPR, n; PLAY_PL GPR" idiom).
func ObjectTargets(m MovieObject, index int) []NavTarget {
	known := map[uint32]uint32{}
	value := func(v uint32, imm bool) (int, bool) {
		if imm {
			return int(v), false
		}
		if v < 4096 {
			if c, ok := known[v]; ok {
				return int(c), true
			}
		}
		return -1, false
	}
	var out []NavTarget
	for pc, c := range m.Commands {
		in := Decode(c)
		switch in.Group {
		case GroupSet:
			if in.Sub != 0 || in.ImmDst || in.Dst >= 4096 {
				continue
			}
			if in.Opt == 1 && in.ImmSrc {
				known[in.Dst] = in.Src
			} else {
				delete(known, in.Dst)
			}
			if in.Opt == 2 && !in.ImmSrc && in.Src < 4096 { // SWAP writes both
				delete(known, in.Src)
			}
		case GroupBranch:
			var op string
			switch in.Sub {
			case 1:
				op = map[int]string{0: "JUMP_OBJECT", 1: "JUMP_TITLE", 2: "CALL_OBJECT", 3: "CALL_TITLE"}[in.Opt]
			case 2:
				op = map[int]string{0: "PLAY_PL", 1: "PLAY_PL_PI", 2: "PLAY_PL_PM"}[in.Opt]
			}
			if op == "" {
				continue
			}
			v, reg := value(in.Dst, in.ImmDst)
			t := NavTarget{Object: index, PC: pc, Op: op, Value: v, Arg: -1, FromRegister: reg}
			if in.Sub == 2 && in.Opt > 0 {
				if a, _ := value(in.Src, in.ImmSrc); a >= 0 && a < 0x80000000 {
					t.Arg = a
				}
			}
			out = append(out, t)
		}
	}
	return out
}

// Reach is what an entry of the index leads to, read statically.
type Reach struct {
	Playlists []int       `json:"playlists"` // in the order found, each once
	Titles    []int       `json:"titles,omitempty"`
	Objects   []int       `json:"objects,omitempty"` // movie objects visited
	Targets   []NavTarget `json:"targets,omitempty"`
	// Unknown counts targets whose operand is a register with no known value.
	Unknown int `json:"unknown,omitempty"`
	// BDJ: the chain reaches a BD-J object, which can't be read statically.
	BDJ bool `json:"bdj,omitempty"`
}

// maxReachObjects bounds how many movie objects one reach visits.
const maxReachObjects = 512

// StaticReach follows an index entry through the movie objects: their
// commands in program order, with JUMP/CALL_OBJECT followed at once (depth
// first, so the order of Playlists is close to the order they would play).
// With followTitles, JUMP/CALL_TITLE to a numbered title is followed too
// (never to the top menu or first play).
func (d *Disc) StaticReach(o IndexObject, followTitles bool) Reach {
	var r Reach
	if d == nil {
		return r
	}
	seenObj := map[int]bool{}
	seenPL := map[int]bool{}
	seenTitle := map[int]bool{}
	var visit func(o IndexObject)
	var walk func(n int)
	visit = func(o IndexObject) {
		switch o.Kind {
		case ObjectBDJ:
			r.BDJ = true
		case ObjectHDMV:
			walk(o.MovieObject)
		}
	}
	walk = func(n int) {
		if n < 0 || n >= len(d.MovieObjects) || seenObj[n] || len(seenObj) >= maxReachObjects {
			return
		}
		seenObj[n] = true
		r.Objects = append(r.Objects, n)
		for _, t := range ObjectTargets(d.MovieObjects[n], n) {
			r.Targets = append(r.Targets, t)
			if t.Value < 0 {
				r.Unknown++
				continue
			}
			switch t.Op {
			case "PLAY_PL", "PLAY_PL_PI", "PLAY_PL_PM":
				if !seenPL[t.Value] {
					seenPL[t.Value] = true
					r.Playlists = append(r.Playlists, t.Value)
				}
			case "JUMP_OBJECT", "CALL_OBJECT":
				walk(t.Value)
			case "JUMP_TITLE", "CALL_TITLE":
				if !seenTitle[t.Value] {
					seenTitle[t.Value] = true
					r.Titles = append(r.Titles, t.Value)
					if followTitles && d.Index != nil && t.Value >= 1 && t.Value <= len(d.Index.Titles) {
						visit(d.Index.Titles[t.Value-1].Object)
					}
				}
			}
		}
	}
	visit(o)
	return r
}

// FirstPlayReach is StaticReach from the index's first play, following
// title jumps.
func (d *Disc) FirstPlayReach() Reach {
	if d == nil || d.Index == nil {
		return Reach{}
	}
	return d.StaticReach(d.Index.FirstPlay, true)
}

// TitleReach is StaticReach from a title (1-based), following title jumps.
func (d *Disc) TitleReach(n int) Reach {
	if d == nil || d.Index == nil || n < 1 || n > len(d.Index.Titles) {
		return Reach{}
	}
	return d.StaticReach(d.Index.Titles[n-1].Object, true)
}
