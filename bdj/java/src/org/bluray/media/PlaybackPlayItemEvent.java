// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.media;
public class PlaybackPlayItemEvent extends java.util.EventObject {
    private final int item;
    public PlaybackPlayItemEvent(Object s, int i) { super(s); item = i; }
    public int getPlayItem() { return item; }
}
