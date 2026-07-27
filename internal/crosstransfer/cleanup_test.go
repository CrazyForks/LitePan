package crosstransfer

import (
	"context"
	"errors"
	"fmt"
	"testing"

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

func newCleanupService(t *testing.T, drv driver.Driver) *Service {
	t.Helper()
	exec := driverexec.New(cleanupProvider{drv: drv}, nil)
	files := file.NewService(exec, nil, nil, nil, nil, nil)
	return New(Options{Exec: exec, Files: files, DataDir: t.TempDir()})
}

type cleanupProvider struct{ drv driver.Driver }

func (p cleanupProvider) Get(context.Context, int64) (driver.Driver, error) { return p.drv, nil }

type cleanupDriver struct {
	nextID   int
	children map[string][]domain.FileItem
	parents  map[string]string
}

func newCleanupDriver() *cleanupDriver {
	return &cleanupDriver{children: map[string][]domain.FileItem{}, parents: map[string]string{}}
}

func (*cleanupDriver) Config() driver.Config      { return driver.Config{Name: "cleanup"} }
func (*cleanupDriver) GetAddition() any           { return &struct{}{} }
func (*cleanupDriver) Init(context.Context) error { return nil }
func (*cleanupDriver) Drop(context.Context) error { return nil }
func (*cleanupDriver) Ping(context.Context) error { return nil }

func (d *cleanupDriver) ListFiles(_ context.Context, parentID string) ([]domain.FileItem, error) {
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
