// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.media;
public interface PlayListChangeControl extends javax.media.Control {
    void selectPlayList(org.bluray.net.BDLocator l) throws InvalidPlayListException, org.davic.net.InvalidLocatorException, org.davic.media.NotAuthorizedException;
    org.bluray.net.BDLocator getCurrentPlayList();
    void addPlayListChangeListener(PlayListChangeListener l);
    void removePlayListChangeListener(PlayListChangeListener l);
}
