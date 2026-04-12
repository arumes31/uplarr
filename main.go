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
	Files                   []string `json:"files"` // Support for specific file queueing
	RateLimitKBps           int      `json:"rate_limit_kbps"`
	MaxLatencyMs            int      `json:"max_latency_ms"`
}

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

type FileInfo struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

var osReadDir = os.ReadDir

func SetupApp(config Config) (*http.ServeMux, error) {
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
		_, _ = w.Write(index) // #nosec G104
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
				_, _ = fmt.Fprintf(w, "data: %s\n\n", msg) // #nosec G104
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	})

	mux.HandleFunc("/api/files", func(w http.ResponseWriter, r *http.Request) {
		relPath := r.URL.Query().Get("path")
		fullPath := filepath.Join(config.LocalDir, relPath)

		// Security: ensure the path is within config.LocalDir
		absLocalDir, _ := filepath.Abs(config.LocalDir)
		absFullPath, err := filepath.Abs(fullPath)
		if err != nil || !strings.HasPrefix(absFullPath, absLocalDir) {
			// If it's outside, just use the root
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
			if err != nil {
				continue
			}
			fileInfos = append(fileInfos, FileInfo{
				Name:  f.Name(),
				Size:  info.Size(),
				IsDir: f.IsDir(),
			})
		}
		if fileInfos == nil {
			fileInfos = []FileInfo{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"current_path": relPath,
			"files":        fileInfos,
		})
	})

	mux.HandleFunc("/api/test-connection", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req UploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		client := SFTPClient{
			Host:                    req.Host,
			Port:                    strconv.Itoa(req.Port),
			User:                    req.User,
			Password:                req.Password,
			KeyPath:                 req.KeyPath,
			RemoteDir:               req.RemoteDir,
			SkipHostKeyVerification: req.SkipHostKeyVerification,
			RateLimitKBps:           req.RateLimitKBps,
			MaxLatencyMs:            req.MaxLatencyMs,
		}

		if err := client.Connect(); err != nil {
			logError(fmt.Sprintf("Connection test failed for %s: %v", req.Host, err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Connection failed: %v", err)}) // #nosec G104
			return
		}
		defer client.Close()

		logInfo(fmt.Sprintf("Connection test successful for %s", req.Host))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Connection successful"}) // #nosec G104
	})

	mux.HandleFunc("/api/remote/files", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req UploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		client := SFTPClient{
			Host:                    req.Host,
			Port:                    strconv.Itoa(req.Port),
			User:                    req.User,
			Password:                req.Password,
			KeyPath:                 req.KeyPath,
			RemoteDir:               req.RemoteDir,
			SkipHostKeyVerification: req.SkipHostKeyVerification,
		}

		if err := client.Connect(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		defer client.Close()

		remotePath := r.URL.Query().Get("path")
		if remotePath == "" {
			remotePath = req.RemoteDir
		}

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

	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req UploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		logInfo("Starting upload process...")
		errs := ProcessUploads(config, req)
		w.Header().Set("Content-Type", "application/json")
		if len(errs) > 0 {
			w.WriteHeader(http.StatusMultiStatus)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104
				"message": "Upload process completed with some errors",
				"errors":  errs,
			})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"message": "All selected files processed successfully"}) // #nosec G104
	})

	return mux, nil
}

var sftpClientConnect = func(s *SFTPClient) error { return s.Connect() }
var sftpClientUpload = func(s *SFTPClient, localPath string, maxRetries int) error { return s.UploadFileWithRetry(localPath, maxRetries) }

func ProcessUploads(config Config, req UploadRequest) []string {
	var errs []string
	client := SFTPClient{
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
	}

	if err := sftpClientConnect(&client); err != nil {
		msg := fmt.Sprintf("SFTP connection failed: %v", err)
		logError(msg)
		return []string{msg}
	}
	defer client.Close()

	var filesToProcess []string
	if len(req.Files) > 0 {
		filesToProcess = req.Files
	} else {
		entries, err := osReadDir(config.LocalDir)
		if err != nil {
			msg := fmt.Sprintf("Failed to read local directory: %v", err)
			logError(msg)
			return []string{msg}
		}
		for _, e := range entries {
			if !e.IsDir() {
				filesToProcess = append(filesToProcess, e.Name())
			}
		}
	}

	for _, fileName := range filesToProcess {
		// Securely join and clean the path
		localPath := filepath.Join(config.LocalDir, fileName)
		
		// Ensure the resulting path is still within config.LocalDir
		absLocalDir, err := filepath.Abs(config.LocalDir)
		if err != nil {
			logError(fmt.Sprintf("Failed to get absolute path of local dir: %v", err))
			continue
		}
		absLocalPath, err := filepath.Abs(localPath)
		if err != nil {
			logError(fmt.Sprintf("Failed to get absolute path of file %s: %v", fileName, err))
			continue
		}

		if !strings.HasPrefix(absLocalPath, absLocalDir) {
			logError(fmt.Sprintf("Security Warning: Blocked traversal attempt for file: %s", fileName))
			errs = append(errs, fmt.Sprintf("Access denied: %s is outside local directory", fileName))
			continue
		}

		retries := req.MaxRetries
		if retries <= 0 {
			retries = 3
		}

		if err := sftpClientUpload(&client, localPath, retries); err != nil {
			msg := fmt.Sprintf("Failed to upload %s: %v", fileName, err)
			logError(msg)
			errs = append(errs, msg)
		}
	}
	return errs
}

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
