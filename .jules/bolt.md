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

## 2026-07-25 - SFTP Uploads Were Serialised One Packet Per Round Trip
**Learning:** `pkg/sftp`'s `File.ReadFrom` only pipelines writes when the client was built with `UseConcurrentWrites(true)`; otherwise it writes one packet and waits for the ack, so a single file's ceiling is `maxPacket / RTT` regardless of link speed. The option was never set, which pinned a 1 GB upload over a 10 ms link to **2.6 MB/s**. The tell was that `progressReader`, `throttledReader` and `contextReader` all carefully implement `Size()` and `Stat()` — methods that exist *only* to let `ReadFrom` size its concurrency window — so the plumbing was there but the switch was off.
**Action:** Set `UseConcurrentWrites(true)`. Measured against an in-memory SFTP server: 1 GB single file 28 → 39 MB/s, 5 GB 34 → 47 MB/s, three parallel files 54 → 82 MB/s, and the 10 ms link 2.6 → 21.8 MB/s (8.4x). When a reader wrapper sits between a file and `ReadFrom`, keep `Size()`/`Stat()` forwarding intact or concurrency silently disappears.

## 2026-07-25 - Payload Size Beats Concurrency for SFTP Throughput
**Learning:** After enabling pipelining, packet size was the larger remaining lever. `MaxPacket` is capped at 32 KiB by `MaxPacketChecked`, but `MaxPacketUnchecked` lifts it: 32K → 128K roughly doubled throughput again (1 GB at 91 MB/s, 10 ms link at 79 MB/s). 256 KiB does *not* work — `pkg/sftp`'s `maxMsgLength` is exactly 256 KiB and the packet header shares that budget, so the server drops the connection mid-transfer.
**Action:** Keep 32768 as the default, since it is the only payload size the SFTP spec requires every server to accept, and expose `UPLARR_SFTP_MAX_PACKET` (clamped to 128 KiB) for servers known to tolerate more. Log the effective transport settings on connect so a slow deployment can be diagnosed from the logs.

## 2026-07-25 - Concurrent Writes Break the Resume Invariant
**Learning:** Concurrent writes target explicit offsets, so when one request fails, later ones may already have succeeded — leaving a partial file that is *longer* than its contiguous data. The resume path treats the temp file as a prefix of the local file and only verifies the first 1 MB, and the final check only compares sizes, so a gap past 1 MB would pass both and silently produce a corrupt upload.
**Action:** On a `ReadFrom` error, truncate the temp file to the last contiguous byte (`pkg/sftp` parks the file offset at the earliest failed write, reachable via `Seek(0, io.SeekCurrent)`). Verify throughput changes with hashes, not sizes: an offset or packet-boundary bug yields a file of exactly the right length with the wrong bytes.

## 2026-07-25 - Renaming a File That Still Has an Open Handle
**Learning:** The upload renamed its temp file to the final name while its own remote handle was still open, since the handle was only released by a deferred close that ran after the function returned. POSIX servers allow this, so it never showed up against Linux hosts — but a Windows-hosted SFTP server fails the rename with "The process cannot access the file because it is being used by another process", stranding a fully uploaded, fully verified file under its `.tmp` name. It surfaced only because the throughput tests ran a real SFTP server on Windows.
**Action:** Close the remote handle explicitly after the copy and before verifying and renaming, with the close made idempotent so the deferred call still covers earlier error paths. Closing first also means the size check reads fully flushed data instead of whatever the server had committed. Assert the final filename in tests, not just the bytes, or this ordering can regress unnoticed.

## 2026-07-25 - Benchmark the Environment Before Optimising the Code
**Learning:** The first measurements said 8.7 MB/s and looked like an application problem. They were not. Running the *same* raw `pkg/sftp` upload with no uplarr involved reproduced 8.7 MB/s inside the container and 61.7 MB/s on the host, which proved uplarr added no measurable overhead. Two Docker Desktop artifacts were responsible: reading through a Windows bind mount (46 MB/s vs 1.5 GB/s from a native volume) and container→host NAT via vpnkit. Fixing the rig moved the same build from 8.7 to 28 MB/s before any code changed.
**Action:** Before optimising, reproduce the workload with the dependency alone and no application code. If the bare library is equally slow, the bottleneck is the environment. For Docker throughput tests on Windows, put both ends on a user-defined bridge network and keep test data in a native volume, never a host bind mount.
