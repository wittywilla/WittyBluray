// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.transport;
public interface TransportStreamCollection extends Transport {
    javax.tv.service.SIRequest retrieveTransportStreams(javax.tv.service.SIRequestor r);
    void addTransportStreamChangeListener(TransportStreamChangeListener l);
    void removeTransportStreamChangeListener(TransportStreamChangeListener l);
}
