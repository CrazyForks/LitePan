package offlinedownload

import "litepan/internal/driver"

const (
	SourceURL     = "url"
	SourceTorrent = "bt"
)

type Capabilities struct {
	Supported         bool     `json:"supported"`
	SupportsURLs      bool     `json:"supports_urls"`
	SupportsBatchURLs bool     `json:"supports_batch_urls"`
	SupportsTorrent   bool     `json:"supports_torrent"`
	URLSchemes        []string `json:"url_schemes"`
	RootTargetAllowed bool     `json:"root_target_allowed"`
	RemoteDelete      bool     `json:"remote_delete"`
}

type Task struct {
	TaskID            string  `json:"task_id"`
	AccountID         int64   `json:"account_id"`
	AccountName       string  `json:"account_name"`
	DriverType        string  `json:"driver_type"`
	SourceKind        string  `json:"source_kind"`
	Source            string  `json:"source"`
	Name              string  `json:"name"`
	ProviderTaskID    string  `json:"provider_task_id,omitempty"`
	InfoHash          string  `json:"info_hash,omitempty"`
	TargetParentID    string  `json:"target_parent_id"`
	TargetDisplayPath string  `json:"target_display_path"`
	Status            string  `json:"status"`
	Progress          int     `json:"progress"`
	Size              int64   `json:"size"`
	FileID            string  `json:"file_id,omitempty"`
	Message           string  `json:"message"`
	Error             string  `json:"error,omitempty"`
	RemoteDelete      bool    `json:"remote_delete"`
	CreatedAt         float64 `json:"created_at"`
	UpdatedAt         float64 `json:"updated_at"`
}

type AddURLParams struct {
	AccountID         int64
	URLs              []string
	FileName          string
	TargetParentID    string
	TargetDisplayPath string
}

type TorrentPreparation struct {
	PreparationID string                      `json:"preparation_id"`
	TorrentName   string                      `json:"torrent_name"`
	TotalSize     int64                       `json:"total_size"`
	Files         []driver.OfflineTorrentFile `json:"files"`
	ExpiresAt     float64                     `json:"expires_at"`
}

type AddTorrentParams struct {
	AccountID         int64
	PreparationID     string
	Wanted            []int
	TargetParentID    string
	TargetDisplayPath string
	SavePath          string
}

type BatchDeleteResult struct {
	DeletedTaskIDs []string          `json:"deleted_task_ids"`
	FailedTaskIDs  []string          `json:"failed_task_ids"`
	FailedMessages map[string]string `json:"failed_messages"`
}
