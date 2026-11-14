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

// Command wittybluray-module is WittyBluray as a helper process, so a new version
// can reach a server without a new server. See WittyModule for the protocol, and
// package helper for what it answers. Started as WittyBluray's service (module API 5), the same
// program answers its routes and SDUI sources instead, and opens no disc (package api).
package main

import (
	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/api"
	"github.com/wittywilla/wittybluray/diagnose"
	"github.com/wittywilla/wittybluray/helper"
	"github.com/wittywilla/wittymodule"
)

func main() {
	h := &diagnose.Handler{Next: (&helper.Helper{}).Handle}
	wittymodule.Main(wittybluray.ManifestJSON, (&api.Dual{Config: api.Config{Module: wittybluray.ModuleID, Bluray: true}, Helper: h.Handle}).Handle)
}
