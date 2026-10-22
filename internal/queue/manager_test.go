package queue_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"uplarr/internal/models"
	"uplarr/internal/queue"
	"uplarr/internal/sftpclient"
)

// waitForTaskStatus polls qm.GetTasks() until a task matching the predicate is found or timeout elapses.
func waitForTaskStatus(t *testing.T, qm *queue.QueueManager, predicate func([]*models.Task) bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate(qm.GetTasks()) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Timed out waiting for expected task status after %v", timeout)
}

type mockClient struct {
	connectErr error
	uploadErr  error
}

func (m *mockClient) Connect() error { return m.connectErr }
func (m *mockClient) Close()         {}
func (m *mockClient) UploadFileWithRetry(ctx context.Context, localPath string, maxRetries int) error {
	return m.uploadErr
}
func (m *mockClient) ReadRemoteDir(p string) ([]models.FileInfo, error) { return nil, nil }
func (m *mockClient) Remove(path string) error                          { return nil }
func (m *mockClient) Rename(oldpath, newpath string) error              { return nil }
func (m *mockClient) Mkdir(path string) error                           { return nil }
func (m *mockClient) SetLimiter(l *sftpclient.Limiter)                  {}

type stalledIOClient struct {
	mockClient
	started   chan struct{}
	unblocked chan struct{}
	closeOnce sync.Once
}

func (c *stalledIOClient) Close() {
	c.closeOnce.Do(func() { close(c.unblocked) })
}

func (c *stalledIOClient) UploadFileWithRetry(ctx context.Context, _ string, _ int) error {
	close(c.started)
	<-c.unblocked
	return ctx.Err()
}

func TestQueueManager(t *testing.T) {
	localDir, err := os.MkdirTemp("", "qm_test_local")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(localDir) })

	configDir, err := os.MkdirTemp("", "qm_test_config")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(configDir) })

	qm := queue.NewQueueManager(localDir, configDir)
	defer qm.Shutdown()

	// Test AddTask
	qm.AddTask("test1.txt", models.UploadRequest{})
	tasks := qm.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("Expected 1 task, got %d", len(tasks))
	}
}

func TestQueueManager_ShutdownInterruptsStalledSFTPIO(t *testing.T) {
	localDir := t.TempDir()
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(localDir, "stalled.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	client := &stalledIOClient{
		started:   make(chan struct{}),
		unblocked: make(chan struct{}),
	}
	t.Cleanup(client.Close)

	oldNewClient := queue.NewClient
	queue.NewClient = func(models.UploadRequest) queue.ClientInterface { return client }
	t.Cleanup(func() { queue.NewClient = oldNewClient })

	qm := queue.NewQueueManager(localDir, configDir)
	qm.AddTask("stalled.txt", models.UploadRequest{})

	select {
	case <-client.started:
	case <-time.After(2 * time.Second):
		t.Fatal("upload did not start")
	}

	shutdownDone := make(chan struct{})
	go func() {
		qm.Shutdown()
		close(shutdownDone)
	}()

	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("Shutdown did not interrupt stalled SFTP I/O")
	}
}

func TestQueueManager_Control(t *testing.T) {
	localDir, err := os.MkdirTemp("", "qm_ctrl_local")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(localDir) })

	configDir, err := os.MkdirTemp("", "qm_ctrl_config")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(configDir) })

	// Mock NewClient to block so we can pause a pending task
	oldNewClient := queue.NewClient
	blockConnect := make(chan struct{})
	blockClosed := false
	t.Cleanup(func() {
		if !blockClosed {
			close(blockConnect)
			blockClosed = true
		}
	})
	queue.NewClient = func(req models.UploadRequest) queue.ClientInterface {
		return &blockingClient{block: blockConnect}
	}
	defer func() { queue.NewClient = oldNewClient }()

	qm := queue.NewQueueManager(localDir, configDir)

	// 1. Task Not Found
	_, err = qm.ControlTask("non-existent", "remove")
	if err == nil {
		t.Error("Expected error for non-existent task")
	}

	// 2. Add first task - will block in Connect (become Running)
	if err := os.WriteFile(filepath.Join(localDir, "task1.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("Failed to write fixture task1.txt: %v", err)
	}
	qm.AddTask("task1.txt", models.UploadRequest{})

	// Add second task - will stay Pending
	qm.AddTask("task2.txt", models.UploadRequest{})

	// Add third task - will stay Pending
	qm.AddTask("task3.txt", models.UploadRequest{})

	waitForTaskStatus(t, qm, func(tasks []*models.Task) bool {
		for _, task := range tasks {
			if task.Status == models.TaskRunning {
				return true
			}
		}
		return false
	}, 2*time.Second)
	tasks := qm.GetTasks()

	var runningID, pendingID, pendingID2 string
	for _, task := range tasks {
		switch task.Status {
		case models.TaskRunning:
			runningID = task.ID
		case models.TaskPending:
			if pendingID == "" {
				pendingID = task.ID
			} else {
				pendingID2 = task.ID
			}
		}
	}

	if runningID != "" {
		// Test pause fail (running)
		_, err = qm.ControlTask(runningID, "pause")
		if err == nil {
			t.Error("Expected error pausing running task")
		}

		// Test remove success (running) - THIS IS NOW ALLOWED
		success, err := qm.ControlTask(runningID, "remove")
		if err != nil || !success {
			t.Errorf("Expected success removing running task, got %v", err)
		}

		// Wait for it to disappear from tasks
		waitForTaskStatus(t, qm, func(tasks []*models.Task) bool {
			for _, t := range tasks {
				if t.ID == runningID {
					return false
				}
			}
			return true
		}, 1*time.Second)
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
		if err == nil {
			t.Error("Expected error resuming already pending task")
		}

		// Test remove success (pending)
		success, err = qm.ControlTask(pendingID, "remove")
		if err != nil || !success {
			t.Errorf("Expected success removing pending task, got %v", err)
		}
	}

	if pendingID2 != "" {
		// Test unknown action
		_, err = qm.ControlTask(pendingID2, "unknown")
		if err == nil {
			t.Error("Expected error for unknown action")
		}
	}

	// Unblock and shutdown
	blockClosed = true
	close(blockConnect)
	qm.Shutdown()
}

func TestQueueManager_ProcessNext_FilepathAbsErrors(t *testing.T) {
	localDir, err := os.MkdirTemp("", "qm_abs_local")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(localDir) })

	configDir, err := os.MkdirTemp("", "qm_abs_config")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(configDir) })

	oldAbs := queue.FilepathAbs
	defer func() { queue.FilepathAbs = oldAbs }()

	qm := queue.NewQueueManager(localDir, configDir)

	// Test first FilepathAbs error
	queue.FilepathAbs = func(path string) (string, error) {
		return "", os.ErrPermission
	}
	qm.AddTask("any.txt", models.UploadRequest{})
	waitForTaskStatus(t, qm, func(tasks []*models.Task) bool {
		return len(tasks) == 1 && tasks[0].Status == models.TaskFailed
	}, 2*time.Second)

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
	waitForTaskStatus(t, qm, func(tasks []*models.Task) bool {
		return len(tasks) == 2 && tasks[1].Status == models.TaskFailed
	}, 2*time.Second)

	qm.Shutdown()
}

func TestQueueManager_ProcessNext_OpenRootError(t *testing.T) {
	localDir, err := os.MkdirTemp("", "qm_root_local")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(localDir) })

	configDir, err := os.MkdirTemp("", "qm_root_config")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(configDir) })

	oldOpenRoot := queue.OsOpenRoot
	queue.OsOpenRoot = func(name string) (*os.Root, error) {
		return nil, os.ErrPermission
	}
	defer func() { queue.OsOpenRoot = oldOpenRoot }()

	qm := queue.NewQueueManager(localDir, configDir)
	qm.AddTask("any.txt", models.UploadRequest{})
	time.Sleep(50 * time.Millisecond)
	qm.Shutdown()
}

func TestQueueManager_ProcessNext_Traversal(t *testing.T) {
	localDir, err := os.MkdirTemp("", "qm_trav_local")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(localDir) })

	configDir, err := os.MkdirTemp("", "qm_trav_config")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(configDir) })

	qm := queue.NewQueueManager(localDir, configDir)
	qm.AddTask("../escaped.txt", models.UploadRequest{})

	waitForTaskStatus(t, qm, func(tasks []*models.Task) bool {
		for _, task := range tasks {
			if task.Status == models.TaskFailed {
				return true
			}
		}
		return false
	}, 2*time.Second)

	tasks := qm.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("Expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Status != models.TaskFailed {
		t.Errorf("Expected TaskFailed, got %s", tasks[0].Status)
	}
	if !strings.Contains(tasks[0].Error, "traversal detected") {
		t.Errorf("Expected error containing 'traversal detected', got %q", tasks[0].Error)
	}
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
	localDir, err := os.MkdirTemp("", "qm_retries_local")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(localDir) })

	configDir, err := os.MkdirTemp("", "qm_retries_config")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(configDir) })

	testFile := filepath.Join(localDir, "retry.txt")
	if err := os.WriteFile(testFile, []byte("data"), 0644); err != nil {
		t.Fatalf("Failed to write fixture retry.txt: %v", err)
	}

	oldNewClient := queue.NewClient
	defer func() { queue.NewClient = oldNewClient }()
	queue.NewClient = func(req models.UploadRequest) queue.ClientInterface {
		return &mockClient{}
	}

	qm := queue.NewQueueManager(localDir, configDir)
	// MaxRetries = 0 should trigger default = 3
	qm.AddTask("retry.txt", models.UploadRequest{MaxRetries: 0})
	// MaxRetries = 5 should hit the other branch
	if err := os.WriteFile(filepath.Join(localDir, "retry5.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("Failed to write fixture retry5.txt: %v", err)
	}
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

func TestQueueManager_PersistsPasswordEncrypted(t *testing.T) {
	localDir := t.TempDir()
	configDir := t.TempDir()

	oldNewClient := queue.NewClient
	queue.NewClient = func(models.UploadRequest) queue.ClientInterface {
		return &mockClient{connectErr: os.ErrPermission}
	}
	t.Cleanup(func() { queue.NewClient = oldNewClient })

	qm := queue.NewQueueManager(localDir, configDir)
	qm.AddTask("missing.txt", models.UploadRequest{Host: "h", Password: "s3cret-pass"})
	qm.Shutdown()

	data, err := os.ReadFile(filepath.Join(configDir, ".queue_state.json"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if strings.Contains(string(data), "s3cret-pass") {
		t.Fatal("queue state contains the plaintext password")
	}
	if !strings.Contains(string(data), "password_enc") {
		t.Fatal("queue state does not contain the encrypted password")
	}

	qm2 := queue.NewQueueManager(localDir, configDir)
	defer qm2.Shutdown()
	tasks := qm2.GetTasks()
	if len(tasks) != 1 || tasks[0].Config.Password != "s3cret-pass" {
		t.Fatalf("expected password to be restored after reload, got %+v", tasks)
	}
}

func TestQueueManager_MigratesPlaintextPassword(t *testing.T) {
	localDir := t.TempDir()
	configDir := t.TempDir()
	statePath := filepath.Join(configDir, ".queue_state.json")
	legacy := `{"tasks":[{"task":{"id":"1","file_name":"a.txt","status":"Paused"},"config":{"host":"h","password":"old-plain"}}],"max_concurrent":1}`
	if err := os.WriteFile(statePath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}

	qm := queue.NewQueueManager(localDir, configDir)
	defer qm.Shutdown()

	tasks := qm.GetTasks()
	if len(tasks) != 1 || tasks[0].Config.Password != "old-plain" {
		t.Fatalf("expected legacy password to load, got %+v", tasks)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "old-plain") {
		t.Fatal("legacy plaintext password was not re-encrypted on load")
	}
}

func TestQueueManager_SecretKeyFromEnv(t *testing.T) {
	t.Setenv("UPLARR_SECRET_KEY", "operator-supplied-key")
	configDir := t.TempDir()

	qm := queue.NewQueueManager(t.TempDir(), configDir)
	qm.Shutdown()

	if _, err := os.Stat(filepath.Join(configDir, ".secret_key")); !os.IsNotExist(err) {
		t.Fatalf("expected no generated key file when UPLARR_SECRET_KEY is set, got %v", err)
	}
}

func TestQueueManager_KeepsCredentialsItCannotDecrypt(t *testing.T) {
	localDir := t.TempDir()
	configDir := t.TempDir()

	oldNewClient := queue.NewClient
	queue.NewClient = func(models.UploadRequest) queue.ClientInterface {
		return &mockClient{connectErr: os.ErrPermission}
	}
	t.Cleanup(func() { queue.NewClient = oldNewClient })

	t.Setenv("UPLARR_SECRET_KEY", "key-a")
	qm := queue.NewQueueManager(localDir, configDir)
	qm.AddTask("missing.txt", models.UploadRequest{Host: "h", Password: "keep-me"})
	qm.Shutdown()

	// A different key cannot decrypt the password; saving must not drop it.
	t.Setenv("UPLARR_SECRET_KEY", "key-b")
	qm = queue.NewQueueManager(localDir, configDir)
	qm.AddTask("other.txt", models.UploadRequest{Host: "h"})
	qm.Shutdown()

	t.Setenv("UPLARR_SECRET_KEY", "key-a")
	qm = queue.NewQueueManager(localDir, configDir)
	defer qm.Shutdown()
	tasks := qm.GetTasks()
	if len(tasks) != 2 || tasks[0].Config.Password != "keep-me" {
		t.Fatalf("expected original password to survive a save with the wrong key, got %+v", tasks)
	}
}

func TestQueueManager_ConnectErrorIsGeneric(t *testing.T) {
	localDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(localDir, "f.txt"), []byte("d"), 0600); err != nil {
		t.Fatal(err)
	}
	oldNewClient := queue.NewClient
	queue.NewClient = func(models.UploadRequest) queue.ClientInterface {
		return &mockClient{connectErr: errors.New("failed to dial: dial tcp 10.1.2.3:2222: connect: connection refused")}
	}
	t.Cleanup(func() { queue.NewClient = oldNewClient })

	qm := queue.NewQueueManager(localDir, t.TempDir())
	defer qm.Shutdown()
	qm.AddTask("f.txt", models.UploadRequest{Host: "10.1.2.3", Port: 2222})

	waitForTaskStatus(t, qm, func(tasks []*models.Task) bool {
		return len(tasks) == 1 && tasks[0].Status == models.TaskFailed
	}, 5*time.Second)
	if got := qm.GetTasks()[0].Error; got != "Connection failed" {
		t.Fatalf("task error = %q, want generic message", got)
	}
}
