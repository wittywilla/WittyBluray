// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.media;
public class ActiveFormatDescriptionChangedEvent extends VideoFormatEvent {
    public ActiveFormatDescriptionChangedEvent(Object source, int afd) { super(source, afd); }
    public int getNewValue() { return value(); }
}
