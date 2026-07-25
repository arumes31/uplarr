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

## 2026-07-25 - Derive Sort Keys Once, Not Per Comparison
**Learning:** The `type` branch of `sortFiles` recomputed each file's extension inside the comparator with `split('.').pop()`, so every one of the O(n log n) comparisons allocated two throwaway arrays. Work done inside a comparator is multiplied by the comparison count, not the element count.
**Action:** Use decorate-sort-undecorate when a sort key needs deriving: map each element to `{ item, key }` once, sort on the precomputed key, then map back. Extract the extension with `lastIndexOf('.')` and `slice` rather than `split`, which allocates.

## 2026-07-25 - Skip Filter Passes That Cannot Exclude Anything
**Learning:** `renderLocalFiles` and `renderRemoteFiles` ran `.filter()` on every render even when the search box was empty, walking the whole list and lowercasing every name to rebuild an identical array. The empty query is the common case, since the filter is only populated while the user is typing.
**Action:** Guard the filter on a non-empty query and pass the source array straight through. This is only safe because `sortFiles` copies its input; verify the downstream consumer does not mutate before removing a defensive copy.

## 2026-07-25 - Verifying a Comparator Refactor Without a Test Suite
**Learning:** Rewriting sort logic in a repo with no JS tests is where silent behaviour changes hide. Extension parsing in particular has edge cases (`.bashrc`, `file.`, `a.b.c.gz`, no dot at all) where a plausible-looking `split`-to-`slice` swap can diverge.
**Action:** Before landing, run the old and new implementations side by side over randomized inputs plus a hand-written edge-case list, comparing full output orderings and asserting the input array is not mutated. A throwaway script is enough and it is fast to write.
