## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
## 2026-03-05 - Accessibility of dynamic form elements and complex rows
**Learning:** Found that dynamically generated checkboxes in file lists lacked `aria-label` attributes and context for their disabled state (`title`), making them inaccessible to screen readers. Additionally, interactive table rows acting as folders lacked keyboard navigation (`tabIndex` and `keydown` handling).
**Action:** Always ensure dynamically generated form elements have proper ARIA attributes (`aria-label`) and explanatory attributes (`title` for disabled state). When implementing keyboard navigation on complex rows, explicitly check the event target to avoid overriding the default behavior of child inputs (like checkboxes).
