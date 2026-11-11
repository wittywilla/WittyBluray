// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.davic.resources;
public interface ResourceServer {
    void addResourceStatusEventListener(ResourceStatusListener l);
    void removeResourceStatusEventListener(ResourceStatusListener l);
}
