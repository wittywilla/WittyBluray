// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.net.rc;
public class ConnectionParameters {
    private final String number, user, password;
    public ConnectionParameters(String number, String username, String password) { this.number = number; user = username; this.password = password; }
    public String getTelephoneNumber() { return number; }
    public String getUsername() { return user; }
    public String getPassword() { return password; }
}
