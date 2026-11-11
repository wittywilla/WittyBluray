// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.bdplus;
/** BD+ is not present on this player, and never will be: linking this class is all that works. */
public class Status {
    public static final int NOT_PRESENT = 0;
    public static int getStatus() { return NOT_PRESENT; }
    public static boolean isPresent() { return false; }
}
