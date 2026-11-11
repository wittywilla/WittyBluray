// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A synthetic BDMV generator. No commercial disc may be kept as a test
// fixture, so the patterns such discs use are written here from their
// description: structurally valid index.bdmv, MovieObject.bdmv, .mpls and
// .clpi files (with entry-point maps), and stream files that are either
// described (a size and a place on the disc, for the in-memory source) or
// written out small with valid packets (for a folder or an image).
//
// A synDisc can be opened three ways: source() gives an in-memory Source
// whose stream files have the sizes and disc positions the clips claim,
// with no bytes behind them; writeFolder() writes a BDMV folder; and
// writeUDF() (udf_test.go) writes a UDF 2.50 image.

type synClip struct {
	name       string
	start, end uint32 // 45 kHz
	packets    uint32
	// size is the stream file's size: 0 means "what the clip says"
	// (packets * 192), -1 no file, -2 an empty file, anything else that
	// many bytes.
	size   int64
	offset int64 // where it lies on the disc (in-memory source and image order)
	coding byte  // video: 0x1b H.264, 0x24 HEVC
	format int   // 6 = 1080p, 8 = 2160p
	epSec  int   // an entry point every so many seconds; 0 = none
}

type synItem struct {
	clip    string
	in, out uint32
	stc     int
	cc      int // connection condition; 0 = 1
	audio   []string
	pg      []string
	angles  []string
	ig      bool // an interactive graphics (menu) stream
}

type synPlaylist struct {
	items   []synItem
	marks   []Mark
	ss      bool // a 3D stream table in the extension data, and a dependent-view sub-path there
	hdr     int  // HEVC dynamic range: 1 HDR10, 2 Dolby Vision
	hdrPlus bool
	dv      bool // a Dolby Vision enhancement-layer stream
}

type synDisc struct {
	version   string
	first     IndexObject
	top       IndexObject
	titles    []IndexObject
	objs      []MovieObject
	clips     map[string]*synClip
	order     []string // clips in the order they lie on the disc
	playlists map[int]*synPlaylist
	extra     map[string][]byte // any other file, by path
	noProgram bool              // write neither index.bdmv nor MovieObject.bdmv
	// folderLike: the in-memory source gives each stream file a path of its
	// own, as a folder of files does, so where clips lie is unknown.
	folderLike bool
	rnd        *rand.Rand
	nextOff    int64
}

func newSyn(seed int64) *synDisc {
	return &synDisc{version: "0200", first: hdmv(0), top: hdmv(0), objs: []MovieObject{{}}, clips: map[string]*synClip{}, playlists: map[int]*synPlaylist{}, extra: map[string][]byte{}, rnd: rand.New(rand.NewSource(seed)), nextOff: 64 << 20}
}

// bytesPerSec gives clips a believable size: about 24 Mbit/s.
const synBytesPerSec = 3 << 20

// addClip adds a clip of the given length whose clock starts at start
// (45 kHz), laid on the disc after every clip added before it.
func (s *synDisc) addClip(name string, start uint32, seconds float64) *synClip {
	ticks := uint32(seconds * sec)
	size := int64(seconds*synBytesPerSec) / 192 * 192
	if size < 192*32 {
		size = 192 * 32
	}
	c := &synClip{name: name, start: start, end: start + ticks, packets: uint32(size / 192), offset: s.nextOff, coding: 0x1b, format: 6}
	s.nextOff += size + 1<<20
	s.clips[name] = c
	s.order = append(s.order, name)
	return c
}

// item plays a whole clip.
func (s *synDisc) item(name string) synItem {
	c := s.clips[name]
	return synItem{clip: name, in: c.start, out: c.end, audio: []string{"eng", "fra"}, pg: []string{"eng"}}
}

func (s *synDisc) items(names ...string) []synItem {
	out := make([]synItem, len(names))
	for i, n := range names {
		out[i] = s.item(n)
	}
	return out
}

func (s *synDisc) pl(n int, p *synPlaylist) *synPlaylist {
	s.playlists[n] = p
	return p
}

// freeNames returns n clip names not yet used, in an order that says
// nothing about the order they are asked for in.
func (s *synDisc) freeNames(n int) []string {
	var out []string
	for len(out) < n {
		name := fmt.Sprintf("%05d", 100+s.rnd.Intn(89000))
		if s.clips[name] != nil || contains(out, name) {
			continue
		}
		out = append(out, name)
	}
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// chaptersAt puts an entry mark every `every` seconds of playing time.
func synChapters(items []synItem, every float64) []Mark {
	var marks []Mark
	next := 0.0
	at := 0.0
	for i, it := range items {
		d := ticks45(it.out - it.in)
		for next < at+d {
			marks = append(marks, Mark{Type: 1, Item: i, Time: it.in + uint32((next-at)*sec), PID: 0xffff})
			next += every
		}
		at += d
	}
	return marks
}

// ---------------------------------------------------------------- bytes

func synStream(coding byte, pid uint16, attrs ...byte) []byte {
	b := []byte{9, 1, byte(pid >> 8), byte(pid), 0, 0, 0, 0, 0, 0, 5, coding}
	b = append(b, attrs...)
	for len(b) < 16 {
		b = append(b, 0)
	}
	return b
}

func (s *synDisc) mpls(p *synPlaylist) []byte {
	head := make([]byte, 58)
	copy(head, "MPLS"+s.version)
	head[45] = 1
	binary.BigEndian.PutUint32(head[8:], 58)
	list := cat(make([]byte, 6), be16(len(p.items)), be16(0))
	for _, it := range p.items {
		cc := it.cc
		if cc == 0 {
			cc = 1
		}
		flags := cc
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
		if it.ig {
			stn[7] = 1
		}
		if p.dv {
			stn[11] = 1
		}
		coding, format := byte(0x1b), 6
		if c := s.clips[it.clip]; c != nil {
			coding, format = c.coding, c.format
		}
		video := []byte{byte(format<<4 | 1), byte(p.hdr<<4 | 2), 0}
		if p.hdrPlus {
			video[2] = 0x40
		}
		stn = append(stn, synStream(coding, 0x1011, video...)...)
		for k, l := range it.audio {
			stn = append(stn, synStream(0x81, 0x1100+uint16(k), 0x61, l[0], l[1], l[2])...)
		}
		for k, l := range it.pg {
			stn = append(stn, synStream(0x90, 0x1200+uint16(k), l[0], l[1], l[2])...)
		}
		if it.ig {
			stn = append(stn, synStream(0x91, 0x1400, 'e', 'n', 'g')...)
		}
		if p.dv {
			stn = append(stn, synStream(0x24, 0x1015, 0x81, 0x22, 0)...)
		}
		binary.BigEndian.PutUint16(stn, uint16(len(stn)-2))
		pi = append(pi, stn...)
		list = append(list, be16(len(pi))...)
		list = append(list, pi...)
	}
	binary.BigEndian.PutUint32(list, uint32(len(list)-4))
	markStart := 58 + len(list)
	binary.BigEndian.PutUint32(head[12:], uint32(markStart))
	marks := cat(make([]byte, 4), be16(len(p.marks)))
	for _, m := range p.marks {
		e := make([]byte, 14)
		e[1] = byte(m.Type)
		binary.BigEndian.PutUint16(e[2:], uint16(m.Item))
		binary.BigEndian.PutUint32(e[4:], m.Time)
		binary.BigEndian.PutUint16(e[8:], 0xffff)
		marks = append(marks, e...)
	}
	binary.BigEndian.PutUint32(marks, uint32(len(marks)-4))
	out := cat(head, list, marks)
	if p.ss {
		// Extension data: a 3D stream table (ID 2,1) and a block of extra
		// sub-paths (ID 2,2) holding one dependent-view sub-path.
		binary.BigEndian.PutUint32(out[16:], uint32(len(out)))
		sub := cat(be32(0), be16(1), be32(6), []byte{0, 8, 0, 0, 0, 0})
		binary.BigEndian.PutUint32(sub, uint32(len(sub)-4))
		table := make([]byte, 8)
		dataStart := 12 + 2*12
		ext := cat(be32(0), be32(uint32(dataStart)), []byte{0, 0, 0, 2},
			be16(2), be16(1), be32(uint32(dataStart)), be32(uint32(len(table))),
			be16(2), be16(2), be32(uint32(dataStart+len(table))), be32(uint32(len(sub))),
			table, sub)
		binary.BigEndian.PutUint32(ext, uint32(len(ext)-4))
		out = append(out, ext...)
	}
	return out
}

func (s *synDisc) clpi(c *synClip) []byte {
	b := make([]byte, 60)
	copy(b, "HDMV"+s.version)
	binary.BigEndian.PutUint32(b[40:], 16)
	b[46], b[47] = 1, 1
	binary.BigEndian.PutUint32(b[52:], synBytesPerSec)
	binary.BigEndian.PutUint32(b[56:], c.packets)
	binary.BigEndian.PutUint32(b[8:], 60)
	seq := cat(be32(0), []byte{0, 1}, be32(0), []byte{1, 0}, be16(0x1001), be32(0), be32(c.start), be32(c.end))
	binary.BigEndian.PutUint32(seq, uint32(len(seq)-4))
	prog := cat(be32(0), []byte{0, 1}, be32(0), be16(0x100), []byte{2, 0},
		be16(0x1011), []byte{5, c.coding, byte(c.format<<4 | 1), 0x30, 0x12, 0, 0},
		be16(0x1100), []byte{5, 0x81, 0x61, 'e', 'n', 'g', 0})
	binary.BigEndian.PutUint32(prog, uint32(len(prog)-4))
	binary.BigEndian.PutUint32(b[12:], uint32(60+len(seq)))
	b = cat(b, seq, prog)
	if c.epSec <= 0 {
		return b
	}
	// The entry-point map as a disc writes it: a coarse entry whenever the
	// high bits of the time or of the packet number change.
	type entry struct{ pts90, spn uint64 }
	var es []entry
	total := uint64(c.end - c.start)
	for t := uint64(0); t < total; t += uint64(c.epSec * sec) {
		es = append(es, entry{(uint64(c.start) + t) * 2, t * uint64(c.packets) / total})
	}
	var coarse, fine []byte
	nCoarse := 0
	for k, e := range es {
		if k == 0 || e.pts90>>19 != es[k-1].pts90>>19 || e.spn>>17 != es[k-1].spn>>17 {
			coarse = append(coarse, cat(be32(uint32(k)<<14|uint32(e.pts90>>19)&0x3fff), be32(uint32(e.spn)))...)
			nCoarse++
		}
		fine = append(fine, be32(uint32((e.pts90>>9)&0x7ff)<<17|uint32(e.spn)&0x1ffff)...)
	}
	v := uint64(1)<<34 | uint64(nCoarse)<<18 | uint64(len(es))
	body := cat([]byte{0, 1}, be16(0x1011), be16(int(v>>32)), be32(uint32(v)), be32(14), be32(uint32(4+len(coarse))), coarse, fine)
	binary.BigEndian.PutUint32(b[16:], uint32(len(b)))
	return cat(b, be32(uint32(len(body)+2)), be16(1), body)
}

// structures returns every file of the disc's program, with a BACKUP copy
// of each.
func (s *synDisc) structures() map[string][]byte {
	files := map[string][]byte{}
	put := func(rel string, b []byte) {
		files["BDMV/"+rel] = b
		files["BDMV/BACKUP/"+rel] = b
	}
	if !s.noProgram {
		put("index.bdmv", buildIndex(s.version, s.first, s.top, s.titles...))
		put("MovieObject.bdmv", buildMobj(s.objs...))
	}
	for n, p := range s.playlists {
		put(fmt.Sprintf("PLAYLIST/%05d.mpls", n), s.mpls(p))
	}
	for n, c := range s.clips {
		put("CLIPINF/"+n+".clpi", s.clpi(c))
	}
	for n, b := range s.extra {
		files[n] = b
	}
	return files
}

func (c *synClip) fileSize() int64 {
	switch c.size {
	case 0:
		return int64(c.packets) * 192
	case -2:
		return 0
	}
	return c.size
}

// ---------------------------------------------------------------- sources

// memSource is a disc held in memory. Stream files have a size and a place
// and no bytes.
type memSource struct {
	files   map[string][]byte // by upper-case path
	names   map[string]string // upper-case path -> the name as written
	streams map[string]FileExtent
	reads   int
}

func (s *synDisc) source() *memSource {
	m := &memSource{files: map[string][]byte{}, names: map[string]string{}, streams: map[string]FileExtent{}}
	for n, b := range s.structures() {
		m.files[strings.ToUpper(n)] = b
		m.names[strings.ToUpper(n)] = n
	}
	for n, c := range s.clips {
		if c.size == -1 {
			continue
		}
		p := "BDMV/STREAM/" + n + ".m2ts"
		e := FileExtent{Path: "synthetic.iso", Offset: c.offset, Length: c.fileSize()}
		if s.folderLike {
			e = FileExtent{Path: "synthetic/" + n + ".m2ts", Length: c.fileSize()}
		}
		m.streams[strings.ToUpper(p)] = e
		m.names[strings.ToUpper(p)] = p
	}
	return m
}

func (m *memSource) Describe() string { return "a synthetic disc" }
func (m *memSource) Close() error     { return nil }
func (m *memSource) ReadFile(name string, max int64) ([]byte, error) {
	m.reads++
	b, ok := m.files[strings.ToUpper(name)]
	if !ok {
		return nil, errors.New(name + ": not on this disc")
	}
	if int64(len(b)) > max {
		b = b[:max]
	}
	return b, nil
}
func (m *memSource) Extents(name string) ([]FileExtent, error) {
	if e, ok := m.streams[strings.ToUpper(name)]; ok {
		return []FileExtent{e}, nil
	}
	return nil, errors.New(name + ": not on this disc")
}
func (m *memSource) List(dir string) ([]string, error) {
	prefix := strings.ToUpper(strings.Trim(dir, "/")) + "/"
	var out []string
	for up, n := range m.names {
		if strings.HasPrefix(up, prefix) && !strings.Contains(up[len(prefix):], "/") {
			out = append(out, n[len(prefix):])
		}
	}
	sort.Strings(out)
	return out, nil
}

// open opens the disc from memory.
func (s *synDisc) open(t testing.TB) *Disc {
	t.Helper()
	d, err := OpenSource(s.source())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// streamBytes is a small stream file of valid packets for a clip written to
// disk: as many packets as the clip claims, up to a megabyte, so the clip
// information has to be consistent with it (see writeFolder).
func synPackets(n int) []byte { return stream(n) }

// writeFolder writes the disc as a BDMV folder. Stream files are written
// whole, so the clips must be small: shrink() first.
func (s *synDisc) writeFolder(t testing.TB) string {
	t.Helper()
	return s.writeFolderAt(t, t.TempDir())
}

// writeFolderAt writes the disc as a BDMV folder in dir.
func (s *synDisc) writeFolderAt(t testing.TB, dir string) string {
	t.Helper()
	write := func(name string, data []byte) {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for n, b := range s.structures() {
		write(n, b)
	}
	for n, c := range s.clips {
		if c.size == -1 {
			continue
		}
		size := c.fileSize()
		if size > 8<<20 {
			t.Fatalf("clip %s is %d bytes: shrink() the disc before writing it out", n, size)
		}
		write("BDMV/STREAM/"+n+".m2ts", synPackets(int(size/192)))
	}
	return dir
}

// shrink makes every clip's stream file a few packets long, with clip
// information to match, so the disc can be written to a folder or an image.
func (s *synDisc) shrink() *synDisc {
	for _, c := range s.clips {
		c.packets = 64
	}
	return s
}

// jar builds a JAR file holding one class whose constants are the given
// strings and whose one method pushes the given numbers.
func synJar(t testing.TB, strs []string, nums []int) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("com/example/Menu.class")
	if err != nil {
		t.Fatal(err)
	}
	w.Write(synClass(strs, nums))
	w, _ = zw.Create("META-INF/MANIFEST.MF")
	w.Write([]byte("Manifest-Version: 1.0\n"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// synClass writes a class file: constant pool, no fields, one method with a
// Code attribute.
func synClass(strs []string, nums []int) []byte {
	var cp []byte
	n := 1
	utf := func(s string) int {
		cp = append(cp, 1)
		cp = append(cp, be16(len(s))...)
		cp = append(cp, s...)
		n++
		return n - 1
	}
	code := utf("Code")
	name := utf("run")
	desc := utf("()V")
	this := utf("Menu")
	cp = append(cp, 7)
	cp = append(cp, be16(this)...)
	thisClass := n
	n++
	var body []byte
	for _, s := range strs {
		u := utf(s)
		cp = append(cp, 8)
		cp = append(cp, be16(u)...)
		body = append(body, 0x13, byte(n>>8), byte(n), 0x57) // ldc_w, pop
		n++
	}
	for _, v := range nums {
		switch {
		case v >= -128 && v <= 127:
			body = append(body, 0x10, byte(v), 0x57)
		case v >= -32768 && v <= 32767:
			body = append(body, 0x11, byte(v>>8), byte(v), 0x57)
		default:
			cp = append(cp, 3)
			cp = append(cp, be32(uint32(v))...)
			body = append(body, 0x13, byte(n>>8), byte(n), 0x57)
			n++
		}
	}
	body = append(body, 0xb1) // return
	attr := cat(be16(2), be16(1), be32(uint32(len(body))), body, be16(0), be16(0))
	method := cat(be16(1), be16(name), be16(desc), be16(1), be16(code), be32(uint32(len(attr))), attr)
	return cat(be32(0xcafebabe), be16(0), be16(47), be16(n), cp,
		be16(0x21), be16(thisClass), be16(0), be16(0), // access, this, super, interfaces
		be16(0),         // fields
		be16(1), method, // methods
		be16(0)) // attributes
}

// bdjo builds a BD-J object with one application in the named JAR and the
// given accessible playlists.
func synBDJO(jar string, playlists ...int) []byte {
	b := make([]byte, 48)
	copy(b, "BDJO0200")
	term := cat(be32(0), []byte("*****"), []byte{0x10}, make([]byte, 4))
	cache := cat(be32(0), []byte{1, 0}, []byte{1}, []byte(jar), []byte("*.*"), make([]byte, 3))
	access := cat(be32(0), be32(uint32(len(playlists))<<21))
	for _, p := range playlists {
		access = append(access, fmt.Sprintf("%05d", p)...)
		access = append(access, 0)
	}
	text := func(s string) []byte {
		out := append([]byte{byte(len(s))}, s...)
		if len(s)%2 == 0 {
			out = append(out, 0)
		}
		return out
	}
	app := cat([]byte{1, 0x10}, be32(0x7fff0001), be16(0x4000), make([]byte, 10), be16(0), []byte{1, 0xc0}, be16(0),
		text(""), be16(0), text(jar), text(""), text("com.example.Menu"), []byte{0, 0})
	apps := cat(be32(0), []byte{1, 0}, app)
	return cat(b, term, cache, access, apps, be32(0), be16(0))
}
