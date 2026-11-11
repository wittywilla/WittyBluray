// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public interface HAdjustmentValue extends HNavigable, HAdjustmentInputPreferred {
    void setUnitIncrement(int n);
    int getUnitIncrement();
    void setBlockIncrement(int n);
    int getBlockIncrement();
    void addAdjustmentListener(org.havi.ui.event.HAdjustmentListener l);
    void removeAdjustmentListener(org.havi.ui.event.HAdjustmentListener l);
    void setAdjustmentSound(HSound s);
    HSound getAdjustmentSound();
}
