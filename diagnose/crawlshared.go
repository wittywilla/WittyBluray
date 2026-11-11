// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// What a crawl of a disc's menus shares with the same check of a DVD.

package diagnose

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Limits on pressing every button of every menu, on a DVD or a Blu-ray.
const (
	maxScreens     = 300
	maxSettleCells = 400
	maxDeadEnds    = 50
	maxCrashes     = 3
	crawlBudget    = 10 * time.Minute
	longTitleSec   = 600 // past this, a title is the film: the walk stops there
)

const encryptedAdvice = "Encrypted discs aren't supported, and nothing here can change that. A disc without copy protection will play."

func kindSentence(src *discSource) string {
	if src.IsDir {
		return "A folder"
	}
	return "A " + formatName(src.Format) + " image"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func pathWords(p []int) string {
	if len(p) == 0 {
		return "Starting the disc"
	}
	parts := make([]string, len(p))
	for i, n := range p {
		parts[i] = strconv.Itoa(n)
	}
	return "Buttons " + strings.Join(parts, " > ")
}

// deadEndList keeps the different dead ends a crawl finds. One broken button can
// be reached along many paths, and the first path says all there is to say.
type deadEndList struct {
	index  map[string]int
	lines  []string
	counts []int
	total  int
}

func (l *deadEndList) add(key, line string) {
	if i, ok := l.index[key]; ok {
		l.counts[i]++
		return
	}
	l.total++
	if len(l.lines) >= maxDeadEnds {
		return
	}
	if l.index == nil {
		l.index = map[string]int{}
	}
	l.index[key] = len(l.lines)
	l.lines = append(l.lines, line)
	l.counts = append(l.counts, 0)
}

// report words the list for a finding.
func (l *deadEndList) report() []string {
	out := make([]string, len(l.lines))
	for i, line := range l.lines {
		out[i] = line
		if n := l.counts[i]; n > 0 {
			out[i] += fmt.Sprintf(" (%s to the same end)", plural(n, "other path leads", "other paths lead"))
		}
	}
	return out
}

// reportCrawl words what a crawl found, for either kind of disc.
func reportCrawl(j *Job, kind string, ok bool, crashes, screens, pressed, unvisited int, first string, dead *deadEndList, outOfTime bool) {
	if crashes > 0 {
		j.Add(Finding{Severity: SevProblem, Title: "The engine crashed on this disc", What: "The server's disc engine crashed while pressing a menu button. That's a bug in the server, and is itself a useful result: the same button would probably crash the player's session.",
			Fix: "Please send this report (it holds no disc content) with a bug report."})
	}
	summary := []string{plural(screens, "menu screen", "menu screens") + " visited, " + plural(pressed, "button", "buttons") + " pressed"}
	if !ok {
		return
	}
	total := dead.total
	if screens == 0 && total == 0 {
		j.Add(Finding{Severity: SevOK, Title: "This disc has no menu to press", What: "On starting, " + first + ".", Details: summary})
		return
	}
	all := unvisited == 0 && !outOfTime
	if !all {
		why := "the limit of " + strconv.Itoa(maxScreens) + " screens"
		if outOfTime {
			why = "the time limit of " + crawlBudget.String()
		}
		j.Add(Finding{Severity: SevInfo, Title: "Not every menu was visited", What: "The check stopped at " + why + ", with " + plural(unvisited, "screen", "screens") + " still to see. Discs with games or many setup menus can go on for a long way.", Details: summary})
	}
	if total == 0 {
		title := "Every menu button leads somewhere"
		if !all {
			title = "Every menu button tried leads somewhere"
		}
		j.Add(Finding{Severity: SevOK, Title: title, What: "Every button pressed led to another menu or to something playing.", Details: summary})
		return
	}
	j.Add(Finding{Severity: SevWarn, Title: plural(total, "menu button leads nowhere", "menu buttons lead nowhere"),
		What:    "These paths end with nothing to do. A disc can do this on purpose (a button that ends the disc), but it's also how a problem in the engine or the copy shows up.",
		Fix:     "If one of these is a button you need, send this report (it holds no disc content) with a bug report. " + kind + " buttons are numbered by the disc.",
		Details: append(summary, dead.report()...), More: total - len(dead.lines)})
}
