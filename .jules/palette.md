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

## 2025-04-21 - Added ARIA labels to icon buttons
**Learning:** Screen readers cannot infer the purpose of icon-only buttons unless an `aria-label` or visible text is present. While the `title` attribute provides tooltips for mouse users, it is not consistently read by screen readers. Thus, providing an explicit `aria-label` is crucial for accessibility on components like toolbar icons.
**Action:** Always include `aria-label` on icon-only buttons during initial development rather than waiting for an accessibility audit.

## 2024-05-18 - Added disabled visual feedback and focus indicators
**Learning:** Screen readers announce `:disabled` buttons appropriately, but without explicit styling, visually impaired or keyboard-only users miss the feedback that a task like an upload or connection attempt is in progress. Adding focus states ensures users tabbing through can see their position.
**Action:** Always include a visual state for disabled inputs and ensure `:focus-visible` exists on interactive elements.

## 2024-04-24 - Preserving Button Icons & Providing Inline Loading States
**Learning:** Overwriting the entire `.textContent` of a button that contains an inline icon (like `<svg>`) accidentally destroys the icon. Furthermore, users often lack immediate feedback on buttons like "Connect" or "Upload" while the action is processing, making the UI feel unresponsive even if a toast appears.
**Action:** Always wrap button text in a `<span class="btn-text">` when the button also contains an SVG icon. Create a reusable `toggleButtonLoading` utility that toggles visibility between the static icon and a spinner icon, and temporarily updates the `.btn-text` content to reflect the loading state (e.g., "Connecting...").

## 2026-07-25 - Case-Variant Changelog Path Split This File in Two
**Learning:** An earlier change wrote to `.Jules/palette.md` while the tracked path is `.jules/palette.md`. Git tracked both, but Windows checkouts map them to one file, so entries silently landed in whichever copy won and every Windows clone showed a permanently dirty working tree.
**Action:** Always append to the lowercase `.jules/` path. The two copies have been merged and the stray uppercase entry removed from the index.

## 2026-07-25 - Contextual Labels for Repeated Row Actions
**Learning:** The queue table renders Pause/Resume/Retry/Remove per row, so a screen reader user tabbing the actions column hears "Remove, Remove, Remove" with nothing to distinguish the rows. The visible text has to stay short for layout, so the context belongs in the accessible name.
**Action:** Give repeated row actions an `aria-label` that names the row subject, and set a matching `title` so mouse users get the same disambiguation. Keep the visible `textContent` short.

## 2026-07-25 - Marking Required Fields Visually
**Learning:** The SFTP form relied only on the HTML5 `required` attribute, so a sighted user could not tell which fields were mandatory until submitting. Screen readers already announce the state from `required`, so a marker that is also announced would be redundant.
**Action:** Pair `required` with a visual asterisk carrying `aria-hidden="true"`, styled by a `.required-indicator` class rather than an inline `style` attribute so the colour stays themeable.
