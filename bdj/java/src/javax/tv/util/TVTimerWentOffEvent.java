// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.util;
public class TVTimerWentOffEvent extends java.util.EventObject {
    private final TVTimerSpec spec;
    public TVTimerWentOffEvent(TVTimer source, TVTimerSpec s) { super(source); spec = s; }
    public TVTimerSpec getTimerSpec() { return spec; }
    public TVTimer getTimer() { return (TVTimer) getSource(); }
}
