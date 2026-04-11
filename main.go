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

	mux.HandleFunc("/api/files", func(w http.ResponseWriter, r *http.Request) {
		files, err := os.ReadDir(config.LocalDir)
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
			fileInfos = []FileInfo{} // Ensure we return [] instead of null
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
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Connection failed: %v", err)})
			return
		}
		defer client.Close()

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

	if err := client.Connect(); err != nil {
		msg := fmt.Sprintf("SFTP connection failed: %v", err)
		log.Printf(`{"level":"error", "msg":"%s"}`, msg)
		return []string{msg}
	}
	defer client.Close()

	files, err := os.ReadDir(config.LocalDir)
	if err != nil {
		msg := fmt.Sprintf("Failed to read local directory: %v", err)
		log.Printf(`{"level":"error", "msg":"%s"}`, msg)
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

		if err := client.UploadFileWithRetry(localPath, retries); err != nil {
			msg := fmt.Sprintf("Failed to upload %s: %v", f.Name(), err)
			log.Printf(`{"level":"error", "msg":"%s"}`, msg)
			errs = append(errs, msg)
		}
	}
	return errs
}

func main() {
	config := Config{
		LocalDir:          getEnv("LOCAL_DIR", "./test_data"),
		WebPort:           getEnv("WEB_PORT", "8080"),
	}

	mux, err := SetupApp(config)
	if err != nil {
		log.Fatalf(`{"level":"error", "msg":"Setup failed", "error":"%v"}`, err)
	}

	log.Printf(`{"level":"info", "msg":"Server starting on port %s"}`, config.WebPort)
	if err := http.ListenAndServe(":"+config.WebPort, mux); err != nil {
		log.Fatalf(`{"level":"fatal", "msg":"Server failed", "error":"%v"}`, err)
	}
}
