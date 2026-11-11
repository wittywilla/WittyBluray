// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.davic.resources;
public interface ResourceClient {
    boolean requestRelease(ResourceProxy p, Object d);
    void release(ResourceProxy p);
    void notifyRelease(ResourceProxy p);
}
