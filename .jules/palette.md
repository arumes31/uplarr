## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
## 2024-05-23 - Keyboard accessibility for interactive container rows
**Learning:** When making container elements (like table rows with `.clickable-row`) accessible via keyboard navigation by adding `tabindex="0"` and a `keydown` listener, failing to check the event target can cause child inputs (like checkboxes) to trigger the container's action unexpectedly when the user tries to interact with the child using the keyboard.
**Action:** Always verify `e.target.type` (e.g., `e.target.type !== 'checkbox'`) in the container's keyboard event listener to ensure nested interactive elements retain their default behavior without triggering the container's action.
