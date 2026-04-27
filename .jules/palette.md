## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
## 2026-03-05 - Adding accessible context to dynamic table actions
**Learning:** Dynamically generated action buttons inside data tables (like pause, retry, and remove buttons in the task queue) often lack explicit screen reader context when multiple rows have identical button labels. A screen reader reading 'Remove' doesn't explain what item is being removed.
**Action:** When rendering iterative actions in lists or tables, dynamically attach context-aware `aria-label` attributes to those buttons (e.g., 'Remove [filename]') to make them fully accessible.
