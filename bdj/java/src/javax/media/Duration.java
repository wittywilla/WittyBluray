// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public interface Duration {
    Time DURATION_UNBOUNDED = new Time(Long.MAX_VALUE);
    Time DURATION_UNKNOWN = new Time(Long.MAX_VALUE - 1);
    Time getDuration();
}
