## 2025-05-24 - Debouncing frontend rendering
**Learning:** Filtering DOM events like keystrokes when rendering large file lists is crucial to avoid jank. Debouncing search inputs was needed here.
**Action:** Always look for debounce/throttle opportunities when filtering large UI lists in vanilla JS.

## 2025-05-25 - Avoid disk I/O in Mutex critical sections
**Learning:** Holding a read/write mutex while performing synchronous disk operations (like `os.Stat`) in a frequently polled endpoint creates massive lock contention and slows down the entire application (e.g., UI freezing, workers blocked).
**Action:** Always extract disk I/O out of the locked scope. Take a quick snapshot of the needed data in memory while locked, release the lock, and then perform the slow I/O operations on the snapshot.
## 2024-04-24 - Optimizing String Sorting Performance in Large Lists
**Learning:** `String.prototype.localeCompare` is significantly slower (up to 40x) than using an initialized `Intl.Collator` instance when executed within tight loops like `Array.prototype.sort()`. This creates notable jank when sorting large arrays, such as a file list.
**Action:** When sorting arrays of strings on the frontend, particularly lists that can grow large, initialize `Intl.Collator` once and reuse its `.compare()` method instead of calling `.localeCompare` directly on the strings.

## 2025-05-25 - Using DocumentFragment for batching DOM insertions
**Learning:** Appending DOM nodes one by one in a loop inside vanilla JS scripts triggers unnecessary browser layout recalculations and repaints on every insertion, causing severe performance issues when the list of elements is large.
**Action:** When creating and inserting multiple elements dynamically in vanilla JavaScript, always batch the operations by creating a `DocumentFragment` first, appending the new elements to it within the loop, and then appending the complete fragment to the target DOM node in a single action. This ensures O(1) DOM reflows instead of O(N) reflows.
