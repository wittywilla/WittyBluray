// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.media;
public interface PiPControl extends javax.media.Control {
    void setDisplay(boolean on);
    boolean getIsDisplay();
    void setFullScreen(boolean on);
    boolean getFullScreen();
    boolean isSynchronized();
    void addPiPStatusListener(PiPStatusListener l);
    void removePiPStatusListener(PiPStatusListener l);
}
