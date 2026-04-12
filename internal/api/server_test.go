package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"uplarr/internal/api"
	"uplarr/internal/models"
	"uplarr/internal/queue"
)

func TestSetupApp(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "setup_app_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	config := models.Config{
		LocalDir: tempDir,
		WebPort:  "8082",
	}

	qm := queue.NewQueueManager(config.LocalDir)
	mux, err := api.SetupApp(config, qm)
	if err != nil {
		t.Fatalf("SetupApp failed: %v", err)
	}

	// Test root endpoint
	req, _ := http.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Test static assets
	reqStatic, _ := http.NewRequest("GET", "/static/style.css", nil)
	rrStatic := httptest.NewRecorder()
	mux.ServeHTTP(rrStatic, reqStatic)
	if rrStatic.Code != http.StatusOK {
		t.Errorf("Expected status 200 for static asset, got %d", rrStatic.Code)
	}

	// Test /api/files with empty dir
	reqFiles, _ := http.NewRequest("GET", "/api/files", nil)
	rrFiles := httptest.NewRecorder()
	mux.ServeHTTP(rrFiles, reqFiles)
	if rrFiles.Code != http.StatusOK {
		t.Errorf("Expected status 200 for /api/files, got %d", rrFiles.Code)
	}
	var response struct {
		CurrentPath string            `json:"current_path"`
		Files       []models.FileInfo `json:"files"`
	}
	if err := json.NewDecoder(rrFiles.Body).Decode(&response); err != nil {
		t.Errorf("Failed to decode JSON: %v", err)
	}
	if len(response.Files) != 0 {
		t.Errorf("Expected 0 files, got %d", len(response.Files))
	}

	// Add file and directory
	os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("hello"), 0644)
	os.Mkdir(filepath.Join(tempDir, "testdir"), 0755)

	rrFiles2 := httptest.NewRecorder()
	mux.ServeHTTP(rrFiles2, reqFiles)
	if err := json.NewDecoder(rrFiles2.Body).Decode(&response); err != nil {
		t.Errorf("Failed to decode JSON: %v", err)
	}
	if len(response.Files) != 2 {
		t.Errorf("Expected 2 items, got %d", len(response.Files))
	}

	// Test /api/test-connection POST (fail connect)
	reqBody := `{"host":"127.0.0.1","port":22,"user":"user","password":"password","skip_host_key_verification":true}`
	reqTestConn, _ := http.NewRequest("POST", "/api/test-connection", strings.NewReader(reqBody))
	rrTestConn := httptest.NewRecorder()
	mux.ServeHTTP(rrTestConn, reqTestConn)
	if rrTestConn.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for failed connect, got %d", rrTestConn.Code)
	}

	// Test /api/upload POST (Queue)
	reqUploadPost, _ := http.NewRequest("POST", "/api/upload", strings.NewReader(reqBody))
	reqUploadPost.Header.Set("Content-Type", "application/json")
	rrUploadPost := httptest.NewRecorder()
	mux.ServeHTTP(rrUploadPost, reqUploadPost)
	if rrUploadPost.Code != http.StatusOK {
		t.Errorf("Expected status 200 for queued upload, got %d", rrUploadPost.Code)
	}

	// Test /api/remote/files (fail connect)
	reqRemote, _ := http.NewRequest("POST", "/api/remote/files", strings.NewReader(reqBody))
	reqRemote.Header.Set("Content-Type", "application/json")
	rrRemote := httptest.NewRecorder()
	mux.ServeHTTP(rrRemote, reqRemote)
	if rrRemote.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for failed remote connect, got %d", rrRemote.Code)
	}
}
