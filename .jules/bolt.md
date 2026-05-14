## 2025-02-27 - O(H * T) nested loop to O(H + T)
**Learning:** Found a nested loop O(H * T) traversing queue tasks within host limiters when fetching queue statistics inside a read/write mutex lock in the backend `QueueManager`.
**Action:** Replaced the implementation to first aggregate statistics into a hash map making a single pass O(T), followed by iterating over the map and limiters O(H). This changes complexity from O(H * T) to O(H + T) and reduces contention.
