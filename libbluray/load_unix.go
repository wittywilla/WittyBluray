// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build (darwin || (linux && libbluray)) && (amd64 || arm64) && !nolibbluray

package libbluray

import (
	"runtime"

	"github.com/ebitengine/purego"
)

func libraryNames() []string {
	if runtime.GOOS == "darwin" {
		// The loader's own search, then where Homebrew and MacPorts install.
		return []string{"libbluray.2.dylib", "/opt/homebrew/lib/libbluray.2.dylib", "/usr/local/lib/libbluray.2.dylib", "/opt/local/lib/libbluray.2.dylib", "libbluray.dylib"}
	}
	return []string{"libbluray.so.2", "libbluray.so"}
}

func openLibrary(name string) (uintptr, error) {
	return purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_LOCAL)
}

// cSetenv is the C library's setenv, which is the environment libbluray
// reads; nil when it cannot be bound.
func cSetenv() func(name, value string) {
	libc := "libc.so.6"
	if runtime.GOOS == "darwin" {
		libc = "/usr/lib/libSystem.B.dylib"
	}
	h, err := purego.Dlopen(libc, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil
	}
	sym, err := purego.Dlsym(h, "setenv")
	if err != nil || sym == 0 {
		return nil
	}
	var setenv func(name, value string, overwrite int32) int32
	purego.RegisterFunc(&setenv, sym)
	return func(name, value string) { setenv(name, value, 1) }
}
