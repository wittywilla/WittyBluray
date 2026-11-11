// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.media;
public class PlaybackMarkEvent extends java.util.EventObject {
    private final int mark;
    public PlaybackMarkEvent(Object s, int m) { super(s); mark = m; }
    public int getMark() { return mark; }
}
