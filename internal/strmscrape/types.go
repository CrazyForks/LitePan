package strmscrape

const (
	WriteModeMissingOnly = "missing_only"
	WriteModeOverwrite   = "overwrite"

	ItemStatusOK    = "ok"
	ItemStatusMiss  = "miss"
	ItemStatusDoubt = "doubt"

	MediaTypeMovie = "movie"
	MediaTypeTV    = "tv"
)

// Item 表示一部作品（电影文件夹 / 剧集根目录），不是单个 .strm。
type Item struct {
	ID         string `json:"id"`
	RelDir     string `json:"rel_dir"`
	StrmName   string `json:"strm_name,omitempty"`
	Title      string `json:"title"`
	Year       *int   `json:"year,omitempty"`
	MediaType  string `json:"media_type"`
	Status     string `json:"status"`
	HasNFO     bool   `json:"has_nfo"`
	HasPoster  bool   `json:"has_poster"`
	HasPending bool   `json:"has_pending"`
	TMDBID     string `json:"tmdb_id,omitempty"`
	PosterURL  string `json:"poster_url,omitempty"`
	FolderName string `json:"folder_name,omitempty"`
	FileCount  int    `json:"file_count"`
	EpLocal    int    `json:"ep_local,omitempty"`
	EpTMDB     int    `json:"ep_tmdb,omitempty"`
	EpScraped  int    `json:"ep_scraped,omitempty"`
	TVState    string `json:"tv_state,omitempty"` // ended|updating
	AddedAt    string `json:"added_at,omitempty"`
}

type Progress struct {
	Running       bool   `json:"running"`
	TaskID        int64  `json:"strm_task_id"`
	Total         int    `json:"total"`
	Done          int    `json:"done"`
	Skipped       int    `json:"skipped"`
	Failed        int    `json:"failed"`
	Message       string `json:"message"`
	Error         string `json:"error,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	CurrentItemID string `json:"current_item_id"`
	ItemRevision  int    `json:"item_revision"`
	UpdatedItem   *Item  `json:"updated_item,omitempty"`
}

type Settings struct {
	WriteMode string `json:"write_mode"`

	TmdbAPIKey            string `json:"tmdb_api_key"`
	TmdbLanguage          string `json:"tmdb_language"`
	TmdbRequestIntervalMS int    `json:"tmdb_request_interval_ms"`
	ProxyEnabled          bool   `json:"proxy_enabled"`
	ProxyURL              string `json:"proxy_url"`
	ProxyUsername         string `json:"proxy_username"`
	ProxyPassword         string `json:"proxy_password"`
}

type RunRequest struct {
	StrmTaskID int64  `json:"strm_task_id"`
	WriteMode  string `json:"write_mode,omitempty"`
}

type RematchRequest struct {
	StrmTaskID int64  `json:"strm_task_id"`
	ItemID     string `json:"item_id"`
	TMDBID     string `json:"tmdb_id"`
	MediaType  string `json:"media_type"`
	Title      string `json:"title,omitempty"`
	Year       *int   `json:"year,omitempty"`
}

type MarkNormalRequest struct {
	StrmTaskID int64  `json:"strm_task_id"`
	ItemID     string `json:"item_id"`
}

type RescrapeRequest struct {
	StrmTaskID int64  `json:"strm_task_id"`
	ItemID     string `json:"item_id"`
}
