// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package javax.tv.service.navigation;
public interface ServiceIterator {
    javax.tv.service.Service nextService();
    javax.tv.service.Service previousService();
    boolean hasNext();
    boolean hasPrevious();
    void toBeginning();
    void toEnd();
}
