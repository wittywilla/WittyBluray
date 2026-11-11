// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public class HRange extends HStaticRange implements HNavigable {
    public HRange() {}
    public HRange(int o, int mn, int mx, int v) { super(o, mn, mx, v); }
    public HRange(int o, int mn, int mx, int v, int x, int y, int w, int h) { super(o, mn, mx, v, x, y, w, h); }
}
