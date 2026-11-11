// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.navigation;
public class FavoriteServicesName {
    private final String name;
    public FavoriteServicesName(String n) { name = n; }
    public String getName() { return name; }
    public boolean equals(Object o) { return o instanceof FavoriteServicesName && String.valueOf(name).equals(String.valueOf(((FavoriteServicesName) o).name)); }
    public int hashCode() { return String.valueOf(name).hashCode(); }
    public String toString() { return String.valueOf(name); }
}
