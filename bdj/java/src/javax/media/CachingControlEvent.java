// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public class CachingControlEvent extends ControllerEvent {
    private final CachingControl control;
    private final long progress;
    public CachingControlEvent(Controller from, CachingControl cc, long progress) { super(from); control = cc; this.progress = progress; }
    public CachingControl getCachingControl() { return control; }
    public long getContentProgress() { return progress; }
}
