// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.dvb.dsmcc;
public class ServiceXFRReference {
    private final byte[] nsap;
    private final String path;
    public ServiceXFRReference(byte[] nsap, String path) { this.nsap = nsap == null ? new byte[0] : nsap.clone(); this.path = path; }
    public byte[] getNSAPAddress() { return nsap.clone(); }
    public String getPathName() { return path; }
}
