// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui.event;
public interface HBackgroundImageListener extends java.util.EventListener {
    void imageLoaded(HBackgroundImageEvent e);
    void imageLoadFailed(HBackgroundImageEvent e);
}
