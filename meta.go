// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittybluray

import (
	"bytes"
	"encoding/xml"
	"sort"
	"strconv"
	"strings"
)

// BDMV/META/DL/bdmt_xxx.xml: the "disc library" a disc may carry, with its
// name and the names of its titles in one file per language. It is plain
// XML and optional; studios fill it in carelessly or not at all, so it is a
// label, never evidence.

// DiscMeta is the disc library metadata in one language.
type DiscMeta struct {
	Language string         `json:"language,omitempty"` // three letters, from the file's name
	Name     string         `json:"name,omitempty"`     // the disc's name
	Titles   map[int]string `json:"titles,omitempty"`   // title number -> name
	// Languages lists every language the disc has such a file for.
	Languages []string `json:"languages,omitempty"`
}

const maxMetaBytes = 1 << 20

// Meta reads the disc library metadata, in English when the disc has it
// and otherwise in its first language. nil when the disc has none that
// reads. Worked out once per opened disc.
func (d *Disc) Meta() *DiscMeta {
	if d == nil || d.Source == nil {
		return nil
	}
	d.metaOnce.Do(func() {
		names, _ := d.Source.List(d.layout.root + "/META/DL")
		var langs []string
		for _, n := range names {
			low := strings.ToLower(n)
			if strings.HasPrefix(low, "bdmt_") && strings.HasSuffix(low, ".xml") && len(low) == len("bdmt_eng.xml") {
				langs = append(langs, low[5:8])
			}
		}
		if len(langs) == 0 {
			return
		}
		sort.Strings(langs)
		order := append([]string{"eng"}, langs...)
		for _, l := range order {
			found := ""
			for _, n := range names {
				if strings.EqualFold(n, "bdmt_"+l+".xml") {
					found = n
				}
			}
			if found == "" {
				continue
			}
			b, err := d.Source.ReadFile(d.layout.root+"/META/DL/"+found, maxMetaBytes)
			if err != nil {
				continue
			}
			if m := ParseDiscMeta(b); m != nil {
				m.Language, m.Languages = l, langs
				d.meta = m
				return
			}
		}
	})
	return d.meta
}

// ParseDiscMeta reads one bdmt_xxx.xml. It follows no reference out of the
// document (the decoder has no external entities) and stops after a fixed
// number of elements. nil when the document names nothing.
func ParseDiscMeta(b []byte) *DiscMeta {
	if len(b) > maxMetaBytes {
		b = b[:maxMetaBytes]
	}
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	m := &DiscMeta{Titles: map[int]string{}}
	var stack []string
	titleNum := 0
	for tokens := 0; tokens < 20000; tokens++ {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) > 64 {
				return nil
			}
			stack = append(stack, t.Name.Local)
			if t.Name.Local == "titleName" {
				titleNum = 0
				for _, a := range t.Attr {
					if a.Name.Local == "titleNumber" {
						titleNum, _ = strconv.Atoi(strings.TrimSpace(a.Value))
					}
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text == "" || len(stack) == 0 {
				continue
			}
			if len(text) > 512 {
				text = text[:512]
			}
			switch top := stack[len(stack)-1]; {
			case top == "name" && len(stack) >= 2 && stack[len(stack)-2] == "title" && m.Name == "":
				m.Name = text
			case top == "titleName" && titleNum > 0 && titleNum <= 1000 && len(m.Titles) < 1000:
				m.Titles[titleNum] = text
			}
		}
	}
	if m.Name == "" && len(m.Titles) == 0 {
		return nil
	}
	if len(m.Titles) == 0 {
		m.Titles = nil
	}
	return m
}
