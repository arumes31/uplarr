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

## 2024-05-18 - Rate Limiter DoS Memory Exhaustion Mitigation
**Vulnerability:** A Denial of Service (DoS) memory exhaustion vulnerability was present in the rate limiter implementation. An attacker could specify an infinite number of unique, arbitrary host strings in the `host` parameter of the upload API, causing the `qm.limiters` map to grow indefinitely until the application panicked via Out-Of-Memory (OOM).
**Learning:** Returning an un-cached fallback rate limiter when an in-memory map limit is reached is a severe security anti-pattern, as it allows attackers to completely bypass rate limits simply by filling up the map. In Go, iterating over maps or slices to identify items to evict can quickly become a CPU bottleneck and a secondary DoS vector if not carefully optimized to an O(T+H) aggregation.
**Prevention:** Implemented an eviction strategy that enforces a strict upper bound (100) on the `qm.limiters` map. When the limit is reached, the system computes active tasks and safely evicts a single inactive host. If all hosts are active, a random host is forced out to make room. This correctly prioritizes application uptime (avoiding OOM crashes) while maintaining a strict O(T+H) CPU complexity on insertion.
