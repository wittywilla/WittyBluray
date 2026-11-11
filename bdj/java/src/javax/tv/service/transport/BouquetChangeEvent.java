// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.transport;
public class BouquetChangeEvent extends javax.tv.service.SIChangeEvent {
    public BouquetChangeEvent(Object source, javax.tv.service.SIChangeType t, Bouquet b) { super(source, t, b); }
    public Bouquet getBouquet() { return (Bouquet) getSIElement(); }
}
