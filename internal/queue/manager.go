package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"uplarr/internal/logger"
	"uplarr/internal/models"
	"uplarr/internal/sftpclient"
)

type QueueManager struct {
	tasks    []*models.Task
	mu       sync.RWMutex
	worker   chan struct{}
	localDir string
	nextID   uint64
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewQueueManager(localDir string) *QueueManager {
	ctx, cancel := context.WithCancel(context.Background())
	qm := &QueueManager{
		tasks:    []*models.Task{},
		worker:   make(chan struct{}, 1),
		localDir: localDir,
		ctx:      ctx,
		cancel:   cancel,
	}
	qm.wg.Add(1)
	go qm.processLoop()
	return qm
}

func (qm *QueueManager) Shutdown() {
	qm.cancel()
	qm.trigger()
	qm.wg.Wait()
}

func (qm *QueueManager) AddTask(fileName string, config models.UploadRequest) {
	qm.mu.Lock()
	id := atomic.AddUint64(&qm.nextID, 1)
	task := &models.Task{
		ID:        strconv.FormatUint(id, 10),
		FileName:  fileName,
		Status:    models.TaskPending,
		CreatedAt: time.Now(),
		Config:    config,
	}
	qm.tasks = append(qm.tasks, task)
	qm.mu.Unlock()
	qm.trigger()
}

func (qm *QueueManager) trigger() {
	select {
	case qm.worker <- struct{}{}:
	default:
	}
}

func (qm *QueueManager) processLoop() {
	defer qm.wg.Done()
	for {
		select {
		case <-qm.ctx.Done():
			return
		case <-qm.worker:
			qm.processNext()
		}
	}
}

func (qm *QueueManager) processNext() {
	qm.mu.Lock()
	var nextTask *models.Task
	for _, t := range qm.tasks {
		if t.Status == models.TaskPending {
			nextTask = t
			break
		}
	}
	if nextTask == nil {
		qm.mu.Unlock()
		return
	}
	nextTask.Status = models.TaskRunning
	qm.mu.Unlock()

	logger.Info(fmt.Sprintf("Starting task: %s", nextTask.FileName))

	client := sftpclient.SFTPClient{
		Host:                    nextTask.Config.Host,
		Port:                    strconv.Itoa(nextTask.Config.Port),
		User:                    nextTask.Config.User,
		Password:                nextTask.Config.Password,
		KeyPath:                 nextTask.Config.KeyPath,
		RemoteDir:               nextTask.Config.RemoteDir,
		DeleteAfterVerify:       nextTask.Config.DeleteAfterVerify,
		Overwrite:               nextTask.Config.Overwrite,
		SkipHostKeyVerification: nextTask.Config.SkipHostKeyVerification,
		RateLimitKBps:           nextTask.Config.RateLimitKBps,
		MaxLatencyMs:            nextTask.Config.MaxLatencyMs,
	}

	err := func() error {
		baseDir, err := filepath.Abs(qm.localDir)
		if err != nil { return err }
		baseDir, _ = filepath.EvalSymlinks(baseDir)

		candidatePath := filepath.Join(baseDir, nextTask.FileName)
		candidatePath, err = filepath.Abs(candidatePath)
		if err != nil { return err }

		if realPath, err := filepath.EvalSymlinks(candidatePath); err == nil {
			candidatePath = realPath
		}

		rel, err := filepath.Rel(baseDir, candidatePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid file path: traversal detected")
		}

		// TOCTOU Fix: Open and verify file immediately before connecting
		f, err := os.Open(candidatePath)
		if err != nil {
			return fmt.Errorf("failed to open file for validation: %v", err)
		}
		_ = f.Close()

		if err := client.Connect(); err != nil {
			return err
		}
		defer client.Close()

		retries := nextTask.Config.MaxRetries
		if retries <= 0 { retries = 3 }

		return client.UploadFileWithRetry(candidatePath, retries)
	}()

	qm.mu.Lock()
	if err != nil {
		nextTask.Status = models.TaskFailed
		nextTask.Error = err.Error()
		logger.Error(fmt.Sprintf("Task failed: %s - %v", nextTask.FileName, err))
	} else {
		nextTask.Status = models.TaskCompleted
		logger.Info(fmt.Sprintf("Task completed: %s", nextTask.FileName))
	}
	qm.mu.Unlock()

	qm.trigger()
}

func (qm *QueueManager) GetTasks() []*models.Task {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	snapshot := make([]*models.Task, len(qm.tasks))
	for i, t := range qm.tasks {
		copyTask := *t
		snapshot[i] = &copyTask
	}
	return snapshot
}

func (qm *QueueManager) ControlTask(id string, action string) (bool, error) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	for i, t := range qm.tasks {
		if t.ID == id {
			switch action {
			case "pause":
				if t.Status == models.TaskPending {
					t.Status = models.TaskPaused
					return true, nil
				}
				return false, fmt.Errorf("task is not pending")
			case "resume":
				if t.Status == models.TaskPaused {
					t.Status = models.TaskPending
					qm.trigger()
					return true, nil
				}
				return false, fmt.Errorf("task is not paused")
			case "remove":
				if t.Status == models.TaskRunning {
					return false, fmt.Errorf("cannot remove a running task")
				}
				qm.tasks = append(qm.tasks[:i], qm.tasks[i+1:]...)
				qm.trigger()
				return true, nil
			default:
				return false, fmt.Errorf("unknown action: %s", action)
			}
		}
	}
	return false, fmt.Errorf("task not found")
}
