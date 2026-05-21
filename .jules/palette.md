## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
## 2026-05-21 - Added Accessibility Attributes to Dynamic Checkboxes
**Learning:** Found that dynamically generated checkboxes in the vanilla JS file list lacked `aria-label` attributes, making them inaccessible to screen readers. Furthermore, when disabled (e.g., for directories), there was no semantic or visual explanation why they were inactive.
**Action:** Always add context-specific `aria-label` attributes (e.g., incorporating the file name) to dynamic form elements like checkboxes. For disabled elements, include a `title` attribute explicitly explaining the restriction to improve UX clarity.
