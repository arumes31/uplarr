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

## 2024-05-18 - Prevent DoS via Memory Exhaustion in Rate Limiters Cache
**Vulnerability:** The application stored rate limiters in a globally scoped map (`qm.limiters`) keyed by the user-provided `host` parameter without any bounds checking or size limits. This allowed an attacker to perform a Denial of Service (DoS) attack by supplying a large number of unique hostnames in requests, causing the application to consume excessive memory until it crashed. Previous attempts to fix this by pseudo-randomly evicting active limits introduced rate-limit bypasses, and returning `nil` caused a crash.
**Learning:** When bounding an in-memory state map (like a cache), failing safely when the cache is full is critical. An eviction policy must never evict actively used items (as it breaks state and introduces bypasses), and must always return a valid, safe fallback object (like an un-cached instance) rather than `nil` or an error to ensure continuous operation without panics or memory unboundedness.
**Prevention:** Added an amortized O(1) cleanup pass to remove inactive hosts when the `qm.limiters` map exceeds 1000 items. If the map remains full (all 1000 items actively used), it falls back to safely returning a standalone, un-cached `Limiter`. This ensures memory stays bounded, rate limits are still strictly applied, and no panics occur.
