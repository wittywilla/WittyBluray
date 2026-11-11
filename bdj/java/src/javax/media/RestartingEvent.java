// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public class RestartingEvent extends StopEvent {
    public RestartingEvent(Controller from, int previous, int current, int target, Time mediaTime) { super(from, previous, current, target, mediaTime); }
}
