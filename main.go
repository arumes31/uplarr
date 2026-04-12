package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed static/*
var staticAssets embed.FS

type FS interface {
	ReadFile(name string) ([]byte, error)
	Open(name string) (fs.File, error)
}

var appFS FS = staticAssets

type Config struct {
	LocalDir string
	WebPort  string
}

type UploadRequest struct {
	Host                    string   `json:"host"`
	Port                    int      `json:"port"`
	User                    string   `json:"user"`
	Password                string   `json:"password"`
	KeyPath                 string   `json:"key_path"`
	RemoteDir               string   `json:"remote_dir"`
	DeleteAfterVerify       bool     `json:"delete_after_verify"`
	Overwrite               bool     `json:"overwrite"`
	MaxRetries              int      `json:"max_retries"`
	SkipHostKeyVerification bool     `json:"skip_host_key_verification"`
	Files                   []string `json:"files"`
	RateLimitKBps           int      `json:"rate_limit_kbps"`
	MaxLatencyMs            int      `json:"max_latency_ms"`
}

type FileActionRequest struct {
	Action    string        `json:"action"` // "delete", "rename", "mkdir"
	Path      string        `json:"path"`
	NewName   string        `json:"new_name,omitempty"`
	Config    UploadRequest `json:"config,omitempty"` // For remote actions
}

// --- Queue Manager ---

type TaskStatus string

const (
	TaskPending   TaskStatus = "Pending"
	TaskRunning   TaskStatus = "Running"
	TaskPaused    TaskStatus = "Paused"
	TaskCompleted TaskStatus = "Completed"
	TaskFailed    TaskStatus = "Failed"
)

type Task struct {
	ID          string        `json:"id"`
	FileName    string        `json:"file_name"`
	Status      TaskStatus    `json:"status"`
	Progress    int           `json:"progress"`
	Error       string        `json:"error,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	Config      UploadRequest `json:"-"`
}

type QueueManager struct {
	tasks  []*Task
	mu     sync.RWMutex
	worker chan struct{}
}

func NewQueueManager() *QueueManager {
	qm := &QueueManager{
		tasks:  []*Task{},
		worker: make(chan struct{}, 1),
	}
	go qm.processLoop()
	return qm
}

func (qm *QueueManager) AddTask(fileName string, config UploadRequest) {
	qm.mu.Lock()
	task := &Task{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		FileName:  fileName,
		Status:    TaskPending,
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
	for range qm.worker {
		qm.processNext()
	}
}

func (qm *QueueManager) processNext() {
	qm.mu.Lock()
	var nextTask *Task
	for _, t := range qm.tasks {
		if t.Status == TaskPending {
			nextTask = t
			break
		}
	}
	if nextTask == nil {
		qm.mu.Unlock()
		return
	}
	nextTask.Status = TaskRunning
	qm.mu.Unlock()

	logInfo(fmt.Sprintf("Starting task: %s", nextTask.FileName))
	
	// Execute task
	client := SFTPClient{
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
		if err := client.Connect(); err != nil {
			return err
		}
		defer client.Close()
		
		retries := nextTask.Config.MaxRetries
		if retries <= 0 { retries = 3 }
		
		localPath := filepath.Join(globalConfig.LocalDir, nextTask.FileName)
		return client.UploadFileWithRetry(localPath, retries)
	}()

	qm.mu.Lock()
	if err != nil {
		nextTask.Status = TaskFailed
		nextTask.Error = err.Error()
		logError(fmt.Sprintf("Task failed: %s - %v", nextTask.FileName, err))
	} else {
		nextTask.Status = TaskCompleted
		logInfo(fmt.Sprintf("Task completed: %s", nextTask.FileName))
	}
	qm.mu.Unlock()

	// Check if more tasks
	qm.trigger()
}

func (qm *QueueManager) GetTasks() []*Task {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	return qm.tasks
}

func (qm *QueueManager) ControlTask(id string, action string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	for _, t := range qm.tasks {
		if t.ID == id {
			switch action {
			case "pause":
				if t.Status == TaskPending { t.Status = TaskPaused }
			case "resume":
				if t.Status == TaskPaused { t.Status = TaskPending; qm.trigger() }
			case "remove":
				// Logic to remove from slice
			}
		}
	}
}

var globalQM *QueueManager
var globalConfig Config

// --- End Queue Manager ---

var (
	logClients = make(map[chan string]bool)
	mu         sync.Mutex
)

func broadcastLog(msg string) {
	mu.Lock()
	defer mu.Unlock()
	for c := range logClients {
		select {
		case c <- msg:
		default:
		}
	}
}

type LogMessage struct {
	Level string      `json:"level"`
	Time  string      `json:"time"`
	Msg   string      `json:"msg"`
	Extra interface{} `json:"extra,omitempty"`
}

func logWithLevel(level, msg string, extra interface{}) {
	entry := LogMessage{
		Level: level,
		Time:  time.Now().Format(time.RFC3339),
		Msg:   msg,
		Extra: extra,
	}
	
	b, _ := json.Marshal(entry)
	log.Println(string(b))
	broadcastLog(string(b))
}

func logInfo(msg string) {
	logWithLevel("info", msg, nil)
}

func logError(msg string) {
	logWithLevel("error", msg, nil)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

type FileInfo struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

var osReadDir = os.ReadDir

func SetupApp(config Config) (*http.ServeMux, error) {
	globalConfig = config
	globalQM = NewQueueManager()

	if err := os.MkdirAll(config.LocalDir, 0750); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	sub, _ := fs.Sub(staticAssets, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		index, err := appFS.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(index)
	})

	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		c := make(chan string, 10)
		mu.Lock()
		logClients[c] = true
		mu.Unlock()

		defer func() {
			mu.Lock()
			delete(logClients, c)
			mu.Unlock()
			close(c)
		}()

		for {
			select {
			case <-r.Context().Done():
				return
			case msg := <-c:
				_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	})

	mux.HandleFunc("/api/files", func(w http.ResponseWriter, r *http.Request) {
		relPath := r.URL.Query().Get("path")
		fullPath := filepath.Join(config.LocalDir, relPath)

		absLocalDir, _ := filepath.Abs(config.LocalDir)
		absFullPath, err := filepath.Abs(fullPath)
		if err != nil || !strings.HasPrefix(absFullPath, absLocalDir) {
			fullPath = config.LocalDir
			relPath = ""
		}

		files, err := osReadDir(fullPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var fileInfos []FileInfo
		for _, f := range files {
			info, err := f.Info()
			if err != nil { continue }
			fileInfos = append(fileInfos, FileInfo{
				Name:  f.Name(),
				Size:  info.Size(),
				IsDir: f.IsDir(),
			})
		}
		if fileInfos == nil { fileInfos = []FileInfo{} }

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"current_path": relPath,
			"files":        fileInfos,
		})
	})

	mux.HandleFunc("/api/files/action", func(w http.ResponseWriter, r *http.Request) {
		var req FileActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		fullPath := filepath.Join(config.LocalDir, req.Path)
		// Security check
		absLocalDir, _ := filepath.Abs(config.LocalDir)
		absPath, _ := filepath.Abs(fullPath)
		if !strings.HasPrefix(absPath, absLocalDir) {
			http.Error(w, "Unauthorized path", http.StatusUnauthorized)
			return
		}

		var err error
		switch req.Action {
		case "delete":
			err = os.RemoveAll(fullPath)
		case "rename":
			newPath := filepath.Join(filepath.Dir(fullPath), req.NewName)
			err = os.Rename(fullPath, newPath)
		case "mkdir":
			err = os.MkdirAll(fullPath, 0755)
		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/test-connection", func(w http.ResponseWriter, r *http.Request) {
		var req UploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		client := SFTPClient{
			Host: req.Host, Port: strconv.Itoa(req.Port), User: req.User, Password: req.Password, KeyPath: req.KeyPath,
			SkipHostKeyVerification: req.SkipHostKeyVerification,
		}

		if err := client.Connect(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		defer client.Close()
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Connection successful"})
	})

	mux.HandleFunc("/api/remote/files", func(w http.ResponseWriter, r *http.Request) {
		var req UploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		client := SFTPClient{
			Host: req.Host, Port: strconv.Itoa(req.Port), User: req.User, Password: req.Password, KeyPath: req.KeyPath,
			SkipHostKeyVerification: req.SkipHostKeyVerification,
		}

		if err := client.Connect(); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		defer client.Close()

		remotePath := r.URL.Query().Get("path")
		if remotePath == "" { remotePath = req.RemoteDir }

		files, err := client.ReadRemoteDir(remotePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"current_path": remotePath,
			"files":        files,
		})
	})

	mux.HandleFunc("/api/remote/files/action", func(w http.ResponseWriter, r *http.Request) {
		var req FileActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		client := SFTPClient{
			Host: req.Config.Host, Port: strconv.Itoa(req.Config.Port), User: req.Config.User, Password: req.Config.Password, KeyPath: req.Config.KeyPath,
			SkipHostKeyVerification: req.Config.SkipHostKeyVerification,
		}

		if err := client.Connect(); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		defer client.Close()

		var err error
		switch req.Action {
		case "delete":
			err = client.sftpClient.Remove(req.Path)
		case "rename":
			newPath := filepath.ToSlash(filepath.Join(filepath.Dir(req.Path), req.NewName))
			err = client.sftpClient.Rename(req.Path, newPath)
		case "mkdir":
			err = client.sftpClient.Mkdir(req.Path)
		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		var req UploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		for _, file := range req.Files {
			globalQM.AddTask(file, req)
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Tasks added to queue"})
	})

	mux.HandleFunc("/api/queue", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(globalQM.GetTasks())
		} else if r.Method == http.MethodPost {
			var req struct{ ID string `json:"id"`; Action string `json:"action"` }
			_ = json.NewDecoder(r.Body).Decode(&req)
			globalQM.ControlTask(req.ID, req.Action)
			w.WriteHeader(http.StatusOK)
		}
	})

	return mux, nil
}

func ProcessUploads(config Config, req UploadRequest) []string {
	var errs []string
	for _, fileName := range req.Files {
		globalQM.AddTask(fileName, req)
	}
	return errs
}

func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fallback
		}
		return b
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		i, err := strconv.Atoi(value)
		if err != nil {
			return fallback
		}
		return i
	}
	return fallback
}

var sftpClientConnect = func(s *SFTPClient) error { return s.Connect() }
var sftpClientUpload = func(s *SFTPClient, localPath string, maxRetries int) error { return s.UploadFileWithRetry(localPath, maxRetries) }

var httpListen = http.ListenAndServe
var osExit = os.Exit

func main() {
	if err := Run(); err != nil {
		logWithLevel("error", "Application failed", map[string]string{"error": err.Error()})
		osExit(1)
	}
}

func Run() error {
	config := Config{
		LocalDir: getEnv("LOCAL_DIR", "./test_data"),
		WebPort:  getEnv("WEB_PORT", "8080"),
	}

	mux, err := SetupApp(config)
	if err != nil {
		return fmt.Errorf("setup failed: %v", err)
	}

	logWithLevel("info", "Server starting", map[string]string{"port": config.WebPort})
	return httpListen(":"+config.WebPort, mux)
}
