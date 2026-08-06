package upload

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"litepan/internal/core/driverexec"
	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/eventbus"
	"litepan/internal/file"
	"litepan/internal/playback"
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

type blockingCrossTransferDriver struct {
	resolved chan string
	started  chan string
	release  chan struct{}
	serverURL string
}

type queuedUploadDriver struct {
	started  chan string
	releases map[string]chan struct{}
}

type priorityCrossTransferDriver struct {
	resolved  chan string
	serverURL string
}

func (d *blockingCrossTransferDriver) Config() driver.Config      { return driver.Config{Name: "mock"} }
func (d *blockingCrossTransferDriver) GetAddition() any           { return &struct{}{} }
func (d *blockingCrossTransferDriver) Init(context.Context) error { return nil }
func (d *blockingCrossTransferDriver) Drop(context.Context) error { return nil }
func (d *blockingCrossTransferDriver) Ping(context.Context) error { return nil }
func (d *blockingCrossTransferDriver) ListFiles(context.Context, string) ([]domain.FileItem, error) {
	return nil, nil
}

func (d *blockingCrossTransferDriver) ResolveDownload(_ context.Context, req driver.DownloadRequest) (*domain.DownloadInfo, error) {
	d.resolved <- req.FileID
	return &domain.DownloadInfo{
		URL:  d.serverURL + "/" + req.FileID,
		Size: 4,
	}, nil
}

func (d *blockingCrossTransferDriver) UploadLocalFile(ctx context.Context, req driver.LocalUploadRequest) (*driver.LocalUploadResult, error) {
	d.started <- req.FileName
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-d.release:
		return &driver.LocalUploadResult{
			FileID:   req.FileName + "-done",
			ParentID: req.ParentID,
			FileName: req.FileName,
			Size:     4,
			Message:  "上传成功",
		}, nil
	}
}

func (d *queuedUploadDriver) Config() driver.Config      { return driver.Config{Name: "mock"} }
func (d *queuedUploadDriver) GetAddition() any           { return &struct{}{} }
func (d *queuedUploadDriver) Init(context.Context) error { return nil }
func (d *queuedUploadDriver) Drop(context.Context) error { return nil }
func (d *queuedUploadDriver) Ping(context.Context) error { return nil }
func (d *queuedUploadDriver) ListFiles(context.Context, string) ([]domain.FileItem, error) {
	return nil, nil
}

func (d *queuedUploadDriver) UploadLocalFile(ctx context.Context, req driver.LocalUploadRequest) (*driver.LocalUploadResult, error) {
	d.started <- req.FileName
	if ch := d.releases[req.FileName]; ch != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
		}
	}
	return &driver.LocalUploadResult{
		FileID:   req.FileName + "-done",
		ParentID: req.ParentID,
		FileName: req.FileName,
		Size:     16,
		Message:  "上传成功",
	}, nil
}

func (d *priorityCrossTransferDriver) Config() driver.Config      { return driver.Config{Name: "mock"} }
func (d *priorityCrossTransferDriver) GetAddition() any           { return &struct{}{} }
func (d *priorityCrossTransferDriver) Init(context.Context) error { return nil }
func (d *priorityCrossTransferDriver) Drop(context.Context) error { return nil }
func (d *priorityCrossTransferDriver) Ping(context.Context) error { return nil }
func (d *priorityCrossTransferDriver) ListFiles(context.Context, string) ([]domain.FileItem, error) {
	return nil, nil
}

func (d *priorityCrossTransferDriver) ResolveDownload(_ context.Context, req driver.DownloadRequest) (*domain.DownloadInfo, error) {
	d.resolved <- req.FileID
	return &domain.DownloadInfo{
		URL:  d.serverURL + "/" + req.FileID,
		Size: 4,
	}, nil
}

func (d *priorityCrossTransferDriver) UploadLocalFile(_ context.Context, req driver.LocalUploadRequest) (*driver.LocalUploadResult, error) {
	return &driver.LocalUploadResult{
		FileID:   req.FileName + "-done",
		ParentID: req.ParentID,
		FileName: req.FileName,
		Size:     4,
		Message:  "上传成功",
	}, nil
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

func TestCrossTransferDownloadReleasesSlotBeforeUploadCompletes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1234"))
	}))
	defer server.Close()

	drv := &blockingCrossTransferDriver{
		resolved:  make(chan string, 8),
		started:   make(chan string, 4),
		release:   make(chan struct{}),
		serverURL: server.URL,
	}
	exec := driverexec.New(fakeProvider{drv: drv}, nil)

	m := NewManager(Options{
		Exec:     exec,
		Files:    file.NewService(exec, nil, nil, nil, nil, nil),
		Playback: playback.NewService(exec, nil),
		Accounts: fakeUploadAccounts{},
		DataDir:  t.TempDir(),
	})

	m.mu.Lock()
	m.limit = 1
	m.mu.Unlock()

	task1, err := m.Create(context.Background(), CreateParams{
		AccountID:         1,
		FileName:          "task-1.bin",
		SourceType:        SourceTypeCrossTransfer,
		SourceAccountID:   11,
		SourceAccountName: "源盘",
		SourceDriverType:  "mock",
		SourceFileID:      "src-1",
		TargetPath:        "dst",
		TotalBytes:        4,
		ConflictPolicy:    "overwrite",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Create(context.Background(), CreateParams{
		AccountID:         1,
		FileName:          "task-2.bin",
		SourceType:        SourceTypeCrossTransfer,
		SourceAccountID:   11,
		SourceAccountName: "源盘",
		SourceDriverType:  "mock",
		SourceFileID:      "src-2",
		TargetPath:        "dst",
		TotalBytes:        4,
		ConflictPolicy:    "overwrite",
	})
	if err != nil {
		t.Fatal(err)
	}

	resolvedSet := map[string]bool{}
	select {
	case fileID := <-drv.resolved:
		resolvedSet[fileID] = true
	case <-time.After(2 * time.Second):
		t.Fatal("未观察到跨盘下载解析启动")
	}

	select {
	case fileID := <-drv.resolved:
		resolvedSet[fileID] = true
	case <-time.After(2 * time.Second):
		t.Fatal("第一个任务进入上传后，第二个跨盘下载没有接上")
	}
	if !resolvedSet["src-1"] || !resolvedSet["src-2"] {
		t.Fatalf("resolved=%v want both src-1 and src-2", resolvedSet)
	}

	select {
	case name := <-drv.started:
		if name != "task-1.bin" && name != "task-2.bin" {
			t.Fatalf("unexpected upload started=%q", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("跨盘下载完成后未进入上传阶段")
	}

	close(drv.release)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := m.Get(context.Background(), task1.TaskID)
		if ok && got.Status == StatusSuccess {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("首个跨盘任务未完成")
}

func TestResumePendingUploadTaskRunsBeforeNormalPending(t *testing.T) {
	drv := &queuedUploadDriver{
		started: make(chan string, 8),
		releases: map[string]chan struct{}{
			"task-1.bin": make(chan struct{}),
			"task-2.bin": make(chan struct{}),
			"task-3.bin": make(chan struct{}),
		},
	}
	for _, ch := range drv.releases {
		defer func(ch chan struct{}) {
			select {
			case <-ch:
			default:
				close(ch)
			}
		}(ch)
	}
	m := NewManager(Options{
		Exec:     driverexec.New(fakeProvider{drv: drv}, nil),
		Accounts: fakeUploadAccounts{},
		DataDir:  t.TempDir(),
	})
	m.mu.Lock()
	m.limit = 1
	m.mu.Unlock()

	createTask := func(name string) *Task {
		t.Helper()
		localPath := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(localPath, []byte("abcdefghijklmnop"), 0o600); err != nil {
			t.Fatal(err)
		}
		task, err := m.Create(context.Background(), CreateParams{
			AccountID:      1,
			FileName:       name,
			TargetPath:     "0",
			LocalPath:      localPath,
			TotalBytes:     16,
			ConflictPolicy: "overwrite",
		})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}

	createTask("task-1.bin")
	select {
	case name := <-drv.started:
		if name != "task-1.bin" {
			t.Fatalf("first started=%q want task-1.bin", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("首个上传任务未启动")
	}

	task2 := createTask("task-2.bin")
	paused, ok := m.Pause(context.Background(), task2.TaskID)
	if !ok || paused.Status != StatusPaused {
		t.Fatalf("pause ok=%v task=%+v", ok, paused)
	}
	_ = createTask("task-3.bin")
	if _, ok := m.Resume(context.Background(), task2.TaskID); !ok {
		t.Fatal("恢复第二个上传任务失败")
	}

	close(drv.releases["task-1.bin"])
	select {
	case name := <-drv.started:
		if name != "task-2.bin" {
			t.Fatalf("next started=%q want task-2.bin", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("恢复任务未优先接棒上传")
	}
}

func TestResumePendingCrossTransferDownloadRunsBeforeNormalPending(t *testing.T) {
	releaseSrc1 := make(chan struct{})
	defer func() {
		select {
		case <-releaseSrc1:
		default:
			close(releaseSrc1)
		}
	}()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/src-1" {
			<-releaseSrc1
		}
		_, _ = w.Write([]byte("1234"))
	}))
	defer server.Close()

	drv := &priorityCrossTransferDriver{
		resolved:  make(chan string, 8),
		serverURL: server.URL,
	}
	exec := driverexec.New(fakeProvider{drv: drv}, nil)
	m := NewManager(Options{
		Exec:     exec,
		Files:    file.NewService(exec, nil, nil, nil, nil, nil),
		Playback: playback.NewService(exec, nil),
		Accounts: fakeUploadAccounts{},
		DataDir:  t.TempDir(),
	})
	m.mu.Lock()
	m.limit = 1
	m.mu.Unlock()

	createTask := func(fileID, name string) *Task {
		t.Helper()
		task, err := m.Create(context.Background(), CreateParams{
			AccountID:         1,
			FileName:          name,
			SourceType:        SourceTypeCrossTransfer,
			SourceAccountID:   11,
			SourceAccountName: "源盘",
			SourceDriverType:  "mock",
			SourceFileID:      fileID,
			TargetPath:        "dst",
			TotalBytes:        4,
			ConflictPolicy:    "overwrite",
		})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}

	createTask("src-1", "task-1.bin")
	select {
	case fileID := <-drv.resolved:
		if fileID != "src-1" {
			t.Fatalf("first resolved=%q want src-1", fileID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("首个跨盘下载未启动")
	}

	task2 := createTask("src-2", "task-2.bin")
	paused, ok := m.Pause(context.Background(), task2.TaskID)
	if !ok || paused.Status != StatusPaused {
		t.Fatalf("pause ok=%v task=%+v", ok, paused)
	}
	_ = createTask("src-3", "task-3.bin")
	if _, ok := m.Resume(context.Background(), task2.TaskID); !ok {
		t.Fatal("恢复第二个跨盘任务失败")
	}

	close(releaseSrc1)
	select {
	case fileID := <-drv.resolved:
		if fileID != "src-2" {
			t.Fatalf("next resolved=%q want src-2", fileID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("恢复任务未优先接棒下载")
	}
}
