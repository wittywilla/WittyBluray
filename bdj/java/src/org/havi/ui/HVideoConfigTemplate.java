// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public class HVideoConfigTemplate extends HScreenConfigTemplate {
    public static final int GRAPHICS_MIXING = 15;
    public HVideoConfigTemplate() {}
    public boolean isConfigSupported(HScreenConfiguration c) { return c instanceof HVideoConfiguration; }
}
