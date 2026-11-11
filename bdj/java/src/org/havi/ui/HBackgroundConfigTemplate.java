// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public class HBackgroundConfigTemplate extends HScreenConfigTemplate {
    public static final int CHANGEABLE_SINGLE_COLOR = 15, STILL_IMAGE = 16;
    public HBackgroundConfigTemplate() {}
    public boolean isConfigSupported(HScreenConfiguration c) { return c instanceof HBackgroundConfiguration; }
}
