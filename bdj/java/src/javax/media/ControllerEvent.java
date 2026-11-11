// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public class ControllerEvent extends java.util.EventObject {
    public ControllerEvent(Controller c) { super(c); }
    public Controller getSourceController() { return (Controller) getSource(); }
}
