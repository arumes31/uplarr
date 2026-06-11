## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
## 2026-03-05 - Accessibility of dynamic form elements and complex rows
**Learning:** Found that dynamically generated checkboxes in file lists lacked `aria-label` attributes and context for their disabled state (`title`), making them inaccessible to screen readers. Additionally, interactive table rows acting as folders lacked keyboard navigation (`tabIndex` and `keydown` handling).
**Action:** Always ensure dynamically generated form elements have proper ARIA attributes (`aria-label`) and explanatory attributes (`title` for disabled state). When implementing keyboard navigation on complex rows, explicitly check the event target to avoid overriding the default behavior of child inputs (like checkboxes).
## 2024-04-30 - Password Visibility Toggle
**Learning:** Users need to verify their master password before submitting, and relying on missing/stubbed JS features (like fa-eye toggles without UI) creates a frustrating dead end.
**Action:** Always implement a functional visibility toggle for sensitive inputs with proper ARIA labels and SVG swapping.
## 2025-04-21 - Dynamic context on inline action buttons
**Learning:** In dynamically generated lists or tables containing identical action buttons (like 'Retry', 'Remove', 'Pause') on every row, screen readers will read only the text content ("Retry button", "Retry button", "Retry button") which lacks sufficient context.
**Action:** Always append dynamic contextual information (like the file name or row subject) to the `aria-label` attribute on inline action buttons in repeating rows to ensure screen reader users understand what the action applies to.
