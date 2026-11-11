// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// Byte-level fixtures for the playlist ranking: MPLS with audio, subtitle
// and angle streams, a 3D sub-path and chapter marks; CLPI with time bases
// and an entry-point map; index.bdmv and MovieObject.bdmv through the
// builders in quirks_test.go. Every fixture is written out and opened with
// Open, so the real parsers read it.

const sec = 45000 // 45 kHz ticks per second

type fxItem struct {
	clip    string
	in, out uint32
	stc     int
	audio   []string // languages
	pg      []string
	angles  []string // extra angle clips
}

type fxPlaylist struct {
	items []fxItem
	marks []Mark
	ss    bool // a 3D dependent-view sub-path
}

func fxStreamEntry(pid uint16) []byte {
	return []byte{9, 1, byte(pid >> 8), byte(pid), 0, 0, 0, 0, 0, 0}
}

func fxMpls(p fxPlaylist) []byte {
	head := make([]byte, 58)
	copy(head, "MPLS0200")
	head[45] = 1
	binary.BigEndian.PutUint32(head[8:], 58)
	nSub := 0
	if p.ss {
		nSub = 1
	}
	list := cat(make([]byte, 6), be16(len(p.items)), be16(nSub))
	for _, it := range p.items {
		flags := 1
		if len(it.angles) > 0 {
			flags |= 0x10
		}
		pi := cat([]byte(it.clip+"M2TS"), be16(flags), []byte{byte(it.stc)}, be32(it.in), be32(it.out), make([]byte, 8), []byte{0, 0}, be16(0))
		if len(it.angles) > 0 {
			pi = append(pi, byte(len(it.angles)+1), 0)
			for _, a := range it.angles {
				pi = append(pi, cat([]byte(a+"M2TS"), []byte{0})...)
			}
		}
		stn := make([]byte, 16)
		stn[4], stn[5], stn[6] = 1, byte(len(it.audio)), byte(len(it.pg))
		stn = append(stn, fxStreamEntry(0x1011)...)
		stn = append(stn, 5, 0x1b, 0x61, 0, 0, 0)
		for k, l := range it.audio {
			stn = append(stn, fxStreamEntry(0x1100+uint16(k))...)
			stn = append(stn, 5, 0x81, 0x31, l[0], l[1], l[2])
		}
		for k, l := range it.pg {
			stn = append(stn, fxStreamEntry(0x1200+uint16(k))...)
			stn = append(stn, 5, 0x90, l[0], l[1], l[2], 0)
		}
		binary.BigEndian.PutUint16(stn, uint16(len(stn)-2))
		pi = append(pi, stn...)
		list = append(list, be16(len(pi))...)
		list = append(list, pi...)
	}
	if p.ss {
		list = append(list, cat(be32(6), []byte{0, 8, 0, 0, 0, 0})...)
	}
	markStart := 58 + len(list)
	binary.BigEndian.PutUint32(head[12:], uint32(markStart))
	out := cat(head, list, make([]byte, 4), be16(len(p.marks)))
	for _, m := range p.marks {
		e := make([]byte, 14)
		e[1] = byte(m.Type)
		binary.BigEndian.PutUint16(e[2:], uint16(m.Item))
		binary.BigEndian.PutUint32(e[4:], m.Time)
		binary.BigEndian.PutUint16(e[8:], 0xffff)
		out = append(out, e...)
	}
	return out
}

// fxClip writes clip information with one time base [start, end) and an
// entry point every `every` seconds, spread over the clip's packets.
func fxClip(packets uint32, start, end uint32, every int) []byte {
	b := buildClip(packets, STCSequence{PCRPID: 0x1001, Start: start, End: end})
	if every <= 0 {
		return b
	}
	var pts []uint32
	for t := start; t < end; t += uint32(every * sec) {
		pts = append(pts, t)
	}
	cpi := len(b)
	binary.BigEndian.PutUint32(b[16:], uint32(cpi))
	n := len(pts)
	v := uint64(n)<<18 | uint64(n)
	body := cat([]byte{0, 1}, be16(0x1011), be16(int(v>>32)), be32(uint32(v)), be32(14))
	body = append(body, be32(uint32(4+8*n))...)
	var fine []byte
	for k, t := range pts {
		p90 := uint64(t) * 2
		spn := (t - start) / sec // one packet per second: enough to tell entry points apart
		body = append(body, cat(be32(uint32(k)<<14|uint32(p90>>19)&0x3fff), be32(spn))...)
		fine = append(fine, be32(uint32((p90>>9)&0x7ff)<<17|spn&0x1ffff)...)
	}
	body = append(body, fine...)
	return cat(b, be32(uint32(len(body)+2)), be16(1), body)
}

// fxDisc is a disc being put together.
type fxDisc struct {
	files map[string][]byte
}

func newFx(first, top IndexObject, titles []IndexObject, objs ...MovieObject) *fxDisc {
	return &fxDisc{files: map[string][]byte{
		"BDMV/index.bdmv":       buildIndex("0200", first, top, titles...),
		"BDMV/MovieObject.bdmv": buildMobj(objs...),
	}}
}

func (f *fxDisc) clip(name string, minutes float64, every int) *fxDisc {
	return f.clipAt(name, sec, uint32(float64(sec)+minutes*60*sec), every)
}

func (f *fxDisc) clipAt(name string, start, end uint32, every int) *fxDisc {
	f.files["BDMV/CLIPINF/"+name+".clpi"] = fxClip(32, start, end, every)
	f.files["BDMV/STREAM/"+name+".m2ts"] = stream(32)
	return f
}

func (f *fxDisc) pl(n int, p fxPlaylist) *fxDisc {
	f.files[fmt.Sprintf("BDMV/PLAYLIST/%05d.mpls", n)] = fxMpls(p)
	return f
}

func (f *fxDisc) open(t testing.TB) *Disc {
	t.Helper()
	dir := t.TempDir()
	for name, data := range f.files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// whole plays a whole clip made by clip(name, minutes).
func whole(name string, minutes float64, langs ...string) fxItem {
	if len(langs) == 0 {
		langs = []string{"eng"}
	}
	return fxItem{clip: name, in: sec, out: uint32(float64(sec) + minutes*60*sec), audio: langs, pg: []string{"eng"}}
}

// chaptersEvery puts a chapter mark every `minutes` into a one-item playlist
// of the given length.
func chaptersEvery(total, minutes float64, item int, in uint32) []Mark {
	var m []Mark
	for t := 0.0; t < total; t += minutes {
		m = append(m, Mark{Type: 1, Item: item, Time: in + uint32(t*60*sec)})
	}
	return m
}

func hdmvTitle(obj int) IndexObject { return IndexObject{Kind: ObjectHDMV, MovieObject: obj} }

// ------------------------------------------------------------- scenarios

// A plain film: three reels in order with even chapters, a trailer, its
// 3D twin and a multi-angle extra.
func fxSimpleMovie() *fxDisc {
	f := newFx(hdmvTitle(0), hdmvTitle(0), []IndexObject{hdmvTitle(1)}, MovieObject{Commands: []Command{play(1)}}, MovieObject{Commands: []Command{play(1)}})
	f.clip("00001", 40, 5).clip("00002", 40, 5).clip("00003", 40, 5).clip("00009", 2, 0).clip("00020", 10, 0).clip("00021", 10, 0)
	film := []fxItem{whole("00001", 40, "eng", "fra"), whole("00002", 40, "eng", "fra"), whole("00003", 40, "eng", "fra")}
	var marks []Mark
	for i := range film {
		marks = append(marks, chaptersEvery(40, 8, i, sec)...)
	}
	f.pl(1, fxPlaylist{items: film, marks: marks})
	f.pl(2, fxPlaylist{items: []fxItem{whole("00009", 2)}})
	f.pl(3, fxPlaylist{items: film, marks: marks, ss: true})
	angled := whole("00020", 10)
	angled.angles = []string{"00021"}
	f.pl(4, fxPlaylist{items: []fxItem{angled}})
	return f
}

// Lionsgate-style obfuscation: 200 playlists of the same 20 reels; only
// one (00123) plays them in order. The rest are permutations.
func fxObfuscated(n, real int) *fxDisc {
	f := newFx(hdmvTitle(0), hdmvTitle(0), nil, MovieObject{})
	var reels []fxItem
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("%05d", 100+i)
		f.clip(name, 6, 0)
		reels = append(reels, whole(name, 6, "eng", "spa"))
	}
	rnd := rand.New(rand.NewSource(7))
	for p := 0; p < n; p++ {
		items := append([]fxItem(nil), reels...)
		if p != real {
			for {
				rnd.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
				if items[0].clip != reels[0].clip || items[1].clip != reels[1].clip {
					break
				}
			}
		}
		f.pl(p, fxPlaylist{items: items})
	}
	return f
}

// A shuffled decoy longer than the real film: it plays the real film's
// segments out of order plus an extra reel. The real one's cuts meet
// through the clip's packet map rather than exactly in time.
func fxShuffledLonger() *fxDisc {
	f := newFx(hdmvTitle(0), hdmvTitle(0), nil, MovieObject{})
	f.clip("00001", 100, 10).clip("00002", 30, 0)
	var real []fxItem
	for k := 0; k < 10; k++ {
		in := uint32(sec + k*10*60*sec)
		out := in + 10*60*sec
		if k > 0 {
			in += 3 * sec // starts at the same entry point as the last one ended, 3 s on
		}
		real = append(real, fxItem{clip: "00001", in: in, out: out, audio: []string{"eng", "fra", "deu"}, pg: []string{"eng", "fra"}})
	}
	decoy := []fxItem{real[7], real[2], real[9], real[0], real[5], real[3], real[8], real[1], real[6], real[4], whole("00002", 30)}
	for i := range decoy {
		decoy[i].audio, decoy[i].pg = []string{"eng"}, nil
	}
	f.pl(10, fxPlaylist{items: decoy})
	f.pl(11, fxPlaylist{items: real, marks: chaptersEvery(100, 10, 0, sec)})
	return f
}

// Clips that are missing or empty, and a playlist running past its clip.
func fxMissingClips() *fxDisc {
	f := newFx(hdmvTitle(0), hdmvTitle(0), nil, MovieObject{})
	f.clip("00001", 90, 0).clip("00002", 150, 0).clip("00003", 150, 0).clip("00004", 30, 0)
	delete(f.files, "BDMV/STREAM/00002.m2ts") // its stream file is gone
	f.files["BDMV/STREAM/00003.m2ts"] = nil   // its stream file is empty
	f.pl(1, fxPlaylist{items: []fxItem{whole("00002", 150)}})
	f.pl(2, fxPlaylist{items: []fxItem{whole("00003", 150)}})
	f.pl(3, fxPlaylist{items: []fxItem{{clip: "00004", in: sec, out: 200 * 60 * sec, audio: []string{"eng"}}}}) // past the clip's end
	f.pl(4, fxPlaylist{items: []fxItem{whole("00077", 160)}})                                                   // no such clip
	f.pl(5, fxPlaylist{items: []fxItem{whole("00001", 90)}})
	return f
}

// HDMV first play shows a logo, then jumps to title 1, whose program puts
// 800 in a register and plays that playlist. 00799 is a little longer.
func fxFirstPlay800() *fxDisc {
	first := MovieObject{Commands: []Command{play(5), jumpTitle(1)}}
	title1 := MovieObject{Commands: []Command{move(10, 800), {0x22000000, 10, 0}}}
	menu := MovieObject{Commands: []Command{{0x22000000, 4000, 0}}} // a register nobody set
	f := newFx(hdmvTitle(0), hdmvTitle(2), []IndexObject{hdmvTitle(1)}, first, title1, menu)
	f.clip("00005", 0.25, 0).clip("00800", 100, 0).clip("00799", 100.5, 0)
	f.pl(5, fxPlaylist{items: []fxItem{whole("00005", 0.25)}})
	f.pl(799, fxPlaylist{items: []fxItem{whole("00799", 100.5)}})
	f.pl(800, fxPlaylist{items: []fxItem{whole("00800", 100)}})
	return f
}

// A BD-J disc: nothing to read statically, so the structure decides.
func fxBDJ() *fxDisc {
	j := IndexObject{Kind: ObjectBDJ, BDJName: "00000"}
	f := newFx(j, j, []IndexObject{j})
	f.clip("00001", 1, 0).clip("00002", 60, 0).clip("00003", 60, 0).clip("00004", 2, 0)
	f.pl(1, fxPlaylist{items: []fxItem{whole("00001", 1)}})
	f.pl(2, fxPlaylist{items: []fxItem{whole("00003", 60), whole("00002", 60)}}) // the reels backwards
	f.pl(3, fxPlaylist{items: []fxItem{whole("00002", 60), whole("00003", 60)}})
	f.pl(4, fxPlaylist{items: []fxItem{whole("00004", 2)}})
	return f
}

// A TV disc: four 22-minute episodes in playlists numbered out of order
// and a play-all playlist.
func fxTV() *fxDisc {
	f := newFx(hdmvTitle(0), hdmvTitle(0), nil, MovieObject{})
	lens := []float64{22, 23, 21.5, 22.5}
	var all []fxItem
	for i, l := range lens {
		name := fmt.Sprintf("%05d", i+1)
		f.clip(name, l, 0)
		all = append(all, whole(name, l))
	}
	f.pl(10, fxPlaylist{items: []fxItem{all[2]}, marks: chaptersEvery(21.5, 5, 0, sec)})
	f.pl(11, fxPlaylist{items: []fxItem{all[0]}, marks: chaptersEvery(22, 5, 0, sec)})
	f.pl(12, fxPlaylist{items: []fxItem{all[3]}, marks: chaptersEvery(22.5, 5, 0, sec)})
	f.pl(13, fxPlaylist{items: []fxItem{all[1]}, marks: chaptersEvery(23, 5, 0, sec)})
	f.pl(20, fxPlaylist{items: all})
	f.clip("00030", 3, 0)
	f.pl(30, fxPlaylist{items: []fxItem{whole("00030", 3)}})
	return f
}

// Segment spam: a decoy of 600 nine-second items (90 minutes) beside an
// 80-minute film, and one of 400 half-second items.
func fxSegmentSpam() *fxDisc {
	f := newFx(hdmvTitle(0), hdmvTitle(0), nil, MovieObject{})
	f.clip("00001", 40, 0).clip("00002", 40, 0).clip("00003", 95, 0)
	f.pl(1, fxPlaylist{items: []fxItem{whole("00001", 40), whole("00002", 40)}})
	var spam, dust []fxItem
	for k := 0; k < 600; k++ {
		in := uint32(sec + k*9*sec)
		spam = append(spam, fxItem{clip: "00003", in: in, out: in + 9*sec, audio: []string{"eng"}})
	}
	for k := 0; k < 400; k++ {
		in := uint32(sec + k*sec)
		dust = append(dust, fxItem{clip: "00003", in: in, out: in + sec/2, audio: []string{"eng"}})
	}
	f.pl(2, fxPlaylist{items: spam})
	f.pl(3, fxPlaylist{items: dust})
	return f
}
