// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.media;
public interface SubtitlingControl extends StreamControl, org.davic.media.SubtitlingLanguageControl {
    boolean setSubtitling(boolean on);
    boolean isSubtitlingOn();
    boolean isSubtitlingAvailable();
}
