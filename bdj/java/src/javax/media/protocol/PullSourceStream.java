// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media.protocol;
public interface PullSourceStream extends SourceStream {
    boolean willReadBlock();
    int read(byte[] buffer, int offset, int length) throws java.io.IOException;
}
