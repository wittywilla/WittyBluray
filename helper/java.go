package helper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/bdj"
	"github.com/wittywilla/wittymodule"
)

// A disc's Java programs run here, beside its navigator: this process starts Java, and tells
// the server what the programs do, unasked, as they do it. Its folders are inside the one the
// server gave this module to write to, in WITTYMODULE_DATA.

// What a helper says about a disc's Java programs, unasked, as they run.
const (
	NotifyJavaEvent  = "java.event"  // bdj.Event: something the applications asked for
	NotifyJavaChange = "java.change" // JavaChange: a new picture
	NotifyJavaLog    = "java.log"    // a line they printed
	NotifyJavaExit   = "java.exit"   // JavaExit: the process has ended
)

// JavaStart starts Java for this disc; Session names its working folder, and Java is the
// program to use ("" finds it).
type JavaStart struct {
	Java    string `json:"java,omitempty"`
	Session string `json:"session,omitempty"`
}

// JavaLine is one line for the programs.
type JavaLine struct {
	Line string `json:"line"`
}

// JavaChange says which picture is the newest.
type JavaChange struct {
	Version int `json:"version"`
}

// JavaExit is how the process ended.
type JavaExit struct {
	Error   string `json:"error,omitempty"`
	Stopped bool   `json:"stopped,omitempty"`
}

// JavaProbe is whether this machine's Java can run a disc's programs.
type JavaProbe struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

var sessionRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// dataDir is the folder this module may write to.
func dataDir() (string, error) {
	d := os.Getenv("WITTYMODULE_DATA")
	if d == "" {
		return "", errors.New("this server gave WittyBluray no folder to write to, so a disc's Java programs can't run")
	}
	return d, nil
}

// storageKey names a disc's storage: the same however the disc is reached and from one server
// start to the next, and different for a different disc. Scores and bookmarks belong to the
// disc's program, so that is what it is worked out from.
func storageKey(d *wittybluray.Disc) string {
	h := sha256.New()
	_, _ = io.WriteString(h, d.Label)
	for _, f := range []string{"index.bdmv", "MovieObject.bdmv"} {
		if b, err := d.ReadProgramFile(f, 4<<20); err == nil {
			_, _ = h.Write(b)
		}
	}
	add := func(o wittybluray.IndexObject) {
		if o.Kind == wittybluray.ObjectBDJ {
			if b, err := d.BDJObject(o.BDJName); err == nil {
				_, _ = fmt.Fprintf(h, "%s %v %+v;", b.Name, b.Playlists, b.Apps)
			}
		}
	}
	add(d.Index.FirstPlay)
	add(d.Index.TopMenu)
	for _, t := range d.Index.Titles {
		add(t.Object)
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

func (h *Helper) javaStart(raw json.RawMessage) (any, []byte, error) {
	var p JavaStart
	if err := json.Unmarshal(raw, &p); err != nil || !sessionRe.MatchString(p.Session) {
		return nil, nil, &wittymodule.CallError{Code: "bad-call", Message: "the call's parameters are unreadable"}
	}
	if h.host != nil {
		return nil, nil, nil
	}
	data, err := dataDir()
	if err != nil {
		return nil, nil, coded(err)
	}
	java, err := bdj.Find(p.Java)
	if err != nil {
		return nil, nil, coded(err)
	}
	work := filepath.Join(data, "bdj", "sessions", p.Session)
	host, err := bdj.Start(context.Background(), h.disc, bdj.Options{
		Java:       java,
		CacheDir:   filepath.Join(data, "bdj"),
		WorkDir:    work,
		PersistDir: filepath.Join(data, "bdj", "discs", storageKey(h.disc)),
		OnEvent:    func(e bdj.Event) { _ = wittymodule.Notify(NotifyJavaEvent, e) },
		OnFrame:    func(v int) { _ = wittymodule.Notify(NotifyJavaChange, JavaChange{Version: v}) },
		Log:        func(l string) { _ = wittymodule.Notify(NotifyJavaLog, l) },
		OnExit: func(err error, stopped bool) {
			x := JavaExit{Stopped: stopped}
			if err != nil {
				x.Error = err.Error()
			}
			_ = wittymodule.Notify(NotifyJavaExit, x)
		},
	})
	if err != nil {
		return nil, nil, coded(err)
	}
	h.host, h.work = host, work
	return nil, nil, nil
}

// javaCall answers the calls about Java that's running.
func (h *Helper) javaCall(method string, raw json.RawMessage, p Params) (any, []byte, error) {
	if method == "javastart" {
		return h.javaStart(raw)
	}
	if h.host == nil {
		return nil, nil, &wittymodule.CallError{Code: "no-java", Message: "Java isn't running for this disc"}
	}
	switch method {
	case "javaframe":
		b, v := h.host.PNG()
		return JavaChange{Version: v}, b, nil
	case "javasend":
		var l JavaLine
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, nil, &wittymodule.CallError{Code: "bad-call", Message: "the line is unreadable"}
		}
		h.host.Send("%s", l.Line)
	case "javasync":
		h.host.Sync(h.nav.Registers())
	case "javatitle":
		o, err := h.disc.BDJObject(p.Name)
		if err != nil {
			return nil, nil, coded(err)
		}
		h.host.StartTitle(p.N, o)
	case "javaclose":
		h.host.Close()
		h.host = nil
		_ = os.RemoveAll(h.work)
	}
	return nil, nil, nil
}

// javaProbe needs no disc open.
func javaProbe(raw json.RawMessage) (any, []byte, error) {
	var p JavaStart
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, nil, &wittymodule.CallError{Code: "bad-call", Message: "the call's parameters are unreadable"}
	}
	data, err := dataDir()
	if err != nil {
		return nil, nil, coded(err)
	}
	j, err := bdj.Find(p.Java)
	if err != nil {
		return nil, nil, coded(err)
	}
	v, err := j.Probe(context.Background(), filepath.Join(data, "bdj"))
	if err != nil {
		return nil, nil, coded(err)
	}
	return JavaProbe{Version: v, Path: j.Path}, nil, nil
}
