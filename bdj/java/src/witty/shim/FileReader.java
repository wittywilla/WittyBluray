// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package witty.shim;

public class FileReader extends java.io.InputStreamReader {
    public FileReader(String path) throws java.io.FileNotFoundException { super(new FileInputStream(path)); }
    public FileReader(File f) throws java.io.FileNotFoundException { super(new FileInputStream(f)); }
}
