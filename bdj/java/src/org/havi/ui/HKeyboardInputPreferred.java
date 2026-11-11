// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public interface HKeyboardInputPreferred {
    int INPUT_NUMERIC = 1, INPUT_ALPHA = 2, INPUT_ANY = 4, INPUT_CUSTOMIZED = 8;
    boolean getEditMode();
    void setEditMode(boolean on);
    int getType();
    char[] getValidInput();
    void processHKeyEvent(org.havi.ui.event.HKeyEvent e);
    void processHTextEvent(org.havi.ui.event.HTextEvent e);
}
