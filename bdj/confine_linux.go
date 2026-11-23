// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package bdj

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// confine is the second wall round a disc's code, outside Java. The first is DiscLoader's
// list of what a disc may link; if that ever had a hole, the process it got into still has no
// network to reach (a network namespace of its own, with only a loopback nobody brought up),
// no other process's System V shared memory or message queues, and it dies with the server.
// The namespaces need unprivileged user namespaces, which some containers and distributions
// refuse, so they are tried first rather than assumed.
func confine(cmd *exec.Cmd) bool {
	attr := cmd.SysProcAttr
	if attr == nil {
		attr = &syscall.SysProcAttr{}
		cmd.SysProcAttr = attr
	}
	attr.Pdeathsig = syscall.SIGKILL
	attr.Setpgid = true
	if !namespacesAllowed() {
		return false
	}
	uid, gid := os.Getuid(), os.Getgid()
	attr.Cloneflags |= syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWIPC
	attr.UidMappings = []syscall.SysProcIDMap{{ContainerID: uid, HostID: uid, Size: 1}}
	attr.GidMappings = []syscall.SysProcIDMap{{ContainerID: gid, HostID: gid, Size: 1}}
	attr.GidMappingsEnableSetgroups = false
	return true
}

var (
	nsOnce    sync.Once
	nsAllowed bool
)

// namespacesAllowed starts something small in the namespaces once, since the settings that
// decide it differ between kernels and a container's seccomp profile can refuse it regardless.
func namespacesAllowed() bool {
	nsOnce.Do(func() {
		// A program that isn't there: nothing runs, and the error says how far it got. "No such
		// file" comes from exec, after the namespaces were made and the IDs mapped; a refusal
		// to make them comes back as a permission or argument error instead.
		probe := exec.Command("/nonexistent/wittybluray-namespace-probe")
		probe.Env = []string{}
		probe.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags:                 syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWIPC,
			UidMappings:                []syscall.SysProcIDMap{{ContainerID: os.Getuid(), HostID: os.Getuid(), Size: 1}},
			GidMappings:                []syscall.SysProcIDMap{{ContainerID: os.Getgid(), HostID: os.Getgid(), Size: 1}},
			GidMappingsEnableSetgroups: false,
		}
		err := probe.Start()
		if err == nil {
			_ = probe.Process.Kill()
			_ = probe.Wait()
		}
		nsAllowed = err == nil || errors.Is(err, syscall.ENOENT)
	})
	return nsAllowed
}
