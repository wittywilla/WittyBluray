// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public interface HSelectionInputPreferred {
    boolean getSelectionMode();
    void setSelectionMode(boolean on);
    void processHItemEvent(org.havi.ui.event.HItemEvent e);
}
