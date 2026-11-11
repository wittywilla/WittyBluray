// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.media;
public interface BackgroundVideoPresentationControl extends VideoPresentationControl {
    boolean setVideoTransformation(VideoTransformation t);
    VideoTransformation getVideoTransformation();
    VideoTransformation getClosestMatch(VideoTransformation t);
}
