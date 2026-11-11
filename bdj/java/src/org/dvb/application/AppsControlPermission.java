// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.application;
public class AppsControlPermission extends java.security.BasicPermission {
    public AppsControlPermission(String name, String actions) { super(name, actions); }
    public AppsControlPermission(String name) { super(name); }
}
