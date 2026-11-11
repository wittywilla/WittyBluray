// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media.protocol;
public interface PushSourceStream extends SourceStream {
    int read(byte[] buffer, int offset, int length) throws java.io.IOException;
    int getMinimumTransferSize();
    void setTransferHandler(SourceTransferHandler transferHandler);
}
