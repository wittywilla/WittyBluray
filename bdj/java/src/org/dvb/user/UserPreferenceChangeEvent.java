// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.user;
public class UserPreferenceChangeEvent extends java.util.EventObject {
    private final String name;
    public UserPreferenceChangeEvent(Object source, String name) { super(source); this.name = name; }
    public String getName() { return name; }
}
