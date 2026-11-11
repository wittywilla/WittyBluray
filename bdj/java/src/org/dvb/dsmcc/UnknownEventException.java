// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.dsmcc;
public class UnknownEventException extends DSMCCException {
    public UnknownEventException() {}
    public UnknownEventException(String m) { super(m); }
}
