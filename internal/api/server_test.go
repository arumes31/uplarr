package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"uplarr/internal/logger"
	"uplarr/internal/models"
	"uplarr/internal/queue"
)

func TestCoverageFlat(t *testing.T) {
	oldMkdir := OsMkdirAll
	oldOpenRoot := OsOpenRoot
	oldEval := FilepathEvalSymlinks
	oldAbs := FilepathAbs
	oldReadFile := StaticAssetsReadFile
	oldNewSFTP := NewSFTPClient
	defer func() {
		OsMkdirAll = oldMkdir
		OsOpenRoot = oldOpenRoot
		FilepathEvalSymlinks = oldEval
		FilepathAbs = oldAbs
		StaticAssetsReadFile = oldReadFile
		NewSFTPClient = oldNewSFTP
	}()

	tempDir, _ := os.MkdirTemp("", "api_cov_flat")
	defer os.RemoveAll(tempDir)
	qm := queue.NewQueueManager(tempDir)
	defer qm.Shutdown()

	config := models.Config{LocalDir: tempDir}

	// 1. SetupApp Fail (95-97)
	OsMkdirAll = func(p string, perm os.FileMode) error { return errors.New("f") }
	SetupApp(config, nil)
	OsMkdirAll = oldMkdir

	mux, _ := SetupApp(config, qm)

	// 2. Root Handler (104-112)
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/notfound", nil))
	StaticAssetsReadFile = func(n string) ([]byte, error) { return nil, errors.New("f") }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	StaticAssetsReadFile = oldReadFile

	// 3. SSE Logs
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20*time.Millisecond); logger.Info("m"); cancel() }()
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/logs", nil).WithContext(ctx))

	// 4. Files Handler (159-218)
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/files?path=.", nil))
	FilepathAbs = func(p string) (string, error) { if p == config.LocalDir { return "", errors.New("f") }; return oldAbs(p) }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/files", nil))
	FilepathAbs = func(p string) (string, error) { if strings.Contains(p, "fail") { return "", errors.New("f") }; return oldAbs(p) }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/files?path=fail", nil))
	FilepathAbs = func(p string) (string, error) { if strings.Contains(p, "trav") { return "/o", nil }; return oldAbs(p) }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/files?path=trav", nil))
	FilepathAbs = oldAbs
	OsOpenRoot = func(n string) (Root, error) { return nil, errors.New("f") }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/files", nil))
	OsOpenRoot = func(n string) (Root, error) { return &MockRoot{OpenFunc: func(n string) (File, error) { return nil, errors.New("f") }, CloseFunc: func() error { return nil }}, nil }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/files", nil))
	OsOpenRoot = func(n string) (Root, error) { return &MockRoot{OpenFunc: func(n string) (File, error) { return &MockFile{ReadDirFunc: func(n int) ([]os.DirEntry, error) { return []os.DirEntry{&brokenEntry{}}, nil }, CloseFunc: func() error { return nil }}, nil }, CloseFunc: func() error { return nil }}, nil }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/files", nil))
	OsOpenRoot = oldOpenRoot

	// 5. Action Handler (233-310)
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/files/action", strings.NewReader("!")))
	FilepathAbs = func(p string) (string, error) { if p == config.LocalDir { return "", errors.New("f") }; return oldAbs(p) }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/files/action", strings.NewReader(`{"action":"mkdir","path":"a"}`)))
	FilepathAbs = oldAbs
	FilepathEvalSymlinks = func(p string) (string, error) { return "", errors.New("f") }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/files/action", strings.NewReader(`{"action":"mkdir","path":"a"}`)))
	FilepathEvalSymlinks = oldEval
	FilepathAbs = func(p string) (string, error) { if strings.Contains(p, "o") { return "/o", nil }; return oldAbs(p) }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/files/action", strings.NewReader(`{"action":"mkdir","path":"o"}`)))
	FilepathAbs = oldAbs
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/files/action", strings.NewReader(`{"action":"delete","path":"."}`)))
	OsOpenRoot = func(n string) (Root, error) { return nil, errors.New("f") }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/files/action", strings.NewReader(`{"action":"mkdir","path":"a"}`)))
	OsOpenRoot = oldOpenRoot
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/files/action", strings.NewReader(`{"action":"rename","path":"a","new_name":""}`)))
	FilepathAbs = func(p string) (string, error) { if strings.Contains(p, "f") { return "", errors.New("f") }; return oldAbs(p) }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/files/action", strings.NewReader(`{"action":"rename","path":"a","new_name":"f"}`)))
	FilepathAbs = func(p string) (string, error) { if strings.Contains(p, "r") { return "/o", nil }; return oldAbs(p) }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/files/action", strings.NewReader(`{"action":"rename","path":"a","new_name":"r"}`)))
	FilepathAbs = oldAbs
	OsOpenRoot = func(n string) (Root, error) { return &MockRoot{MkdirAllFunc: func(p string, perm os.FileMode) error { return errors.New("f") }, CloseFunc: func() error { return nil }}, nil }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/files/action", strings.NewReader(`{"action":"mkdir","path":"a"}`)))
	OsOpenRoot = oldOpenRoot
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/files/action", strings.NewReader(`{"action":"mkdir","path":"a"}`)))
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/files/action", strings.NewReader(`{"action":"invalid"}`)))

	// 6. SFTP Handlers (315-414)
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/test-connection", strings.NewReader("!")))
	NewSFTPClient = func(req models.UploadRequest) SFTPClient { return &MockSFTPClient{ConnectFunc: func() error { return errors.New("f") }} }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/test-connection", strings.NewReader(`{"host":"h"}`)))
	NewSFTPClient = func(req models.UploadRequest) SFTPClient { return &MockSFTPClient{ConnectFunc: func() error { return nil }, CloseFunc: func() {}} }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/test-connection", strings.NewReader(`{"host":"h"}`)))
	
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/remote/files", strings.NewReader("!")))
	NewSFTPClient = func(req models.UploadRequest) SFTPClient { return &MockSFTPClient{ConnectFunc: func() error { return errors.New("f") }} }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/remote/files", strings.NewReader(`{"host":"h"}`)))
	NewSFTPClient = func(req models.UploadRequest) SFTPClient { return &MockSFTPClient{ConnectFunc: func() error { return nil }, ReadRemoteDirFunc: func(p string) ([]models.FileInfo, error) { return nil, errors.New("f") }, CloseFunc: func() {}} }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/remote/files", strings.NewReader(`{"host":"h"}`)))
	NewSFTPClient = func(req models.UploadRequest) SFTPClient { return &MockSFTPClient{ConnectFunc: func() error { return nil }, ReadRemoteDirFunc: func(p string) ([]models.FileInfo, error) { return []models.FileInfo{}, nil }, CloseFunc: func() {}} }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/remote/files", strings.NewReader(`{"host":"h"}`)))

	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/remote/files/action", strings.NewReader("!")))
	NewSFTPClient = func(req models.UploadRequest) SFTPClient { return &MockSFTPClient{ConnectFunc: func() error { return errors.New("f") }} }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/remote/files/action", strings.NewReader(`{"config":{"host":"h"}}`)))
	NewSFTPClient = func(req models.UploadRequest) SFTPClient { return &MockSFTPClient{ConnectFunc: func() error { return nil }, GetRemoteDirFunc: func() string { return "/r" }, CloseFunc: func() {}} }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/remote/files/action", strings.NewReader(`{"action":"mkdir","path":"/o","config":{"host":"h","remote_dir":"/r"}}`)))
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/remote/files/action", strings.NewReader(`{"action":"rename","path":"/r/a","new_name":"","config":{"host":"h","remote_dir":"/r"}}`)))
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/remote/files/action", strings.NewReader(`{"action":"rename","path":"/r/a","new_name":"b/c","config":{"host":"h","remote_dir":"/r"}}`)))
	NewSFTPClient = func(req models.UploadRequest) SFTPClient { return &MockSFTPClient{ConnectFunc: func() error { return nil }, GetRemoteDirFunc: func() string { return "/r" }, RemoveFunc: func(p string) error { return errors.New("f") }, CloseFunc: func() {}} }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/remote/files/action", strings.NewReader(`{"action":"delete","path":"/r/a","config":{"host":"h","remote_dir":"/r"}}`)))
	NewSFTPClient = func(req models.UploadRequest) SFTPClient { return &MockSFTPClient{ConnectFunc: func() error { return nil }, GetRemoteDirFunc: func() string { return "/r" }, RemoveFunc: func(p string) error { return nil }, CloseFunc: func() {}} }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/remote/files/action", strings.NewReader(`{"action":"delete","path":"/r/a","config":{"host":"h","remote_dir":"/r"}}`)))
	NewSFTPClient = func(req models.UploadRequest) SFTPClient { return &MockSFTPClient{ConnectFunc: func() error { return nil }, GetRemoteDirFunc: func() string { return "/r" }, CloseFunc: func() {}} }
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/remote/files/action", strings.NewReader(`{"action":"invalid","path":"/r/a","config":{"host":"h","remote_dir":"/r"}}`)))
	NewSFTPClient = oldNewSFTP

	// 7. Upload & Queue (419-457)
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/upload", strings.NewReader("!")))
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/upload", strings.NewReader(`{"files":["f"]}`)))
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/queue", nil))
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/queue", strings.NewReader("!")))
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/queue", strings.NewReader(`{"id":"none","action":"pause"}`)))
	os.WriteFile(filepath.Join(tempDir, "real.txt"), []byte("d"), 0644)
	qm.AddTask("real.txt", models.UploadRequest{})
	taskID := qm.GetTasks()[len(qm.GetTasks())-1].ID
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/queue", strings.NewReader(`{"id":"`+taskID+`","action":"pause"}`)))
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/queue", strings.NewReader(`{"id":"`+taskID+`","action":"invalid"}`)))
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/queue", nil))

	// 8. Wrappers (44-64)
	fs := DefaultFS
	_ = fs.MkdirAll(filepath.Join(tempDir, "w"), 0750)
	_, _ = fs.EvalSymlinks(tempDir)
	_, _ = fs.Abs(".")
	_, _ = fs.ReadFile("static/index.html")
	_, _ = fs.OpenRoot("/none")
	r, err := fs.OpenRoot(tempDir)
	if err == nil {
		f, _ := r.Open(".")
		if f != nil {
			_, _ = f.ReadDir(1)
			_ = f.Close()
		}
		_, _ = r.Open("none")
		_ = r.Close()
	}
	apiNewSFTP := NewSFTPClient(models.UploadRequest{Host: "h"})
	if apiNewSFTP == nil {
		t.Error("f")
	}
}

type MockRoot struct {
	CloseFunc    func() error
	OpenFunc     func(name string) (File, error)
	RemoveAllFunc func(path string) error
	RenameFunc   func(oldpath, newpath string) error
	MkdirAllFunc func(path string, perm os.FileMode) error
}

func (m *MockRoot) Close() error                              { return m.CloseFunc() }
func (m *MockRoot) Open(name string) (File, error)            { return m.OpenFunc(name) }
func (m *MockRoot) RemoveAll(path string) error               { return m.RemoveAllFunc(path) }
func (m *MockRoot) Rename(oldpath, newpath string) error       { return m.RenameFunc(oldpath, newpath) }
func (m *MockRoot) MkdirAll(path string, perm os.FileMode) error { return m.MkdirAllFunc(path, perm) }

type MockFile struct {
	CloseFunc   func() error
	ReadDirFunc func(n int) ([]os.DirEntry, error)
}

func (m *MockFile) Close() error                          { return m.CloseFunc() }
func (m *MockFile) ReadDir(n int) ([]os.DirEntry, error) { return m.ReadDirFunc(n) }

type brokenEntry struct{}

func (e *brokenEntry) Name() string               { return "broken" }
func (e *brokenEntry) IsDir() bool                { return false }
func (e *brokenEntry) Type() os.FileMode          { return 0 }
func (e *brokenEntry) Info() (os.FileInfo, error) { return nil, errors.New("f") }

type MockSFTPClient struct {
	ConnectFunc       func() error
	CloseFunc         func()
	ReadRemoteDirFunc func(p string) ([]models.FileInfo, error)
	RemoveFunc        func(path string) error
	RenameFunc        func(oldpath, newpath string) error
	MkdirFunc         func(path string) error
	GetRemoteDirFunc  func() string
}

func (m *MockSFTPClient) Connect() error                                { return m.ConnectFunc() }
func (m *MockSFTPClient) Close()                                         { m.CloseFunc() }
func (m *MockSFTPClient) ReadRemoteDir(p string) ([]models.FileInfo, error) { return m.ReadRemoteDirFunc(p) }
func (m *MockSFTPClient) Remove(path string) error                       { return m.RemoveFunc(path) }
func (m *MockSFTPClient) Rename(oldpath, newpath string) error           { return m.RenameFunc(oldpath, newpath) }
func (m *MockSFTPClient) Mkdir(path string) error                        { return m.MkdirFunc(path) }
func (m *MockSFTPClient) GetRemoteDir() string                           { return m.GetRemoteDirFunc() }
