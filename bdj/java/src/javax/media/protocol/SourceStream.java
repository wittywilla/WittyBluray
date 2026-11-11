// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media.protocol;
public interface SourceStream extends javax.media.Controls {
    long LENGTH_UNKNOWN = -1L;
    ContentDescriptor getContentDescriptor();
    long getContentLength();
    boolean endOfStream();
}
