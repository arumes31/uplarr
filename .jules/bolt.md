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
## 2025-05-26 - Batch Disk Operations for State Preservation
**Learning:** Saving state to a `.json` file synchronously within a loop for individual items (e.g., adding multiple tasks) causes O(N) disk writes and can significantly block performance during bulk operations.
**Action:** Expose batching variants of queue addition functions (e.g., `AddTasks`) that loop through state modifications in memory under a lock, and then perform a single disk I/O operation (`saveState()`) at the end.
## 2026-06-04 - Pre-aggregate Task Stats Under Mutexes
**Learning:** Running an O(T * H) nested loop under a `sync.RWMutex` to aggregate stats can cause high lock contention for frequently polled API endpoints (like `/api/stats`).
**Action:** Pre-aggregate task data in a single O(T) pass into a map, and then iterate through the map in an O(H) pass to compute host stats. This brings the complexity down to O(T + H) and minimizes time spent under the read lock.
## 2024-04-24 - Avoiding Array Allocation in Array Sorting Comparators
**Learning:** Instantiating arrays within a sorting comparator function inside an Array's `sort` method (like `a.name.split('.')`) incurs unnecessary memory allocations and garbage collection overhead, particularly inside tight loops like `O(N log N)` array sorts.
**Action:** Replace string-to-array methods with direct primitive string operations (like `lastIndexOf` and `substring`) inside tight comparator loops to minimize allocation overhead.

## 2024-04-24 - Conditionally Bypassing List Filtering
**Learning:** Executing `.filter()` over large lists allocates a new array unconditionally, even if the filter condition matches all elements. This introduces a redundant `O(N)` loop and `O(N)` memory allocation if the user hasn't provided a filter query.
**Action:** Before running `.filter()` on large arrays for UI rendering, check if the filter criteria are active (e.g. check if the search string is not empty). If inactive, bypass the filter step and operate directly on the original array reference.
