package offlinedownload

import (
	"context"
	"sync"
	"testing"
	"time"

	"litepan/internal/core/driverexec"
	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/eventbus"
)

type offlineTestDriver struct {
	addResults  []driver.OfflineAddResult
	updates     []driver.OfflineTaskUpdate
	deleteCalls []offlineDeleteCall
}

type offlineDeleteCall struct {
	ref              driver.OfflineTaskRef
	deleteSourceFile bool
}

func (*offlineTestDriver) Config() driver.Config      { return driver.Config{Name: "offline-test"} }
func (*offlineTestDriver) GetAddition() any           { return &struct{}{} }
func (*offlineTestDriver) Init(context.Context) error { return nil }
func (*offlineTestDriver) Drop(context.Context) error { return nil }
func (*offlineTestDriver) Ping(context.Context) error { return nil }
func (*offlineTestDriver) ListFiles(context.Context, string) ([]domain.FileItem, error) {
	return nil, nil
}
func (*offlineTestDriver) OfflineDownloadCapabilities() driver.OfflineDownloadCapabilities {
	return driver.OfflineDownloadCapabilities{
		SupportsURLs: true, SupportsBatchURLs: true, URLSchemes: []string{"https"}, RemoteDelete: true,
	}
}

func (d *offlineTestDriver) AddOfflineURLs(_ context.Context, req driver.OfflineURLRequest) ([]driver.OfflineAddResult, error) {
	if len(d.addResults) > 0 {
		return append([]driver.OfflineAddResult(nil), d.addResults...), nil
	}
	return []driver.OfflineAddResult{{Source: req.URLs[0], InfoHash: "hash-1", Success: true}}, nil
}
func (d *offlineTestDriver) RefreshOfflineTasks(context.Context, []driver.OfflineTaskRef) ([]driver.OfflineTaskUpdate, error) {
	return append([]driver.OfflineTaskUpdate(nil), d.updates...), nil
}
func (d *offlineTestDriver) DeleteOfflineTask(_ context.Context, ref driver.OfflineTaskRef, deleteSourceFile bool) error {
	d.deleteCalls = append(d.deleteCalls, offlineDeleteCall{ref: ref, deleteSourceFile: deleteSourceFile})
	return nil
}

type offlineTestProvider struct{ drv driver.Driver }

func (p offlineTestProvider) Get(context.Context, int64) (driver.Driver, error) { return p.drv, nil }

type offlineAccountRepo struct{ account *domain.Account }

func (r offlineAccountRepo) Create(context.Context, *domain.Account) (int64, error) { return 0, nil }
func (r offlineAccountRepo) Update(context.Context, *domain.Account) error          { return nil }
func (r offlineAccountRepo) Delete(context.Context, int64) error                    { return nil }
func (r offlineAccountRepo) Get(_ context.Context, id int64) (*domain.Account, error) {
	if r.account != nil && r.account.ID == id {
		copy := *r.account
		return &copy, nil
	}
	return nil, domain.Errf(domain.CodeNotFound)
}
func (r offlineAccountRepo) List(context.Context) ([]*domain.Account, error) {
	return []*domain.Account{r.account}, nil
}
func (r offlineAccountRepo) SetDefault(context.Context, int64) error { return nil }
func (r offlineAccountRepo) NameTaken(context.Context, string, int64) (bool, error) {
	return false, nil
}

type offlineTaskRepo struct {
	mu    sync.Mutex
	tasks map[string]*domain.OfflineDownloadTaskRecord
}

func newOfflineTaskRepo() *offlineTaskRepo {
	return &offlineTaskRepo{tasks: make(map[string]*domain.OfflineDownloadTaskRecord)}
}
func (r *offlineTaskRepo) Upsert(_ context.Context, rec *domain.OfflineDownloadTaskRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := *rec
	r.tasks[rec.TaskID] = &copy
	return nil
}
func (r *offlineTaskRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tasks, id)
	return nil
}
func (r *offlineTaskRepo) DeleteByAccount(_ context.Context, accountID int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var count int64
	for id, task := range r.tasks {
		if task.AccountID == accountID {
			delete(r.tasks, id)
			count++
		}
	}
	return count, nil
}
func (r *offlineTaskRepo) List(context.Context) ([]*domain.OfflineDownloadTaskRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*domain.OfflineDownloadTaskRecord, 0, len(r.tasks))
	for _, task := range r.tasks {
		copy := *task
		out = append(out, &copy)
	}
	return out, nil
}

func TestAddAndRefreshOfflineTask(t *testing.T) {
	drv := &offlineTestDriver{}
	repo := newOfflineTaskRepo()
	bus := eventbus.New(nil)
	t.Cleanup(func() { _ = bus.Close(context.Background()) })
	mutations := make(chan eventbus.FileMutated, 1)
	eventbus.Subscribe(bus, func(_ context.Context, event eventbus.FileMutated) { mutations <- event })
	svc := New(Options{
		Exec:     driverexec.New(offlineTestProvider{drv: drv}, nil),
		Accounts: offlineAccountRepo{account: &domain.Account{ID: 7, Name: "测试盘", DriverType: "offline-test"}},
		Repo:     repo,
		Bus:      bus,
	})

	created, err := svc.AddURLs(context.Background(), AddURLParams{
		AccountID: 7, URLs: []string{"https://example.com/movie.mkv"},
		TargetParentID: "folder", TargetDisplayPath: "/电影",
	})
	if err != nil || len(created) != 1 {
		t.Fatalf("创建任务失败: tasks=%#v err=%v", created, err)
	}
	if created[0].Status != driver.OfflineStatusPending || created[0].InfoHash != "hash-1" {
		t.Fatalf("创建后的任务不正确: %#v", created[0])
	}
	drv.updates = []driver.OfflineTaskUpdate{{
		InfoHash: "hash-1", Status: driver.OfflineStatusSuccess, Progress: 100, FileID: "file-1", Name: "movie.mkv",
	}}
	if err := svc.Refresh(context.Background(), 7, true); err != nil {
		t.Fatalf("刷新任务失败: %v", err)
	}
	tasks, err := svc.List(context.Background(), 7, false)
	if err != nil || len(tasks) != 1 || tasks[0].Status != driver.OfflineStatusSuccess {
		t.Fatalf("刷新后的任务不正确: tasks=%#v err=%v", tasks, err)
	}
	select {
	case event := <-mutations:
		if event.AccountID != 7 || event.ParentID != "folder" || event.Op != "offline_download" {
			t.Fatalf("文件变更事件不正确: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("任务完成后没有发布文件变更事件")
	}
}

func TestRejectUnsupportedOfflineURLScheme(t *testing.T) {
	svc := New(Options{
		Exec:     driverexec.New(offlineTestProvider{drv: &offlineTestDriver{}}, nil),
		Accounts: offlineAccountRepo{account: &domain.Account{ID: 7, Name: "测试盘", DriverType: "offline-test"}},
		Repo:     newOfflineTaskRepo(),
	})
	if _, err := svc.AddURLs(context.Background(), AddURLParams{AccountID: 7, URLs: []string{"magnet:?xt=urn:btih:test"}}); err == nil {
		t.Fatal("不支持的链接协议应被拒绝")
	}
}

func TestDeleteCompletedTaskAlsoDeletesRemoteHistory(t *testing.T) {
	drv := &offlineTestDriver{}
	repo := newOfflineTaskRepo()
	svc := New(Options{
		Exec:     driverexec.New(offlineTestProvider{drv: drv}, nil),
		Accounts: offlineAccountRepo{account: &domain.Account{ID: 7, Name: "测试盘", DriverType: "offline-test"}},
		Repo:     repo,
	})

	created, err := svc.AddURLs(context.Background(), AddURLParams{
		AccountID: 7,
		URLs:      []string{"https://example.com/movie.mkv"},
	})
	if err != nil || len(created) != 1 {
		t.Fatalf("创建任务失败: tasks=%#v err=%v", created, err)
	}
	drv.updates = []driver.OfflineTaskUpdate{{
		InfoHash: "hash-1", Status: driver.OfflineStatusSuccess, Progress: 100,
	}}
	if err := svc.Refresh(context.Background(), 7, true); err != nil {
		t.Fatalf("刷新任务失败: %v", err)
	}
	if err := svc.Delete(context.Background(), created[0].TaskID); err != nil {
		t.Fatalf("删除已完成任务失败: %v", err)
	}
	if len(drv.deleteCalls) != 1 {
		t.Fatalf("已完成任务应同步删除远端历史: calls=%#v", drv.deleteCalls)
	}
	call := drv.deleteCalls[0]
	if call.ref.InfoHash != "hash-1" || call.deleteSourceFile {
		t.Fatalf("远端删除参数不正确: %#v", call)
	}
	tasks, err := svc.List(context.Background(), 7, false)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("本地任务记录未删除: tasks=%#v err=%v", tasks, err)
	}
}

func TestDeleteFailedTaskWithoutRemoteReferenceOnlyDeletesLocal(t *testing.T) {
	drv := &offlineTestDriver{addResults: []driver.OfflineAddResult{{
		Source: "https://example.com/invalid", Success: false, Message: "创建失败",
	}}}
	svc := New(Options{
		Exec:     driverexec.New(offlineTestProvider{drv: drv}, nil),
		Accounts: offlineAccountRepo{account: &domain.Account{ID: 7, Name: "测试盘", DriverType: "offline-test"}},
		Repo:     newOfflineTaskRepo(),
	})
	created, err := svc.AddURLs(context.Background(), AddURLParams{
		AccountID: 7,
		URLs:      []string{"https://example.com/invalid"},
	})
	if err != nil || len(created) != 1 || created[0].Status != driver.OfflineStatusFailed {
		t.Fatalf("创建失败记录不正确: tasks=%#v err=%v", created, err)
	}
	if err := svc.Delete(context.Background(), created[0].TaskID); err != nil {
		t.Fatalf("无远端标识的失败记录应可直接删除: %v", err)
	}
	if len(drv.deleteCalls) != 0 {
		t.Fatalf("无远端标识时不应调用网盘删除: %#v", drv.deleteCalls)
	}
}

func TestValidateSchemesAcceptsThunderAndMagnet(t *testing.T) {
	thunder := "thunder://QUFodHRwOi8vZXhhbXBsZS5jb20vZmlsZS56aXBaWg=="
	allowed := []string{"http", "https", "ftp", "thunder", "magnet"}
	if err := validateSchemes([]string{thunder, "magnet:?xt=urn:btih:abc", "https://a.com/b"}, allowed); err != nil {
		t.Fatalf("合法离线链接应通过校验: %v", err)
	}
	ed2k := "ed2k://|file|demo.bin|1|ABCDEFABCDEFABCDEFABCDEFABCDEFAB|/"
	if err := validateSchemes([]string{ed2k}, allowed); err == nil {
		t.Fatal("未声明的 ed2k 协议应被拒绝")
	}
	if err := validateSchemes([]string{"not-a-url"}, allowed); err == nil {
		t.Fatal("无协议链接应被拒绝")
	}
}

func TestDisplayNameForThunder(t *testing.T) {
	if got := displayNameForURL("thunder://QUFodHRwOi8vZXhhbXBsZS5jb20v"); got != "迅雷链接任务" {
		t.Fatalf("thunder 显示名错误: %q", got)
	}
}
