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

## 2026-05-27 - Prevent DoS Memory Exhaustion in Tracking Maps
**Vulnerability:** Unbounded in-memory map tracking rate limiters (`qm.limiters`) per host allows a malicious user or numerous unauthenticated requests with unique hosts to exhaust server memory, leading to a Denial of Service (DoS).
**Learning:** Maps used for state tracking (e.g., rate limiting, metrics) without eviction mechanisms represent a severe memory leak vector. Bounding and evicting items safely prevents this.
**Prevention:** Implement safe map bounds and evictions. Evict inactive entries when exceeding a threshold (e.g., 100). When full and all entries are active, forceful eviction of an arbitrary/oldest entry must be used rather than throwing errors or skipping caching to maintain rate-limiting properties and bound memory.
