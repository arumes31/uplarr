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
## 2024-05-25 - Rate Limiter Map Eviction Memory Protection
**Vulnerability:** Unbounded map growth during active rate limiting could lead to memory exhaustion (DoS). Standard map inserts without checks would grow indefinitely on malicious inputs or distributed attacks where IP entropy is high.
**Learning:** Simply checking `len(map) > limit` and skipping rate limiting or returning a generic error is insufficient because it bypasses security on new attackers or panics on nil objects. A bounded map must safely evict elements when full.
**Prevention:** Always bound map-based memory trackers (like rate limiters) by implementing an eviction check *on insertion*. If full, safely evict inactive elements. If still full after that (extreme entropy), forcefully evict an arbitrary element to make room for the new one, ensuring bounded memory without blocking or crashing the application.
