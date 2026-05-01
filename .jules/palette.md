## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).

## 2025-02-23 - Added Accessibility to Dynamic Checkboxes
**Learning:** Dynamically generated checkboxes in file tables without labels are completely invisible to screen readers, and disabled checkboxes without an explanation or tooltip cause confusion because users don't know why they cannot interact with them.
**Action:** When dynamically generating checkboxes using JS, always set an `aria-label` to provide the necessary context (e.g., "Select file: X"). For dynamically disabled inputs, always add a `title` explaining the reason for the disabled state to improve clarity.
