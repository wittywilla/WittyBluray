// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.media;
public class AspectRatioChangedEvent extends VideoFormatEvent {
    public AspectRatioChangedEvent(Object source, int ratio) { super(source, ratio); }
    public int getNewValue() { return value(); }
}
