// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.guide;
public interface ContentRatingAdvisory {
    String getDisplayText();
    String getRatingRegion();
    String[] getDimensionNames();
    String getRatingDescription(String dimensionName);
}
