// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public abstract class HEmulatedGraphicsDevice extends HGraphicsDevice {
    protected HEmulatedGraphicsDevice() {}
    public boolean setGraphicsConfiguration(HEmulatedGraphicsConfiguration n) throws SecurityException, HPermissionDeniedException, HConfigurationException { return n != null; }
}
