// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.davic.media;
public interface FreezeControl extends javax.media.Control {
    void freeze() throws MediaFreezeException;
    void resume() throws MediaFreezeException;
}
