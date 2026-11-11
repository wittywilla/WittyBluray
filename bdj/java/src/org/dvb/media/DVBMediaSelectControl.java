// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.media;
public interface DVBMediaSelectControl extends javax.tv.media.MediaSelectControl {
    void selectServiceMediaHandler(javax.tv.locator.Locator l) throws org.davic.net.InvalidLocatorException, javax.tv.service.selection.InvalidServiceComponentException, org.davic.media.NotAuthorizedException;
}
