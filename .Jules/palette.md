## 2025-04-21 - Added ARIA labels to icon buttons
**Learning:** Screen readers cannot infer the purpose of icon-only buttons unless an `aria-label` or visible text is present. While the `title` attribute provides tooltips for mouse users, it is not consistently read by screen readers. Thus, providing an explicit `aria-label` is crucial for accessibility on components like toolbar icons.
**Action:** Always include `aria-label` on icon-only buttons during initial development rather than waiting for an accessibility audit.
