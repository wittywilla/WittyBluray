// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.davic.media;
public interface MediaTimePositionControl extends javax.media.Control {
    javax.media.Time setMediaTimePosition(javax.media.Time t) throws MediaTimeOutOfBoundsException;
    javax.media.Time getMediaTimePosition();
}
