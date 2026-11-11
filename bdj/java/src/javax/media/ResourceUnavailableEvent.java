// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public class ResourceUnavailableEvent extends ControllerErrorEvent {
    public ResourceUnavailableEvent(Controller c) { super(c); }
    public ResourceUnavailableEvent(Controller c, String m) { super(c, m); }
}
