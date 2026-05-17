## 2024-05-18 - Prevent XSS in UI and Timing Attacks in Auth
**Vulnerability:**
1. The web application dynamically interpolates user-controlled data (file/directory names) directly into `innerHTML` strings (in Toasts, renaming preview, folder tree). This creates a Cross-Site Scripting (XSS) vulnerability.
2. The authentication endpoint uses string comparison (`!=`) for password checking, which creates a timing attack vulnerability.

**Learning:**
1. Client-side template literals and `innerHTML` make it extremely easy to inadvertently introduce XSS vulnerabilities. Always use `textContent` when dealing with plain text data, or escape variables before using them in HTML strings.
2. Standard string comparison operations in Go short-circuit on mismatched characters, allowing an attacker to determine the password byte-by-byte by measuring the response time.

**Prevention:**
1. Used a robust `escapeHTML` helper function in `ui/static/app.js` to encode HTML entities (`&`, `<`, `>`, `"`, `'`) before using them in DOM elements constructed via `innerHTML`.
2. Replaced the standard password string comparison (`!=`) in `internal/api/server.go` with `subtle.ConstantTimeCompare` from the `crypto/subtle` package to ensure the comparison time is independent of the input contents.
## 2025-02-23 - Prevent DoS by Bounding In-Memory State Maps
**Vulnerability:** The in-memory map tracking rate limiters (`qm.limiters`) grew unbounded over time for each unique host configured in upload requests. This created a DoS vulnerability via memory exhaustion if an attacker dynamically generated unique hostnames.
**Learning:** In-memory maps tracking dynamically provided inputs (like hosts or client IPs) without a bounding mechanism (like a maximum length or TTL eviction scheme) are highly vulnerable to resource exhaustion. Simply returning `nil` or clearing the map completely when a limit is reached introduces logic bugs (panics or limit bypasses), and scanning all items per-request causes high CPU usage. Eviction logic must be performant, run only on insertion of new keys (not retrievals), and safely fall back to an un-cached instance if the limit remains exceeded.
**Prevention:** Implement a hard limit check (e.g., `len(map) >= 100`) before inserting new items. If exceeded, aggregate active identifiers in $O(N)$ and clean up inactive ones. Return a temporary un-cached instance instead of adding it to the map if the limit is still saturated.
