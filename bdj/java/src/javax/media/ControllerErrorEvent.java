// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public class ControllerErrorEvent extends ControllerClosedEvent {
    public ControllerErrorEvent(Controller c) { super(c); }
    public ControllerErrorEvent(Controller c, String m) { super(c, m); }
}
