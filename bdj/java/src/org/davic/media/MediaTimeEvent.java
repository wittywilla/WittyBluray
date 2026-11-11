// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.davic.media;
public class MediaTimeEvent extends java.util.EventObject {
    private final javax.media.Time time;
    public MediaTimeEvent(Object source, javax.media.Time mediaTime) { super(source); time = mediaTime; }
    public javax.media.Time getMediaTime() { return time; }
}
