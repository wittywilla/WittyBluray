// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public class ConnectionErrorEvent extends ControllerErrorEvent {
    public ConnectionErrorEvent(Controller c) { super(c); }
    public ConnectionErrorEvent(Controller c, String m) { super(c, m); }
}
