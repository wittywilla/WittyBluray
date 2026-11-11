// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.media;
public class VideoFormatEvent extends java.util.EventObject {
    private final int value;
    public VideoFormatEvent(Object source) { this(source, 0); }
    protected VideoFormatEvent(Object source, int value) { super(source); this.value = value; }
    protected int value() { return value; }
}
