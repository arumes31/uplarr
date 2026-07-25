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

## 2026-07-25 - Server-Side Session Expiry and Bounded Session Map
**Vulnerability:** The `sessions` map in `internal/api/server.go` stored `map[string]bool`, so a token stayed valid on the server forever. Expiry was enforced only by the client-side cookie `MaxAge`, and the map had no size bound.
**Learning:** A cookie `MaxAge` is a client-side hint that an attacker holding a stolen token simply ignores. Server-side session state needs its own expiry, and any map keyed by attacker-influenced input needs a bound.
**Prevention:** Store the expiry instant alongside the token (`map[string]time.Time`), derive it from a `sessionTTL` constant that mirrors the cookie `MaxAge`, reject and delete expired entries on access, and on insert prune expired entries before evicting the entry closest to expiring. Keep the read path on `RLock` so validation does not serialise every authenticated request.

## 2026-07-25 - Rate Limiter Bound Bypassed on a Second Insert Path
**Vulnerability:** `loginAttempts` was bounded only where the entry was created on the initial lookup. The failed-password path re-acquired the lock and inserted a second time without any capacity check, so the bound could be bypassed.
**Learning:** When bounding logic is inlined at a call site rather than attached to the insert, a later code path that also inserts will silently skip it. The bug is invisible at the second call site because nothing there mentions the bound.
**Prevention:** Extract eviction into a single `evictLoginAttemptsLocked` helper named for its locking contract and call it from every insert path.

## 2026-07-25 - Hand-Built JSON in the Logger Fallback
**Vulnerability:** When `json.Marshal` failed, `LogWithLevel` built the fallback entry with `fmt.Sprintf` and `%s`, so a quote or newline in the message could inject arbitrary JSON into the SSE log stream consumed by the UI.
**Learning:** Fallback and error paths are exactly where hand-rolled serialisation tends to survive, because they are rarely exercised and rarely tested.
**Prevention:** Re-marshal a reduced struct with `encoding/json` instead of formatting JSON by hand. `Extra` is the only field that can be unmarshalable, so dropping it guarantees the retry succeeds.

## 2026-07-25 - gosec G124 on Dynamically Set Cookie Attributes
**Vulnerability:** None in practice, but gosec >= 2.26 reports G124 on the login cookie because `Secure` is assigned from `isSecureRequest(r)` rather than a literal `true`, which it cannot evaluate statically.
**Learning:** A scanner upgrade can block an otherwise routine dependency bump on a false positive. The finding needs triage, not a blanket suppression of the rule.
**Prevention:** Annotate the specific statement with `// #nosec G124` and a comment explaining why the attribute is dynamic, matching the existing annotation on the logout cookie.
