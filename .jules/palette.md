## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
## 2025-02-12 - File List Checkbox Accessibility
**Learning:** Dynamically generated input elements, like the checkboxes in the local file list, often lack context for screen readers when they are not associated with a specific `<label>`. Additionally, disabled states on inputs without explanation can lead to a confusing user experience.
**Action:** Always include an `aria-label` attribute on standalone input elements to provide context (e.g., "Select [filename]"). Also, when programmatically disabling an element, provide a `title` or tooltip explaining why the element is inactive (e.g., "Directories cannot be selected for upload").
