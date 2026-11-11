// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public class RateChangeEvent extends ControllerEvent {
    private final float r;
    public RateChangeEvent(Controller c, float v) { super(c); r = v; }
    public float getRate() { return r; }
}
