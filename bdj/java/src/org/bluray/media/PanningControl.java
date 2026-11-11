// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.media;
public interface PanningControl extends javax.media.Control {
    void setLeftRight(float x);
    float getLeftRight();
    void setFrontRear(float y);
    float getFrontRear();
    void addPanningChangeListener(PanningChangeListener l);
    void removePanningChangeListener(PanningChangeListener l);
}
