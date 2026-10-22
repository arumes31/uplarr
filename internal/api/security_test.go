package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"uplarr/internal/models"
	"uplarr/internal/queue"
	"uplarr/internal/sftpclient"
)

// newJSONRequest builds a request carrying the JSON content type that every
// state-changing API endpoint now requires.
func newJSONRequest(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.Header.Set("Content-Type", "application/json")
	return r
}

func newSecurityTestApp(t *testing.T, password string) (http.Handler, string) {
	t.Helper()
	dir := t.TempDir()
	qm := queue.NewQueueManager(dir, t.TempDir())
	t.Cleanup(qm.Shutdown)
	mux, err := SetupApp(models.Config{LocalDir: dir, AuthPassword: password}, qm)
	if err != nil {
		t.Fatal(err)
	}
	return mux, dir
}

func TestSecurityHeaders(t *testing.T) {
	app, _ := newSecurityTestApp(t, "")
	for _, path := range []string{"/", "/api/health", "/static/style.css"} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		h := rec.Header()
		if !strings.Contains(h.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Errorf("%s: missing CSP, got %q", path, h.Get("Content-Security-Policy"))
		}
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Frame-Options") != "DENY" || h.Get("Referrer-Policy") == "" {
			t.Errorf("%s: missing hardening headers: %v", path, h)
		}
	}
}

func TestStateChangingRequestsRequireJSON(t *testing.T) {
	app, _ := newSecurityTestApp(t, "")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/files/action", strings.NewReader(`{"action":"mkdir","path":"x"}`))
	req.Header.Set("Content-Type", "text/plain")
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain POST: got %d, want 415", rec.Code)
	}

	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, newJSONRequest("POST", "/api/files/action", strings.NewReader(`{"action":"mkdir","path":"x"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("JSON POST: got %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest("POST", "/api/logout", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("logout without body: got %d, want 200", rec.Code)
	}
}

func TestCrossOriginRequestsRejected(t *testing.T) {
	app, _ := newSecurityTestApp(t, "")

	rec := httptest.NewRecorder()
	req := newJSONRequest("POST", "/api/upload", strings.NewReader(`{"host":"evil","files":[]}`))
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site POST: got %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = newJSONRequest("POST", "/api/upload", strings.NewReader(`{"host":"evil","files":[]}`))
	req.Header.Set("Origin", "http://evil.example")
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign Origin POST: got %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = newJSONRequest("POST", "/api/upload", strings.NewReader(`{"files":[]}`))
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("same-origin POST: got %d, want 200", rec.Code)
	}
}

func TestRequestBodyIsBounded(t *testing.T) {
	app, _ := newSecurityTestApp(t, "secret")
	body := `{"password":"` + strings.Repeat("a", maxJSONBodyBytes) + `"}`
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, newJSONRequest("POST", "/api/login", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized login body: got %d, want 400", rec.Code)
	}
}

func TestClientIPUsesRightmostForwardedFor(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/login", nil)
	r.RemoteAddr = "10.0.0.2:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.7")

	if got := clientIP(r, false); got != "10.0.0.2" {
		t.Errorf("untrusted proxy: got %q", got)
	}
	if got := clientIP(r, true); got != "198.51.100.7" {
		t.Errorf("trusted proxy: got %q, want rightmost entry", got)
	}
	r.Header.Add("X-Forwarded-For", "203.0.113.9")
	if got := clientIP(r, true); got != "203.0.113.9" {
		t.Errorf("multiple headers: got %q, want last header's rightmost entry", got)
	}
}

func resetLoginLimiter(t *testing.T) {
	t.Helper()
	loginAttemptsMu.Lock()
	loginAttempts = make(map[string]*loginAttempt)
	globalLoginFailures = 0
	globalLoginWindowStart = time.Time{}
	loginAttemptsMu.Unlock()
}

func TestLoginSpoofedForwardedForDoesNotBypassLimit(t *testing.T) {
	resetLoginLimiter(t)
	t.Cleanup(func() { resetLoginLimiter(t) })

	dir := t.TempDir()
	qm := queue.NewQueueManager(dir, t.TempDir())
	t.Cleanup(qm.Shutdown)
	app, err := SetupApp(models.Config{LocalDir: dir, AuthPassword: "secret", TrustProxy: true}, qm)
	if err != nil {
		t.Fatal(err)
	}

	var last int
	for i := 0; i < maxLoginAttempts+1; i++ {
		rec := httptest.NewRecorder()
		req := newJSONRequest("POST", "/api/login", strings.NewReader(`{"password":"wrong"}`))
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.9.9.%d, 198.51.100.7", i))
		app.ServeHTTP(rec, req)
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("expected spoofed client to be rate limited, got %d", last)
	}
}

func TestGlobalLoginFailureBudget(t *testing.T) {
	resetLoginLimiter(t)
	t.Cleanup(func() { resetLoginLimiter(t) })
	oldMax := maxGlobalLoginFailures
	maxGlobalLoginFailures = 3
	t.Cleanup(func() { maxGlobalLoginFailures = oldMax })

	app, _ := newSecurityTestApp(t, "secret")
	codes := make([]int, 0, 4)
	for i := 0; i < 4; i++ {
		rec := httptest.NewRecorder()
		req := newJSONRequest("POST", "/api/login", strings.NewReader(`{"password":"wrong"}`))
		req.RemoteAddr = fmt.Sprintf("192.0.2.%d:1000", i+1)
		app.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}
	if codes[3] != http.StatusTooManyRequests {
		t.Fatalf("expected global budget to block logins, got %v", codes)
	}
}

func TestDownloadServesThroughRoot(t *testing.T) {
	app, dir := newSecurityTestApp(t, "")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0750); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest("GET", "/api/files/download?path=a.txt", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "hello" {
		t.Fatalf("download: got %d %q", rec.Code, rec.Body.String())
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, `filename=a.txt`) {
		t.Errorf("unexpected Content-Disposition %q", cd)
	}

	// The UI builds "//a.txt" after navigating to the root as "/".
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest("GET", "/api/files/download?path=%2F%2Fa.txt", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "hello" {
		t.Fatalf("download of root-prefixed path: got %d %q", rec.Code, rec.Body.String())
	}

	for path, want := range map[string]int{
		"sub":     http.StatusBadRequest,
		"missing": http.StatusNotFound,
	} {
		rec = httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest("GET", "/api/files/download?path="+path, nil))
		if rec.Code != want {
			t.Errorf("download %q: got %d, want %d", path, rec.Code, want)
		}
	}

	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest("GET", "/api/files/download?path=link.txt", nil))
	if rec.Code == http.StatusOK || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("symlink escape was served: %d %q", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, newJSONRequest("POST", "/api/upload", strings.NewReader(`{"files":["link.txt"]}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("upload of escaping symlink: got %d, want 400", rec.Code)
	}
}

func TestConnectErrorsAreGeneric(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("failed to dial: dial tcp 10.0.0.1:22: connect: connection refused"), "Connection failed"},
		{fmt.Errorf("failed to dial: i/o timeout"), "Connection failed"},
		{fmt.Errorf("ssh: handshake failed: ssh: unable to authenticate"), "Authentication failed"},
		{fmt.Errorf("%w: /etc/shadow: permission denied", sftpclient.ErrPrivateKey), "Private key could not be loaded from the keys directory"},
		{fmt.Errorf("x: %w", sftpclient.ErrHostKeyMismatch), "Host key verification failed: the server key does not match the pinned key"},
		{fmt.Errorf("x: %w", sftpclient.ErrHostKeyUnknown), "Host key verification failed: the server key is not trusted"},
		{sftpclient.ErrHostKeyRequired, "Host key verification required"},
		{sftpclient.ErrNoAuthMethods, "No authentication methods available"},
		{fmt.Errorf("failed to dial: %w", sftpclient.ErrDialTargetDenied), "Connection target is not allowed"},
	}
	for _, tc := range tests {
		if got := connectErrorMessage(tc.err); got != tc.want {
			t.Errorf("connectErrorMessage(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}

	oldNewSFTP := NewSFTPClient
	t.Cleanup(func() { NewSFTPClient = oldNewSFTP })
	NewSFTPClient = func(models.UploadRequest) SFTPClient {
		return &MockSFTPClient{ConnectFunc: func() error {
			return errors.New("failed to dial: dial tcp 169.254.169.254:22: connect: connection refused")
		}}
	}
	app, _ := newSecurityTestApp(t, "")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, newJSONRequest("POST", "/api/test-connection", strings.NewReader(`{"host":"h"}`)))
	if strings.Contains(rec.Body.String(), "169.254") || strings.Contains(rec.Body.String(), "refused") {
		t.Fatalf("raw dial error leaked: %s", rec.Body.String())
	}
}
