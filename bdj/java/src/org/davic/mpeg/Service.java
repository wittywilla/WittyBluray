// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.davic.mpeg;
public class Service {
    private final int id;
    protected Service(int id) { this.id = id; }
    public int getServiceId() { return id; }
    public ElementaryStream[] retrieveElementaryStreams() { return new ElementaryStream[0]; }
}
