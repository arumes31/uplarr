## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
## 2025-02-18 - Contextual Accessibility in Dynamic Tables
**Learning:** Screen readers struggle with generic inputs (like checkboxes) inside dynamic data tables unless explicitly labeled. Additionally, disabling interactive elements (like directory checkboxes) without visual explanation creates a confusing experience.
**Action:** Always add contextual `aria-label`s to dynamically generated table inputs (e.g., `Select file [name]`) and provide `title` tooltips for explicitly disabled elements so users understand why interaction is blocked.
