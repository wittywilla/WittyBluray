// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.media;
public class SystemTimeBase implements TimeBase {
    public SystemTimeBase() {}
    public Time getTime() { return new Time(getNanoseconds()); }
    public long getNanoseconds() { return java.lang.System.nanoTime(); }
}
