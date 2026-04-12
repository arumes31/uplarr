package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRun_Failures(t *testing.T) {
	// Mock httpListen early to prevent any actual server from starting
	oldListen := httpListen
	httpListen = func(addr string, handler http.Handler) error {
		return fmt.Errorf("listen error")
	}
	defer func() { httpListen = oldListen }()

	// Test SetupApp failure in Run by pointing LOCAL_DIR to a file instead of a directory
	tmpFile, _ := os.CreateTemp("", "not_a_dir")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	os.Setenv("LOCAL_DIR", tmpFile.Name())
	os.Setenv("WEB_PORT", "8081")
	err := Run()
	// On some OSes MkdirAll on a file might fail or return error later.
	// We expect either setup failed or listen error (if setup somehow worked)
	if err == nil {
		t.Errorf("Expected failure when LOCAL_DIR is a file, got nil")
	}
	os.Unsetenv("LOCAL_DIR")
	os.Unsetenv("WEB_PORT")

	tempDir, _ := os.MkdirTemp("", "run_test")
	defer os.RemoveAll(tempDir)
	os.Setenv("LOCAL_DIR", tempDir)
	os.Setenv("WEB_PORT", "8082")
	defer os.Unsetenv("LOCAL_DIR")
	defer os.Unsetenv("WEB_PORT")

	err = Run()
	if err == nil || !strings.Contains(err.Error(), "listen error") {
		t.Errorf("Expected listen error, got %v", err)
	}

	// Test main() failure
	oldExit := osExit
	exitCalled := false
	osExit = func(code int) { exitCalled = true }
	main()
	if !exitCalled {
		t.Error("Expected osExit to be called on main() failure")
	}
	osExit = oldExit
}

func TestSetupApp_MkdirFail(t *testing.T) {
	tmpFile, _ := os.CreateTemp("", "not_a_dir_setup")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	config := Config{
		LocalDir: tmpFile.Name(),
	}
	_, err := SetupApp(config)
	if err == nil {
		t.Error("Expected SetupApp to fail when LocalDir is a file")
	}
}

func TestGetEnvFunctions(t *testing.T) {
	os.Setenv("TEST_INT", "123")
	defer os.Unsetenv("TEST_INT")

	if val := getEnvInt("TEST_INT", 0); val != 123 {
		t.Errorf("Expected 123, got %d", val)
	}

	if val := getEnvInt("NON_EXISTENT", 42); val != 42 {
		t.Errorf("Expected fallback 42, got %d", val)
	}

	os.Setenv("TEST_INT_INVALID", "abc")
	defer os.Unsetenv("TEST_INT_INVALID")
	if val := getEnvInt("TEST_INT_INVALID", 42); val != 42 {
		t.Errorf("Expected fallback 42 for invalid int, got %d", val)
	}

	os.Setenv("TEST_BOOL", "true")
	defer os.Unsetenv("TEST_BOOL")

	if val := getEnvBool("TEST_BOOL", false); val != true {
		t.Errorf("Expected true, got false")
	}

	if val := getEnvBool("NON_EXISTENT", true); val != true {
		t.Errorf("Expected fallback true, got false")
	}

	os.Setenv("TEST_BOOL_INVALID", "abc")
	defer os.Unsetenv("TEST_BOOL_INVALID")
	if val := getEnvBool("TEST_BOOL_INVALID", false); val != false {
		t.Errorf("Expected fallback false for invalid bool, got true")
	}

	os.Setenv("TEST_STR", "hello")
	defer os.Unsetenv("TEST_STR")

	if val := getEnv("TEST_STR", "fallback"); val != "hello" {
		t.Errorf("Expected hello, got %s", val)
	}

	if val := getEnv("NON_EXISTENT", "fallback"); val != "fallback" {
		t.Errorf("Expected fallback, got %s", val)
	}
}

func TestSetupApp(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "setupapp_test")
	defer os.RemoveAll(tempDir)

	config := Config{
		LocalDir: tempDir,
	}

	mux, err := SetupApp(config)
	if err != nil {
		t.Fatalf("SetupApp failed: %v", err)
	}

	// Test root handler
	req, _ := http.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200 for root, got %d", rr.Code)
	}

	// Test 404
	req404, _ := http.NewRequest("GET", "/invalid", nil)
	rr404 := httptest.NewRecorder()
	mux.ServeHTTP(rr404, req404)
	if rr404.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rr404.Code)
	}

	// Test /api/files with empty dir
	reqFiles, _ := http.NewRequest("GET", "/api/files", nil)
	rrFiles := httptest.NewRecorder()
	mux.ServeHTTP(rrFiles, reqFiles)
	if rrFiles.Code != http.StatusOK {
		t.Errorf("Expected status 200 for /api/files, got %d", rrFiles.Code)
	}
	var files []FileInfo
	if err := json.NewDecoder(rrFiles.Body).Decode(&files); err != nil {
		t.Errorf("Failed to decode JSON: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("Expected 0 files, got %d", len(files))
	}

	// Add file and directory
	os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("hello"), 0644)
	os.Mkdir(filepath.Join(tempDir, "testdir"), 0755)

	rrFiles2 := httptest.NewRecorder()
	mux.ServeHTTP(rrFiles2, reqFiles)
	if err := json.NewDecoder(rrFiles2.Body).Decode(&files); err != nil {
		t.Errorf("Failed to decode JSON: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 items, got %d", len(files))
	}

	// Test /api/files ReadDir fail
	oldReadDir := osReadDir
	osReadDir = func(name string) ([]os.DirEntry, error) { return nil, fmt.Errorf("readdir fail") }
	rrFiles3 := httptest.NewRecorder()
	mux.ServeHTTP(rrFiles3, reqFiles)
	if rrFiles3.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 for readdir fail, got %d", rrFiles3.Code)
	}
	osReadDir = oldReadDir

	reqBody := `{"host":"127.0.0.1","port":22,"user":"user","password":"password"}`
	reqTestConn, _ := http.NewRequest("POST", "/api/test-connection", strings.NewReader(reqBody))
	rrTestConn := httptest.NewRecorder()
	mux.ServeHTTP(rrTestConn, reqTestConn)
	// Failed because we don't have mock server here, but it should hit the verification error if not set
	if rrTestConn.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for failed connect, got %d", rrTestConn.Code)
	}

	// Test /api/upload GET (method not allowed)
	reqUploadGet, _ := http.NewRequest("GET", "/api/upload", nil)
	rrUploadGet := httptest.NewRecorder()
	mux.ServeHTTP(rrUploadGet, reqUploadGet)
	if rrUploadGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for GET /api/upload, got %d", rrUploadGet.Code)
	}

	// Test /api/upload POST (will fail connect -> 207)
	reqUploadPost, _ := http.NewRequest("POST", "/api/upload", strings.NewReader(reqBody))
	reqUploadPost.Header.Set("Content-Type", "application/json")
	rrUploadPost := httptest.NewRecorder()
	mux.ServeHTTP(rrUploadPost, reqUploadPost)
	if rrUploadPost.Code != http.StatusMultiStatus {
		t.Errorf("Expected status 207 for failed upload connect, got %d", rrUploadPost.Code)
	}

	// Test /api/upload POST with bad JSON
	reqUploadBad, _ := http.NewRequest("POST", "/api/upload", strings.NewReader(`{bad json`))
	rrUploadBad := httptest.NewRecorder()
	mux.ServeHTTP(rrUploadBad, reqUploadBad)
	if rrUploadBad.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for bad JSON, got %d", rrUploadBad.Code)
	}

	// Test index.html read error
	oldFS := appFS
	appFS = mockErrFS{}
	defer func() { appFS = oldFS }()

	reqRoot, _ := http.NewRequest("GET", "/", nil)
	rrRoot := httptest.NewRecorder()
	mux.ServeHTTP(rrRoot, reqRoot)
	if rrRoot.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 for index.html read error, got %d", rrRoot.Code)
	}
}

type mockErrFS struct{}

func (m mockErrFS) ReadFile(name string) ([]byte, error) {
	return nil, fmt.Errorf("read error")
}
func (m mockErrFS) Open(name string) (fs.File, error) {
	return nil, fmt.Errorf("open error")
}

func TestProcessUploads(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "process_uploads_test")
	defer os.RemoveAll(tempDir)
	remoteDir := filepath.Join(tempDir, "remote")
	os.MkdirAll(remoteDir, 0755)
	localDir := filepath.Join(tempDir, "local")
	os.MkdirAll(localDir, 0755)

	port, cleanup := startMockSFTPServer(t, "user", "pass", remoteDir)
	defer cleanup()

	config := Config{
		LocalDir: localDir,
	}

	req := UploadRequest{
		Host:       "127.0.0.1",
		Port:       func() int { p, _ := strconv.Atoi(port); return p }(),
		User:       "user",
		Password:   "pass",
		RemoteDir:  ".",
		MaxRetries: 1,
	}

	// Create test file
	os.WriteFile(filepath.Join(localDir, "upload.txt"), []byte("data"), 0644)
	os.MkdirAll(filepath.Join(localDir, "subdir"), 0755)

	errs := ProcessUploads(config, req)
	if len(errs) != 0 {
		t.Errorf("ProcessUploads failed with errors: %v", errs)
	}

	// Verify upload
	// Note: Our mock server in sftp_client_test doesn't actually write files to disk yet.
	// But it does verify size if we use SFTPClient.UploadFile.
	// ProcessUploads uses sftpClientUpload mockable var.
}

func TestApiLogs(t *testing.T) {
	config := Config{LocalDir: os.TempDir()}
	mux, _ := SetupApp(config)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Connect to the real SetupApp's /api/logs
	resp, err := http.Get(server.URL + "/api/logs")
	if err != nil {
		t.Fatalf("Failed to connect to /api/logs: %v", err)
	}
	defer resp.Body.Close()

	// Poll for client registration instead of Sleep
	timeout := time.After(2 * time.Second)
	tick := time.Tick(10 * time.Millisecond)
	registered := false
	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for client registration")
		case <-tick:
			mu.Lock()
			if len(logClients) > 0 {
				registered = true
			}
			mu.Unlock()
		}
		if registered {
			break
		}
	}

	broadcastLog("test message 1")

	// Create a dummy client to hit the default case in broadcastLog by filling its channel
	dummyChan := make(chan string, 1)
	mu.Lock()
	logClients[dummyChan] = true
	mu.Unlock()
	dummyChan <- "fill"
	broadcastLog("dropped message") // Hits the default case
	mu.Lock()
	delete(logClients, dummyChan)
	mu.Unlock()

	// Read one message from the response to verify it was sent
	buf := make([]byte, 1024)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "test message 1") {
		t.Errorf("Expected to receive test message 1 via SSE, got %s", string(buf[:n]))
	}
}

func TestApiUpload_Success(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "upload_success")
	defer os.RemoveAll(tempDir)
	
	config := Config{LocalDir: tempDir}
	mux, _ := SetupApp(config)

	oldConnect := sftpClientConnect
	sftpClientConnect = func(s *SFTPClient) error { return nil }
	defer func() { sftpClientConnect = oldConnect }()

	reqBody := `{"host":"127.0.0.1","port":22,"user":"user","password":"password"}`
	reqUploadPost, _ := http.NewRequest("POST", "/api/upload", strings.NewReader(reqBody))
	reqUploadPost.Header.Set("Content-Type", "application/json")
	rrUploadPost := httptest.NewRecorder()
	mux.ServeHTTP(rrUploadPost, reqUploadPost)

	if rrUploadPost.Code != http.StatusOK {
		t.Errorf("Expected status 200 for successful upload, got %d", rrUploadPost.Code)
	}
}

func TestProcessUploads_Errors(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "process_uploads_errors")
	defer os.RemoveAll(tempDir)
	
	config := Config{LocalDir: tempDir}
	
	tempFile := filepath.Join(tempDir, "test.txt")
	os.WriteFile(tempFile, []byte("data"), 0644)

	req := UploadRequest{Host: "127.0.0.1", Port: 22, User: "u", Password: "p", MaxRetries: 0}
	
	oldConnect := sftpClientConnect
	sftpClientConnect = func(s *SFTPClient) error { return nil }
	defer func() { sftpClientConnect = oldConnect }()

	oldUpload := sftpClientUpload
	sftpClientUpload = func(s *SFTPClient, localPath string, maxRetries int) error { return fmt.Errorf("upload fail") }
	defer func() { sftpClientUpload = oldUpload }()

	errs := ProcessUploads(config, req)
	if len(errs) == 0 {
		t.Errorf("Expected ProcessUploads errors, got nil")
	}

	// Test ReadDir fail
	config.LocalDir = filepath.Join(tempDir, "nonexistent")
	errs = ProcessUploads(config, req)
	if len(errs) == 0 || !strings.Contains(errs[0], "Failed to read local directory") {
		t.Errorf("Expected ReadDir failure, got %v", errs)
	}
}

func TestApiTestConnection(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "apitestconn_test")
	defer os.RemoveAll(tempDir)
	port, cleanup := startMockSFTPServer(t, "user", "pass", tempDir)
	defer cleanup()

	config := Config{LocalDir: tempDir}
	mux, _ := SetupApp(config)

	// Success
	reqBody := `{"host":"127.0.0.1","port":` + port + `,"user":"user","password":"pass"}`
	req, _ := http.NewRequest("POST", "/api/test-connection", strings.NewReader(reqBody))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Method not allowed
	reqGet, _ := http.NewRequest("GET", "/api/test-connection", nil)
	rrGet := httptest.NewRecorder()
	mux.ServeHTTP(rrGet, reqGet)
	if rrGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", rrGet.Code)
	}

	// Bad JSON
	reqBad, _ := http.NewRequest("POST", "/api/test-connection", strings.NewReader(`{bad json`))
	rrBad := httptest.NewRecorder()
	mux.ServeHTTP(rrBad, reqBad)
	if rrBad.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rrBad.Code)
	}
}

func TestMainFunc(t *testing.T) {
	os.Setenv("WEB_PORT", "0")
	os.Setenv("LOCAL_DIR", os.TempDir())
	defer os.Unsetenv("WEB_PORT")
	defer os.Unsetenv("LOCAL_DIR")

	go func() {
		main()
	}()
	time.Sleep(100 * time.Millisecond)
}
