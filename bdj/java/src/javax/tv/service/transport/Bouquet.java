// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.transport;
public interface Bouquet extends javax.tv.service.SIElement {
    String getName();
    int getBouquetID();
    javax.tv.service.SIRequest retrieveServiceDetails(javax.tv.service.SIRequestor r);
}
