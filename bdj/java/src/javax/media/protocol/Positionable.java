// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media.protocol;
public interface Positionable {
    int RoundUp = 1, RoundDown = 2, RoundNearest = 3;
    javax.media.Time setPosition(javax.media.Time where, int rounding);
    boolean isRandomAccess();
}
