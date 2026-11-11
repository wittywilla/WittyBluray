// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.navigation;
public abstract class ServiceFilter {
    protected ServiceFilter() {}
    public abstract boolean accept(javax.tv.service.Service s);
}
