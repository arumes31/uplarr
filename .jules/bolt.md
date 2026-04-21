## 2026-04-21 - [Optimize GetTasks stat call]
**Learning:** Checking file existence with `os.Stat` on every task in the queue during `GetTasks()` is a major bottleneck since `GetTasks()` is polled frequently by the UI. Only tasks that are Completed or Failed actually need the `local_file_exists` status for the UI to display the "Retry" button.
**Action:** Move the `os.Stat` check inside an `if` block so it only runs for `TaskCompleted` and `TaskFailed` states.
