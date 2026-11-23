// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package bdj

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"testing"
)

// What runs a disc's code has no network beyond a loopback of its own, so even a way out of
// DiscLoader couldn't reach the server, the home network or the internet.
func TestConfinedHasNoNetwork(t *testing.T) {
	if !namespacesAllowed() {
		t.Skip("user namespaces aren't allowed here")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	cmd := exec.Command("/bin/sh", "-c", "cat /proc/net/dev; echo ==; cat /proc/net/tcp")
	if !confine(cmd) {
		t.Fatal("namespaces are allowed, and weren't used")
	}
	if cmd.SysProcAttr.Pdeathsig == 0 {
		t.Fatal("a confined process would outlive the server")
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	dev, tcp, _ := strings.Cut(string(out), "==")
	for _, line := range strings.Split(dev, "\n")[2:] {
		if name, _, ok := strings.Cut(strings.TrimSpace(line), ":"); ok && name != "lo" {
			t.Fatalf("the confined process sees the network interface %q", name)
		}
	}
	// The listener above is in the server's network: the confined process can't see it.
	port := fmt.Sprintf(":%04X ", l.Addr().(*net.TCPAddr).Port)
	if strings.Contains(tcp, port) {
		t.Fatal("the confined process sees the server's own sockets")
	}
}
