// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.dsmcc;
public class InvalidAddressException extends DSMCCException {
    public InvalidAddressException() {}
    public InvalidAddressException(String m) { super(m); }
}
