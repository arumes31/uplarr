## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).

## 2026-05-14 - Checkbox Accessibility in File Tables
**Learning:** Dynamically generated inputs in table structures often miss contextual labels. Screen readers simply announce "checkbox" without the file name context. Similarly, disabling elements (like directory checkboxes) without explanation creates a confusing UX gap for both visual and non-visual users.
**Action:** Always add dynamic `aria-label`s (e.g., "Select {filename}") to generated checkboxes. For disabled interactive elements, explicitly add a `title` or `aria-description` explaining the inactive state.
