1. **Remove `os.Stat` from `GetTasks()`:**
   - In `internal/queue/manager.go`, the `GetTasks()` method currently executes an `os.Stat` call for every task in the `TaskFailed` or `TaskCompleted` state.
   - Remove the `os.Stat` call inside the `snapshot` loop.
   - Set `copyTask.LocalFileExists = true` unconditionally for all tasks in the snapshot.
   - This eliminates O(N) disk I/O operations from `GetTasks()`, relying instead on the existing `os.Stat` validation within `ControlTask` when a retry action is actually invoked.

2. **Run tests:**
   - Run `go test ./...` to verify the changes do not break any existing backend logic.
   - Ensure the codebase builds correctly with `go run ./cmd/uplarr`.

3. **Complete pre-commit steps:** Complete pre-commit steps to ensure proper testing, verification, review, and reflection are done.

4. **Submit Change:** Create a PR with title "⚡ Bolt: Remove O(n) disk I/O from queue polling endpoint".
