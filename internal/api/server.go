package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"

	"uplarr/internal/logger"
	"uplarr/internal/models"
	"uplarr/internal/queue"
	"uplarr/internal/sftpclient"
	"uplarr/ui"
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

var (
	sessions = make(map[string]bool)
	sessionsMu sync.RWMutex
)

func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// This should never happen with crypto/rand
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return base64.StdEncoding.EncodeToString(b)
}

// FileSystem interface for easier mocking of Go 1.24+ os.Root features
type FileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	OpenRoot(name string) (Root, error)
	EvalSymlinks(path string) (string, error)
	Abs(path string) (string, error)
	ReadFile(name string) ([]byte, error)
}

type Root interface {
	io.Closer
	Open(name string) (File, error)
	RemoveAll(path string) error
	Rename(oldpath, newpath string) error
	MkdirAll(path string, perm os.FileMode) error
}

type File interface {
	io.Closer
	ReadDir(n int) ([]os.DirEntry, error)
}

type realFileSystem struct{}

func (f realFileSystem) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (f realFileSystem) OpenRoot(name string) (Root, error) {
	r, err := os.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return realRoot{r}, nil
}
func (f realFileSystem) EvalSymlinks(path string) (string, error) { return filepath.EvalSymlinks(path) }
func (f realFileSystem) Abs(path string) (string, error)          { return filepath.Abs(path) }
func (f realFileSystem) ReadFile(name string) ([]byte, error)     { return ui.StaticAssets.ReadFile(name) }

type realRoot struct{ *os.Root }

func (r realRoot) Open(name string) (File, error) {
	f, err := r.Root.Open(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

var DefaultFS FileSystem = realFileSystem{}

var (
	OsMkdirAll           = DefaultFS.MkdirAll
	OsOpenRoot           = DefaultFS.OpenRoot
	FilepathEvalSymlinks = DefaultFS.EvalSymlinks
	FilepathAbs          = DefaultFS.Abs
	StaticAssetsReadFile = DefaultFS.ReadFile
)

type SFTPClient interface {
	Connect() error
	Close()
	ReadRemoteDir(p string) ([]models.FileInfo, error)
	Remove(path string) error
	Rename(oldpath, newpath string) error
	Mkdir(path string) error
	GetRemoteDir() string
}

var NewSFTPClient = func(req models.UploadRequest) SFTPClient {
	return &sftpclient.SFTPClient{
		Host: req.Host, Port: strconv.Itoa(req.Port), User: req.User, Password: req.Password, KeyPath: req.KeyPath,
		SkipHostKeyVerification: req.SkipHostKeyVerification,
		RemoteDir:               req.RemoteDir,
	}
}

func SetupApp(config models.Config, qm *queue.QueueManager) (*http.ServeMux, error) {
	if err := OsMkdirAll(config.LocalDir, 0750); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()

	withAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if config.AuthPassword == "" {
				next(w, r)
				return
			}
			cookie, err := r.Cookie("uplarr_session")
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			sessionsMu.RLock()
			valid := sessions[cookie.Value]
			sessionsMu.RUnlock()
			if !valid {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	mux.Handle("/static/", http.FileServer(http.FS(ui.StaticAssets)))

	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if config.AuthPassword == "" {
			w.WriteHeader(http.StatusOK)
			return
		}
		var loginReq struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		if loginReq.Password != config.AuthPassword {
			http.Error(w, "Invalid password", http.StatusUnauthorized)
			return
		}

		token := generateToken()
		sessionsMu.Lock()
		sessions[token] = true
		sessionsMu.Unlock()

		// #nosec G124
		http.SetCookie(w, &http.Cookie{
			Name:     "uplarr_session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   false, // Set to true if using HTTPS
			SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/logout", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("uplarr_session")
		if err == nil {
			sessionsMu.Lock()
			delete(sessions, cookie.Value)
			sessionsMu.Unlock()
		}
		// #nosec G124
		http.SetCookie(w, &http.Cookie{
			Name:     "uplarr_session",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
			SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		// Check auth
		authenticated := true
		if config.AuthPassword != "" {
			cookie, err := r.Cookie("uplarr_session")
			if err != nil {
				authenticated = false
			} else {
				sessionsMu.RLock()
				authenticated = sessions[cookie.Value]
				sessionsMu.RUnlock()
			}
		}

		var page string
		if authenticated {
			page = "static/index.html"
		} else {
			page = "static/login.html"
		}

		index, err := StaticAssetsReadFile(page)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(index)
	})

	mux.HandleFunc("/api/logs", withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		c := logger.Subscribe()
		defer logger.Unsubscribe(c)

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
	}))

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/files", withAuth(func(w http.ResponseWriter, r *http.Request) {
		relPath := filepath.Clean(r.URL.Query().Get("path"))
		if relPath == "." {
			relPath = ""
		}
		fullPath := filepath.Join(config.LocalDir, relPath)

		absLocalDir, err := FilepathAbs(config.LocalDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		absLocalDir, _ = FilepathEvalSymlinks(absLocalDir)
		
		absFullPath, err := FilepathAbs(fullPath)
		if err == nil {
			absFullPath, _ = FilepathEvalSymlinks(absFullPath)
		}

		if err != nil {
			relPath = ""
		} else {
			rel, err := filepath.Rel(absLocalDir, absFullPath)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				relPath = ""
			}
		}

		files, err := func() ([]os.DirEntry, error) {
			root, err := OsOpenRoot(absLocalDir)
			if err != nil {
				return nil, err
			}
			defer root.Close()

			// Compute rel path from evaluated root
			rel, err := filepath.Rel(absLocalDir, absFullPath)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				rel = "."
			}

			f, err := root.Open(rel)
			if err != nil {
				return nil, err
			}
			defer f.Close()

			return f.ReadDir(-1)
		}()

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var fileInfos []models.FileInfo
		for _, f := range files {
			info, err := f.Info()
			if err != nil {
				continue
			}
			fileInfos = append(fileInfos, models.FileInfo{
				Name:  f.Name(),
				Size:  info.Size(),
				IsDir: f.IsDir(),
			})
		}
		if fileInfos == nil {
			fileInfos = []models.FileInfo{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"current_path": relPath,
			"files":        fileInfos,
		})
	}))

	mux.HandleFunc("/api/files/action", withAuth(func(w http.ResponseWriter, r *http.Request) {
		var req models.FileActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		fullPath := filepath.Join(config.LocalDir, req.Path)
		// Security check
		absLocalDir, err := FilepathAbs(config.LocalDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		absLocalDir, _ = FilepathEvalSymlinks(absLocalDir)

		absPath, err := FilepathAbs(fullPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// EvalSymlinks might fail if path doesn't exist yet (mkdir), so we check error
		if evalPath, err := FilepathEvalSymlinks(absPath); err == nil {
			absPath = evalPath
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

		// Security check: use OsOpenRoot for hardware-level containment
		root, err := OsOpenRoot(absLocalDir)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer root.Close()

		var errAct error
		switch req.Action {
		case "delete":
			errAct = root.RemoveAll(rel)
		case "rename":
			if req.NewName == "" || req.NewName == "." || req.NewName == ".." || filepath.Base(req.NewName) != req.NewName {
				http.Error(w, "Invalid new name", http.StatusBadRequest)
				return
			}
			newPath := filepath.Join(filepath.Dir(absPath), req.NewName)
			newPath, err = FilepathAbs(newPath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			newRel, err := filepath.Rel(absLocalDir, newPath)
			if err != nil || newRel == ".." || strings.HasPrefix(newRel, ".."+string(filepath.Separator)) {
				http.Error(w, "Invalid target name", http.StatusBadRequest)
				return
			}
			errAct = root.Rename(rel, newRel)
		case "mkdir":
			errAct = root.MkdirAll(rel, 0750)
		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}

		if errAct != nil {
			http.Error(w, errAct.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	mux.HandleFunc("/api/test-connection", withAuth(func(w http.ResponseWriter, r *http.Request) {
		var req models.UploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		client := NewSFTPClient(req)

		if err := client.Connect(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		defer client.Close()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Connection successful"})
	}))

	mux.HandleFunc("/api/remote/files", withAuth(func(w http.ResponseWriter, r *http.Request) {
		var req models.UploadRequest
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Bad request: " + err.Error()})
			return
		}

		client := NewSFTPClient(req)

		if err := client.Connect(); err != nil {
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
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"current_path": remotePath,
			"files":        files,
		})
	}))

	mux.HandleFunc("/api/remote/files/action", withAuth(func(w http.ResponseWriter, r *http.Request) {
		var req models.FileActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		client := NewSFTPClient(req.Config)

		if err := client.Connect(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		defer client.Close()

		// Remote security check: ensure req.Path is within RemoteDir
		// Use path (POSIX) package for SFTP paths which always use forward slashes
		normalizedBase := pathpkg.Clean(strings.ReplaceAll(client.GetRemoteDir(), "\\", "/"))
		normalizedTarget := pathpkg.Clean(strings.ReplaceAll(req.Path, "\\", "/"))
		rel, err := filepath.Rel(normalizedBase, normalizedTarget)
		normalizedRel := strings.ReplaceAll(rel, "\\", "/")
		if err != nil || normalizedRel == ".." || strings.HasPrefix(normalizedRel, "../") {
			http.Error(w, "Unauthorized remote path", http.StatusUnauthorized)
			return
		}

		var errAct error
		switch req.Action {
		case "delete":
			errAct = client.Remove(req.Path)
		case "rename":
			if req.NewName == "" || req.NewName == "." || req.NewName == ".." || strings.Contains(req.NewName, "/") {
				http.Error(w, "Invalid new name", http.StatusBadRequest)
				return
			}
			newPath := filepath.ToSlash(filepath.Join(filepath.Dir(req.Path), req.NewName))
			errAct = client.Rename(req.Path, newPath)
		case "mkdir":
			errAct = client.Mkdir(req.Path)
		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}

		if errAct != nil {
			http.Error(w, errAct.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	mux.HandleFunc("/api/upload", withAuth(func(w http.ResponseWriter, r *http.Request) {
		var req models.UploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Validate file paths exist before enqueuing and ensure they stay
		// within LocalDir (fixes CodeQL: uncontrolled data in path expression)
		baseDirAbs, err := filepath.Abs(config.LocalDir)
		if err != nil {
			http.Error(w, "Server configuration error", http.StatusInternalServerError)
			return
		}

		var invalidFiles []string
		var validFiles []string
		for _, file := range req.Files {
			fullPath := filepath.Join(baseDirAbs, file)
			fullPathAbs, err := filepath.Abs(fullPath)
			if err != nil {
				invalidFiles = append(invalidFiles, file)
				continue
			}

			rel, err := filepath.Rel(baseDirAbs, fullPathAbs)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				invalidFiles = append(invalidFiles, file)
				continue
			}

			if _, err := os.Stat(fullPathAbs); err != nil {
				invalidFiles = append(invalidFiles, file)
				continue
			}

			validFiles = append(validFiles, file)
		}
		if len(invalidFiles) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":         "Some files are invalid or do not exist",
				"invalid_files": invalidFiles,
			})
			return
		}

		for _, file := range validFiles {
			// Update the host-wide limiter with the latest config provided with this upload request
			qm.UpdateHostLimiter(req.Host, req.RateLimitKBps, req.MinLimitKBps, req.MaxLatencyMs)
			qm.AddTask(file, req)
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Tasks added to queue"})
	}))

	mux.HandleFunc("/api/throttle/update", withAuth(func(w http.ResponseWriter, r *http.Request) {
		var req models.UploadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		qm.UpdateHostLimiter(req.Host, req.RateLimitKBps, req.MinLimitKBps, req.MaxLatencyMs)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Throttling updated"})
	}))

	mux.HandleFunc("/api/queue", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
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
		} else {
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/stats", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(qm.GetHostStats())
	}))
	return mux, nil
}
