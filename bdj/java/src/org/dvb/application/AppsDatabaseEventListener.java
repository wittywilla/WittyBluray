// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.application;
public interface AppsDatabaseEventListener extends java.util.EventListener {
    void newDatabase(AppsDatabaseEvent e);
    void entryAdded(AppsDatabaseEvent e);
    void entryRemoved(AppsDatabaseEvent e);
    void entryChanged(AppsDatabaseEvent e);
}
