// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"container/heap"
	"sort"
)

// A static reading of the disc's HDMV program: where the commands of a movie
// object lead (the playlists they play, the objects and titles they jump or
// call to) without running them. The VM in vm.go runs the program for real;
// this is what the playlist ranking uses, because it needs no player state,
// follows every way a comparison could go, and always ends.
//
// It is an abstract interpretation with a budget. Each general register is
// known as a small set of constants or as "anything". Commands are followed
// along every path the register values allow: a comparison whose operands
// are known goes one way, any other goes both; GOTO through a register goes
// to each value it may hold. Register values travel with JUMP_OBJECT,
// CALL_OBJECT and title jumps into the object they enter, which is how
// "MOVE GPR7, 800" in one object and "PLAY_PL GPR7" in another are joined.
// When a playlist has played, a menu may have written any register, so from
// there on comparisons go both ways. Work is counted, and past the budget
// the reading stops and says so.

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
	// FromRegister: the operand is a register, and Value is a constant the
	// program put there.
	FromRegister bool `json:"fromRegister,omitempty"`
	// Alternatives is how many values the register may hold at this
	// command (each is listed as a target of its own); 0 or 1 for one.
	Alternatives int `json:"alternatives,omitempty"`
	// Conditional: the command is only reached past a comparison that the
	// reading could not decide.
	Conditional bool `json:"conditional,omitempty"`
	// Dead: no path through the object's commands reaches this one. It is
	// listed because a reading can be wrong, and never followed.
	Dead bool `json:"dead,omitempty"`
}

// IsPlay says the target starts a playlist.
func (t NavTarget) IsPlay() bool {
	return t.Op == "PLAY_PL" || t.Op == "PLAY_PL_PI" || t.Op == "PLAY_PL_PM"
}

// How far a static reading goes.
const (
	maxReachObjects = 512     // movie objects one reach visits
	maxAbsValues    = 8       // constants a register is tracked as, before it is "anything"
	maxAbsWork      = 2 << 20 // units of work one reach may spend
	maxAbsTargets   = 1 << 16 // targets one reach lists
)

// absVal is what is known of a register: one of a few constants, or
// anything.
type absVal struct {
	top  bool
	vals []uint32 // sorted, distinct, at most maxAbsValues
}

var (
	absTop  = absVal{top: true}
	absZero = absVal{vals: []uint32{0}}
)

func absConst(v uint32) absVal { return absVal{vals: []uint32{v}} }

func (a absVal) with(v uint32) absVal {
	if a.top {
		return a
	}
	i := sort.Search(len(a.vals), func(i int) bool { return a.vals[i] >= v })
	if i < len(a.vals) && a.vals[i] == v {
		return a
	}
	if len(a.vals) >= maxAbsValues {
		return absTop
	}
	out := make([]uint32, 0, len(a.vals)+1)
	out = append(out, a.vals[:i]...)
	out = append(out, v)
	return absVal{vals: append(out, a.vals[i:]...)}
}

func (a absVal) join(b absVal) absVal {
	if a.top || b.top {
		return absTop
	}
	for _, v := range b.vals {
		a = a.with(v)
	}
	return a
}

func (a absVal) equal(b absVal) bool {
	if a.top != b.top || len(a.vals) != len(b.vals) {
		return false
	}
	for i := range a.vals {
		if a.vals[i] != b.vals[i] {
			return false
		}
	}
	return true
}

// absState is the registers at one point of a program.
type absState struct {
	def     absVal            // every register not in m
	m       map[uint32]absVal // general registers written on the way here
	tainted bool              // a playlist or a called object has run: a menu may have written anything
	cond    bool              // reached past an undecided comparison
}

func (s *absState) get(reg uint32) absVal {
	if v, ok := s.m[reg]; ok {
		return v
	}
	return s.def
}

func (s *absState) clone() *absState {
	c := &absState{def: s.def, tainted: s.tainted, cond: s.cond, m: make(map[uint32]absVal, len(s.m)+1)}
	for k, v := range s.m {
		c.m[k] = v
	}
	return c
}

func (s *absState) set(reg uint32, v absVal) *absState {
	c := s.clone()
	c.m[reg] = v
	return c
}

// join merges b into s and says whether s changed.
func (s *absState) join(b *absState) bool {
	changed := false
	if b.tainted && !s.tainted {
		s.tainted, changed = true, true
	}
	if b.cond && !s.cond {
		s.cond, changed = true, true
	}
	for k, bv := range b.m {
		old := s.get(k)
		if nv := old.join(bv); !nv.equal(old) {
			s.m[k], changed = nv, true
		}
	}
	for k, sv := range s.m {
		if _, ok := b.m[k]; !ok {
			if nv := sv.join(b.def); !nv.equal(sv) {
				s.m[k], changed = nv, true
			}
		}
	}
	if nd := s.def.join(b.def); !nd.equal(s.def) {
		s.def, changed = nd, true
	}
	return changed
}

// operandVal is what an operand may be: an immediate, a general register,
// or a player register (which only a running player knows).
func (s *absState) operandVal(v uint32, imm bool) absVal {
	switch {
	case imm:
		return absConst(v)
	case v&0x80000000 != 0:
		if v&0x7fffff80 != 0 {
			return absZero // not a register: reads as zero
		}
		return absTop
	case v&0xfffff000 != 0:
		return absZero
	}
	return s.get(v)
}

// absArith applies a SET command to every pair of values; the result is
// "anything" when it is not a small set.
func absArith(opt int, a, b absVal) absVal {
	if opt == 1 { // MOVE
		return b
	}
	if opt == 8 { // RND: 1..n, whatever the register held
		if !b.top && len(b.vals) == 1 && b.vals[0] >= 1 && b.vals[0] <= maxAbsValues {
			out := absVal{}
			for v := uint32(1); v <= b.vals[0]; v++ {
				out = out.with(v)
			}
			return out
		}
		return absTop
	}
	if a.top || b.top || len(a.vals)*len(b.vals) > 64 {
		return absTop
	}
	out := absVal{}
	for _, dst := range a.vals {
		for _, src := range b.vals {
			var v uint32
			switch opt {
			case 3:
				v = saturate(uint64(dst) + uint64(src))
			case 4:
				if dst > src {
					v = dst - src
				}
			case 5:
				v = saturate(uint64(dst) * uint64(src))
			case 6:
				v = 0xffffffff
				if src != 0 {
					v = dst / src
				}
			case 7:
				v = 0xffffffff
				if src != 0 {
					v = dst % src
				}
			case 9:
				v = dst & src
			case 10:
				v = dst | src
			case 11:
				v = dst ^ src
			case 12:
				v = dst | shl1(src)
			case 13:
				v = dst &^ shl1(src)
			case 14:
				if src < 32 {
					v = dst << src
				}
			case 15:
				if src < 32 {
					v = dst >> src
				}
			default:
				v = dst
			}
			if out = out.with(v); out.top {
				return absTop
			}
		}
	}
	return out
}

func absCompare(opt int, dst, src uint32) bool {
	switch opt {
	case 1:
		return dst&^src != 0
	case 2:
		return dst == src
	case 3:
		return dst != src
	case 4:
		return dst >= src
	case 5:
		return dst > src
	case 6:
		return dst <= src
	case 7:
		return dst < src
	}
	return true
}

// absEvent is a target found in an object, with the registers as they are
// when it is taken.
type absEvent struct {
	t  NavTarget
	st *absState
}

type pcHeap []int

func (h pcHeap) Len() int           { return len(h) }
func (h pcHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h pcHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *pcHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *pcHeap) Pop() any {
	old := *h
	v := old[len(old)-1]
	*h = old[:len(old)-1]
	return v
}

// absRun reads one object from its first command with the given registers,
// to a fixed point or until the work runs out. Events come in command order.
//
// passive says a playlist can be told to write no register while it plays
// (it has no menu); nil means none can.
func absRun(m MovieObject, index int, entry *absState, work *int, passive func(playlist int) bool) (events []absEvent, unknownGoto int) {
	n := len(m.Commands)
	if n == 0 {
		return nil, 0
	}
	in := make([]*absState, n)
	queued := make([]bool, n)
	var h pcHeap
	flow := func(pc int, st *absState) {
		if pc < 0 || pc >= n {
			return
		}
		if in[pc] == nil {
			in[pc] = st.clone()
		} else if !in[pc].join(st) {
			return
		}
		if !queued[pc] {
			queued[pc] = true
			heap.Push(&h, pc)
		}
	}
	type evKey struct {
		pc    int
		value int
	}
	found := map[evKey]int{} // -> index in events
	emit := func(pc int, op string, in Instr, st *absState) (vals absVal) {
		vals = st.operandVal(in.Dst, in.ImmDst)
		arg := -1
		if in.Sub == 2 && in.Opt > 0 {
			if a := st.operandVal(in.Src, in.ImmSrc); !a.top && len(a.vals) == 1 && a.vals[0] < 0x80000000 {
				arg = int(a.vals[0])
			}
		}
		put := func(value int, alt int) {
			k := evKey{pc, value}
			if i, ok := found[k]; ok {
				events[i].st.join(st)
				events[i].t.Conditional = events[i].t.Conditional && st.cond
				return
			}
			if len(events) >= maxAbsTargets {
				return
			}
			found[k] = len(events)
			events = append(events, absEvent{t: NavTarget{Object: index, PC: pc, Op: op, Value: value, Arg: arg, FromRegister: !in.ImmDst && value >= 0, Alternatives: alt, Conditional: st.cond}, st: st.clone()})
		}
		if vals.top {
			put(-1, 0)
			return vals
		}
		alt := 0
		if len(vals.vals) > 1 {
			alt = len(vals.vals)
		}
		for _, v := range vals.vals {
			if v > 0x7fffffff {
				put(-1, 0)
				continue
			}
			put(int(v), alt)
		}
		return vals
	}
	flow(0, entry)
	for h.Len() > 0 {
		pc := heap.Pop(&h).(int)
		queued[pc] = false
		st := in[pc]
		if *work -= 1 + len(st.m)/8; *work < 0 {
			break
		}
		ins := Decode(m.Commands[pc])
		switch ins.Group {
		case GroupBranch:
			switch ins.Sub {
			case 0:
				switch ins.Opt {
				case 1: // GOTO
					to := st.operandVal(ins.Dst, ins.ImmDst)
					if to.top {
						unknownGoto++
						continue
					}
					for _, v := range to.vals {
						if v < uint32(n) {
							flow(int(v), st)
						}
					}
					continue
				case 2: // BREAK
					continue
				}
			case 1:
				op := pick(ins.Opt, "JUMP_OBJECT", "JUMP_TITLE", "CALL_OBJECT", "CALL_TITLE", "RESUME")
				switch ins.Opt {
				case 0, 1:
					emit(pc, op, ins, st)
					continue // a jump does not come back
				case 2, 3:
					emit(pc, op, ins, st)
					after := st.clone()
					after.tainted = true // what was called may have written anything
					flow(pc+1, after)
					continue
				case 4:
					continue // RESUME leaves the object
				}
			case 2:
				if ins.Opt <= 2 {
					played := emit(pc, pick(ins.Opt, "PLAY_PL", "PLAY_PL_PI", "PLAY_PL_PM"), ins, st)
					quiet := passive != nil && !played.top
					for _, v := range played.vals {
						quiet = quiet && passive(int(v))
					}
					after := st
					if !st.tainted && !quiet {
						after = st.clone()
						after.tainted = true
					}
					flow(pc+1, after)
					continue
				}
			}
		case GroupCompare:
			a, b := st.operandVal(ins.Dst, ins.ImmDst), st.operandVal(ins.Src, ins.ImmSrc)
			yes, no := true, true
			if !st.tainted && !a.top && !b.top && len(a.vals)*len(b.vals) <= 64 {
				yes, no = false, false
				for _, x := range a.vals {
					for _, y := range b.vals {
						if absCompare(ins.Opt, x, y) {
							yes = true
						} else {
							no = true
						}
					}
				}
			}
			t, f := st, st
			if yes && no && !st.cond {
				t = st.clone()
				t.cond = true
				f = t
			}
			// What an equality settles is known on the side it holds.
			if reg := ins.Dst; !ins.ImmDst && reg < 4096 && ins.ImmSrc && yes && no {
				switch ins.Opt {
				case 2:
					t = t.set(reg, absConst(ins.Src))
				case 3:
					f = f.set(reg, absConst(ins.Src))
				}
			}
			if yes {
				flow(pc+1, t)
			}
			if no {
				flow(pc+2, f)
			}
			continue
		case GroupSet:
			if ins.Sub == 0 && !ins.ImmDst && ins.Dst < 4096 && ins.Opt >= 1 && ins.Opt <= 15 {
				a, b := st.get(ins.Dst), st.operandVal(ins.Src, ins.ImmSrc)
				next := st.set(ins.Dst, absArith(ins.Opt, a, b))
				if ins.Opt == 2 && !ins.ImmSrc && ins.Src < 4096 { // SWAP writes both
					next.m[ins.Src] = a
				}
				flow(pc+1, next)
				continue
			}
		}
		flow(pc+1, st)
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].t.PC < events[j].t.PC })
	// Commands no path reached are listed too, marked dead.
	reached := map[int]bool{}
	for _, e := range events {
		reached[e.t.PC] = true
	}
	if *work >= 0 {
		for pc := range m.Commands {
			if in[pc] != nil || reached[pc] || len(events) >= maxAbsTargets {
				continue
			}
			ins := Decode(m.Commands[pc])
			var op string
			if ins.Group == GroupBranch {
				switch ins.Sub {
				case 1:
					op = pick(ins.Opt, "JUMP_OBJECT", "JUMP_TITLE", "CALL_OBJECT", "CALL_TITLE", "")
				case 2:
					op = pick(ins.Opt, "PLAY_PL", "PLAY_PL_PI", "PLAY_PL_PM", "", "", "")
				}
			}
			if op == "" || op[0] == '?' {
				continue
			}
			v := -1
			if ins.ImmDst && ins.Dst <= 0x7fffffff {
				v = int(ins.Dst)
			}
			arg := -1
			if ins.Sub == 2 && ins.Opt > 0 && ins.ImmSrc && ins.Src < 0x80000000 {
				arg = int(ins.Src)
			}
			events = append(events, absEvent{t: NavTarget{Object: index, PC: pc, Op: op, Value: v, Arg: arg, Dead: true}})
		}
		sort.SliceStable(events, func(i, j int) bool { return events[i].t.PC < events[j].t.PC })
	}
	return events, unknownGoto
}

// ObjectTargets reads a movie object's commands by itself, knowing nothing
// of the registers on entry, and lists every PLAY_PL, PLAY_PL_PI,
// PLAY_PL_PM, JUMP/CALL_OBJECT and JUMP/CALL_TITLE in command order. A
// register operand gives one target for each constant the object's own
// commands may have put there, and a target with Value -1 when it cannot be
// told. Commands that no path reaches are listed with Dead set.
func ObjectTargets(m MovieObject, index int) []NavTarget {
	work := maxAbsWork
	events, _ := absRun(m, index, &absState{def: absTop, m: map[uint32]absVal{}}, &work, nil)
	out := make([]NavTarget, 0, len(events))
	for _, e := range events {
		out = append(out, e.t)
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
	// Certain lists the playlists reached by commands that are not behind
	// an undecided comparison and whose operand had one value: the ones the
	// program plays whatever the player's state.
	Certain []int `json:"certain,omitempty"`
	// Incomplete: the reading ran out of its budget of work or objects
	// before it had followed everything.
	Incomplete bool `json:"incomplete,omitempty"`
}

// reachStart says what the registers are when a reach begins.
type reachStart int

const (
	startUnknown reachStart = iota // any register may hold anything
	startZero                      // the disc has just been put in: registers are zero
)

// StaticReach follows an index entry through the movie objects: their
// commands in command order, with JUMP/CALL_OBJECT followed at once (depth
// first, so the order of Playlists is close to the order they would play).
// With followTitles, JUMP/CALL_TITLE to a numbered title is followed too
// (never to the top menu or first play).
func (d *Disc) StaticReach(o IndexObject, followTitles bool) Reach {
	return d.staticReach(o, followTitles, startUnknown)
}

func (d *Disc) staticReach(o IndexObject, followTitles bool, start reachStart) Reach {
	var r Reach
	if d == nil {
		return r
	}
	work := maxAbsWork
	entries := map[int]*absState{}
	seenPL := map[int]bool{}
	certain := map[int]bool{}
	seenTitle := map[int]bool{}
	seenTarget := map[[3]int]bool{}
	visits := 0
	var visit func(o IndexObject, st *absState)
	var walk func(n int, st *absState)
	visit = func(o IndexObject, st *absState) {
		switch o.Kind {
		case ObjectBDJ:
			r.BDJ = true
		case ObjectHDMV:
			walk(o.MovieObject, st)
		}
	}
	walk = func(n int, st *absState) {
		if n < 0 || n >= len(d.MovieObjects) {
			return
		}
		entry := entries[n]
		switch {
		case entry == nil:
			if len(entries) >= maxReachObjects {
				r.Incomplete = true
				return
			}
			entry = st.clone()
			entries[n] = entry
			r.Objects = append(r.Objects, n)
		case !entry.join(st):
			return // already read with registers that cover these
		}
		if visits++; visits > 4*maxReachObjects || work < 0 {
			r.Incomplete = true
			return
		}
		events, unknownGoto := absRun(d.MovieObjects[n], n, entry, &work, d.passivePlaylist)
		if work < 0 {
			r.Incomplete = true
		}
		r.Unknown += unknownGoto
		for _, e := range events {
			t := e.t
			if k := [3]int{t.Object, t.PC, t.Value}; !seenTarget[k] {
				seenTarget[k] = true
				if len(r.Targets) < maxAbsTargets {
					r.Targets = append(r.Targets, t)
				}
				if t.Value < 0 && !t.Dead {
					r.Unknown++
				}
			}
			if t.Value < 0 || t.Dead {
				continue
			}
			switch t.Op {
			case "PLAY_PL", "PLAY_PL_PI", "PLAY_PL_PM":
				if !seenPL[t.Value] {
					seenPL[t.Value] = true
					r.Playlists = append(r.Playlists, t.Value)
				}
				if !t.Conditional && t.Alternatives <= 1 && !e.st.cond && !certain[t.Value] {
					certain[t.Value] = true
					r.Certain = append(r.Certain, t.Value)
				}
			case "JUMP_OBJECT", "CALL_OBJECT":
				walk(t.Value, e.st)
			case "JUMP_TITLE", "CALL_TITLE":
				if !seenTitle[t.Value] {
					seenTitle[t.Value] = true
					r.Titles = append(r.Titles, t.Value)
				}
				if followTitles && d.Index != nil && t.Value >= 1 && t.Value <= len(d.Index.Titles) {
					visit(d.Index.Titles[t.Value-1].Object, e.st)
				}
			}
		}
	}
	st := &absState{def: absTop, m: map[uint32]absVal{}}
	if start == startZero {
		st.def = absZero
	}
	visit(o, st)
	return r
}

// FirstPlayReach is StaticReach from the index's first play, following
// title jumps, with the registers as they are when a disc is put in (all
// zero), so a program's "have I been here before" test goes the way it
// does on a first run.
func (d *Disc) FirstPlayReach() Reach {
	if d == nil || d.Index == nil {
		return Reach{}
	}
	return d.staticReach(d.Index.FirstPlay, true, startZero)
}

// TitleReach is StaticReach from a title (1-based), following title jumps.
func (d *Disc) TitleReach(n int) Reach {
	if d == nil || d.Index == nil || n < 1 || n > len(d.Index.Titles) {
		return Reach{}
	}
	return d.StaticReach(d.Index.Titles[n-1].Object, true)
}

// passivePlaylist says a playlist has no menu: no interactive graphics
// stream in any play item and no menu sub-path, so nothing a viewer does
// while it plays can run a command. A playlist the disc does not have
// plays nothing at all.
func (d *Disc) passivePlaylist(n int) bool {
	pl := d.Playlists[n]
	if pl == nil {
		return true
	}
	for i := range pl.Items {
		if len(pl.Items[i].IG) > 0 {
			return false
		}
	}
	for _, sp := range pl.SubPaths {
		if sp.Type == 3 {
			return false
		}
	}
	return true
}
