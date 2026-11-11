// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.davic.media;
public class MediaLocator extends javax.media.MediaLocator {
    public MediaLocator(String s) { super(s); }
    public MediaLocator(org.davic.net.Locator l) { super(l.toExternalForm()); }
}
