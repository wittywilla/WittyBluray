// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.media;
public class DFCChangedEvent extends VideoFormatEvent {
    public DFCChangedEvent(Object source, int dfc) { super(source, dfc); }
    public int getNewValue() { return value(); }
}
