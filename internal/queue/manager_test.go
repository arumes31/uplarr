package queue_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"uplarr/internal/models"
	"uplarr/internal/queue"
)

type mockClient struct {
	connectErr error
	uploadErr  error
}

func (m *mockClient) Connect() error { return m.connectErr }
func (m *mockClient) Close() {}
func (m *mockClient) UploadFileWithRetry(localPath string, maxRetries int) error { return m.uploadErr }
func (m *mockClient) ReadRemoteDir(p string) ([]models.FileInfo, error) { return nil, nil }
func (m *mockClient) Remove(path string) error { return nil }
func (m *mockClient) Rename(oldpath, newpath string) error { return nil }
func (m *mockClient) Mkdir(path string) error { return nil }

func TestQueueManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "qm_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	qm := queue.NewQueueManager(tempDir)
	defer qm.Shutdown()

	// Test AddTask
	qm.AddTask("test1.txt", models.UploadRequest{})
	tasks := qm.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("Expected 1 task, got %d", len(tasks))
	}
}

func TestQueueManager_Control(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "qm_test_ctrl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Mock NewClient to block so we can pause a pending task
	oldNewClient := queue.NewClient
	blockConnect := make(chan struct{})
	queue.NewClient = func(req models.UploadRequest) queue.ClientInterface {
		return &blockingClient{block: blockConnect}
	}
	defer func() { queue.NewClient = oldNewClient }()

	qm := queue.NewQueueManager(tempDir)

	// 1. Task Not Found
	_, err = qm.ControlTask("non-existent", "remove")
	if err == nil {
		t.Error("Expected error for non-existent task")
	}

	// 2. Add first task - will block in Connect (become Running)
	os.WriteFile(filepath.Join(tempDir, "task1.txt"), []byte("data"), 0644)
	qm.AddTask("task1.txt", models.UploadRequest{})
	
	// Add second task - will stay Pending
	qm.AddTask("task2.txt", models.UploadRequest{})
	
	// Add third task - will stay Pending
	qm.AddTask("task3.txt", models.UploadRequest{})
	
	time.Sleep(100 * time.Millisecond)
	tasks := qm.GetTasks()
	
	var runningID, pendingID, pendingID2 string
	for _, t := range tasks {
		if t.Status == models.TaskRunning {
			runningID = t.ID
		} else if t.Status == models.TaskPending {
			if pendingID == "" {
				pendingID = t.ID
			} else {
				pendingID2 = t.ID
			}
		}
	}

	if runningID != "" {
		// Test pause fail (running)
		_, err = qm.ControlTask(runningID, "pause")
		if err == nil { t.Error("Expected error pausing running task") }
		
		// Test remove fail (running)
		_, err = qm.ControlTask(runningID, "remove")
		if err == nil { t.Error("Expected error removing running task") }
	}

	if pendingID != "" {
		// Test pause success
		success, err := qm.ControlTask(pendingID, "pause")
		if err != nil || !success {
			t.Errorf("Expected success pausing pending task, got %v", err)
		}
		
		// Test resume success
		success, err = qm.ControlTask(pendingID, "resume")
		if err != nil || !success {
			t.Errorf("Expected success resuming paused task, got %v", err)
		}
		
		// Test resume fail (already pending)
		_, err = qm.ControlTask(pendingID, "resume")
		if err == nil { t.Error("Expected error resuming already pending task") }
		
		// Test remove success (pending)
		success, err = qm.ControlTask(pendingID, "remove")
		if err != nil || !success {
			t.Errorf("Expected success removing pending task, got %v", err)
		}
	}
	
	if pendingID2 != "" {
		// Test unknown action
		_, err = qm.ControlTask(pendingID2, "unknown")
		if err == nil { t.Error("Expected error for unknown action") }
	}

	// Unblock and shutdown
	close(blockConnect)
	qm.Shutdown()
}

func TestQueueManager_ProcessNext_FilepathAbsErrors(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "qm_test_abs_errs")
	defer os.RemoveAll(tempDir)

	oldAbs := queue.FilepathAbs
	defer func() { queue.FilepathAbs = oldAbs }()

	qm := queue.NewQueueManager(tempDir)

	// Test first FilepathAbs error
	queue.FilepathAbs = func(path string) (string, error) {
		return "", os.ErrPermission
	}
	qm.AddTask("any.txt", models.UploadRequest{})
	time.Sleep(50 * time.Millisecond)
	
	// Test second FilepathAbs error
	callCount := 0
	queue.FilepathAbs = func(path string) (string, error) {
		callCount++
		if callCount == 2 {
			return "", os.ErrPermission
		}
		return oldAbs(path)
	}
	qm.AddTask("any2.txt", models.UploadRequest{})
	time.Sleep(50 * time.Millisecond)

	qm.Shutdown()
}

func TestQueueManager_ProcessNext_OpenRootError(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "qm_test_root_err")
	defer os.RemoveAll(tempDir)

	oldOpenRoot := queue.OsOpenRoot
	queue.OsOpenRoot = func(name string) (*os.Root, error) {
		return nil, os.ErrPermission
	}
	defer func() { queue.OsOpenRoot = oldOpenRoot }()

	qm := queue.NewQueueManager(tempDir)
	qm.AddTask("any.txt", models.UploadRequest{})
	time.Sleep(50 * time.Millisecond)
	qm.Shutdown()
}

func TestQueueManager_ProcessNext_Traversal(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "qm_test_trav")
	defer os.RemoveAll(tempDir)
	qm := queue.NewQueueManager(tempDir)
	qm.AddTask("../escaped.txt", models.UploadRequest{})
	time.Sleep(50 * time.Millisecond)
	qm.Shutdown()
}

func TestQueueManager_DefaultNewClient(t *testing.T) {
	// Test the default factory does not panic
	client := queue.NewClient(models.UploadRequest{Port: 22})
	if client == nil {
		t.Fatal("Expected client")
	}
}

func TestQueueManager_RetriesDefault(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "qm_test_retries")
	defer os.RemoveAll(tempDir)
	testFile := filepath.Join(tempDir, "retry.txt")
	os.WriteFile(testFile, []byte("data"), 0644)

	oldNewClient := queue.NewClient
	defer func() { queue.NewClient = oldNewClient }()
	queue.NewClient = func(req models.UploadRequest) queue.ClientInterface {
		return &mockClient{}
	}

	qm := queue.NewQueueManager(tempDir)
	// MaxRetries = 0 should trigger default = 3
	qm.AddTask("retry.txt", models.UploadRequest{MaxRetries: 0})
	// MaxRetries = 5 should hit the other branch
	os.WriteFile(filepath.Join(tempDir, "retry5.txt"), []byte("data"), 0644)
	qm.AddTask("retry5.txt", models.UploadRequest{MaxRetries: 5})
	// Also add a non-existent file to trigger root.Open error
	qm.AddTask("non-existent.txt", models.UploadRequest{})
	
	time.Sleep(200 * time.Millisecond)
	qm.Shutdown()
}

type blockingClient struct {
	mockClient
	block chan struct{}
}
func (b *blockingClient) Connect() error {
	<-b.block
	return nil
}
