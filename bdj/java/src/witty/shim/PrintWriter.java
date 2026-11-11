// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package witty.shim;

import java.io.OutputStream;
import java.io.Writer;

/** Without the constructors that open files. */
public class PrintWriter extends java.io.PrintWriter {
    public PrintWriter(Writer w) { super(w); }
    public PrintWriter(Writer w, boolean flush) { super(w, flush); }
    public PrintWriter(OutputStream o) { super(o); }
    public PrintWriter(OutputStream o, boolean flush) { super(o, flush); }
}
