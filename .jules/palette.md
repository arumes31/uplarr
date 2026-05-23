## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
## 2026-05-23 - Adding accessibility to file rows and checkboxes
**Learning:** Found that custom interactive elements like `.clickable-row` representing file and directory lists lacked keyboard support (tabIndex, keydown) and descriptive ARIA labels, making them inaccessible to screen readers and keyboard-only users. Additionally, disabled checkboxes lacked context for why they were disabled.
**Action:** When creating custom interactive rows or lists, ensure they receive focus via `tabIndex`, respond to `Enter` and `Space` keys, and have descriptive `aria-label`s. Always provide tooltips (`title` attribute) for disabled elements to explain the constraint to the user.
