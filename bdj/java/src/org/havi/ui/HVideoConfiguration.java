// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public class HVideoConfiguration extends HScreenConfiguration {
    protected HVideoConfiguration() {}
    public HVideoDevice getDevice() { return HScreen.getDefaultHScreen().getDefaultHVideoDevice(); }
    public HVideoConfigTemplate getConfigTemplate() { return new HVideoConfigTemplate(); }
}
