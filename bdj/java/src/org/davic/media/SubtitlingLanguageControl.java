// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.davic.media;
public interface SubtitlingLanguageControl extends LanguageControl {
    boolean isSubtitlingOn();
    boolean setSubtitling(boolean on);
}
