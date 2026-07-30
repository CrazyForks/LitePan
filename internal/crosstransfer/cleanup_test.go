package crosstransfer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"litepan/internal/core/driverexec"
	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/file"
)

func TestRemovableCreatedRoots(t *testing.T) {
	created := []createdTargetDir{
		{ID: "a", ParentID: "root", RelDir: "A"},
		{ID: "b", ParentID: "a", RelDir: "A/B"},
		{ID: "c", ParentID: "a", RelDir: "A/C"},
	}

	allUnused := removableCreatedRoots(created, nil)
	if len(allUnused) != 1 || allUnused[0].ID != "a" {
		t.Fatalf("全部未命中时应只删除最上层目录，得到 %#v", allUnused)
	}

	kept := map[string]struct{}{}
	markKeptDir(kept, "A/B")
	partlyUsed := removableCreatedRoots(created, kept)
	if len(partlyUsed) != 1 || partlyUsed[0].ID != "c" {
		t.Fatalf("部分命中时应保留成功分支，只删除未使用分支，得到 %#v", partlyUsed)
	}
}

func TestExecuteCleansCreatedDirsWhenStreamStops(t *testing.T) {
	drv := newCleanupDriver()
	exec := driverexec.New(cleanupProvider{drv: drv}, nil)
	files := file.NewService(exec, nil, nil, nil, nil, nil)
	service := New(Options{Exec: exec, Files: files, DataDir: t.TempDir()})

	err := service.ExecuteStream(context.Background(), ExecuteInput{
		TargetAccountID: 1,
		TargetParentID:  "root",
		MethodID:        "md5",
		Files: []TransferFile{{
			RelPath: "A/B/miss.bin",
			RelDir:  "A/B",
			Name:    "miss.bin",
			Size:    1,
			Hash:    "00000000000000000000000000000000",
		}},
	}, func(event StreamEvent) error {
		if event["event"] == "item" {
			return errors.New("连接中断")
		}
		return nil
	})
	if err == nil {
		t.Fatal("模拟流中断应返回错误")
	}
	if items, listErr := drv.ListFiles(context.Background(), "root"); listErr != nil || len(items) != 0 {
		t.Fatalf("流中断后不应残留本次创建的目录，items=%#v err=%v", items, listErr)
	}
}

func TestProbeUsesDriverPrecheckWithoutTempFile(t *testing.T) {
	drv := &probeOnlyDriver{cleanupDriver: newCleanupDriver()}
	service := newCleanupService(t, drv)
	var events []StreamEvent

	err := service.ProbeStream(context.Background(), 1, 1, "root", "md5", []TransferFile{
		{RelPath: "hit.bin", Name: "hit.bin", Size: 1, Hash: "11111111111111111111111111111111"},
		{RelPath: "miss.bin", Name: "miss.bin", Size: 1, Hash: "22222222222222222222222222222222"},
	}, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("试探失败: %v", err)
	}
	if drv.probeCalls != 2 || drv.uploadCalls != 0 || drv.nextID != 0 {
		t.Fatalf("预判驱动不应创建临时目录或真实秒传，probe=%d upload=%d dirs=%d", drv.probeCalls, drv.uploadCalls, drv.nextID)
	}
	end := events[len(events)-1]
	if end["event"] != "end" || end["ok"] != 1 || end["no"] != 1 {
		t.Fatalf("试探汇总不正确: %#v", end)
	}
}

func TestProbeStopsAfterTerminalDriverError(t *testing.T) {
	drv := &probeOnlyDriver{cleanupDriver: newCleanupDriver(), terminal: true}
	service := newCleanupService(t, drv)

	err := service.ProbeStream(context.Background(), 1, 1, "root", "md5", []TransferFile{
		{RelPath: "a.bin", Name: "a.bin", Size: 1, Hash: "11111111111111111111111111111111"},
		{RelPath: "b.bin", Name: "b.bin", Size: 1, Hash: "22222222222222222222222222222222"},
	}, func(StreamEvent) error { return nil })
	if err == nil || !driver.IsRapidProbeTerminal(err) {
		t.Fatalf("应返回终止试探错误，得到 %v", err)
	}
	if drv.probeCalls != 1 {
		t.Fatalf("账号级错误后不应继续逐文件试探，调用次数=%d", drv.probeCalls)
	}
}

func TestProbeFallsBackToTemporaryRapidUpload(t *testing.T) {
	drv := &rapidOnlyDriver{cleanupDriver: newCleanupDriver()}
	service := newCleanupService(t, drv)

	err := service.ProbeStream(context.Background(), 1, 1, "root", "md5", []TransferFile{{
		RelPath: "a.bin", Name: "a.bin", Size: 1, Hash: "11111111111111111111111111111111",
	}}, func(StreamEvent) error { return nil })
	if err != nil {
		t.Fatalf("试探失败: %v", err)
	}
	if drv.uploadCalls != 1 || drv.nextID != 1 {
		t.Fatalf("不支持预判时应创建临时目录真实试传，upload=%d dirs=%d", drv.uploadCalls, drv.nextID)
	}
	if items, listErr := drv.ListFiles(context.Background(), "root"); listErr != nil || len(items) != 0 {
		t.Fatalf("临时探测目录应清理，items=%#v err=%v", items, listErr)
	}
}

func TestScanSourceDeepTreeDoesNotDeadlock(t *testing.T) {
	drv := newCleanupDriver()
	parentID := ""
	for i := 0; i < 12; i++ {
		childID := fmt.Sprintf("level-%d", i)
		drv.children[parentID] = []domain.FileItem{{ID: childID, Name: childID, IsDir: true}}
		parentID = childID
	}
	drv.children[parentID] = []domain.FileItem{{
		ID: "file-1", Name: "song.flac", Size: 1,
		Hash: map[domain.HashType]string{domain.HashMD5: "11111111111111111111111111111111"},
	}}
	service := newCleanupService(t, drv)
	done := make(chan struct{})
	var result *ScanResult
	var scanErr error
	var progressEvents int

	go func() {
		scanErr = service.ScanSourceStream(context.Background(), 1, "root", "md5", "/music", func(event StreamEvent) error {
			if event["event"] == "progress" {
				progressEvents++
			}
			if event["event"] == "end" {
				result, _ = event["result"].(*ScanResult)
			}
			return nil
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("深层目录扫描发生阻塞")
	}
	if scanErr != nil {
		t.Fatalf("扫描失败: %v", scanErr)
	}
	if result == nil || result.Total != 1 || result.Directories != 13 || result.Truncated {
		t.Fatalf("扫描结果不正确: %#v", result)
	}
	if progressEvents == 0 {
		t.Fatal("扫描过程应持续返回进度")
	}
}

func TestScanSourceReturnsDirectoryError(t *testing.T) {
	drv := newCleanupDriver()
	drv.children[""] = []domain.FileItem{{ID: "broken", Name: "损坏目录", IsDir: true}}
	drv.listErrors["broken"] = errors.New("上游列表失败")
	service := newCleanupService(t, drv)

	_, err := service.ScanSource(context.Background(), 1, "root", "md5", "/媒体")
	if err == nil || !strings.Contains(err.Error(), "/媒体/损坏目录") {
		t.Fatalf("目录错误应带路径返回，得到 %v", err)
	}
}

func TestScanSourceMarksIncompleteResult(t *testing.T) {
	drv := newCleanupDriver()
	items := make([]domain.FileItem, 0, maxScanFiles+1)
	for i := 0; i <= maxScanFiles; i++ {
		items = append(items, domain.FileItem{
			ID: fmt.Sprintf("file-%d", i), Name: fmt.Sprintf("%d.bin", i), Size: 1,
			Hash: map[domain.HashType]string{domain.HashMD5: "11111111111111111111111111111111"},
		})
	}
	drv.children[""] = items
	service := newCleanupService(t, drv)

	result, err := service.ScanSource(context.Background(), 1, "root", "md5", "/大目录")
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if !result.Truncated || result.Total != maxScanFiles || result.TruncatedReason == "" {
		t.Fatalf("超限扫描必须标记为不完整: %#v", result)
	}
}

func newCleanupService(t *testing.T, drv driver.Driver) *Service {
	t.Helper()
	exec := driverexec.New(cleanupProvider{drv: drv}, nil)
	files := file.NewService(exec, nil, nil, nil, nil, nil)
	return New(Options{Exec: exec, Files: files, DataDir: t.TempDir()})
}

type cleanupProvider struct{ drv driver.Driver }

func (p cleanupProvider) Get(context.Context, int64) (driver.Driver, error) { return p.drv, nil }

type cleanupDriver struct {
	nextID     int
	children   map[string][]domain.FileItem
	parents    map[string]string
	listErrors map[string]error
}

func newCleanupDriver() *cleanupDriver {
	return &cleanupDriver{
		children:   map[string][]domain.FileItem{},
		parents:    map[string]string{},
		listErrors: map[string]error{},
	}
}

func (*cleanupDriver) Config() driver.Config      { return driver.Config{Name: "cleanup"} }
func (*cleanupDriver) GetAddition() any           { return &struct{}{} }
func (*cleanupDriver) Init(context.Context) error { return nil }
func (*cleanupDriver) Drop(context.Context) error { return nil }
func (*cleanupDriver) Ping(context.Context) error { return nil }

func (d *cleanupDriver) ListFiles(_ context.Context, parentID string) ([]domain.FileItem, error) {
	if err := d.listErrors[parentID]; err != nil {
		return nil, err
	}
	return append([]domain.FileItem(nil), d.children[parentID]...), nil
}

func (d *cleanupDriver) CreateFolder(_ context.Context, parentID, name string) (*domain.FileItem, error) {
	d.nextID++
	item := domain.FileItem{ID: fmt.Sprintf("dir-%d", d.nextID), Name: name, IsDir: true}
	d.children[parentID] = append(d.children[parentID], item)
	d.parents[item.ID] = parentID
	return &item, nil
}

func (*cleanupDriver) RapidUploadByHash(context.Context, driver.RapidUploadRequest) (*driver.RapidUploadResult, error) {
	return &driver.RapidUploadResult{Reuse: false}, nil
}

func (d *cleanupDriver) DeleteFiles(_ context.Context, ids []string) error {
	for _, id := range ids {
		parentID := d.parents[id]
		items := d.children[parentID]
		for index := range items {
			if items[index].ID == id {
				d.children[parentID] = append(items[:index], items[index+1:]...)
				break
			}
		}
		d.deleteTree(id)
	}
	return nil
}

func (d *cleanupDriver) deleteTree(id string) {
	for _, child := range d.children[id] {
		if child.IsDir {
			d.deleteTree(child.ID)
		}
		delete(d.parents, child.ID)
	}
	delete(d.children, id)
	delete(d.parents, id)
}

type probeOnlyDriver struct {
	*cleanupDriver
	probeCalls  int
	uploadCalls int
	terminal    bool
}

func (d *probeOnlyDriver) ProbeRapidUploadByHash(_ context.Context, req driver.RapidUploadRequest) (*driver.RapidUploadResult, error) {
	d.probeCalls++
	if d.terminal {
		return nil, driver.StopRapidProbe(domain.Errorf(domain.CodeRateLimited, "今日额度已用尽"))
	}
	return &driver.RapidUploadResult{Reuse: req.FileName == "hit.bin"}, nil
}

func (*probeOnlyDriver) SupportsRapidUploadProbe(method string) bool { return method == "md5" }

func (d *probeOnlyDriver) RapidUploadByHash(context.Context, driver.RapidUploadRequest) (*driver.RapidUploadResult, error) {
	d.uploadCalls++
	return &driver.RapidUploadResult{Reuse: false}, nil
}

type rapidOnlyDriver struct {
	*cleanupDriver
	uploadCalls int
}

func (d *rapidOnlyDriver) RapidUploadByHash(context.Context, driver.RapidUploadRequest) (*driver.RapidUploadResult, error) {
	d.uploadCalls++
	return &driver.RapidUploadResult{Reuse: false}, nil
}
