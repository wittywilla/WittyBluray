// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package bdj

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func hide(c *exec.Cmd) {}

// javaCommand runs Java slightly niced where nice exists, so a disc's code gives
// way to the server without stalling beside a busy FFmpeg. nice replaces itself
// with Java: the process is the same one.
func javaCommand(path string, args ...string) *exec.Cmd {
	if n, err := exec.LookPath("nice"); err == nil {
		return exec.Command(n, append([]string{"-n", "5", path}, args...)...)
	}
	return exec.Command(path, args...)
}

// usagePS asks ps, for systems without a /proc to read. The thread count is a
// second question (threadsPS), and stays 0 where ps has no way to say.
func usagePS(pid int) (u procUsage, ok bool) {
	out, err := exec.Command("ps", "-o", "rss=", "-o", "time=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return u, false
	}
	w := strings.Fields(string(out))
	if len(w) != 2 {
		return u, false
	}
	kb, err := strconv.ParseUint(w[0], 10, 64)
	if err != nil {
		return u, false
	}
	u.memory = kb * 1024
	// [dd-]hh:mm:ss[.cc], or mm:ss.cc
	t := w[1]
	days := 0
	if d, rest, found := strings.Cut(t, "-"); found {
		days, _ = strconv.Atoi(d)
		t = rest
	}
	var secs float64
	for _, p := range strings.Split(t, ":") {
		f, _ := strconv.ParseFloat(p, 64)
		secs = secs*60 + f
	}
	u.cpu = time.Duration((float64(days)*86400 + secs) * float64(time.Second))
	u.threads = threadsPS(pid)
	return u, true
}

// threadsPS counts a process's threads with ps, which lists one line for each: on
// a Mac with -M, under a heading, and on FreeBSD with -H. Without it the thread
// limit was never applied on either, so a disc's code could start threads until
// the machine had no more to give. 0 when ps can't say.
func threadsPS(pid int) int {
	var args []string
	heading := 0
	switch runtime.GOOS {
	case "darwin":
		args, heading = []string{"-M", "-p", strconv.Itoa(pid)}, 1
	case "freebsd", "dragonfly":
		args = []string{"-H", "-o", "lwp=", "-p", strconv.Itoa(pid)}
	default:
		return 0
	}
	out, err := exec.Command("ps", args...).Output()
	if err != nil {
		return 0
	}
	n := -heading
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return max(n, 0)
}
