package strm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"litepan/internal/core/driverexec"
	"litepan/internal/domain"
	"litepan/internal/file"
)

func TestScanTaskKeepsLocalStrmWhenRemoteScanIsEmpty(t *testing.T) {
	root := t.TempDir()
	localFile := filepath.Join(root, "任务", "影片.strm")
	if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localFile, []byte("https://example.test/video"), 0o644); err != nil {
		t.Fatal(err)
	}

	drv := &metadataTestDriver{items: map[string][]domain.FileItem{"library": {}}}
	files := file.NewService(driverexec.New(metadataTestProvider{drv: drv}, nil), nil, nil, nil, nil, nil)
	task := &domain.StrmTask{
		ID:           1,
		AccountID:    1,
		ParentID:     "library",
		Recursive:    true,
		ScanMode:     domain.StrmScanModeIncrementalUpdate,
		Extensions:   "mkv",
		OutputFolder: "任务",
	}

	result, err := ScanTask(context.Background(), task, ScanDeps{Files: files, StrmDir: root}, domain.StrmRunModeFull)
	if err == nil || !strings.Contains(err.Error(), "为防止误删已停止清理") {
		t.Fatalf("空扫描应被安全拦截，实际错误=%v", err)
	}
	if result.RemovedCount != 0 {
		t.Fatalf("空扫描不应删除文件，删除数=%d", result.RemovedCount)
	}
	if _, err := os.Stat(localFile); err != nil {
		t.Fatalf("本地 STRM 应保留：%v", err)
	}
}

func TestValidateMonitorBranchesRejectsTaskRoot(t *testing.T) {
	task := &domain.StrmTask{Path: "/云影音"}
	broken := &domain.StrmBranch{
		ParentID:      "",
		Path:          "",
		RelativePath:  "",
		BranchType:    domain.StrmBranchTypeTemporary,
		RetentionDays: 90,
	}
	if err := validateMonitorBranches(task, []*domain.StrmBranch{broken}); err == nil {
		t.Fatal("指向任务根目录的临时监控分支应被拒绝")
	}

	valid := &domain.StrmBranch{
		ParentID:     "movie-id",
		Path:         "/云影音/电影",
		RelativePath: "电影",
		BranchType:   domain.StrmBranchTypeTemporary,
	}
	if err := validateMonitorBranches(task, []*domain.StrmBranch{valid}); err != nil {
		t.Fatalf("有效监控分支不应被拒绝：%v", err)
	}
}
