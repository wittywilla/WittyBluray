// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service;
public abstract class SIChangeEvent extends java.util.EventObject {
    private final SIChangeType type;
    private final SIElement element;
    public SIChangeEvent(Object source, SIChangeType t, SIElement e) { super(source); type = t; element = e; }
    public SIChangeType getChangeType() { return type; }
    public SIElement getSIElement() { return element; }
}
