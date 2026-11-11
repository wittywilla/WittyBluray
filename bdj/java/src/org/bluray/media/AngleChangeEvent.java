// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.media;
public class AngleChangeEvent extends java.util.EventObject {
    private final int a;
    public AngleChangeEvent(Object s, int v) { super(s); a = v; }
    public int getAngle() { return a; }
}
