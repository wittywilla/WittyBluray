// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.dsmcc;
public class NotEntitledException extends DSMCCException {
    public NotEntitledException() {}
    public NotEntitledException(String m) { super(m); }
}
