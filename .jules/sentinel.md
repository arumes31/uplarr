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

## 2026-04-25 - [Missing Secure Download Endpoint]
**Vulnerability:** [The frontend allowed initiating file downloads but the backend lacked an endpoint, leaving downloads completely broken and unverified]
**Learning:** [A path traversal validation framework already existed in `internal/api/server.go` for action endpoints, but no specific read/download endpoint used it.]
**Prevention:** [When exposing local files securely, always use `filepath.Abs`, `filepath.EvalSymlinks`, and strict `filepath.Rel` to ensure no path traversal out of the defined base dir. Ensure all file operations map directly to implemented handlers.]
## 2024-05-25 - Rate Limiter Map Eviction Memory Protection
**Vulnerability:** Unbounded map growth during active rate limiting could lead to memory exhaustion (DoS). Standard map inserts without checks would grow indefinitely on malicious inputs or distributed attacks where IP entropy is high.
**Learning:** Simply checking `len(map) > limit` and skipping rate limiting or returning a generic error is insufficient because it bypasses security on new attackers or panics on nil objects. A bounded map must safely evict elements when full.
**Prevention:** Always bound map-based memory trackers (like rate limiters) by implementing an eviction check *on insertion*. If full, safely evict inactive elements. If still full after that (extreme entropy), forcefully evict an arbitrary element to make room for the new one, ensuring bounded memory without blocking or crashing the application.

## 2026-05-27 - Prevent DoS Memory Exhaustion in Tracking Maps
**Vulnerability:** Unbounded in-memory map tracking rate limiters (`qm.limiters`) per host allows a malicious user or numerous unauthenticated requests with unique hosts to exhaust server memory, leading to a Denial of Service (DoS).
**Learning:** Maps used for state tracking (e.g., rate limiting, metrics) without eviction mechanisms represent a severe memory leak vector. Bounding and evicting items safely prevents this.
**Prevention:** Implement safe map bounds and evictions. Evict inactive entries when exceeding a threshold (e.g., 100). When full and all entries are active, forceful eviction of an arbitrary/oldest entry must be used rather than throwing errors or skipping caching to maintain rate-limiting properties and bound memory.

## 2026-07-17 - Prevent Session Map Memory Exhaustion and Replay Attacks
**Vulnerability:** The application stored active authentication session tokens in an unbounded `map[string]bool` without backend expiration checks or eviction logic. This created two risks: (1) Old session tokens never truly expired on the server, permitting replay/reuse if intercepted, and (2) malicious mass-generation of valid tokens (or simply high volumes of legitimate logins) could cause the map to grow indefinitely, leading to a Denial of Service (DoS) through memory exhaustion.
**Learning:** Even if a session cookie is configured with a `MaxAge` on the client, the backend must independently validate the token's lifetime and evict expired/excessive tokens. A `bool` flag is insufficient; server-side tracking must include an expiration timestamp. Furthermore, any stateful tracking map must be bounded to prevent DoS.
**Prevention:** Track sessions using `map[string]time.Time` to enforce strict backend expiration matching the cookie's lifetime. Always implement eviction limits on unbounded tracking maps. When establishing a new session, check if the map has reached its maximum allowed size (e.g., 1000). If full, eagerly prune expired tokens, and if still full, forcefully evict an active token to safely enforce a memory bound without crashing.
