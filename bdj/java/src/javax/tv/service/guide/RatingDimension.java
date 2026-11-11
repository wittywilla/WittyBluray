// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.guide;
public interface RatingDimension extends javax.tv.service.SIElement {
    String getDimensionName();
    String getRatingRegion();
    int getNumberOfLevels();
    String getRatingLevelDescription(int level);
}
