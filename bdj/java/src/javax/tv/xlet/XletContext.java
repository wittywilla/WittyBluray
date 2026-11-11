// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.xlet;
public interface XletContext {
    String ARGS = "javax.tv.xlet.args";
    void notifyDestroyed();
    void notifyPaused();
    Object getXletProperty(String key);
    void resumeRequest();
}
