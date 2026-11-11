// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build windows && (amd64 || arm64) && !nolibbluray

package libbluray

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

func libraryNames() []string { return []string{"libbluray-2.dll", "libbluray.dll", "bluray.dll"} }

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	loadLibraryExW = kernel32.NewProc("LoadLibraryExW")
)

// openLibrary loads a library named by a full path from that path, and one
// named without from the system folder or the program's own folder only,
// never from the current folder or the search path: a disc folder must not
// be able to supply a library.
func openLibrary(name string) (uintptr, error) {
	const (
		searchDLLLoadDir     = 0x00000100
		searchApplicationDir = 0x00000200
		searchSystem32       = 0x00000800
	)
	flags := uintptr(searchApplicationDir | searchSystem32)
	if filepath.IsAbs(name) {
		flags = searchDLLLoadDir | searchSystem32
	}
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	h, _, callErr := loadLibraryExW.Call(uintptr(unsafe.Pointer(p)), 0, flags)
	if h == 0 {
		return 0, callErr
	}
	return h, nil
}

// cSetenv sets the variable in the C runtime libbluray uses, when that is
// the universal one.
func cSetenv() func(name, value string) {
	ucrt := syscall.NewLazyDLL("ucrtbase.dll")
	put := ucrt.NewProc("_putenv_s")
	if put.Find() != nil {
		return nil
	}
	return func(name, value string) {
		n, _ := syscall.BytePtrFromString(name)
		v, _ := syscall.BytePtrFromString(value)
		put.Call(uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(v)))
	}
}
