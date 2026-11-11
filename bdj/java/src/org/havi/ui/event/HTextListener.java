// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui.event;
public interface HTextListener extends java.util.EventListener {
    void textChanged(HTextEvent e);
    void caretMoved(HTextEvent e);
}
