// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public class HGraphicsDevice extends HScreenDevice {
    private final HGraphicsConfiguration c = new HGraphicsConfiguration();
    protected HGraphicsDevice() {}
    public HGraphicsConfiguration[] getConfigurations() { return new HGraphicsConfiguration[] {c}; }
    public HGraphicsConfiguration getDefaultConfiguration() { return c; }
    public HGraphicsConfiguration getCurrentConfiguration() { return c; }
    public boolean setGraphicsConfiguration(HGraphicsConfiguration n) throws SecurityException, HPermissionDeniedException, HConfigurationException { return n != null; }
    public HGraphicsConfiguration getBestConfiguration(HGraphicsConfigTemplate t) { return c; }
    public HGraphicsConfiguration getBestConfiguration(HGraphicsConfigTemplate[] t) { return c; }
}
