## 2026-03-05 - Adding accessibility to toggle buttons
**Learning:** Found that layout toggles (`view-toggle-btn`) and view options (`compact-toggle`) missed dynamic ARIA attributes, leaving screen reader users without proper context of the current interface state.
**Action:** When creating toggle buttons or dropdown buttons, always pair with `aria-pressed` or `aria-expanded` and `aria-haspopup` to properly convey state changes and control relationships.
## 2024-05-18 - Added Empty States for Queues and Lists
**Learning:** Tables representing local file lists and background task queues that are initially empty appear broken to users if only headers are displayed. Providing explicit "empty state" messages confirms system status and avoids user confusion.
**Action:** Always include empty states for lists/tables that may be empty, and style them consistently to be visually distinct (e.g., center alignment, italic, muted text).
## 2026-05-13 - Context for Dynamically Generated Disabled Elements
**Learning:** Discovered that dynamically generated disabled form elements (like checkboxes for directories in file lists) lack context, confusing users about why they cannot interact with them. In addition, dynamically generated checkboxes for files didn't have `aria-label` set, reducing accessibility for screen reader users.
**Action:** Always add an `aria-label` to dynamically generated inputs. For disabled inputs, add a `title` attribute to explain why the element is disabled, improving clarity and UX without requiring additional UI changes.
