// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public interface HActionable extends HNavigable, HActionInputPreferred {
    void addHActionListener(org.havi.ui.event.HActionListener l);
    void removeHActionListener(org.havi.ui.event.HActionListener l);
    void setActionCommand(String c);
    void setActionSound(HSound s);
    HSound getActionSound();
}
