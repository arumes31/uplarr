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
