// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.media;
public interface UOMaskTableControl extends javax.media.Control {
    boolean[] getMaskedUOTable();
    void addUOMaskTableEventListener(UOMaskTableListener l);
    void removeUOMaskTableEventListener(UOMaskTableListener l);
}
