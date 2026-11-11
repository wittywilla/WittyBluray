// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public class HGraphicLook extends HDefaultLook {
    public void renderVisible(java.awt.Graphics g, HVisible v, int state) { drawGraphic(g, v, graphicFor(v, state)); }
}
