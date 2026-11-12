// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// A UDF 2.50 image writer for tests: the layout a Blu-ray uses, with a
// physical partition, a metadata partition (a file whose blocks hold every
// directory and file entry, and a mirror of it), extended file entries, and
// long allocation descriptors that a file of many gigabytes needs several
// of. Stream files are holes in a sparse file unless the test gives them
// bytes, so an image the size of an Ultra HD disc costs a few kilobytes.
// Descriptor tags carry their identifier and location and no checksum,
// which the reader in WittyDisc does not ask for.

const udfSector = 2048

type udfOptions struct {
	// breakMetadataFile damages the metadata file's entry, so the reader
	// must turn to the mirror.
	breakMetadataFile bool
	// embed names files (upper-case paths) whose bytes are kept inside
	// their file entry, as UDF allows for small files.
	embed map[string]bool
	// chainADs gives each file entry room for only this many allocation
	// descriptors before it continues in another block (0 = no limit).
	chainADs int
}

type udfFile struct {
	path string
	data []byte // nil for a hole of `size` bytes
	size int64
	head []byte // for a hole: bytes written at its start, to tell it by
}

type udfWriter struct {
	t       testing.TB
	f       *os.File
	part    int64 // first sector of the physical partition
	meta    [][]byte
	next    int64 // next free block of the physical partition
	opt     udfOptions
	extents map[string][]int64 // path -> [block, bytes] pairs of each file's data
}

func udfTag(b []byte, id uint16, loc uint32) {
	binary.LittleEndian.PutUint16(b[0:], id)
	binary.LittleEndian.PutUint16(b[2:], 3)
	binary.LittleEndian.PutUint32(b[12:], loc)
}

func udfLongAD(length uint32, block uint32, part uint16) []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b[0:], length)
	binary.LittleEndian.PutUint32(b[4:], block)
	binary.LittleEndian.PutUint16(b[8:], part)
	return b
}

func (w *udfWriter) sectorAt(sector int64, b []byte) {
	if _, err := w.f.WriteAt(b, sector*udfSector); err != nil {
		w.t.Fatal(err)
	}
}

// metaBlock reserves a block of the metadata partition.
func (w *udfWriter) metaBlock() (int, []byte) {
	b := make([]byte, udfSector)
	w.meta = append(w.meta, b)
	return len(w.meta) - 1, b
}

// entry writes an extended file entry into a metadata block.
func (w *udfWriter) entry(b []byte, block int, fileType byte, size int64, adType uint16, ads []byte) {
	udfTag(b, 266, uint32(block))
	b[16+11] = fileType
	binary.LittleEndian.PutUint16(b[16+18:], adType)
	binary.LittleEndian.PutUint64(b[56:], uint64(size))
	binary.LittleEndian.PutUint32(b[212:], uint32(len(ads)))
	copy(b[216:], ads)
}

// place gives a file's data room in the physical partition and returns its
// allocation descriptors (long, naming partition 0).
func (w *udfWriter) place(f *udfFile) []byte {
	size := f.size
	if f.data != nil {
		size = int64(len(f.data))
		w.sectorAt(w.part+w.next, f.data)
	} else if len(f.head) > 0 {
		w.sectorAt(w.part+w.next, f.head)
	}
	var ads []byte
	const maxExtent = 0x3ffff800 // the most one descriptor holds, in whole sectors
	for left, block := size, w.next; left > 0; {
		l := min(left, maxExtent)
		ads = append(ads, udfLongAD(uint32(l), uint32(block), 0)...)
		w.extents[f.path] = append(w.extents[f.path], block, l)
		block += (l + udfSector - 1) / udfSector
		left -= l
	}
	w.next += (size+udfSector-1)/udfSector + 16 // a gap, as between files on a disc
	return ads
}

// fileEntry writes a file's entry and returns its metadata block.
func (w *udfWriter) fileEntry(f *udfFile) int {
	block, b := w.metaBlock()
	if w.opt.embed[strings.ToUpper(f.path)] && f.data != nil && len(f.data) < udfSector-216 {
		w.entry(b, block, 5, int64(len(f.data)), 3, f.data)
		return block
	}
	ads := w.place(f)
	size := f.size
	if f.data != nil {
		size = int64(len(f.data))
	}
	if n := w.opt.chainADs; n > 0 && len(ads) > n*16 {
		// The first descriptors, then one of type 3 naming a block of the
		// metadata partition that holds the rest.
		more, mb := w.metaBlock()
		rest := ads[n*16:]
		udfTag(mb, 258, uint32(more))
		binary.LittleEndian.PutUint32(mb[20:], uint32(len(rest)))
		copy(mb[24:], rest)
		ads = append(append([]byte(nil), ads[:n*16]...), udfLongAD(3<<30|udfSector, uint32(more), 1)...)
	}
	w.entry(b, block, 5, size, 1, ads)
	return block
}

// dir writes a directory and everything under it; returns its block.
func (w *udfWriter) dir(prefix string, files []*udfFile, parent int) int {
	block, b := w.metaBlock()
	if parent < 0 {
		parent = block
	}
	fid := func(name string, chars byte, icb int) []byte {
		var nb []byte
		if name != "" {
			nb = append([]byte{8}, name...)
		}
		e := make([]byte, 38)
		udfTag(e, 257, 0)
		binary.LittleEndian.PutUint16(e[16:], 1)
		e[18], e[19] = chars, byte(len(nb))
		copy(e[20:], udfLongAD(udfSector, uint32(icb), 1))
		e = append(e, nb...)
		for len(e)%4 != 0 {
			e = append(e, 0)
		}
		return e
	}
	data := fid("", 8|2, parent)
	subs := map[string][]*udfFile{}
	var names []string
	for _, f := range files {
		rest := strings.TrimPrefix(f.path, prefix)
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			if subs[rest[:i]] == nil {
				names = append(names, rest[:i])
			}
			subs[rest[:i]] = append(subs[rest[:i]], f)
			continue
		}
		data = append(data, fid(rest, 0, w.fileEntry(f))...)
	}
	sort.Strings(names)
	for _, n := range names {
		data = append(data, fid(n, 2, w.dir(prefix+n+"/", subs[n], block))...)
	}
	// The directory's own data: blocks of the metadata partition.
	first := len(w.meta)
	for off := 0; off < len(data); off += udfSector {
		_, db := w.metaBlock()
		copy(db, data[off:])
	}
	ad := make([]byte, 8)
	binary.LittleEndian.PutUint32(ad[0:], uint32(len(data)))
	binary.LittleEndian.PutUint32(ad[4:], uint32(first))
	w.entry(b, block, 4, int64(len(data)), 0, ad)
	return block
}

// writeUDFImage writes the files into a UDF 2.50 image and returns where
// each file's data lies (sector and length pairs) for checking.
func writeUDFImage(t testing.TB, path, label string, files []*udfFile, opt udfOptions) map[string][]int64 {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := &udfWriter{t: t, f: f, part: 288, opt: opt, extents: map[string][]int64{}}
	sort.Slice(files, func(i, j int) bool { return false }) // the order given is the order on the disc
	// The metadata partition is built in memory first: its size decides
	// where file data may start. File data goes after room for it and its
	// mirror; 4,096 blocks hold more entries than any test disc has.
	const metaRoom = 16384
	w.next = 2 + 2*metaRoom
	_, fsd := w.metaBlock()
	udfTag(fsd, 256, 0)
	root := w.dir("", files, -1)
	copy(fsd[400:], udfLongAD(udfSector, uint32(root), 1))
	if len(w.meta) > metaRoom {
		t.Fatalf("%d metadata blocks", len(w.meta))
	}
	for i, b := range w.meta {
		w.sectorAt(w.part+2+int64(i), b)
		w.sectorAt(w.part+2+metaRoom+int64(i), b)
	}
	// The metadata file and its mirror: entries in the physical partition
	// whose one extent is the blocks above.
	for i, at := range []int64{2, 2 + metaRoom} {
		b := make([]byte, udfSector)
		ad := make([]byte, 8)
		binary.LittleEndian.PutUint32(ad[0:], uint32(len(w.meta)*udfSector))
		binary.LittleEndian.PutUint32(ad[4:], uint32(at))
		udfTag(b, 266, uint32(i))
		b[16+11] = byte(250 + i)
		binary.LittleEndian.PutUint64(b[56:], uint64(len(w.meta)*udfSector))
		binary.LittleEndian.PutUint32(b[212:], 8)
		copy(b[216:], ad)
		if i == 0 && opt.breakMetadataFile {
			b = make([]byte, udfSector)
		}
		w.sectorAt(w.part+int64(i), b)
	}
	// Volume recognition, the volume descriptors and the anchor.
	for i, id := range []string{"BEA01", "NSR03", "TEA01"} {
		b := make([]byte, udfSector)
		copy(b[1:], id)
		b[6] = 1
		w.sectorAt(16+int64(i), b)
	}
	pvd := make([]byte, udfSector)
	udfTag(pvd, 1, 32)
	pvd[24] = 8
	copy(pvd[25:], label)
	pvd[55] = byte(len(label) + 1)
	pd := make([]byte, udfSector)
	udfTag(pd, 5, 33)
	binary.LittleEndian.PutUint32(pd[188:], uint32(w.part))
	binary.LittleEndian.PutUint32(pd[192:], uint32(w.next))
	lvd := make([]byte, udfSector)
	udfTag(lvd, 6, 34)
	binary.LittleEndian.PutUint32(lvd[212:], udfSector)
	copy(lvd[248:], udfLongAD(udfSector, 0, 1)) // the file set descriptor: block 0 of the metadata partition
	binary.LittleEndian.PutUint32(lvd[264:], 6+64)
	binary.LittleEndian.PutUint32(lvd[268:], 2)
	m := lvd[440:]
	m[0], m[1] = 1, 6 // type 1: the physical partition, number 0
	m = m[6:]
	m[0], m[1] = 2, 64 // type 2: the metadata partition
	copy(m[5:], "*UDF Metadata Partition")
	binary.LittleEndian.PutUint16(m[28:], 0x0250)
	binary.LittleEndian.PutUint32(m[40:], 0) // the metadata file's entry
	binary.LittleEndian.PutUint32(m[44:], 1) // its mirror's
	binary.LittleEndian.PutUint32(m[48:], 0xffffffff)
	td := make([]byte, udfSector)
	udfTag(td, 8, 35)
	for i, b := range [][]byte{pvd, pd, lvd, td} {
		w.sectorAt(32+int64(i), b)
	}
	avdp := make([]byte, udfSector)
	udfTag(avdp, 2, 256)
	binary.LittleEndian.PutUint32(avdp[16:], 4*udfSector)
	binary.LittleEndian.PutUint32(avdp[20:], 32)
	w.sectorAt(256, avdp)
	if err := f.Truncate((w.part + w.next + 257) * udfSector); err != nil {
		t.Fatal(err)
	}
	out := map[string][]int64{}
	for p, e := range w.extents {
		for i := 0; i < len(e); i += 2 {
			out[p] = append(out[p], (w.part+e[i])*udfSector, e[i+1])
		}
	}
	return out
}

// udfFiles lists a synthetic disc's files for an image: the program files
// with their bytes, the stream files in the order the clips lie on the
// disc. With whole, the stream files are holes of the size the clips say.
func (s *synDisc) udfFiles(whole bool) []*udfFile {
	var files []*udfFile
	st := s.structures()
	var names []string
	for n := range st {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		files = append(files, &udfFile{path: n, data: st[n]})
	}
	for _, n := range s.order {
		c := s.clips[n]
		if c.size == -1 {
			continue
		}
		f := &udfFile{path: "BDMV/STREAM/" + n + ".m2ts"}
		if whole {
			f.size, f.head = c.fileSize(), stream(64)
		} else {
			f.data = stream(int(c.fileSize() / 192))
		}
		files = append(files, f)
	}
	return files
}

// An image reads the same as the folder it was made from, and its stream
// files are where the image says, byte for byte.
func TestUDFImage(t *testing.T) {
	s, film := smallDisc(31)
	s.extra["AACS_NOT/readme.txt"] = []byte("not an AACS folder")
	dir := t.TempDir()
	for name, opt := range map[string]udfOptions{
		"plain":                  {},
		"metadata mirror":        {breakMetadataFile: true},
		"chained descriptors":    {chainADs: 1},
		"small files in entries": {embed: map[string]bool{"BDMV/INDEX.BDMV": true, "BDMV/MOVIEOBJECT.BDMV": true}},
	} {
		t.Run(name, func(t *testing.T) {
			img := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".iso")
			where := writeUDFImage(t, img, "SYNTH_DISC", s.udfFiles(false), opt)
			d, err := Open(img)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if d.Label != "SYNTH_DISC" || len(d.Playlists) != len(s.playlists) || len(d.Clips) != len(s.clips) {
				t.Errorf("label %q, %d playlists, %d clips; damaged %v", d.Label, len(d.Playlists), len(d.Clips), d.Damaged)
			}
			// Files kept inside their entries are read by WittyDisc, which
			// places them right from 0.2.1.
			if len(d.Damaged) != 0 || len(d.FromBackup) != 0 {
				t.Errorf("damaged %v, from backup %v", d.Damaged, d.FromBackup)
			}
			if d.Encrypted || d.AACSFolder {
				t.Errorf("encrypted %v, AACS folder %v", d.Encrypted, d.AACSFolder)
			}
			r := d.Analyze(RankOptions{})
			if r.Main != 800 || r.FirstPlay != 800 || !sameClips(d.MainPlaylist(), film) {
				t.Errorf("main %d, first play %d\n%s", r.Main, r.FirstPlay, r.Explain())
			}
			raw, _ := os.ReadFile(img)
			for _, n := range film {
				ext, err := d.ClipExtents(n)
				want := where["BDMV/STREAM/"+n+".m2ts"]
				if err != nil || len(ext) != 1 || ext[0].Offset != want[0] || ext[0].Length != want[1] {
					t.Fatalf("clip %s: %+v %v, want %v", n, ext, err, want)
				}
				if got := raw[ext[0].Offset : ext[0].Offset+ext[0].Length]; string(got) != string(stream(len(got)/192)) {
					t.Errorf("clip %s reads other bytes than were written", n)
				}
			}
			for _, x := range d.Consistency() {
				if x.Kind != "bitrate" { // the test's stream files are a few packets long
					t.Errorf("inconsistency: %+v", x)
				}
			}
		})
	}
}

// An image the size of a 100 GB Ultra HD disc: stream files far past 4 GB
// (each in many extents), the image itself past what 32 bits of sectors
// times 512 would hold, and a film told from its decoys only by where its
// clips lie on the disc.
func TestUDFImageOfUltraHDSize(t *testing.T) {
	if testing.Short() {
		t.Skip("writes a sparse 90 GB image")
	}
	if runtime.GOOS == "windows" {
		t.Skip("needs a file system with sparse files")
	}
	s := newSyn(32)
	s.version = "0300"
	film := s.film(6, 1150, evidence{layout: true})
	for _, n := range film {
		c := s.clips[n]
		c.coding, c.format = 0x24, 8
		c.packets = uint32(int64(ticks45(c.end-c.start)*11.5e6) / 192) // 92 Mbit/s
	}
	s.feature(1, film).hdr = 1
	s.decoys(film, 40, 2, "permute")
	img := filepath.Join(t.TempDir(), "uhd.iso")
	where := writeUDFImage(t, img, "UHD_SYNTH", s.udfFiles(true), udfOptions{chainADs: 5})
	st, err := os.Stat(img)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() < 70<<30 {
		t.Fatalf("the image is only %d bytes", st.Size())
	}
	d, err := Open(img)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var total int64
	for _, n := range film {
		ext, err := d.ClipExtents(n)
		if err != nil {
			t.Fatal(err)
		}
		var size int64
		for i, e := range ext {
			size += e.Length
			if e.Offset != where["BDMV/STREAM/"+n+".m2ts"][2*i] {
				t.Fatalf("clip %s, extent %d at %d, want %d", n, i, e.Offset, where["BDMV/STREAM/"+n+".m2ts"][2*i])
			}
		}
		if want := s.clips[n].fileSize(); size != want || want < 8<<30 || len(ext) < 9 {
			t.Fatalf("clip %s: %d bytes in %d extents, want %d", n, size, len(ext), want)
		}
		total += size
	}
	// The last clip begins some 56 GB into the file, and reads there.
	last, _ := d.ClipExtents(film[len(film)-1])
	if last[0].Offset < 48<<30 {
		t.Errorf("the last clip starts at %d", last[0].Offset)
	}
	buf := make([]byte, 192*4)
	cr := &clipReader{ext: last}
	defer cr.close()
	if n, _ := cr.readAt(buf, 0); n != len(buf) || buf[4] != 0x47 || buf[192+4] != 0x47 {
		t.Errorf("reading the last clip's first packets: %d bytes, sync %x", n, buf[4])
	}
	r := d.Analyze(RankOptions{})
	c := r.Candidate(1)
	if r.Main != 1 || len(r.Decoys) != 40 || !r.UHD || r.HDR != "HDR10" || c.Size != total {
		t.Errorf("main %d, %d decoys, UHD %v, HDR %q, size %d of %d\n%s", r.Main, len(r.Decoys), r.UHD, r.HDR, c.Size, total, r.Explain())
	}
	if c.Continuity < 0.79 {
		t.Errorf("the layout on the disc was not used: continuity %v", c.Continuity)
	}
	if d.Encrypted || len(d.Consistency()) != 0 {
		t.Errorf("encrypted %v, inconsistencies %+v", d.Encrypted, d.Consistency())
	}
}

// Opening a real image of a disc with 2,000 playlists: the UDF directory,
// 4,000 structure files and their backups, and the ranking.
func BenchmarkOpenImage2000Playlists(b *testing.B) {
	s, feature := bigDisc(2000)
	img := filepath.Join(b.TempDir(), "big.iso")
	writeUDFImage(b, img, "BIG", s.udfFiles(true), udfOptions{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d, err := Open(img)
		if err != nil {
			b.Fatal(err)
		}
		if mp := d.MainPlaylist(); mp == nil || mp.Number != feature {
			b.Fatalf("main %v, want %d", mp, feature)
		}
		d.Close()
	}
}
