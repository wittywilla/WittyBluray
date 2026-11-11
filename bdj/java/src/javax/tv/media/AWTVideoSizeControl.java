// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.media;
public interface AWTVideoSizeControl extends javax.media.Control {
    AWTVideoSize getSize();
    AWTVideoSize getDefaultSize();
    java.awt.Dimension getSourceVideoSize();
    boolean setSize(AWTVideoSize s);
    AWTVideoSize checkSize(AWTVideoSize s);
}
