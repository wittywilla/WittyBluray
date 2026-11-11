// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/helper"
	"github.com/wittywilla/wittybluray/libbluray"
)

// The cross-check with a libbluray the user has installed, on real files:
// a synthetic disc as a folder and as a UDF image. On a machine without
// libbluray the same calls must come back with the ranking and a note.
func TestCrossCheckWithInstalledLibbluray(t *testing.T) {
	cap := libbluray.Capability()
	dir, feature := wittybluray.SynthFolder(t, 41)
	img, _ := wittybluray.SynthImage(t, 41)
	for _, path := range []string{dir, img} {
		d, err := wittybluray.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		r, note := libbluray.Analyze(d, wittybluray.RankOptions{})
		if r.Main != feature {
			t.Errorf("%s: main %05d, want %05d", path, r.Main, feature)
		}
		if !cap.Available {
			if note == "" || r.SecondOpinion != nil {
				t.Errorf("without libbluray: note %q, second opinion %+v", note, r.SecondOpinion)
			}
			d.Close()
			continue
		}
		// With the library: it must have been asked, and every playlist it
		// lists must agree with this engine on length, chapters and angles.
		if note != "" || r.SecondOpinion == nil {
			t.Fatalf("%s: libbluray %s was not asked: %s", path, cap.Version, note)
		}
		for _, x := range r.SecondOpinion.Disagreements {
			if x.Field != "main" && x.Field != "present" {
				t.Errorf("%s: libbluray %s disagrees: %+v", path, cap.Version, x)
			}
		}
		t.Logf("%s: libbluray %s picks %05d (agrees: %v); confidence %.2f", path, cap.Version, r.SecondOpinion.Main, r.SecondOpinion.AgreesOnMain, r.Confidence)
		d.Close()
	}
	if !cap.Available {
		t.Skipf("no libbluray on this machine (%s): the comparison itself did not run", cap.Reason)
	}
}

// The helper's additive calls: ranking (with and without a second opinion),
// chapters, consistency, capabilities.
func TestHelperRankingCalls(t *testing.T) {
	dir, feature := wittybluray.SynthFolder(t, 42)
	h := &helper.Helper{}
	call := func(method string, params any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(params)
		res, _, err := h.Handle(method, raw)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		b, err := json.Marshal(res)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]any{}
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("%s: %s", method, b)
		}
		return out
	}
	caps := call("capabilities", map[string]any{})
	if _, ok := caps["libbluray"].(map[string]any)["available"]; !ok {
		t.Errorf("capabilities: %v", caps)
	}
	call("open", map[string]any{"path": dir})
	for _, second := range []bool{false, true} {
		got := call("ranking", map[string]any{"shown": second})
		r := got["ranking"].(map[string]any)
		if int(r["main"].(float64)) != feature || r["confidenceLabel"] == "" || len(r["why"].([]any)) == 0 {
			t.Errorf("ranking: main %v, confidence %v, why %v", r["main"], r["confidenceLabel"], r["why"])
		}
		if got["total"].(float64) != 15 {
			t.Errorf("total %v", got["total"])
		}
		first := r["candidates"].([]any)[0].(map[string]any)
		if len(first["evidence"].([]any)) == 0 || !strings.Contains(strings.Join(toStrings(first["flags"]), ","), "main") {
			t.Errorf("first candidate: %v", first)
		}
	}
	if ch := call("chapters", map[string]any{"n": feature})["chapters"].([]any); len(ch) < 2 || ch[1].(map[string]any)["startSec"].(float64) != 600 {
		t.Errorf("chapters: %v", ch)
	}
	if got := call("ranking", map[string]any{"limit": 3}); len(got["ranking"].(map[string]any)["candidates"].([]any)) != 3 || got["total"].(float64) != 15 {
		t.Errorf("a limit of 3 gave %v candidates of %v", len(got["ranking"].(map[string]any)["candidates"].([]any)), got["total"])
	}
	if c := call("consistency", map[string]any{}); c["issues"] == nil {
		t.Errorf("consistency: %v", c)
	}
}

func toStrings(v any) []string {
	var out []string
	list, _ := v.([]any)
	for _, x := range list {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}
