// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.guide;
public class ProgramScheduleEvent extends javax.tv.service.SIChangeEvent {
    public ProgramScheduleEvent(Object source, javax.tv.service.SIChangeType t, ProgramEvent e) { super(source, t, e); }
    public ProgramEvent getProgramEvent() { return (ProgramEvent) getSIElement(); }
}
