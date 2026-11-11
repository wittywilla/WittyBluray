// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public interface HSwitchable extends HActionable {
    boolean getSwitchableState();
    void setSwitchableState(boolean on);
    void setUnsetActionSound(HSound s);
    HSound getUnsetActionSound();
}
