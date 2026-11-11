// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public class SizeChangeEvent extends ControllerEvent {
    private final int width, height;
    private final float scale;
    public SizeChangeEvent(Controller from, int width, int height, float scale) { super(from); this.width = width; this.height = height; this.scale = scale; }
    public int getWidth() { return width; }
    public int getHeight() { return height; }
    public float getScale() { return scale; }
}
