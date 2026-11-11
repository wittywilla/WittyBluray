// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.media;
public class PiPStatusEvent extends java.util.EventObject {
    private final boolean available;
    public PiPStatusEvent(boolean a, Object s) { super(s); available = a; }
    public boolean getAvailable() { return available; }
}
