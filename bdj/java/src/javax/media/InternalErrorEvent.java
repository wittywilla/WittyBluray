// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public class InternalErrorEvent extends ControllerErrorEvent {
    public InternalErrorEvent(Controller c) { super(c); }
    public InternalErrorEvent(Controller c, String m) { super(c, m); }
}
