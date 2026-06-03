## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
## 2026-06-03 - Adding context to dynamic table elements
**Learning:** Found that dynamic elements in tables, such as row checkboxes or action buttons, lacked contextual details in their `aria-label` or `title` attributes, making them confusing for screen reader users as they all sounded identical (e.g. "checkbox", "Pause", "Remove").
**Action:** When adding interactive elements like checkboxes or inline buttons to dynamic tables (e.g., file lists, task queues), dynamically inject contextual details like the file name into the `aria-label` or `title` attributes to provide adequate context for screen readers. Add `title` attributes on disabled elements to explicitly explain why they are inactive.
