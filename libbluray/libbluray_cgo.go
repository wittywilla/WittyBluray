//go:build libbluray && cgo

package libbluray

/*
#cgo pkg-config: libbluray
#include <stdlib.h>
#include <libbluray/bluray.h>
#include <libbluray/meta_data.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"unsafe"
)

// libbluray reads LIBAACS_PATH and LIBBDPLUS_PATH when it opens a disc, so
// the environment is set (in Go and in C) under a lock for each open.
var mu sync.Mutex

// Available reports whether libbluray is built in.
func Available() bool { return true }

func noDecryptionLibraries() {
	for _, k := range []string{"LIBAACS_PATH", "LIBBDPLUS_PATH"} {
		_ = os.Setenv(k, NoLibraryPath)
		ck, cv := C.CString(k), C.CString(NoLibraryPath)
		C.setenv(ck, cv, 1)
		C.free(unsafe.Pointer(ck))
		C.free(unsafe.Pointer(cv))
	}
}

// Read opens a disc (a BDMV folder's parent, or an image libbluray can
// read) with libbluray, reads its titles, flags and metadata, and closes
// it. It never calls bd_read and never lets libaacs or libbdplus load.
func Read(path string, o ReadOptions) (*Info, error) {
	if path == "" {
		return nil, errors.New("libbluray: no path to open")
	}
	mu.Lock()
	defer mu.Unlock()
	noDecryptionLibraries()

	var major, minor, micro C.int
	C.bd_get_version(&major, &minor, &micro)
	info := &Info{Version: fmt.Sprintf("%d.%d.%d", int(major), int(minor), int(micro))}

	bd := C.bd_init()
	if bd == nil {
		return nil, errors.New("libbluray: bd_init failed")
	}
	defer C.bd_close(bd)
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	// No key file: nothing is decrypted.
	if C.bd_open_disc(bd, cpath, nil) == 0 {
		return nil, fmt.Errorf("libbluray: can't open %s", path)
	}
	di := C.bd_get_disc_info(bd)
	if di == nil {
		return nil, errors.New("libbluray: no disc info")
	}
	f := &info.Flags
	f.BlurayDetected = di.bluray_detected != 0
	f.AACSDetected, f.LibAACSDetected, f.AACSHandled = di.aacs_detected != 0, di.libaacs_detected != 0, di.aacs_handled != 0
	f.BDPlusDetected, f.LibBDPlusDetected, f.BDPlusHandled = di.bdplus_detected != 0, di.libbdplus_detected != 0, di.bdplus_handled != 0
	f.BDJDetected = di.bdj_detected != 0
	f.FirstPlay, f.TopMenu = di.first_play_supported != 0, di.top_menu_supported != 0
	f.HDMVTitles, f.BDJTitles = int(di.num_hdmv_titles), int(di.num_bdj_titles)
	if f.LibAACSDetected || f.LibBDPlusDetected || f.AACSHandled || f.BDPlusHandled {
		return nil, ErrDecryptionLibraryLoaded
	}
	if (f.AACSDetected || f.BDPlusDetected) && !o.KnownClear {
		return nil, ErrEncrypted
	}
	if m := C.bd_get_meta(bd); m != nil {
		info.Name = C.GoString(m.di_name)
		info.Language = C.GoString(&m.language_code[0])
		if m.toc_count > 0 && m.toc_entries != nil {
			for _, e := range unsafe.Slice(m.toc_entries, int(m.toc_count)) {
				info.Library = append(info.Library, fmt.Sprintf("%d: %s", int(e.title_number), C.GoString(e.title_name)))
			}
		}
	}
	n := int(C.bd_get_titles(bd, C.TITLES_ALL, 0))
	for i := 0; i < n; i++ {
		ti := C.bd_get_title_info(bd, C.uint32_t(i), 0)
		if ti == nil {
			continue
		}
		t := TitleInfo{Playlist: int(ti.playlist), Duration: float64(ti.duration) / 90000, Chapters: int(ti.chapter_count), Marks: int(ti.mark_count), Angles: int(ti.angle_count)}
		if ti.clip_count > 0 && ti.clips != nil {
			for _, c := range unsafe.Slice(ti.clips, int(ti.clip_count)) {
				t.Clips = append(t.Clips, C.GoString(&c.clip_id[0]))
			}
		}
		C.bd_free_title_info(ti)
		info.Titles = append(info.Titles, t)
	}
	return info, nil
}
