// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public interface HAdjustmentInputPreferred {
    boolean getAdjustMode();
    void setAdjustMode(boolean on);
    void processHAdjustmentEvent(org.havi.ui.event.HAdjustmentEvent e);
}
