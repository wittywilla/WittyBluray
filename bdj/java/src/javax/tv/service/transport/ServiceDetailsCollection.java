// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.transport;
public interface ServiceDetailsCollection extends Transport {
    javax.tv.service.SIRequest retrieveServiceDetails(javax.tv.service.SIRequestor r);
    void addServiceDetailsChangeListener(ServiceDetailsChangeListener l);
    void removeServiceDetailsChangeListener(ServiceDetailsChangeListener l);
}
