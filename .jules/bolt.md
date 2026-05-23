## 2025-05-24 - Debouncing frontend rendering
**Learning:** Filtering DOM events like keystrokes when rendering large file lists is crucial to avoid jank. Debouncing search inputs was needed here.
**Action:** Always look for debounce/throttle opportunities when filtering large UI lists in vanilla JS.

## 2025-05-25 - Avoid disk I/O in Mutex critical sections
**Learning:** Holding a read/write mutex while performing synchronous disk operations (like `os.Stat`) in a frequently polled endpoint creates massive lock contention and slows down the entire application (e.g., UI freezing, workers blocked).
**Action:** Always extract disk I/O out of the locked scope. Take a quick snapshot of the needed data in memory while locked, release the lock, and then perform the slow I/O operations on the snapshot.
## 2024-04-24 - Optimizing String Sorting Performance in Large Lists
**Learning:** `String.prototype.localeCompare` is significantly slower (up to 40x) than using an initialized `Intl.Collator` instance when executed within tight loops like `Array.prototype.sort()`. This creates notable jank when sorting large arrays, such as a file list.
**Action:** When sorting arrays of strings on the frontend, particularly lists that can grow large, initialize `Intl.Collator` once and reuse its `.compare()` method instead of calling `.localeCompare` directly on the strings.
## 2025-05-25 - Avoid O(T*H) Nested Loops in QueueManager
**Learning:** The Go backend's `QueueManager` uses an in-memory task slice (`qm.tasks`) and host limiters map (`qm.limiters`) protected by a read/write mutex (`qm.mu`). Operations within this lock must remain optimized (e.g., using $O(T+H)$ map aggregation instead of $O(T \times H)$ nested loops) to prevent lock contention and avoid blocking frequently polled API endpoints like `/api/stats`.
**Action:** When aggregating statistics across multiple collections inside a locked region, prioritize $O(N)$ single-pass aggregation into intermediate maps instead of $O(N^2)$ nested iterations.
