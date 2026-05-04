## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
## 2024-05-04 - Dynamic Element Accessibility
**Learning:** Dynamically generated tables (like file lists and queues) often contain identical generic text for interactive elements (e.g., "Pause" or empty checkboxes), causing confusion for screen reader users as they navigate rows without context.
**Action:** When generating rows dynamically in Vanilla JS, always inject context-specific data from the row object (like `file.name` or `task.file_name`) directly into the `aria-label` attribute of its interactive elements to ensure individual element distinctness.
