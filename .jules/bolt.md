## 2025-05-24 - Debouncing frontend rendering
**Learning:** Filtering DOM events like keystrokes when rendering large file lists is crucial to avoid jank. Debouncing search inputs was needed here.
**Action:** Always look for debounce/throttle opportunities when filtering large UI lists in vanilla JS.

## 2025-05-25 - Avoid disk I/O in Mutex critical sections
**Learning:** Holding a read/write mutex while performing synchronous disk operations (like `os.Stat`) in a frequently polled endpoint creates massive lock contention and slows down the entire application (e.g., UI freezing, workers blocked).
**Action:** Always extract disk I/O out of the locked scope. Take a quick snapshot of the needed data in memory while locked, release the lock, and then perform the slow I/O operations on the snapshot.
## 2024-04-24 - Optimizing String Sorting Performance in Large Lists
**Learning:** `String.prototype.localeCompare` is significantly slower (up to 40x) than using an initialized `Intl.Collator` instance when executed within tight loops like `Array.prototype.sort()`. This creates notable jank when sorting large arrays, such as a file list.
**Action:** When sorting arrays of strings on the frontend, particularly lists that can grow large, initialize `Intl.Collator` once and reuse its `.compare()` method instead of calling `.localeCompare` directly on the strings.
## 2024-05-25 - Avoid O(T*H) Nested Loops in Mutex Critical Sections
**Learning:** Using nested loops for data aggregation inside a read/write mutex critical section, especially in frequently polled API endpoints like `GetHostStats`, causes significant lock contention. Here, iterating tasks per host resulted in O(T*H) complexity.
**Action:** When aggregating states from multiple collections protected by a read/write mutex, use map aggregation to convert O(T*H) operations into O(T+H). Build an aggregated state map from one collection first, then correlate it with the other to minimize time spent holding the lock.
