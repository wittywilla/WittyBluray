// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.user;
public final class UserPreferencePermission extends java.security.BasicPermission {
    public UserPreferencePermission(String name) { super(name); }
    public UserPreferencePermission(String name, String actions) { super(name, actions); }
}
