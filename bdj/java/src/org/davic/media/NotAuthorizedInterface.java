// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.davic.media;
public interface NotAuthorizedInterface {
    int NOT_ENTITLED = 0, MATURITY_RATING = 1, TECHNICAL = 2, GEOGRAPHICAL = 3;
    int getReason();
}
