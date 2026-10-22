package api

import (
	"errors"
	"log"
	"mime"
	"net/http"
	"strings"

	"uplarr/internal/sftpclient"
)

const (
	// maxJSONBodyBytes bounds every JSON request body.
	maxJSONBodyBytes = 1 << 20
	// maxUploadBodyBytes is larger because /api/upload carries the file list.
	maxUploadBodyBytes = 16 << 20
)

const contentSecurityPolicy = "default-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'"

// securityHeaders adds browser hardening headers to every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// isSafeMethod reports whether a request method cannot change server state.
func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

// requireJSONBody rejects state-changing API requests that are not
// application/json, so cross-site "simple" form or text/plain POSTs never
// reach a handler, and caps the request body size.
func requireJSONBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSafeMethod(r.Method) || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		limit := int64(maxJSONBodyBytes)
		if r.URL.Path == "/api/upload" {
			limit = maxUploadBodyBytes
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)

		// Logout carries no body; cross-origin protection still applies to it.
		if r.URL.Path == "/api/logout" {
			next.ServeHTTP(w, r)
			return
		}

		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// secureHandler wraps the application mux with CSRF defences and security
// headers. Cross-origin protection checks Sec-Fetch-Site and Origin on every
// non-safe request, whether or not authentication is enabled.
func secureHandler(next http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	return securityHeaders(cop.Handler(requireJSONBody(next)))
}

// connectErrorMessage maps an SFTP connection error to a message that is safe
// to return to the client. Raw dial errors are only logged server side so the
// endpoints cannot be used to fingerprint internal hosts and ports.
func connectErrorMessage(err error) string {
	log.Printf("SFTP connection failed: %v", err)
	switch {
	case errors.Is(err, sftpclient.ErrHostKeyMismatch):
		return "Host key verification failed: the server key does not match the pinned key"
	case errors.Is(err, sftpclient.ErrHostKeyUnknown):
		return "Host key verification failed: the server key is not trusted"
	case errors.Is(err, sftpclient.ErrHostKeyRequired):
		return "Host key verification required"
	case errors.Is(err, sftpclient.ErrPrivateKey):
		return "Private key could not be loaded from the keys directory"
	case errors.Is(err, sftpclient.ErrNoAuthMethods):
		return "No authentication methods available"
	case errors.Is(err, sftpclient.ErrDialTargetDenied):
		return "Connection target is not allowed"
	case strings.Contains(err.Error(), "unable to authenticate"):
		return "Authentication failed"
	default:
		return "Connection failed"
	}
}
