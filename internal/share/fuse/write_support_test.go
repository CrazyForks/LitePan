//go:build fuse

package fuse

import (
	"context"
	"os"
	"syscall"
	"testing"

	"litepan/internal/upload"
)

type recordingUploadManager struct {
	tempDir string
	params  []upload.CreateParams
}

func (m *recordingUploadManager) Create(_ context.Context, params upload.CreateParams) (*upload.Task, error) {
	m.params = append(m.params, params)
	return &upload.Task{TaskID: "empty-upload"}, nil
}

func (m *recordingUploadManager) TempDir() string { return m.tempDir }

func (m *recordingUploadManager) TempRegistry() *upload.TempRegistry { return nil }

func TestStagingUploadFlushQueuesEmptyFile(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "empty-*")
	if err != nil {
		t.Fatalf("创建空文件失败: %v", err)
	}
	manager := &recordingUploadManager{tempDir: t.TempDir()}
	handle := &stagingUploadHandle{
		node:      &stagingFile{},
		uploads:   manager,
		accountID: 1,
		parentID:  "root",
		fileName:  "empty.txt",
		tempPath:  tmp.Name(),
		file:      tmp,
		flags:     syscall.O_WRONLY,
	}
	t.Cleanup(func() { _ = handle.Release(context.Background()) })

	if errno := handle.Flush(context.Background()); errno != 0 {
		t.Fatalf("空文件 Flush 不应失败，errno=%v", errno)
	}
	if len(manager.params) != 1 {
		t.Fatalf("期望创建一个上传任务，实际 %d", len(manager.params))
	}
	if manager.params[0].TotalBytes != 0 {
		t.Fatalf("空文件任务大小应为 0，实际 %d", manager.params[0].TotalBytes)
	}
}
