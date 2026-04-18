package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"uplarr/internal/logger"
	"uplarr/internal/models"
	"uplarr/internal/sftpclient"

	"golang.org/x/time/rate"
)

var (
	FilepathAbs = filepath.Abs
	OsOpenRoot  = os.OpenRoot
)

type ClientInterface interface {
	Connect() error
	Close()
	UploadFileWithRetry(ctx context.Context, localPath string, maxRetries int) error
	ReadRemoteDir(p string) ([]models.FileInfo, error)
	Remove(path string) error
	Rename(oldpath, newpath string) error
	Mkdir(path string) error
	SetLimiter(l *sftpclient.Limiter)
}

var NewClient = func(req models.UploadRequest) ClientInterface {
	return &sftpclient.SFTPClient{
		Host:                    req.Host,
		Port:                    strconv.Itoa(req.Port),
		User:                    req.User,
		Password:                req.Password,
		KeyPath:                 req.KeyPath,
		RemoteDir:               req.RemoteDir,
		DeleteAfterVerify:       req.DeleteAfterVerify,
		Overwrite:               req.Overwrite,
		SkipHostKeyVerification: req.SkipHostKeyVerification,
		RateLimitKBps:           req.RateLimitKBps,
		MaxLatencyMs:            req.MaxLatencyMs,
		MinLimitKBps:            req.MinLimitKBps,
	}
}

type QueueManager struct {
	tasks         []*models.Task
	mu            sync.RWMutex
	worker        chan struct{}
	localDir      string
	configDir     string
	nextID        uint64
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	activeCancels map[string]context.CancelFunc
	limiters      map[string]*sftpclient.Limiter
}

func NewQueueManager(localDir, configDir string) *QueueManager {
	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0750); err != nil {
		logger.Error(fmt.Sprintf("failed to create config directory %q: %v — queue persistence will be unavailable", configDir, err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	qm := &QueueManager{
		tasks:         []*models.Task{},
		worker:        make(chan struct{}, 1),
		localDir:      localDir,
		configDir:     configDir,
		ctx:           ctx,
		cancel:        cancel,
		activeCancels: make(map[string]context.CancelFunc),
		limiters:      make(map[string]*sftpclient.Limiter),
	}
	qm.loadState()
	qm.wg.Add(1)
	go qm.processLoop()
	return qm
}

type diskTask struct {
	Task   *models.Task         `json:"task"`
	Config models.UploadRequest `json:"config"`
}

func (qm *QueueManager) saveStateLocked() {
	var dt []diskTask
	for _, t := range qm.tasks {
		dt = append(dt, diskTask{Task: t, Config: t.Config})
	}
	data, err := json.MarshalIndent(dt, "", "  ")
	if err == nil {
		root, err := OsOpenRoot(qm.configDir)
		if err != nil {
			logger.Error(fmt.Sprintf("failed to open config root for saving state: %v", err))
			return
		}
		defer root.Close()
		_ = root.WriteFile(".queue_state.json", data, 0600)
	}
}

func (qm *QueueManager) saveState() {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	qm.saveStateLocked()
}

func (qm *QueueManager) loadState() {
	root, err := OsOpenRoot(qm.configDir)
	if err != nil {
		return
	}
	defer root.Close()

	data, err := root.ReadFile(".queue_state.json")
	if err == nil {
		var dt []diskTask
		if err := json.Unmarshal(data, &dt); err == nil {
			var loaded []*models.Task
			for _, d := range dt {
				if d.Task != nil {
					d.Task.Config = d.Config
					if d.Task.Status == models.TaskRunning {
						d.Task.Status = models.TaskPending
					}
					idVal, _ := strconv.ParseUint(d.Task.ID, 10, 64)
					if idVal > qm.nextID {
						qm.nextID = idVal
					}
					loaded = append(loaded, d.Task)
				}
			}
			qm.tasks = loaded
			if len(qm.tasks) > 0 {
				go func() {
					// ensure worker is triggered to process pending tasks on startup
					qm.trigger()
				}()
			}
		}
	}
}

func (qm *QueueManager) Shutdown() {
	qm.cancel()
	qm.trigger()
	qm.wg.Wait()
}

func (qm *QueueManager) getOrCreateLimiter(config models.UploadRequest) *sftpclient.Limiter {
	if config.RateLimitKBps <= 0 && config.MaxLatencyMs <= 0 {
		return nil
	}

	qm.mu.Lock()
	defer qm.mu.Unlock()

	limit := rate.Limit(config.RateLimitKBps * 1024)
	if limit == 0 {
		limit = rate.Limit(100 * 1024 * 1024) // 100MB/s default
	}

	minLimit := rate.Limit(config.MinLimitKBps * 1024)
	if minLimit == 0 {
		minLimit = 10240 // Default 10 KB/s floor
	}

	maxLat := time.Duration(config.MaxLatencyMs) * time.Millisecond
	host := config.Host

	limiter, exists := qm.limiters[host]
	if !exists {
		burst := 16 * 1024
		if int(limit)/10 > burst {
			burst = int(limit) / 10
		}
		limiter = sftpclient.NewLimiter(limit, rate.Limit(burst), minLimit, maxLat)
		qm.limiters[host] = limiter
		return limiter
	}

	// Update existing limiter settings thread-safely
	limiter.UpdateConfig(limit, minLimit, maxLat)
	return limiter
}

func (qm *QueueManager) UpdateHostLimiter(host string, rateLimitKBps, minLimitKBps, maxLatencyMs int) {
	qm.mu.Lock()
	limiter, exists := qm.limiters[host]
	qm.mu.Unlock()

	if exists {
		limit := rate.Limit(rateLimitKBps * 1024)
		if limit == 0 && maxLatencyMs > 0 {
			limit = rate.Limit(100 * 1024 * 1024)
		}

		minLimit := rate.Limit(minLimitKBps * 1024)
		if minLimit == 0 {
			minLimit = 10240
		}

		maxLat := time.Duration(maxLatencyMs) * time.Millisecond
		limiter.UpdateConfig(limit, minLimit, maxLat)
		logger.Info(fmt.Sprintf("Live-updated throttling for host %s: %v KB/s, %v KB/s min, %v ms latency", host, rateLimitKBps, minLimitKBps, maxLatencyMs))
	}
}

func (qm *QueueManager) AddTask(fileName string, config models.UploadRequest) {
	qm.mu.Lock()
	qm.nextID++
	id := qm.nextID
	task := &models.Task{
		ID:        strconv.FormatUint(id, 10),
		FileName:  fileName,
		RemoteDir: config.RemoteDir,
		Status:    models.TaskPending,
		CreatedAt: time.Now(),
		Config:    config,
	}
	qm.tasks = append(qm.tasks, task)
	qm.mu.Unlock()
	qm.saveState()
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
	now := time.Now()
	nextTask.StartedAt = &now
	nextTask.BytesUploaded = 0
	nextTask.TotalBytes = 0
	nextTask.Progress = 0

	taskCtx, taskCancel := context.WithCancel(qm.ctx)
	qm.activeCancels[nextTask.ID] = taskCancel
	qm.mu.Unlock()

	defer taskCancel()
	defer func() {
		qm.mu.Lock()
		delete(qm.activeCancels, nextTask.ID)
		qm.mu.Unlock()
	}()

	logger.Info(fmt.Sprintf("Starting task: %s", nextTask.FileName))

	client := NewClient(nextTask.Config)

	// Setup persistent host-wide dynamic throttling
	limiter := qm.getOrCreateLimiter(nextTask.Config)
	if limiter != nil {
		client.SetLimiter(limiter)
	}

	// Wire progress callbacks if this is a real SFTPClient
	if sc, ok := client.(*sftpclient.SFTPClient); ok {
		sc.FileSizeCallback = func(totalBytes int64) {
			qm.mu.Lock()
			nextTask.TotalBytes = totalBytes
			qm.mu.Unlock()
		}
		sc.ProgressCallback = func(bytesWritten int64) {
			qm.mu.Lock()
			nextTask.BytesUploaded = bytesWritten
			if nextTask.TotalBytes > 0 {
				nextTask.Progress = int(bytesWritten * 100 / nextTask.TotalBytes)
			}
			qm.mu.Unlock()
		}
	}

	err := func() error {
		baseDir, err := FilepathAbs(qm.localDir)
		if err != nil {
			return err
		}
		if evalBase, evalErr := filepath.EvalSymlinks(baseDir); evalErr != nil {
			logger.Info(fmt.Sprintf("Warning: could not evaluate symlinks for base dir %s: %v", baseDir, evalErr))
		} else {
			baseDir = evalBase
		}

		candidatePath := filepath.Join(baseDir, nextTask.FileName)
		candidatePath, err = FilepathAbs(candidatePath)
		if err != nil {
			return err
		}

		if realPath, evalErr := filepath.EvalSymlinks(candidatePath); evalErr != nil {
			logger.Info(fmt.Sprintf("Warning: could not evaluate symlinks for candidate path %s: %v", candidatePath, evalErr))
		} else {
			candidatePath = realPath
		}

		rel, err := filepath.Rel(baseDir, candidatePath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid file path: traversal detected")
		}

		// TOCTOU mitigation: Open and verify file immediately before connecting.
		// Uses OsOpenRoot (Go 1.24+) to strictly scope access to baseDir.
		// NOTE: The file handle is closed before upload to allow UploadFileWithRetry
		// to reopen it by path. This is a best-effort mitigation; a small TOCTOU
		// window remains between this check and the actual upload read.
		root, err := OsOpenRoot(baseDir)
		if err != nil {
			return fmt.Errorf("failed to open root for validation: %v", err)
		}
		defer root.Close()

		f, err := root.Open(rel)
		if err != nil {
			return fmt.Errorf("failed to open file for validation: %v", err)
		}
		_ = f.Close()

		if err := client.Connect(); err != nil {
			return err
		}
		defer client.Close()

		retries := 3
		if nextTask.Config.MaxRetries > 0 {
			retries = nextTask.Config.MaxRetries
		}

		return client.UploadFileWithRetry(taskCtx, candidatePath, retries)
	}()

	qm.mu.Lock()
	if err != nil {
		nextTask.Status = models.TaskFailed
		nextTask.Error = err.Error()
		logger.Error(fmt.Sprintf("Task failed: %s - %v", nextTask.FileName, err))
	} else {
		nextTask.Status = models.TaskCompleted
		nextTask.Progress = 100
		if nextTask.TotalBytes > 0 {
			nextTask.BytesUploaded = nextTask.TotalBytes
		}
		logger.Info(fmt.Sprintf("Task completed: %s", nextTask.FileName))
	}
	qm.mu.Unlock()
	qm.saveState()

	qm.trigger()
}

func (qm *QueueManager) GetTasks() []*models.Task {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	snapshot := make([]*models.Task, len(qm.tasks))
	for i, t := range qm.tasks {
		copyTask := *t
		// Check if local file still exists
		fullPath := filepath.Join(qm.localDir, t.FileName)
		_, err := os.Stat(fullPath)
		copyTask.LocalFileExists = (err == nil)

		// Deep-copy mutable reference fields so snapshot doesn't share state
		if t.Config.Files != nil {
			copyTask.Config.Files = make([]string, len(t.Config.Files))
			copy(copyTask.Config.Files, t.Config.Files)
		}
		snapshot[i] = &copyTask
	}
	return snapshot
}

func (qm *QueueManager) GetHostStats() []models.HostStats {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	var stats []models.HostStats
	for host, limiter := range qm.limiters {
		// Only report hosts that are actually relevant (have tasks)
		hasActiveTasks := false
		activeCount := 0
		for _, t := range qm.tasks {
			if t.Config.Host == host && (t.Status == models.TaskRunning || t.Status == models.TaskPending) {
				hasActiveTasks = true
				if t.Status == models.TaskRunning {
					activeCount++
				}
			}
		}

		if hasActiveTasks {
			curr, max, lat := limiter.GetStats()

			// Compute per-host total speed from running tasks
			var hostSpeedBps float64
			for _, t := range qm.tasks {
				if t.Config.Host == host && t.Status == models.TaskRunning && t.StartedAt != nil && t.BytesUploaded > 0 {
					elapsed := time.Since(*t.StartedAt).Seconds()
					if elapsed > 0 {
						hostSpeedBps += float64(t.BytesUploaded) / elapsed
					}
				}
			}

			stats = append(stats, models.HostStats{
				Host:           host,
				LastLatencyMs:  lat.Milliseconds(),
				CurrentLimitKB: curr,
				MaxLimitKB:     max,
				ActiveTasks:    activeCount,
				TotalSpeedKBps: hostSpeedBps / 1024,
			})
		}
	}
	return stats
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
					if cancel, ok := qm.activeCancels[t.ID]; ok {
						cancel()
					}
					// Note: the task will find its way out of the list when it finishes with error,
					// but we also remove it immediately so it disappears from UI.
				}
				qm.tasks = append(qm.tasks[:i], qm.tasks[i+1:]...)
				qm.saveStateLocked()
				qm.trigger()
				return true, nil
			case "retry":
				if t.Status == models.TaskFailed || t.Status == models.TaskCompleted {
					fullPath := filepath.Join(qm.localDir, t.FileName)
					if _, err := os.Stat(fullPath); err != nil {
						return false, fmt.Errorf("local file no longer exists: %s", t.FileName)
					}

					t.Status = models.TaskPending
					t.Error = ""
					t.Progress = 0
					t.BytesUploaded = 0
					t.StartedAt = nil
					qm.saveStateLocked()
					qm.trigger()
					return true, nil
				}
				return false, fmt.Errorf("task is not in a retryable state")
			default:
				return false, fmt.Errorf("unknown action: %s", action)
			}
		}
	}

	if action == "retry_all_failed" {
		found := false
		for _, t := range qm.tasks {
			if t.Status == models.TaskFailed {
				fullPath := filepath.Join(qm.localDir, t.FileName)
				if _, err := os.Stat(fullPath); err != nil {
					continue // file missing, skip
				}
				t.Status = models.TaskPending
				t.Error = ""
				t.Progress = 0
				t.BytesUploaded = 0
				t.StartedAt = nil
				found = true
			}
		}
		if found {
			qm.saveStateLocked()
			qm.trigger()
		}
		return true, nil
	}

	if action == "clear_finished" {
		var active []*models.Task
		for _, t := range qm.tasks {
			if t.Status != models.TaskCompleted && t.Status != models.TaskFailed {
				active = append(active, t)
			}
		}
		qm.tasks = active
		qm.saveStateLocked()
		qm.trigger()
		return true, nil
	}
	return false, fmt.Errorf("task not found")
}
