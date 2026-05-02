## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
## 2024-05-02 - Accessible Sortable Table Headers
**Learning:** Found table columns providing visual cue for sorting but missing screen-reader indication of which columns were sortable, and which direction they're currently sorted.
**Action:** Always verify `aria-sort` accompanies sorting directional carets for a fully accessible UI. Ensure dynamic sorting JS logic includes setting the aria attribute (e.g. `aria-sort="ascending"`, `descending`, `none`) accordingly.
