// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public interface HItemValue extends HNavigable, HSelectionInputPreferred {
    void addItemListener(org.havi.ui.event.HItemListener l);
    void removeItemListener(org.havi.ui.event.HItemListener l);
    void setSelectionSound(HSound s);
    HSound getSelectionSound();
}
