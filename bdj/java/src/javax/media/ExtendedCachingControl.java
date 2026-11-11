// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public interface ExtendedCachingControl extends CachingControl {
    void addDownloadProgressListener(DownloadProgressListener l, int numIntervals);
    void removeDownloadProgressListener(DownloadProgressListener l);
    long getStartOffset();
    long getEndOffset();
    void pauseDownload();
    void resumeDownload();
}
