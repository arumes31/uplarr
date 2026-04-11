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
	Host              string `json:"host"`
	Port              int    `json:"port"`
	User              string `json:"user"`
	Password          string `json:"password"`
	KeyPath           string `json:"key_path"`
	RemoteDir         string `json:"remote_dir"`
	DeleteAfterVerify bool   `json:"delete_after_verify"`
	MaxRetries        int    `json:"max_retries"`
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
	Level string `json:"level"`
	Time  string `json:"time"`
	Msg   string `json:"msg"`
	Error string `json:"error,omitempty"`
}

func logWithLevel(level, msg string, err error) {
	entry := LogMessage{
		Level: level,
		Time:  time.Now().Format(time.RFC3339),
		Msg:   msg,
	}
	if err != nil {
		entry.Error = err.Error()
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
	if err := os.MkdirAll(config.LocalDir, 0755); err != nil {
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
		w.Write(index)
	})

	mux.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

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
				fmt.Fprintf(w, "data: %s\n\n", msg)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	})

	mux.HandleFunc("/api/files", func(w http.ResponseWriter, r *http.Request) {
		files, err := osReadDir(config.LocalDir)
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
		json.NewEncoder(w).Encode(fileInfos)
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
			Host:      req.Host,
			Port:      strconv.Itoa(req.Port),
			User:      req.User,
			Password:  req.Password,
			KeyPath:   req.KeyPath,
			RemoteDir: req.RemoteDir,
		}

		if err := client.Connect(); err != nil {
			logError(fmt.Sprintf("Connection test failed for %s: %v", req.Host, err))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Connection failed: %v", err)})
			return
		}
		defer client.Close()

		logInfo(fmt.Sprintf("Connection test successful for %s", req.Host))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Connection successful"})
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"message": "Upload process completed with some errors",
				"errors":  errs,
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"message": "All files uploaded successfully"})
	})

	return mux, nil
}

var sftpClientConnect = func(s *SFTPClient) error { return s.Connect() }
var sftpClientUpload = func(s *SFTPClient, localPath string, maxRetries int) error { return s.UploadFileWithRetry(localPath, maxRetries) }

func ProcessUploads(config Config, req UploadRequest) []string {
	var errs []string
	client := SFTPClient{
		Host:              req.Host,
		Port:              strconv.Itoa(req.Port),
		User:              req.User,
		Password:          req.Password,
		KeyPath:           req.KeyPath,
		RemoteDir:         req.RemoteDir,
		DeleteAfterVerify: req.DeleteAfterVerify,
	}

	if err := sftpClientConnect(&client); err != nil {
		msg := fmt.Sprintf("SFTP connection failed: %v", err)
		logError(msg)
		return []string{msg}
	}
	defer client.Close()

	files, err := osReadDir(config.LocalDir)
	if err != nil {
		msg := fmt.Sprintf("Failed to read local directory: %v", err)
		logError(msg)
		return []string{msg}
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		localPath := filepath.Join(config.LocalDir, f.Name())
		
		retries := req.MaxRetries
		if retries <= 0 {
			retries = 3
		}

		if err := sftpClientUpload(&client, localPath, retries); err != nil {
			msg := fmt.Sprintf("Failed to upload %s: %v", f.Name(), err)
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
		log.Printf(`{"level":"error", "msg":"Application failed", "error":"%v"}`, err)
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

	log.Printf(`{"level":"info", "msg":"Server starting on port %s"}`, config.WebPort)
	return httpListen(":"+config.WebPort, mux)
}
