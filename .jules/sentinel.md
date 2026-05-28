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
## 2025-05-28 - [High] Missing Rate Limit on Login Endpoint
**Vulnerability:** The `/api/login` endpoint lacked any form of rate limiting, allowing brute-force attempts.
**Learning:** Security by string comparison or basic auth password logic alone is insufficient; endpoints need IP-based or session-based rate limiting to prevent brute force or dictionary attacks.
**Prevention:** Ensured the login endpoint applies an IP-based rate limiter that limits requests and prevents attackers from easily guessing passwords via brute force. Used `golang.org/x/time/rate` with a burst bucket and a time-based eviction cache map to protect the memory.
