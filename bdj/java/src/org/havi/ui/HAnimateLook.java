// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public class HAnimateLook extends HDefaultLook {
    public void renderVisible(java.awt.Graphics g, HVisible v, int state) {
        java.awt.Image[] a = framesFor(v, state);
        if (a == null) return;
        int p = v instanceof HAnimateEffect ? ((HAnimateEffect) v).getPosition() : 0;
        drawGraphic(g, v, a[Math.max(0, Math.min(a.length - 1, p))]);
    }
}
