// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.bluray.storage;
public class BindingUnitDataArea extends DataArea {
    public String getRoot() { String r = witty.shim.System.getProperty("bluray.bindingunit.root"); return r != null ? r : super.getRoot(); }
}
