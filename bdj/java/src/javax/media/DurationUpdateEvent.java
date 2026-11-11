// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public class DurationUpdateEvent extends ControllerEvent {
    private final Time d;
    public DurationUpdateEvent(Controller c, Time v) { super(c); d = v; }
    public Time getDuration() { return d; }
}
