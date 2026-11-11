// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.graphics;
public class TVContainer {
    private TVContainer() {}
    public static java.awt.Container getRootContainer(javax.tv.xlet.XletContext ctx) { return org.havi.ui.HSceneFactory.getInstance().getDefaultHScene(); }
}
