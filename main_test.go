package main

import (
	"encoding/json"
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

	// Test /api/upload GET (method not allowed)
	reqUploadGet, _ := http.NewRequest("GET", "/api/upload", nil)
	rrUploadGet := httptest.NewRecorder()
	mux.ServeHTTP(rrUploadGet, reqUploadGet)
	if rrUploadGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for GET /api/upload, got %d", rrUploadGet.Code)
	}

	// Test /api/upload POST
	reqBody := `{"host":"127.0.0.1","port":22,"user":"user","password":"password","remote_dir":"/","max_retries":1}`
	reqUploadPost, _ := http.NewRequest("POST", "/api/upload", strings.NewReader(reqBody))
	reqUploadPost.Header.Set("Content-Type", "application/json")
	rrUploadPost := httptest.NewRecorder()
	mux.ServeHTTP(rrUploadPost, reqUploadPost)
	if rrUploadPost.Code != http.StatusOK {
		t.Errorf("Expected status 200 for POST /api/upload, got %d", rrUploadPost.Code)
	}

	// Test /api/upload POST with bad JSON
	reqUploadBad, _ := http.NewRequest("POST", "/api/upload", strings.NewReader(`{bad json`))
	rrUploadBad := httptest.NewRecorder()
	mux.ServeHTTP(rrUploadBad, reqUploadBad)
	if rrUploadBad.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for bad JSON, got %d", rrUploadBad.Code)
	}
	// Test SetupApp failure
	_, errSetup := SetupApp(Config{LocalDir: "/invalid/path/that/cannot/be/created/" + string([]byte{0})})
	if errSetup == nil {
		t.Errorf("Expected SetupApp to fail with invalid path")
	}

	// Give the background goroutine a moment to run and fail
	time.Sleep(50 * time.Millisecond)

	// Test appFS failure
	oldFS := appFS
	defer func() { appFS = oldFS }()
	appFS = mockErrFS{}

	rrRootErr := httptest.NewRecorder()
	mux.ServeHTTP(rrRootErr, req)
	if rrRootErr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 when index.html read fails")
	}

	// Test /api/files ReadDir failure
	reqFilesErr, _ := http.NewRequest("GET", "/api/files", nil)
	rrFilesErr := httptest.NewRecorder()
	
	errDir := filepath.Join(tempDir, "to_be_deleted")
	os.MkdirAll(errDir, 0755)
	
	configErr := config
	configErr.LocalDir = errDir
	muxErr, _ := SetupApp(configErr)
	
	// Delete it so ReadDir fails
	os.RemoveAll(errDir)
	
	muxErr.ServeHTTP(rrFilesErr, reqFilesErr)
	if rrFilesErr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500 when ReadDir fails, got %d", rrFilesErr.Code)
	}
}

type mockErrFS struct{}

func (m mockErrFS) ReadFile(name string) ([]byte, error) {
	return nil, os.ErrNotExist
}
func (m mockErrFS) Open(name string) (fs.File, error) {
	return nil, os.ErrNotExist
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
		RemoteDir:  "/",
		MaxRetries: 1,
	}

	// Create test file
	os.WriteFile(filepath.Join(localDir, "upload.txt"), []byte("data"), 0644)
	os.MkdirAll(filepath.Join(localDir, "subdir"), 0755)

	ProcessUploads(config, req)

	// Verify upload
	if content, _ := os.ReadFile(filepath.Join(remoteDir, "upload.txt")); string(content) != "data" {
		t.Errorf("ProcessUploads failed to upload file")
	}

	// Test invalid connect
	reqInvalid := req
	reqInvalid.Password = "wrong"
	ProcessUploads(config, reqInvalid) // Should log error and return

	// Test ProcessUploads ReadDir failure
	configReadDirErr := config
	configReadDirErr.LocalDir = filepath.Join(tempDir, "non_existent_local_dir")
	ProcessUploads(configReadDirErr, req)
}

func TestMainFunc(t *testing.T) {
	os.Setenv("WEB_PORT", "0") // 0 finds free port
	os.Setenv("LOCAL_DIR", os.TempDir())
	defer os.Unsetenv("WEB_PORT")
	defer os.Unsetenv("LOCAL_DIR")

	go func() {
		main()
	}()
	time.Sleep(100 * time.Millisecond)
}
