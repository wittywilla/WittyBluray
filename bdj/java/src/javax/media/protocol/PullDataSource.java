// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media.protocol;
public abstract class PullDataSource extends DataSource {
    public PullDataSource() {}
    public PullDataSource(javax.media.MediaLocator l) { super(l); }
    public abstract PullSourceStream[] getStreams();
}
