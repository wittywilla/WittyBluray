// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public class HEmulatedGraphicsConfiguration extends HGraphicsConfiguration {
    protected HEmulatedGraphicsConfiguration() {}
    public HGraphicsConfiguration getImplementation() { return HScreen.getDefaultHScreen().getDefaultHGraphicsDevice().getCurrentConfiguration(); }
    public HGraphicsConfigTemplate getConfigTemplate() { return new HEmulatedGraphicsConfigTemplate(); }
}
