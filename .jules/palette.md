## 2026-05-22 - Improved accessibility for dynamically generated form elements
**Learning:** Dynamically generated form elements (like checkboxes in lists) often lack context for screen reader users and disabled states can be confusing without explanation.
**Action:** Always add `aria-label` attributes to dynamically generated interactive elements for screen reader context and use `title` attributes on disabled elements to explicitly explain why they are inactive.

## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
