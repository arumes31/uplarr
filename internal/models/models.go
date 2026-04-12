package models

import "time"

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
	Action  string        `json:"action"`
	Path    string        `json:"path"`
	NewName string        `json:"new_name,omitempty"`
	Config  UploadRequest `json:"config,omitempty"`
}

type FileInfo struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

type TaskStatus string

const (
	TaskPending   TaskStatus = "Pending"
	TaskRunning   TaskStatus = "Running"
	TaskPaused    TaskStatus = "Paused"
	TaskCompleted TaskStatus = "Completed"
	TaskFailed    TaskStatus = "Failed"
)

type Task struct {
	ID        string        `json:"id"`
	FileName  string        `json:"file_name"`
	Status    TaskStatus    `json:"status"`
	Progress  int           `json:"progress"`
	Error     string        `json:"error,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	Config    UploadRequest `json:"-"`
}
