package domain

import "context"

// UploadTaskRecord 是上传任务持久化行（含运行时字段）。
type UploadTaskRecord struct {
	TaskID              string
	ClientTaskID        string
	AccountID           int64
	AccountName         string
	DriverType          string
	FileName            string
	TargetPath          string
	TargetDisplayPath   string
	Status              string
	Progress            int
	UploadedBytes       int64
	SpeedBytesPerSecond float64
	TotalBytes          int64
	Message             string
	Error               string
	ResultJSON          string
	ResumeDataJSON      string
	QueueOrder          int
	CreatedAt           float64
	UpdatedAt           float64
	LocalPath           string
	ConflictPolicy      string
}

// UploadTaskRepository 定义上传任务持久化端口。
type UploadTaskRepository interface {
	Upsert(ctx context.Context, rec *UploadTaskRecord) error
	Delete(ctx context.Context, taskID string) error
	List(ctx context.Context) ([]*UploadTaskRecord, error)
}
