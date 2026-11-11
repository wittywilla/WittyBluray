// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.ti.selection;
public interface TitleContext extends javax.tv.service.selection.ServiceContext {
    void start(org.bluray.ti.Title t, boolean restart) throws SecurityException;
}
