// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/wittywilla/wittydisc/isofs"
)

// Opening a disc: an .iso/.img image or a folder that holds BDMV. The
// disc's program (index, movie objects), playlists and clip information are
// read at once; the stream files are only ever described as byte extents,
// never copied.

// MaxStructureBytes bounds how much of any .bdmv/.mpls/.clpi file is read.
const MaxStructureBytes = 16 << 20

// FileExtent is a byte range of a stream file: inside the image, or a file
// on disk when the disc is a folder.
type FileExtent struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
}

// Source abstracts an image or a BDMV folder.
type Source interface {
	ReadFile(name string, max int64) ([]byte, error) // name is relative to the disc root, e.g. BDMV/index.bdmv
	Extents(name string) ([]FileExtent, error)
	List(dir string) ([]string, error) // file names in a folder of the disc
	Describe() string
	Close() error
}

// Disc is an opened Blu-ray.
type Disc struct {
	Source       Source
	Label        string
	Index        *Index
	MovieObjects []MovieObject
	Playlists    map[int]*Playlist
	Clips        map[string]*Clip
	// Encrypted is set when the disc's streams are scrambled (AACS or
	// otherwise). The engine never decrypts anything: such discs are refused
	// for playback. An AACS folder alone does not count, because a decrypted
	// copy usually keeps it; what matters is whether the video is readable.
	Encrypted bool
	// AACSFolder: the disc has an AACS folder. BDPlus: it has a BD+ program
	// (BDSVM). The engine does not run BD+ either; on such a copy some of the
	// picture may be damaged by design, which only the disc's own conversion
	// repairs.
	AACSFolder, BDPlus bool
	// Damaged names structures that could not be read from the disc or their
	// backups, and which were done without.
	Damaged []string
	// Recording: a BDAV or similar recording with no disc program; each clip
	// was made a playlist.
	Recording bool
	// Stereo3D: the disc has 3D (MVC) content: an SSIF folder, a dependent
	// view stream, or the index's 3D flag.
	Stereo3D bool
	// BDJ: the disc's first play, top menu or a title is a BD-J (Java)
	// program. Those run in a separate process when the server has Java (see
	// bdj.go); otherwise the disc plays from its playlists.
	BDJ bool

	// FromBackup names the structures read from the BACKUP folder because
	// the first copy was missing, damaged, or contradicted the rest of the
	// disc where the backup did not.
	FromBackup []string
	// Notes are remarks on how the disc was read that are not damage: a
	// backup that differs from the first copy, a copy with no disc program,
	// structures left out because the disc holds more than a disc can.
	Notes []string
	// MovieOnly: the copy has playlists and clips but neither index.bdmv nor
	// MovieObject.bdmv (a "movie only" copy). There is no disc program to
	// run or read; the playlists' structure alone decides the feature.
	MovieOnly bool

	layout     layout
	stn        *stnCache
	offsets    map[string]int64
	sameFile   bool
	consOnce   sync.Once
	cons       []Inconsistency
	bdjOnce    sync.Once
	bdjHints   *BDJHints
	metaOnce   sync.Once
	meta       *DiscMeta
	soundsOnce sync.Once
	sounds     *Sounds
	// The stream files' sizes and the main playlist, worked out once.
	sizesOnce sync.Once
	sizes     map[string]int64
	mainOnce  sync.Once
	main      *Playlist
}

// layout is where a disc keeps its files. Blu-ray discs use long names under
// BDMV; AVCHD discs and camcorder cards use 8.3 names, often under
// PRIVATE/AVCHD; recordings use a BDAV folder.
type layout struct {
	root             string
	index, mobj      string
	playlists, clips []string // file extensions, in order of preference
	streams          []string
}

var (
	longNames  = layout{index: "index.bdmv", mobj: "MovieObject.bdmv", playlists: []string{".mpls"}, clips: []string{".clpi"}, streams: []string{".m2ts", ".mts", ".m2t"}}
	shortNames = layout{index: "INDEX.BDM", mobj: "MOVIEOBJ.BDM", playlists: []string{".mpl", ".mpls"}, clips: []string{".cpi", ".clpi"}, streams: []string{".mts", ".m2ts", ".m2t"}}
)

// findLayout looks for a disc's structure in the places discs and cards put
// it. ok is false when there is none.
func findLayout(exists func(string) bool) (layout, bool) {
	for _, root := range []string{"BDMV", "PRIVATE/AVCHD/BDMV", "AVCHD/BDMV", "PRIVATE/SONY/AVCHD/BDMV"} {
		for _, l := range []layout{longNames, shortNames} {
			if exists(root + "/" + l.index) {
				l.root = root
				return l, true
			}
		}
	}
	for _, root := range []string{"BDAV", "BDMV"} { // a recording, or a disc with no program left
		if exists(root+"/CLIPINF") || exists(root+"/info.bdav") {
			l := longNames
			l.root = root
			return l, true
		}
	}
	return layout{}, false
}

// Open opens an image file or a disc folder (the folder that contains BDMV,
// or BDMV itself).
func Open(p string) (*Disc, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	var src Source
	var lay layout
	var ok bool
	if st.IsDir() {
		dir := p
		if strings.EqualFold(filepath.Base(dir), "BDMV") || strings.EqualFold(filepath.Base(dir), "BDAV") {
			dir = filepath.Dir(dir)
		}
		fs := &folderSource{dir: dir}
		lay, ok = findLayout(func(n string) bool { _, err := os.Stat(fs.path(n)); return err == nil })
		if !ok {
			return nil, fmt.Errorf("%s: no BDMV/index.bdmv (not a Blu-ray folder)", p)
		}
		src = fs
	} else {
		img, err := isofs.Open(p)
		if err != nil {
			return nil, err
		}
		lay, ok = findLayout(func(n string) bool {
			if _, found := img.Find(n); found {
				return true
			}
			// Folders are not always listed by themselves.
			up := strings.ToUpper(n) + "/"
			for _, f := range img.Files() {
				if strings.HasPrefix(strings.ToUpper(f.Path), up) {
					return true
				}
			}
			return false
		})
		if !ok {
			img.Close()
			return nil, fmt.Errorf("%s: no BDMV/index.bdmv (not a Blu-ray image)", p)
		}
		src = &imageSource{img: img}
	}
	d := &Disc{Source: src, Playlists: map[int]*Playlist{}, Clips: map[string]*Clip{}, layout: lay}
	if err := d.load(); err != nil {
		src.Close()
		return nil, err
	}
	return d, nil
}

// OpenSource opens a disc from any Source: a reader of the caller's own
// (a network share, an archive, a test's synthetic disc). The disc is laid
// out as on a Blu-ray, with BDMV at the root. Close closes the source.
func OpenSource(src Source) (*Disc, error) {
	if src == nil {
		return nil, errors.New("no source to open")
	}
	lay, ok := findLayout(func(n string) bool {
		if names, err := src.List(n); err == nil && len(names) > 0 {
			return true
		}
		dir, base := path.Split(n)
		names, _ := src.List(strings.TrimSuffix(dir, "/"))
		for _, f := range names {
			if strings.EqualFold(f, base) {
				return true
			}
		}
		return false
	})
	if !ok {
		return nil, fmt.Errorf("%s: no BDMV/index.bdmv (not a Blu-ray)", src.Describe())
	}
	d := &Disc{Source: src, Playlists: map[int]*Playlist{}, Clips: map[string]*Clip{}, layout: lay}
	if err := d.load(); err != nil {
		return nil, err
	}
	return d, nil
}

// Close releases the image.
func (d *Disc) Close() error { return d.Source.Close() }

// readStructure reads a file of the disc's program, and its copy in the
// BACKUP folder (every Blu-ray has one) when it is missing or does not parse.
func readStructure[T any](d *Disc, rel string, parse func([]byte) (T, error)) (T, error) {
	var zero T
	b, err := d.Source.ReadFile(d.layout.root+"/"+rel, MaxStructureBytes)
	if err == nil {
		var v T
		if v, err = parse(b); err == nil {
			return v, nil
		}
	}
	if v, berr := readBackup(d, rel, parse); berr == nil {
		d.FromBackup = append(d.FromBackup, rel)
		return v, nil
	}
	return zero, err
}

// readBackup reads only the BACKUP copy of a structure.
func readBackup[T any](d *Disc, rel string, parse func([]byte) (T, error)) (T, error) {
	var zero T
	bb, err := d.Source.ReadFile(d.layout.root+"/BACKUP/"+rel, MaxStructureBytes)
	if err != nil {
		return zero, err
	}
	v, err := parse(bb)
	if err != nil {
		return zero, err
	}
	return v, nil
}

// listStructures names the files of a folder of the disc's program, with
// those only its backup still has.
func (d *Disc) listStructures(dir string, exts []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, base := range []string{d.layout.root + "/" + dir, d.layout.root + "/BACKUP/" + dir} {
		names, _ := d.Source.List(base)
		for _, n := range names {
			if _, ok := structureNumber(n, exts); ok && !seen[strings.ToUpper(n)] {
				seen[strings.ToUpper(n)] = true
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// What a whole disc may make the engine keep in memory. A commercial disc
// with playlist obfuscation has a few thousand playlists, a few hundred
// clips and some tens of thousands of play items; these are many times
// that, and what is past them is left out and named in Notes.
const (
	maxDiscPlaylists = 20000
	maxDiscClips     = 20000
	maxDiscItems     = 400000
	maxDiscMarks     = 1 << 20
	maxDiscStreams   = 1 << 20 // in distinct stream tables
	maxDiscEPEntries = 8 << 20
)

func (d *Disc) load() error {
	if s, ok := d.Source.(*imageSource); ok {
		d.Label = s.img.Label
	} else if s, ok := d.Source.(*folderSource); ok {
		d.Label = filepath.Base(s.dir)
	}
	d.stn = newSTNCache()
	var ixErr, moErr error
	if d.Index, ixErr = readStructure(d, d.layout.index, ParseIndex); ixErr != nil {
		// No readable program: the streams can still be played from their
		// clips. Only a recording is expected to be like this.
		d.Index = &Index{}
		if d.layout.root == "BDAV" {
			d.Recording = true
		}
	}
	if d.MovieObjects, moErr = readStructure(d, d.layout.mobj, ParseMovieObjects); moErr != nil {
		d.MovieObjects = nil
	}
	if !d.Recording {
		top, _ := d.Source.List(d.layout.root)
		bak, _ := d.Source.List(d.layout.root + "/BACKUP")
		has := func(name string) bool {
			for _, n := range append(top, bak...) {
				if strings.EqualFold(n, name) {
					return true
				}
			}
			return false
		}
		switch {
		case ixErr != nil && moErr != nil && !has(d.layout.index) && !has(d.layout.mobj):
			d.MovieOnly = true
			d.Notes = append(d.Notes, "This copy has no index.bdmv and no MovieObject.bdmv (a movie-only copy): there is no disc program, so the playlists' structure decides what the feature is.")
		default:
			if ixErr != nil {
				d.Damaged = append(d.Damaged, d.layout.index)
			}
			if moErr != nil {
				d.Damaged = append(d.Damaged, d.layout.mobj)
			}
		}
	}
	items, marks, left := 0, 0, 0
	for _, n := range d.listStructures("PLAYLIST", d.layout.playlists) {
		num, _ := structureNumber(n, d.layout.playlists)
		if len(d.Playlists) >= maxDiscPlaylists || items >= maxDiscItems || marks >= maxDiscMarks || d.stn.streams >= maxDiscStreams {
			left++
			continue
		}
		pl, err := readStructure(d, "PLAYLIST/"+n, func(b []byte) (*Playlist, error) { return parsePlaylist(num, b, d.stn) })
		if err != nil {
			d.Damaged = append(d.Damaged, n) // one damaged playlist should not stop the disc
			continue
		}
		items += len(pl.Items)
		marks += len(pl.Marks)
		d.Playlists[num] = pl
	}
	if left > 0 {
		d.Notes = append(d.Notes, fmt.Sprintf("%d playlists were left out: the disc holds more playlists, play items or streams than a disc can.", left))
	}
	entries, left := 0, 0
	for _, n := range d.listStructures("CLIPINF", d.layout.clips) {
		if len(d.Clips) >= maxDiscClips {
			left++
			continue
		}
		name := strings.TrimSuffix(n, filepath.Ext(n))
		c, err := readStructure(d, "CLIPINF/"+n, func(b []byte) (*Clip, error) { return ParseClip(name, b) })
		if err != nil {
			d.Damaged = append(d.Damaged, n)
			continue
		}
		for i := range c.EPMaps {
			entries += len(c.EPMaps[i].Entries)
		}
		if entries > maxDiscEPEntries {
			// Past the budget a clip keeps its length and time bases and
			// loses its entry-point map; seeking in it starts from its head.
			for i := range c.EPMaps {
				entries -= len(c.EPMaps[i].Entries)
			}
			c.EPMaps = nil
			c.Warnings = append(c.Warnings, "entry-point map: left out, the disc's maps together are larger than a disc can hold")
		}
		d.Clips[c.Name] = c
	}
	if left > 0 {
		d.Notes = append(d.Notes, fmt.Sprintf("%d clips were left out: the disc holds more clips than a disc can.", left))
	}
	d.repairFromBackup()
	if len(d.Playlists) == 0 {
		// A recording, or a disc whose playlists are all gone: every clip
		// plays by itself.
		d.synthesizePlaylists()
		if len(d.Playlists) > 0 && !d.Recording {
			d.Notes = append(d.Notes, "No playlist could be read: each clip was made a playlist of its own.")
		}
	}
	if len(d.Playlists) == 0 {
		return errors.New("the disc has no playlists")
	}
	d.BDJ = d.Index.FirstPlay.Kind == ObjectBDJ || d.Index.TopMenu.Kind == ObjectBDJ
	for _, t := range d.Index.Titles {
		if t.Object.Kind == ObjectBDJ {
			d.BDJ = true
		}
	}
	if ssif, _ := d.Source.List(d.layout.root + "/STREAM/SSIF"); len(ssif) > 0 {
		d.Stereo3D = true
	}
	if d.Index.Stereo3D {
		d.Stereo3D = true
	}
	for _, pl := range d.Playlists {
		if pl.Stereo3D {
			d.Stereo3D = true
		}
	}
	aacs, _ := d.Source.List("AACS")
	d.AACSFolder = len(aacs) > 0
	svm, _ := d.Source.List("BDSVM")
	d.BDPlus = len(svm) > 0
	var sampled bool
	d.Encrypted, sampled = d.streamsScrambled()
	if !sampled && d.AACSFolder {
		d.Encrypted = true // nothing could be checked: assume the worst
	}
	return nil
}

// playlistFaults counts what in a playlist contradicts the clips: play
// items whose clip has no information, or whose times lie outside the
// clip's time base.
func (d *Disc) playlistFaults(pl *Playlist) int {
	n := 0
	for i := range pl.Items {
		it := &pl.Items[i]
		c := d.Clips[it.Clip]
		switch {
		case c == nil:
			n++
		case it.Out <= it.In:
			n++
		default:
			if end := c.EndPTS(it.STCID); end > 0 && it.Out > end+45000 {
				n++
			}
		}
	}
	return n
}

// repairFromBackup looks again at structures that parsed but contradict the
// rest of the disc. A file can be damaged and still parse: a playlist with
// a clip name or a time flipped, an index that names movie objects that are
// not there. When the BACKUP copy parses and has fewer such faults, it is
// the one used. Only suspect files are read twice, so a clean disc with
// thousands of playlists opens no slower.
func (d *Disc) repairFromBackup() {
	if len(d.Clips) > 0 {
		tried := 0
		for _, pl := range d.sortedPlaylists() {
			faults := d.playlistFaults(pl)
			if faults == 0 || tried >= 4096 {
				continue
			}
			tried++
			num := pl.Number
			for _, ext := range d.layout.playlists {
				rel := fmt.Sprintf("PLAYLIST/%05d%s", num, ext)
				bp, err := readBackup(d, rel, func(b []byte) (*Playlist, error) { return parsePlaylist(num, b, d.stn) })
				if err != nil {
					continue
				}
				if d.playlistFaults(bp) < faults {
					d.Playlists[num] = bp
					d.FromBackup = append(d.FromBackup, rel)
					d.Notes = append(d.Notes, fmt.Sprintf("Playlist %05d contradicts the clips it names and its BACKUP copy does not: the backup is used.", num))
				}
				break
			}
		}
	}
	// The index and the movie objects go together: an index naming objects
	// past the end of MovieObject.bdmv is a mismatched or damaged pair.
	bad := func(ix *Index, mo []MovieObject) int {
		if ix == nil {
			return 0
		}
		n := 0
		objs := []IndexObject{ix.FirstPlay, ix.TopMenu}
		for _, t := range ix.Titles {
			objs = append(objs, t.Object)
		}
		for _, o := range objs {
			if o.Kind == ObjectHDMV && o.MovieObject >= len(mo) {
				n++
			}
		}
		return n
	}
	if faults := bad(d.Index, d.MovieObjects); faults > 0 {
		bix, e1 := readBackup(d, d.layout.index, ParseIndex)
		bmo, e2 := readBackup(d, d.layout.mobj, ParseMovieObjects)
		best, pick := faults, 0
		if e2 == nil && bad(d.Index, bmo) < best {
			best, pick = bad(d.Index, bmo), 1
		}
		if e1 == nil && bad(bix, d.MovieObjects) < best {
			best, pick = bad(bix, d.MovieObjects), 2
		}
		if e1 == nil && e2 == nil && bad(bix, bmo) < best {
			pick = 3
		}
		if pick&1 != 0 {
			d.MovieObjects = bmo
			d.FromBackup = append(d.FromBackup, d.layout.mobj)
		}
		if pick&2 != 0 {
			d.Index = bix
			d.FromBackup = append(d.FromBackup, d.layout.index)
		}
		if pick != 0 {
			d.Notes = append(d.Notes, "The index names movie objects that MovieObject.bdmv does not have, and the BACKUP copies agree with each other: the backup is used.")
		} else {
			d.Notes = append(d.Notes, "The index names movie objects that MovieObject.bdmv does not have; those titles cannot run.")
		}
	} else if len(d.Damaged) == 0 && !d.MovieOnly && !d.Recording {
		// A backup that differs from a first copy that reads well is worth
		// a remark, nothing more: the first copy is what a player uses.
		for _, rel := range []string{d.layout.index, d.layout.mobj} {
			a, e1 := d.Source.ReadFile(d.layout.root+"/"+rel, MaxStructureBytes)
			b, e2 := d.Source.ReadFile(d.layout.root+"/BACKUP/"+rel, MaxStructureBytes)
			if e1 == nil && e2 == nil && string(a) != string(b) {
				d.Notes = append(d.Notes, rel+" and its BACKUP copy differ; the first copy is used.")
			}
		}
	}
}

// synthesizePlaylists makes one playlist per clip, with the streams the clip
// itself lists.
func (d *Disc) synthesizePlaylists() {
	names := make([]string, 0, len(d.Clips))
	for n := range d.Clips {
		names = append(names, n)
	}
	sort.Strings(names)
	for i, n := range names {
		c := d.Clips[n]
		if len(c.STC) == 0 {
			continue
		}
		num := i
		if v, err := strconv.Atoi(n); err == nil {
			num = v
		}
		it := PlayItem{Clip: n, Codec: "M2TS", STCID: c.STC[0].ID, In: c.STC[0].Start, Out: c.STC[0].End}
		it.Angles = []AngleClip{{Clip: n, Codec: "M2TS", STCID: it.STCID}}
		for _, p := range c.Programs {
			for _, cs := range p.Streams {
				st := Stream{EntryType: 1, PID: cs.PID, Coding: cs.Coding, Format: cs.Format, Rate: cs.Rate, Lang: cs.Lang, DynamicRange: cs.DynamicRange, ColorSpace: cs.ColorSpace}
				switch cs.Coding {
				case 0x01, 0x02, 0x1b, 0x24, 0xea:
					st.Kind = "video"
					it.Video = append(it.Video, st)
				case 0x03, 0x04, 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86:
					st.Kind = "audio"
					it.Audio = append(it.Audio, st)
				case 0x90:
					st.Kind = "pg"
					it.PG = append(it.PG, st)
				}
			}
		}
		pl := &Playlist{Number: num, Version: "0000", PlaybackType: 1, RandomAccess: true, Items: []PlayItem{it}}
		pl.Marks = []Mark{{Type: 1, Item: 0, Time: it.In, PID: 0xffff}}
		d.Playlists[num] = pl
	}
}

// How AACS shows in a stream. Blu-ray streams are written in aligned units
// of 32 source packets (6144 bytes), and only the first 16 bytes of an
// encrypted unit are in the clear: its first packet header survives, with
// the copy-permission bits at its very start set, and every other packet is
// noise with no sync byte. Damaged rips keep their sync bytes, and clear
// streams have those bits at zero, so it takes both to call a unit encrypted.
const alignedUnit = 32 * packetSize

func unitEncrypted(u []byte) bool {
	if len(u) < alignedUnit || u[0]&0xc0 == 0 {
		return false
	}
	lost := 0
	for i := packetSize; i+packetSize <= alignedUnit; i += packetSize {
		if u[i+4] != 0x47 {
			lost++
		}
	}
	return lost >= 16
}

// streamsScrambled samples the feature's clips and the disc's largest ones,
// at the start and in the middle (a clear intro can front an encrypted
// film). sampled is false when no stream could be read at all.
func (d *Disc) streamsScrambled() (scrambled, sampled bool) {
	var clips []string
	seen := map[string]bool{}
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			clips = append(clips, n)
		}
	}
	for _, pl := range d.sortedPlaylists() {
		if len(pl.Items) > 0 && len(clips) < 2 {
			add(pl.Items[0].Clip)
		}
	}
	bySize := make([]*Clip, 0, len(d.Clips))
	for _, c := range d.Clips {
		bySize = append(bySize, c)
	}
	sort.Slice(bySize, func(i, j int) bool {
		if bySize[i].Packets != bySize[j].Packets {
			return bySize[i].Packets > bySize[j].Packets
		}
		return bySize[i].Name < bySize[j].Name
	})
	for i := 0; i < len(bySize) && i < 3; i++ {
		add(bySize[i].Name)
	}
	for _, name := range clips {
		ext, err := d.ClipExtents(name)
		if err != nil || len(ext) == 0 {
			continue
		}
		var total int64
		for _, e := range ext {
			total += e.Length
		}
		cr := &clipReader{ext: ext}
		for _, at := range []int64{0, total / 2 / alignedUnit * alignedUnit} {
			buf := make([]byte, alignedUnit*8)
			n, _ := cr.readAt(buf, at)
			if n < alignedUnit {
				continue
			}
			sampled = true
			units, tsScrambled := 0, 0
			for u := 0; u+alignedUnit <= n; u += alignedUnit {
				if unitEncrypted(buf[u : u+alignedUnit]) {
					units++
				}
			}
			for i := 0; i+packetSize <= n; i += packetSize {
				if buf[i+4] == 0x47 && buf[i+7]&0xc0 != 0 {
					tsScrambled++
				}
			}
			if units >= 2 || tsScrambled > 16 {
				cr.close()
				return true, true
			}
		}
		cr.close()
	}
	return false, sampled
}

// Playlist returns a playlist by number.
func (d *Disc) Playlist(n int) *Playlist { return d.Playlists[n] }

// Clip returns a clip's information by name.
func (d *Disc) Clip(name string) *Clip { return d.Clips[name] }

// ClipExtents returns the byte extents of a clip's stream file.
func (d *Disc) ClipExtents(name string) ([]FileExtent, error) {
	// A clip's name comes from a playlist: it is five digits, not a path.
	if strings.ContainsAny(name, `/\:.`) {
		return nil, fmt.Errorf("bad clip name %q", name)
	}
	var err error
	for _, ext := range d.layout.streams {
		var out []FileExtent
		if out, err = d.Source.Extents(d.layout.root + "/STREAM/" + name + ext); err == nil {
			return out, nil
		}
	}
	return nil, err
}

// sortedPlaylists lists playlists by number.
func (d *Disc) sortedPlaylists() []*Playlist {
	out := make([]*Playlist, 0, len(d.Playlists))
	for _, pl := range d.Playlists {
		out = append(out, pl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// PlaylistDuration is the playlist's length in seconds.
func (d *Disc) PlaylistDuration(pl *Playlist) float64 {
	var t float64
	for i := range pl.Items {
		t += pl.Items[i].Duration()
	}
	return t
}

// MainPlaylist finds the feature with the playlist ranking engine
// (heuristics.go): distinct, readable video counts most, because longest is
// what some discs try to defeat: copy-protection schemes add hundreds of
// playlists that loop a clip, name clips that are not there or are empty,
// run past a clip's end, or shuffle the real film's clips into nonsense.
// Those are scored down or ruled out; among the rest the one the disc's
// program leads to, the one whose clips follow on from each other, and the
// one with even chapters gain. A disc with no video at all (a music disc)
// picks by sound. The answer is worked out once per opened disc.
func (d *Disc) MainPlaylist() *Playlist {
	d.mainOnce.Do(func() {
		if r := d.Analyze(RankOptions{}); r.Main >= 0 {
			d.main = d.Playlists[r.Main]
		}
	})
	return d.main
}

func structureNumber(name string, exts []string) (int, bool) {
	ok := false
	for _, e := range exts {
		ok = ok || strings.EqualFold(filepath.Ext(name), e)
	}
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSuffix(name, filepath.Ext(name)))
	return n, err == nil
}

// ExportProgram copies the disc's program files (not its video) into a
// folder laid out as the disc is, for a BD-J runtime that reads them as files.
// Every name is checked: a disc cannot place a file outside dst.
func (d *Disc) ExportProgram(dst string, maxBytes int64) error {
	var total int64
	put := func(rel string, read func() ([]byte, error)) error {
		rel = strings.ReplaceAll(rel, "\\", "/")
		if !filepath.IsLocal(filepath.FromSlash(rel)) {
			return fmt.Errorf("%q is not a file name a disc can have", rel)
		}
		b, err := read()
		if err != nil {
			return nil // one unreadable file should not stop the rest
		}
		if total += int64(len(b)); total > maxBytes {
			return errors.New("the disc's program files are too large")
		}
		p := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		return os.WriteFile(p, b, 0o644)
	}
	wanted := func(rel string) bool {
		up := strings.ToUpper(rel)
		root := strings.ToUpper(d.layout.root) + "/"
		if !strings.HasPrefix(up, root) {
			return false
		}
		return !strings.HasPrefix(up[len(root):], "STREAM/")
	}
	normal := func(rel string) string { return "BDMV" + rel[len(d.layout.root):] }
	switch s := d.Source.(type) {
	case *imageSource:
		for _, f := range s.img.Files() {
			if f.IsDir || !wanted(f.Path) {
				continue
			}
			name := f.Path
			if err := put(normal(name), func() ([]byte, error) { return s.img.ReadFile(name, maxBytes) }); err != nil {
				return err
			}
		}
	case *folderSource:
		root := s.path(d.layout.root)
		return filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, rerr := filepath.Rel(root, p)
			if rerr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if e.IsDir() {
				if strings.EqualFold(rel, "STREAM") {
					return filepath.SkipDir
				}
				return nil
			}
			if e.Type()&os.ModeSymlink != 0 {
				return nil
			}
			return put("BDMV/"+rel, func() ([]byte, error) { return os.ReadFile(p) })
		})
	}
	return nil
}

// ---------------------------------------------------------------- sources

type imageSource struct{ img *isofs.Disc }

func (s *imageSource) Describe() string { return s.img.Path }
func (s *imageSource) Close() error     { return s.img.Close() }
func (s *imageSource) ReadFile(name string, max int64) ([]byte, error) {
	return s.img.ReadFile(name, max)
}
func (s *imageSource) Extents(name string) ([]FileExtent, error) {
	f, ok := s.img.Find(name)
	if !ok || f.IsDir {
		return nil, fmt.Errorf("%s: not on this disc", name)
	}
	out := make([]FileExtent, 0, len(f.Extents))
	for _, e := range f.Extents {
		out = append(out, FileExtent{Path: s.img.Path, Offset: e.Offset, Length: e.Length})
	}
	return out, nil
}
func (s *imageSource) List(dir string) ([]string, error) {
	dir = strings.ToUpper(strings.Trim(dir, "/")) + "/"
	var out []string
	for _, f := range s.img.Files() {
		up := strings.ToUpper(f.Path)
		if f.IsDir || !strings.HasPrefix(up, dir) || strings.Contains(up[len(dir):], "/") {
			continue
		}
		out = append(out, f.Path[len(dir):])
	}
	sort.Strings(out)
	return out, nil
}

type folderSource struct{ dir string }

func (s *folderSource) Describe() string { return s.dir }
func (s *folderSource) Close() error     { return nil }

// path finds a file by the disc's name for it, whatever the case: copied
// discs arrive with names in any case, and some file systems care.
func (s *folderSource) path(name string) string {
	p := s.dir
	for _, part := range strings.Split(name, "/") {
		next := filepath.Join(p, part)
		if _, err := os.Lstat(next); err != nil {
			if ents, rerr := os.ReadDir(p); rerr == nil {
				for _, e := range ents {
					if strings.EqualFold(e.Name(), part) {
						next = filepath.Join(p, e.Name())
						break
					}
				}
			}
		}
		p = next
	}
	return p
}
func (s *folderSource) ReadFile(name string, max int64) ([]byte, error) {
	f, err := os.Open(s.path(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}
func (s *folderSource) Extents(name string) ([]FileExtent, error) {
	st, err := os.Stat(s.path(name))
	if err != nil {
		return nil, err
	}
	return []FileExtent{{Path: s.path(name), Offset: 0, Length: st.Size()}}, nil
}
func (s *folderSource) List(dir string) ([]string, error) {
	ents, err := os.ReadDir(s.path(dir))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}
