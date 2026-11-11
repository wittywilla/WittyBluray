// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package org.havi.ui;
public interface HAnimateEffect {
    int PLAY_REPEATING = 1, PLAY_ALTERNATING = 2, REPEAT_INFINITE = -1;
    void start();
    void stop();
    boolean isAnimated();
    void setPosition(int p);
    int getPosition();
    void setRepeatCount(int c);
    int getRepeatCount();
    void setDelay(int d);
    int getDelay();
    void setPlayMode(int m);
    int getPlayMode();
}
