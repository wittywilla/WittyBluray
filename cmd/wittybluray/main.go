// SPDX-License-Identifier: AGPL-3.0-or-later
//
// WittyBluray - Blu-ray images with their menus, including 4K, HDR and 3D discs.
// Copyright (C) 2026 WittyWilla and contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

// Command wittybluray inspects Blu-ray images and BDMV folders with the
// WittyBluray engine. It reads nothing but the disc, and its output holds no
// video or artwork, so it is safe to attach to a bug report.
package main

import (
	"fmt"
	"os"

	"github.com/wittywilla/wittybluray/cli"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "info", "playlist", "menu", "walk", "render", "bdj":
			os.Exit(cli.Bluray(os.Args[1:]))
		case "version", "--version", "-v":
			fmt.Println("wittybluray", version)
			return
		}
	}
	fmt.Fprint(os.Stderr, `wittybluray `+version+`: inspect Blu-ray images and BDMV folders

  wittybluray info      <disc>            titles, the disc's program, playlists, clips and streams
  wittybluray playlist  <disc> <number>   every item, stream and chapter of one playlist
  wittybluray menu      <disc> <playlist>   a playlist's menu: pages, buttons and every command
  wittybluray walk      <disc> [button...] simulate inserting the disc, then pressing menu buttons by id
  wittybluray render    <disc> <out-dir>  save the first menu's pages as PNG files
  wittybluray bdj       <disc> [seconds [picture.png [key...]]]  BD-J titles and what their code needs; with Java, run them

<disc> is an .iso/.img image, or a folder that contains BDMV. Encrypted (AACS) discs are refused.
`)
	os.Exit(2)
}
