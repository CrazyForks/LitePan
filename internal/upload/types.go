package upload

import (
	"context"
	"time"

	"litepan/pkg/speedsmoother"
)

const (
	progressInterval = 250 * time.Millisecond
	defaultLimit     = 3
)

// Task 是对外暴露的上传任务快照。
type Task struct {
	TaskID              string         `json:"task_id"`
	ClientTaskID        string         `json:"client_task_id,omitempty"`
	AccountID           int64          `json:"account_id"`
	AccountName         string         `json:"account_name"`
	DriverType          string         `json:"driver_type"`
	FileName            string         `json:"file_name"`
	TargetPath          string         `json:"target_path"`
	TargetDisplayPath   string         `json:"target_display_path,omitempty"`
	Status              string         `json:"status"`
	Progress            int            `json:"progress"`
	UploadedBytes       int64          `json:"uploaded_bytes"`
	SpeedBytesPerSecond float64        `json:"speed_bytes_per_second"`
	TotalBytes          int64          `json:"total_bytes"`
	Message             string         `json:"message"`
	Error               string         `json:"error,omitempty"`
	Result              map[string]any `json:"result,omitempty"`
	QueueOrder          int            `json:"queue_order"`
	CreatedAt           float64        `json:"created_at"`
	UpdatedAt           float64        `json:"updated_at"`
}

const (
	StatusPending  = "pending"
	StatusRunning  = "running"
	StatusPaused   = "paused"
	StatusSuccess  = "success"
	StatusFailed   = "failed"
	StatusCanceled = "canceled"
	StatusSkipped  = "skipped"
)

func unixFloat(t time.Time) float64 {
	return float64(t.UnixNano()) / 1e9
}

type taskState struct {
	Task
	localPath      string
	conflictPolicy string
	cancel         context.CancelFunc
	cancelMode     string
	runDone        chan struct{}
	resumeData     map[string]any
	lastEmit       time.Time
	lastProgress   int
	lastMessage    string
	speed          speedsmoother.Tracker
}

type CreateParams struct {
	ClientTaskID      string
	AccountID         int64
	FileName          string
	DisplayName       string
	TargetPath        string
	TargetDisplayPath string
	LocalPath         string
	TotalBytes        int64
	ConflictPolicy    string
}

type BatchDeleteResult struct {
	DeletedTaskIDs []string          `json:"deleted_task_ids"`
	FailedTaskIDs  []string          `json:"failed_task_ids"`
	MissingTaskIDs []string          `json:"missing_task_ids"`
	FailedMessages map[string]string `json:"failed_messages"`
}
