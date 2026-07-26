package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"uplarr/internal/models"
	"uplarr/internal/queue"
)

func TestClientIPFromForwardedFor(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "empty header", header: "", want: ""},
		{name: "single entry", header: "203.0.113.7", want: "203.0.113.7"},
		// The rightmost entry is the one the trusted proxy appended; everything
		// to its left is client-supplied and must not be trusted.
		{name: "spoofed prefix is ignored", header: "1.2.3.4, 203.0.113.7", want: "203.0.113.7"},
		{name: "multiple hops take the last", header: "1.2.3.4, 10.0.0.1, 203.0.113.7", want: "203.0.113.7"},
		{name: "whitespace is trimmed", header: "  1.2.3.4 ,  203.0.113.7  ", want: "203.0.113.7"},
		{name: "trailing empty entry is skipped", header: "203.0.113.7, ", want: "203.0.113.7"},
		{name: "only separators yields nothing", header: " , , ", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := clientIPFromForwardedFor(tc.header); got != tc.want {
				t.Errorf("clientIPFromForwardedFor(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

// A spoofed X-Forwarded-For must not hand each request its own rate-limit
// bucket, which is what keying on the leftmost entry allowed.
func TestLoginRateLimitIgnoresSpoofedForwardedFor(t *testing.T) {
	tempDir := t.TempDir()
	qm := queue.NewQueueManager(tempDir, tempDir)
	defer qm.Shutdown()

	mux, err := SetupApp(models.Config{LocalDir: tempDir, AuthPassword: "secret", TrustProxy: true}, qm)
	if err != nil {
		t.Fatalf("SetupApp: %v", err)
	}

	loginAttemptsMu.Lock()
	loginAttempts = make(map[string]*loginAttempt)
	loginAttemptsMu.Unlock()

	// Every attempt claims a different leftmost address while the proxy-appended
	// entry stays constant, so all of them must land in one bucket.
	for i := 0; i < maxLoginAttempts+2; i++ {
		req := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"password":"wrong"}`))
		req.RemoteAddr = "10.0.0.1:1234"
		req.Header.Set("X-Forwarded-For", "198.51.100."+string(rune('0'+i%10))+", 203.0.113.7")
		mux.ServeHTTP(httptest.NewRecorder(), req)
	}

	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()
	if len(loginAttempts) != 1 {
		t.Fatalf("expected one rate-limit bucket keyed on the trusted entry, got %d: %v", len(loginAttempts), keysOf(loginAttempts))
	}
	entry, ok := loginAttempts["203.0.113.7"]
	if !ok {
		t.Fatalf("expected bucket keyed on 203.0.113.7, got %v", keysOf(loginAttempts))
	}
	if entry.blockedUntil.IsZero() {
		t.Error("expected the shared bucket to reach the block threshold")
	}
}

func keysOf(m map[string]*loginAttempt) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ServeFile renders a directory listing when handed a directory, which would
// expose the tree instead of downloading a file.
func TestDownloadRejectsDirectory(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "subdir"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	qm := queue.NewQueueManager(tempDir, tempDir)
	defer qm.Shutdown()

	mux, err := SetupApp(models.Config{LocalDir: tempDir}, qm)
	if err != nil {
		t.Fatalf("SetupApp: %v", err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/files/download?path=subdir", nil))
	if rec.Code != 400 {
		t.Errorf("directory download returned %d, want 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/files/download?path=file.txt", nil))
	if rec.Code != 200 {
		t.Fatalf("file download returned %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != "hello" {
		t.Errorf("body = %q, want %q", body, "hello")
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename=file.txt` {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

// A quote in a filename must not break out of the Content-Disposition value.
func TestDownloadEscapesFilename(t *testing.T) {
	tempDir := t.TempDir()
	name := `we"ird.txt`
	if err := os.WriteFile(filepath.Join(tempDir, name), []byte("x"), 0o600); err != nil {
		t.Skipf("filesystem rejects %q: %v", name, err)
	}

	qm := queue.NewQueueManager(tempDir, tempDir)
	defer qm.Shutdown()

	mux, err := SetupApp(models.Config{LocalDir: tempDir}, qm)
	if err != nil {
		t.Fatalf("SetupApp: %v", err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/files/download?path="+name, nil))

	cd := rec.Header().Get("Content-Disposition")
	if cd == `attachment; filename="we"ird.txt"` {
		t.Errorf("quote was not escaped: %q", cd)
	}
	if cd == "" {
		t.Error("expected a Content-Disposition header")
	}
}
