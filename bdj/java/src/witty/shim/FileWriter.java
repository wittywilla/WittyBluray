// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package witty.shim;

import java.io.IOException;

public class FileWriter extends java.io.OutputStreamWriter {
    public FileWriter(String path) throws IOException { super(new FileOutputStream(path)); }
    public FileWriter(String path, boolean append) throws IOException { super(new FileOutputStream(path, append)); }
    public FileWriter(File f) throws IOException { super(new FileOutputStream(f)); }
    public FileWriter(File f, boolean append) throws IOException { super(new FileOutputStream(f, append)); }
}
