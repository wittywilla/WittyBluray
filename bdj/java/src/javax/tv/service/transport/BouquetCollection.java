// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.transport;
public interface BouquetCollection extends Transport {
    javax.tv.service.SIRequest retrieveBouquets(javax.tv.service.SIRequestor r);
    void addBouquetChangeListener(BouquetChangeListener l);
    void removeBouquetChangeListener(BouquetChangeListener l);
}
