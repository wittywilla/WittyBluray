// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.storage;
public class StorageManager {
    private static final StorageManager it = new StorageManager();
    private StorageManager() {}
    public static StorageManager instance() { return it; }
    public DataArea getBindingUnitDataArea() { return new BindingUnitDataArea(); }
    public DataArea getApplicationDataArea() { return new ApplicationDataArea(); }
    public DataArea getPersistentDataArea() { return new PersistentDataArea(); }
}
