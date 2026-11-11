// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public interface Player extends MediaHandler, Controller {
    java.awt.Component getVisualComponent();
    GainControl getGainControl();
    java.awt.Component getControlPanelComponent();
    void start();
    void addController(Controller c) throws IncompatibleTimeBaseException;
    void removeController(Controller c);
}
