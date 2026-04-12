package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"uplarr/internal/logger"
	"uplarr/internal/models"
	"uplarr/internal/queue"
	"uplarr/internal/sftpclient"
	"uplarr/ui"
)

func SetupApp(config models.Config, qm *queue.QueueManager) (*http.ServeMux, error) {
	if err := os.MkdirAll(config.LocalDir, 0750); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	mux.Handle("/static/", http.FileServer(http.FS(ui.StaticAssets)))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		index, err := ui.StaticAssets.ReadFile("static/index.html")
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
		logger.Mu.Lock()
		logger.LogClients[c] = true
		logger.Mu.Unlock()

		defer func() {
			logger.Mu.Lock()
			delete(logger.LogClients, c)
			logger.Mu.Unlock()
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
		if err != nil {
			fullPath = absLocalDir
			relPath = ""
		} else {
			rel, err := filepath.Rel(absLocalDir, absFullPath)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				fullPath = absLocalDir
				relPath = ""
			}
		}

		files, err := os.ReadDir(fullPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var fileInfos []models.FileInfo
		for _, f := range files {
			info, err := f.Info()
			if err != nil { continue }
			fileInfos = append(fileInfos, models.FileInfo{
				Name:  f.Name(),
				Size:  info.Size(),
				IsDir: f.IsDir(),
			})
		}
		if fileInfos == nil { fileInfos = []models.FileInfo{} }

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"current_path": relPath,
			"files":        fileInfos,
		})
	})

	mux.HandleFunc("/api/files/action", func(w http.ResponseWriter, r *http.Request) {
		var req models.FileActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		fullPath := filepath.Join(config.LocalDir, req.Path)
		absLocalDir, err := filepath.Abs(config.LocalDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		absPath, err := filepath.Abs(fullPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		rel, err := filepath.Rel(absLocalDir, absPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			http.Error(w, "Unauthorized path", http.StatusUnauthorized)
			return
		}

		if rel == "." || rel == "" {
			if req.Action == "delete" || req.Action == "rename" {
				http.Error(w, "Cannot perform action on root directory", http.StatusForbidden)
				return
			}
		}

		var errAct error
		switch req.Action {
		case "delete":
			errAct = os.RemoveAll(absPath)
		case "rename":
			if req.NewName == "" || req.NewName == "." || req.NewName == ".." || filepath.Base(req.NewName) != req.NewName {
				http.Error(w, "Invalid new name", http.StatusBadRequest)
				return
			}
			newPath := filepath.Join(filepath.Dir(absPath), req.NewName)
			errAct = os.Rename(absPath, newPath)
		case "mkdir":
			errAct = os.MkdirAll(absPath, 0750)
		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}

		if errAct != nil {
			http.Error(w, errAct.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/test-connection", func(w http.ResponseWriter, r *http.Request) {
		var req models.UploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		client := sftpclient.SFTPClient{
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
		var req models.UploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		client := sftpclient.SFTPClient{
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
		var req models.FileActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		client := sftpclient.SFTPClient{
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
			err = client.Remove(req.Path)
		case "rename":
			newPath := filepath.ToSlash(filepath.Join(filepath.Dir(req.Path), req.NewName))
			err = client.Rename(req.Path, newPath)
		case "mkdir":
			err = client.Mkdir(req.Path)
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
		var req models.UploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		for _, file := range req.Files {
			qm.AddTask(file, req)
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Tasks added to queue"})
	})

	mux.HandleFunc("/api/queue", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(qm.GetTasks())
		} else if r.Method == http.MethodPost {
			var req struct {
				ID     string `json:"id"`
				Action string `json:"action"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
				return
			}
			success, err := qm.ControlTask(req.ID, req.Action)
			if err != nil {
				if !success && strings.Contains(err.Error(), "not found") {
					http.Error(w, err.Error(), http.StatusNotFound)
				} else {
					http.Error(w, err.Error(), http.StatusBadRequest)
				}
				return
			}
			w.WriteHeader(http.StatusOK)
		}
	})

	return mux, nil
}
