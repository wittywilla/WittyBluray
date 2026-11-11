// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.user;
public class UnsupportedPreferenceException extends Exception {
    private final String preference;
    public UnsupportedPreferenceException() { preference = null; }
    public UnsupportedPreferenceException(String m, String preference) { super(m); this.preference = preference; }
    public String getPreference() { return preference; }
}
