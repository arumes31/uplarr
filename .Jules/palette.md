## 2025-04-21 - Added ARIA labels to icon buttons
**Learning:** Screen readers cannot infer the purpose of icon-only buttons unless an `aria-label` or visible text is present. While the `title` attribute provides tooltips for mouse users, it is not consistently read by screen readers. Thus, providing an explicit `aria-label` is crucial for accessibility on components like toolbar icons.
**Action:** Always include `aria-label` on icon-only buttons during initial development rather than waiting for an accessibility audit.

## 2024-05-18 - Added disabled visual feedback and focus indicators
**Learning:** Screen readers announce `:disabled` buttons appropriately, but without explicit styling, visually impaired or keyboard-only users miss the feedback that a task like an upload or connection attempt is in progress. Adding focus states ensures users tabbing through can see their position.
**Action:** Always include a visual state for disabled inputs and ensure `:focus-visible` exists on interactive elements.
## 2024-04-24 - Preserving Button Icons & Providing Inline Loading States
**Learning:** Overwriting the entire `.textContent` of a button that contains an inline icon (like `<svg>`) accidentally destroys the icon. Furthermore, users often lack immediate feedback on buttons like "Connect" or "Upload" while the action is processing, making the UI feel unresponsive even if a toast appears.
**Action:** Always wrap button text in a `<span class="btn-text">` when the button also contains an SVG icon. Create a reusable `toggleButtonLoading` utility that toggles visibility between the static icon and a spinner icon, and temporarily updates the `.btn-text` content to reflect the loading state (e.g., "Connecting...").

## 2026-06-08 - Added contextual ARIA labels to dynamically generated action buttons
**Learning:** Generic action buttons in lists or queues (e.g., "Remove", "Retry", "Resume") lack context for screen reader users and users navigating with keyboards or hovering. When these buttons are generated dynamically based on an underlying data model (like a queue task), appending the relevant data context (like the file name) to the button's `aria-label` and `title` attributes significantly improves accessibility and clarity.
**Action:** Always inject contextual identifying information (like item names or IDs) into the `aria-label` and `title` of generic action buttons within dynamic lists or tables during their creation.
