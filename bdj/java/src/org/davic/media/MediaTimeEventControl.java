// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.davic.media;
public interface MediaTimeEventControl extends javax.media.Control {
    void notifyWhen(MediaTimeEventListener l, javax.media.Time mediaTime);
    void removeMediaTimeEventListener(MediaTimeEventListener l);
}
