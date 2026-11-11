// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public interface HExtendedLook extends HLook {
    void fillBackground(java.awt.Graphics g, HVisible v, int state);
    void renderBorders(java.awt.Graphics g, HVisible v, int state);
    void renderVisible(java.awt.Graphics g, HVisible v, int state);
}
