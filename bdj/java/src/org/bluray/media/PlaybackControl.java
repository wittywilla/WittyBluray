// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.media;
public interface PlaybackControl extends javax.media.Control {
    void addPlaybackControlListener(PlaybackListener l);
    void removePlaybackControlListener(PlaybackListener l);
    void skipToMark(int mark) throws IllegalArgumentException;
    boolean skipToNextMark(int type) throws IllegalArgumentException;
    boolean skipToPreviousMark(int type) throws IllegalArgumentException;
    void skipToPlayItem(int item) throws IllegalArgumentException;
}
