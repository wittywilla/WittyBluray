// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.navigation;
public abstract class SIElementFilter {
    protected SIElementFilter() {}
    public abstract boolean accept(javax.tv.service.SIElement e);
}
