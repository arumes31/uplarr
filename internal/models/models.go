package models

import "time"

type Config struct {
	LocalDir     string
	ConfigDir    string
	WebPort      string
	AuthPassword string
	TrustProxy   bool
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
	MinLimitKBps            int      `json:"min_limit_kbps"`
	ConcurrentFiles         int      `json:"concurrent_files"`
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
	ID              string        `json:"id"`
	FileName        string        `json:"file_name"`
	RemoteDir       string        `json:"remote_dir"`
	Status          TaskStatus    `json:"status"`
	Progress        int           `json:"progress"`
	BytesUploaded   int64         `json:"bytes_uploaded"`
	TotalBytes      int64         `json:"total_bytes"`
	StartedAt       *time.Time    `json:"started_at,omitempty"`
	Error           string        `json:"error,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	LocalFileExists bool          `json:"local_file_exists"`
	Config          UploadRequest `json:"-"`
}

type HostStats struct {
	Host           string  `json:"host"`
	LastLatencyMs  int64   `json:"last_latency_ms"`
	CurrentLimitKB int     `json:"current_limit_kb"`
	MaxLimitKB     int     `json:"max_limit_kb"`
	ActiveTasks    int     `json:"active_tasks"`
	TotalSpeedKBps float64 `json:"total_speed_kbps"`
}
