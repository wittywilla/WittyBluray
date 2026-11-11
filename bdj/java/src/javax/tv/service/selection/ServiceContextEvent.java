// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.selection;
public class ServiceContextEvent extends java.util.EventObject {
    private final ServiceContext c;
    public ServiceContextEvent(ServiceContext s) { super(s); c = s; }
    public ServiceContext getServiceContext() { return c; }
}
