// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.dsmcc;
public class InsufficientResourcesException extends DSMCCException {
    public InsufficientResourcesException() {}
    public InsufficientResourcesException(String m) { super(m); }
}
