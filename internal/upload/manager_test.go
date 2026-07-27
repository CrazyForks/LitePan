package upload

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"litepan/internal/core/driverexec"
	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/eventbus"
)

type fakeDeleterDriver struct{ deleted [][]string }

func (d *fakeDeleterDriver) Config() driver.Config      { return driver.Config{Name: "x"} }
func (d *fakeDeleterDriver) GetAddition() any           { return &struct{}{} }
func (d *fakeDeleterDriver) Init(context.Context) error { return nil }
func (d *fakeDeleterDriver) Drop(context.Context) error { return nil }
func (d *fakeDeleterDriver) Ping(context.Context) error { return nil }
func (d *fakeDeleterDriver) ListFiles(context.Context, string) ([]domain.FileItem, error) {
	return nil, nil
}
func (d *fakeDeleterDriver) DeleteFiles(_ context.Context, ids []string) error {
	d.deleted = append(d.deleted, ids)
	return nil
}

type fakeProvider struct{ drv driver.Driver }

func (p fakeProvider) Get(context.Context, int64) (driver.Driver, error) { return p.drv, nil }

type fakeUploadAccounts struct{}

func (fakeUploadAccounts) LookupUploadAccount(context.Context, int64) (string, string, error) {
	return "测试账号", "mock", nil
}

type blockingResumeDriver struct {
	calls         atomic.Int32
	firstStarted  chan struct{}
	firstCanceled chan struct{}
	releaseFirst  chan struct{}
	secondState   chan map[string]any
}

func (d *blockingResumeDriver) Config() driver.Config      { return driver.Config{Name: "mock"} }
func (d *blockingResumeDriver) GetAddition() any           { return &struct{}{} }
func (d *blockingResumeDriver) Init(context.Context) error { return nil }
func (d *blockingResumeDriver) Drop(context.Context) error { return nil }
func (d *blockingResumeDriver) Ping(context.Context) error { return nil }
func (d *blockingResumeDriver) ListFiles(context.Context, string) ([]domain.FileItem, error) {
	return nil, nil
}

func (d *blockingResumeDriver) UploadLocalFile(ctx context.Context, req driver.LocalUploadRequest) (*driver.LocalUploadResult, error) {
	if d.calls.Add(1) == 1 {
		req.OnResumeState(map[string]any{
			"completed_slices": []any{1},
			"uploaded_bytes":   int64(4),
			"progress":         25,
		})
		close(d.firstStarted)
		<-ctx.Done()
		close(d.firstCanceled)
		<-d.releaseFirst
		return nil, ctx.Err()
	}
	d.secondState <- cloneMap(req.ResumeState)
	return &driver.LocalUploadResult{
		FileID:   "uploaded",
		ParentID: req.ParentID,
		FileName: req.FileName,
		Size:     16,
		Message:  "上传成功",
	}, nil
}

type failingDeleterDriver struct{}

func (d *failingDeleterDriver) Config() driver.Config      { return driver.Config{Name: "x"} }
func (d *failingDeleterDriver) GetAddition() any           { return &struct{}{} }
func (d *failingDeleterDriver) Init(context.Context) error { return nil }
func (d *failingDeleterDriver) Drop(context.Context) error { return nil }
func (d *failingDeleterDriver) Ping(context.Context) error { return nil }
func (d *failingDeleterDriver) ListFiles(context.Context, string) ([]domain.FileItem, error) {
	return nil, nil
}
func (d *failingDeleterDriver) DeleteFiles(context.Context, []string) error {
	return errors.New("cloud delete failed")
}

// 勾选「同时删除网盘文件」删除成功后，应发 FileMutated 让对应目录缓存精准失效。
func TestDeleteUploadedFilePublishesMutation(t *testing.T) {
	bus := eventbus.New(nil)
	t.Cleanup(func() { _ = bus.Close(context.Background()) })
	got := make(chan eventbus.FileMutated, 1)
	eventbus.Subscribe(bus, func(_ context.Context, e eventbus.FileMutated) { got <- e })

	m := NewManager(Options{
		Exec:    driverexec.New(fakeProvider{drv: &fakeDeleterDriver{}}, nil),
		Bus:     bus,
		DataDir: t.TempDir(),
	})

	const id = "task1"
	m.mu.Lock()
	m.tasks[id] = &taskState{
		Task: Task{
			TaskID:     id,
			AccountID:  7,
			Status:     StatusSuccess,
			TargetPath: "dirX",
			Result:     map[string]any{"file_id": "f9", "parent_id": "dirX"},
		},
		runDone: make(chan struct{}),
	}
	m.mu.Unlock()

	found, err := m.Delete(context.Background(), id, true)
	if !found || err != nil {
		t.Fatalf("delete found=%v err=%v", found, err)
	}

	select {
	case e := <-got:
		if e.Op != "delete" || e.AccountID != 7 || e.ParentID != "dirX" || len(e.FileIDs) != 1 || e.FileIDs[0] != "f9" {
			t.Fatalf("unexpected event %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for delete event")
	}
}

func TestDeleteUploadedFileFailureKeepsTask(t *testing.T) {
	m := NewManager(Options{
		Exec:    driverexec.New(fakeProvider{drv: &failingDeleterDriver{}}, nil),
		DataDir: t.TempDir(),
	})

	const id = "task1"
	m.mu.Lock()
	m.tasks[id] = &taskState{
		Task: Task{
			TaskID:     id,
			AccountID:  7,
			Status:     StatusSuccess,
			TargetPath: "dirX",
			Result:     map[string]any{"file_id": "f9", "parent_id": "dirX"},
		},
		runDone: make(chan struct{}),
	}
	m.mu.Unlock()

	found, err := m.Delete(context.Background(), id, true)
	if !found || err == nil {
		t.Fatalf("delete found=%v err=%v", found, err)
	}
	m.mu.Lock()
	_, stillThere := m.tasks[id]
	m.mu.Unlock()
	if !stillThere {
		t.Fatal("task removed before cloud delete succeeded")
	}
}

func TestResumeWaitsForPreviousRunAndReusesCheckpoint(t *testing.T) {
	releaseFirst := make(chan struct{})
	defer func() {
		select {
		case <-releaseFirst:
		default:
			close(releaseFirst)
		}
	}()
	drv := &blockingResumeDriver{
		firstStarted:  make(chan struct{}),
		firstCanceled: make(chan struct{}),
		releaseFirst:  releaseFirst,
		secondState:   make(chan map[string]any, 1),
	}
	m := NewManager(Options{
		Exec:     driverexec.New(fakeProvider{drv: drv}, nil),
		Accounts: fakeUploadAccounts{},
		DataDir:  t.TempDir(),
	})
	localPath := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(localPath, []byte("abcdefghijklmnop"), 0o600); err != nil {
		t.Fatal(err)
	}
	task, err := m.Create(context.Background(), CreateParams{
		AccountID:      1,
		FileName:       "sample.bin",
		TargetPath:     "0",
		LocalPath:      localPath,
		TotalBytes:     16,
		ConflictPolicy: "overwrite",
	})
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-drv.firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("首次上传未启动")
	}
	paused, ok := m.Pause(context.Background(), task.TaskID)
	if !ok || paused.Status != StatusPaused {
		t.Fatalf("pause ok=%v task=%+v", ok, paused)
	}
	select {
	case <-drv.firstCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("暂停未取消旧上传")
	}

	resumeReturned := make(chan struct{})
	go func() {
		_, _ = m.Resume(context.Background(), task.TaskID)
		close(resumeReturned)
	}()
	select {
	case <-resumeReturned:
		t.Fatal("旧上传尚未退出时 Resume 已返回")
	case <-drv.secondState:
		t.Fatal("旧上传尚未退出时启动了新上传")
	case <-time.After(80 * time.Millisecond):
	}

	close(releaseFirst)
	select {
	case <-resumeReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("旧上传退出后 Resume 未返回")
	}
	var state map[string]any
	select {
	case state = <-drv.secondState:
	case <-time.After(2 * time.Second):
		t.Fatal("继续上传未启动")
	}
	if uploaded, ok := mapInt64(state["uploaded_bytes"]); !ok || uploaded != 4 {
		t.Fatalf("resume uploaded_bytes=%v want 4", state["uploaded_bytes"])
	}
	parts, ok := state["completed_slices"].([]any)
	if !ok || len(parts) != 1 {
		t.Fatalf("resume completed_slices=%#v want [1]", state["completed_slices"])
	}
}
