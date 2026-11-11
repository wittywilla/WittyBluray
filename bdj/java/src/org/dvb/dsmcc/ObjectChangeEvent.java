// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.dsmcc;
public class ObjectChangeEvent extends java.util.EventObject {
    private final int version;
    public ObjectChangeEvent(Object s) { this(s, 0); }
    public ObjectChangeEvent(Object s, int v) { super(s); version = v; }
    public int getNewVersion() { return version; }
}
