package fuse

import (
	"context"
	"log/slog"

	"litepan/internal/domain"
	"litepan/internal/file"
	"litepan/internal/playback"
	"litepan/internal/upload"
)

type Deps struct {
	Files     *file.Service
	Playback  *playback.Service
	Uploads   UploadManager
	Accounts  domain.AccountRepository
	ReadCache ReadCache
	Log       *slog.Logger
}

type ReadCache interface {
	Enabled(ctx context.Context) bool
	ReadAt(ctx context.Context, accountID int64, fileID string, dest []byte, off int64, fetch func([]byte, int64) (int, error)) (int, error)
}

type UploadManager interface {
	Create(ctx context.Context, p upload.CreateParams) (*upload.Task, error)
	TempDir() string
	TempRegistry() *upload.TempRegistry
}

type Manager interface {
	Mount(ctx context.Context, m *domain.FuseMount) error
	Unmount(ctx context.Context, id int64) error
}
