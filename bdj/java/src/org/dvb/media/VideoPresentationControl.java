// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.media;
public interface VideoPresentationControl extends javax.media.Control {
    java.awt.Dimension getInputVideoSize();
    java.awt.Dimension getVideoSize();
    org.havi.ui.HScreenRectangle getActiveVideoArea();
    org.havi.ui.HScreenRectangle getActiveVideoAreaOnScreen();
    org.havi.ui.HScreenRectangle getTotalVideoArea();
    org.havi.ui.HScreenRectangle getTotalVideoAreaOnScreen();
    boolean supportsClipping();
    java.awt.Rectangle setClipRegion(java.awt.Rectangle r);
    java.awt.Rectangle getClipRegion();
    float[] supportsArbitraryHorizontalScaling();
    float[] supportsArbitraryVerticalScaling();
    float[] getHorizontalScalingFactors();
    float[] getVerticalScalingFactors();
    byte getPositioningCapability();
}
