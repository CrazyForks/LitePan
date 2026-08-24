package spacecleanup

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"litepan/internal/domain"
)

type strmTaskRepoStub struct {
	mu    sync.Mutex
	tasks []*domain.StrmTask
}

func (r *strmTaskRepoStub) Create(context.Context, *domain.StrmTask) (int64, error) { return 0, nil }
func (r *strmTaskRepoStub) Update(context.Context, *domain.StrmTask) error          { return nil }
func (r *strmTaskRepoStub) Delete(context.Context, int64) error                     { return nil }
func (r *strmTaskRepoStub) UpdateScan(context.Context, int64, domain.StrmScanPatch) error {
	return nil
}
func (r *strmTaskRepoStub) ListByAccount(context.Context, int64) ([]*domain.StrmTask, error) {
	return r.List(context.Background())
}
func (r *strmTaskRepoStub) Get(_ context.Context, id int64) (*domain.StrmTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, task := range r.tasks {
		if task != nil && task.ID == id {
			copy := *task
			return &copy, nil
		}
	}
	return nil, domain.Errorf(domain.CodeNotFound, "STRM 任务不存在")
}
func (r *strmTaskRepoStub) List(context.Context) ([]*domain.StrmTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*domain.StrmTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task == nil {
			continue
		}
		copy := *task
		out = append(out, &copy)
	}
	return out, nil
}
func (r *strmTaskRepoStub) set(tasks ...*domain.StrmTask) {
	r.mu.Lock()
	r.tasks = tasks
	r.mu.Unlock()
}

func TestScanProtectsActiveStrmAndClassifiesOrphans(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	strmDir := filepath.Join(root, "strm")
	activeDir := filepath.Join(strmDir, "电影", "现有任务")
	orphanDir := filepath.Join(strmDir, "电影", "旧任务")
	emptyDir := filepath.Join(strmDir, "空分组")
	pendingDir := filepath.Join(strmDir, "待刮削")
	for _, dir := range []string{activeDir, orphanDir, emptyDir, pendingDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(activeDir, "a.strm"), "active")
	writeTestFile(t, filepath.Join(activeDir, ".DS_Store"), "junk")
	writeTestFile(t, filepath.Join(orphanDir, "b.strm"), "orphan")
	writeTestFile(t, filepath.Join(pendingDir, ".litepan-scrape-pending"), "pending")

	repo := &strmTaskRepoStub{tasks: []*domain.StrmTask{{ID: 1, GroupDir: "电影", OutputFolder: "现有任务"}}}
	service, err := New(Options{DataDir: dataDir, StrmDir: strmDir, StrmTasks: repo})
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	items := reportItems(report)
	if item, ok := findItemByPath(items, activeDir); ok {
		t.Fatalf("现有任务目录不应成为清理项：%+v", item)
	}
	if item, ok := findItemByPath(items, filepath.Join(activeDir, ".DS_Store")); !ok || !item.DefaultSelected {
		t.Fatalf("现有任务中的系统杂项应可安全清理：%+v", item)
	}
	if item, ok := findItemByPath(items, orphanDir); !ok || item.Risk != RiskReview || item.DefaultSelected {
		t.Fatalf("未关联且含媒体内容的目录应要求确认：%+v", item)
	}
	if item, ok := findItemByPath(items, emptyDir); !ok || item.Risk != RiskSafe || !item.DefaultSelected {
		t.Fatalf("空目录应默认选择：%+v", item)
	}
	if item, ok := findItemByPath(items, pendingDir); !ok || item.Risk != RiskReview || item.DefaultSelected {
		t.Fatalf("刮削待处理标记不得被当成系统垃圾或空目录：%+v", item)
	}
}

func TestScanProtectsActiveUploadTemp(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	strmDir := filepath.Join(root, "strm")
	tempDir := filepath.Join(dataDir, "upload_tasks")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(tempDir, "active.tmp")
	orphan := filepath.Join(tempDir, "orphan.tmp")
	writeTestFile(t, active, "active")
	writeTestFile(t, orphan, "orphan")

	service, err := New(Options{
		DataDir:           dataDir,
		StrmDir:           strmDir,
		StrmTasks:         &strmTaskRepoStub{},
		UploadActivePaths: func() []string { return []string{active} },
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	items := reportItems(report)
	if _, ok := findItemByPath(items, active); ok {
		t.Fatal("正在使用的上传临时文件不应出现在扫描结果中")
	}
	if item, ok := findItemByPath(items, orphan); !ok || !item.DefaultSelected || item.Risk != RiskSafe {
		t.Fatalf("无任务引用的上传临时文件应可安全清理：%+v", item)
	}
}

func TestCleanupRemovesSelectedSafeTempOnly(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	strmDir := filepath.Join(root, "strm")
	tempDir := filepath.Join(dataDir, "upload_tasks")
	selectedPath := filepath.Join(tempDir, "selected.tmp")
	untouchedPath := filepath.Join(tempDir, "untouched.tmp")
	writeTestFile(t, selectedPath, "selected")
	writeTestFile(t, untouchedPath, "untouched")

	service, err := New(Options{DataDir: dataDir, StrmDir: strmDir, StrmTasks: &strmTaskRepoStub{}})
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item, ok := findItemByPath(reportItems(report), selectedPath)
	if !ok {
		t.Fatal("预期扫描到无引用的上传临时文件")
	}
	result, err := service.Cleanup(context.Background(), CleanupRequest{ScanID: report.ScanID, ItemIDs: []string{item.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedItems != 1 || result.FailedItems != 0 || result.FreedBytes != int64(len("selected")) {
		t.Fatalf("清理结果不符合预期：%+v", result)
	}
	if _, err := os.Stat(selectedPath); !os.IsNotExist(err) {
		t.Fatalf("选中的临时文件应被删除：%v", err)
	}
	if _, err := os.Stat(untouchedPath); err != nil {
		t.Fatalf("未选中的临时文件不应受影响：%v", err)
	}
}

func TestLogsKeepTodayAndCleanEarlierDays(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	strmDir := filepath.Join(root, "strm")
	logDir := filepath.Join(dataDir, "log")
	today := time.Now().Local().Format("2006-01-02") + ".log"
	yesterday := time.Now().Local().AddDate(0, 0, -1).Format("2006-01-02") + ".log"
	writeTestFile(t, filepath.Join(logDir, today), "today")
	writeTestFile(t, filepath.Join(logDir, yesterday), "yesterday")

	service, err := New(Options{DataDir: dataDir, StrmDir: strmDir, StrmTasks: &strmTaskRepoStub{}})
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	items := reportItems(report)
	if _, ok := findItemByPath(items, filepath.Join(logDir, today)); ok {
		t.Fatal("今天的日志不应出现在清理结果中")
	}
	oldItem, ok := findItemByPath(items, filepath.Join(logDir, yesterday))
	if !ok || oldItem.Name != "历史日志" || !oldItem.DefaultSelected {
		t.Fatalf("今天之前的日志应默认可清理：%+v", oldItem)
	}
	result, err := service.Cleanup(context.Background(), CleanupRequest{ScanID: report.ScanID, ItemIDs: []string{oldItem.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if result.CleanedItems != 1 {
		t.Fatalf("历史日志应清理成功：%+v", result)
	}
	if _, err := os.Stat(filepath.Join(logDir, today)); err != nil {
		t.Fatalf("今天的日志必须保留：%v", err)
	}
}

func TestCleanupRechecksStrmTaskBeforeDeleting(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	strmDir := filepath.Join(root, "strm")
	target := filepath.Join(strmDir, "刚创建的任务")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(target, "a.strm"), "content")

	repo := &strmTaskRepoStub{}
	service, err := New(Options{DataDir: dataDir, StrmDir: strmDir, StrmTasks: repo})
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item, ok := findItemByPath(reportItems(report), target)
	if !ok {
		t.Fatal("预期扫描到未关联 STRM 目录")
	}

	repo.set(&domain.StrmTask{ID: 2, OutputFolder: "刚创建的任务"})
	cleaned, err := service.Cleanup(context.Background(), CleanupRequest{ScanID: report.ScanID, ItemIDs: []string{item.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if cleaned.SkippedItems != 1 || cleaned.CleanedItems != 0 {
		t.Fatalf("任务在扫描后占用目录时应跳过删除：%+v", cleaned)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("已被任务使用的目录不应删除：%v", err)
	}
}

func TestScanPlansAreBounded(t *testing.T) {
	root := t.TempDir()
	service, err := New(Options{
		DataDir:   filepath.Join(root, "data"),
		StrmDir:   filepath.Join(root, "strm"),
		StrmTasks: &strmTaskRepoStub{},
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < maxScanPlans+3; index++ {
		if _, err := service.Scan(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.scans) != maxScanPlans {
		t.Fatalf("扫描计划应限制为 %d 份，实际为 %d", maxScanPlans, len(service.scans))
	}
}

func reportItems(report Report) []Item {
	var out []Item
	for _, group := range report.Groups {
		out = append(out, group.Items...)
	}
	return out
}

func findItemByPath(items []Item, path string) (Item, bool) {
	path = filepath.Clean(path)
	for _, item := range items {
		if filepath.Clean(item.Path) == path {
			return item, true
		}
	}
	return Item{}, false
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
