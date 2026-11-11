// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.io.persistent;
public class FileAccessPermissions {
    private final boolean rw, ww, ro, wo, ra, wa;
    public FileAccessPermissions(boolean readWorld, boolean writeWorld, boolean readOrg, boolean writeOrg, boolean readApp, boolean writeApp) { rw = readWorld; ww = writeWorld; ro = readOrg; wo = writeOrg; ra = readApp; wa = writeApp; }
    public boolean hasReadWorldAccess() { return rw; }
    public boolean hasWriteWorldAccess() { return ww; }
    public boolean hasReadOrganisationAccess() { return ro; }
    public boolean hasWriteOrganisationAccess() { return wo; }
    public boolean hasReadApplicationAccess() { return ra; }
    public boolean hasWriteApplicationAccess() { return wa; }
}
