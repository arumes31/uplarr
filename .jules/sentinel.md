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

## 2024-05-18 - Brute-Force Prevention via Rate Limiting
**Vulnerability:** The `/api/login` endpoint lacked any rate limiting. An attacker could rapidly test passwords, making the application susceptible to brute-force credential stuffing.
**Learning:** Adding rate limits is a multi-step process. Using an unbounded IP map as a store will lead to memory exhaustion attacks (DoS), and 5 attempts per second is too high.
**Prevention:** Added an IP-based rate limiter to the `/api/login` route. Configured it to allow 5 attempts per minute with a burst of 5. Added a background cleanup routine that prunes inactive limiters after 10 minutes.
