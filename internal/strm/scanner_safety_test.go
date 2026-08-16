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

func TestIsStrmUnderSkippedRootSkipProtectsWholeTaskTree(t *testing.T) {
	skipped := map[string]struct{}{"": {}}
	taskFolder := "任务"

	cases := []struct {
		name string
		rel  string
		want bool
	}{
		{name: "root file", rel: "任务/影片.strm", want: true},
		{name: "nested file", rel: "任务/电视剧/Season 1/第01集.strm", want: true},
		{name: "other task", rel: "别的任务/影片.strm", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isStrmUnderSkipped(tc.rel, taskFolder, skipped); got != tc.want {
				t.Fatalf("isStrmUnderSkipped(%q)=%v, want %v", tc.rel, got, tc.want)
			}
		})
	}
}

func TestCleanupMissingRemoteChildDirsCountsOnlyStrmFiles(t *testing.T) {
	root := t.TempDir()
	localDir := filepath.Join(root, "任务", "电视剧", "Season 1")
	if err := os.MkdirAll(filepath.Join(localDir, "extras"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "E01.strm"), []byte("https://example.test/E01"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "poster.jpg"), []byte("poster"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "extras", "E02.strm"), []byte("https://example.test/E02"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "extras", "note.txt"), []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, err := cleanupMissingRemoteChildDirs(root, "任务", map[string]map[string]struct{}{
		dirKey([]string{"电视剧"}): {},
	}, nil, nil)
	if err != nil {
		t.Fatalf("cleanupMissingRemoteChildDirs() error = %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2 strm files", removed)
	}
	if _, err := os.Stat(localDir); !os.IsNotExist(err) {
		t.Fatalf("远端已删除目录应被清理，stat err = %v", err)
	}
}

func TestLocalChildDirsWithStrmFindsNestedChildSubtrees(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "任务", "电视剧")
	if err := os.MkdirAll(filepath.Join(base, "Season 1", "extras"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "Season 2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "Season 1", "extras", "E01.strm"), []byte("https://example.test/E01"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "Season 2", "E01.strm"), []byte("https://example.test/S02E01"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "poster.strm"), []byte("https://example.test/poster"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := localChildDirsWithStrm(root, "任务", []string{"电视剧"})
	if len(got) != 2 {
		t.Fatalf("len(got)=%d, want 2", len(got))
	}
	if _, ok := got[SafeName("Season 1")]; !ok {
		t.Fatal("Season 1 应命中本地已有 STRM 的子树集合")
	}
	if _, ok := got[SafeName("Season 2")]; !ok {
		t.Fatal("Season 2 应命中本地已有 STRM 的子树集合")
	}
	if _, ok := got[SafeName("poster.strm")]; ok {
		t.Fatal("父目录直下的 STRM 文件不应被误判成子目录")
	}
}
