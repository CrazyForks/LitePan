package upload

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"litepan/internal/core/driverexec"
	"litepan/internal/domain"
	"litepan/internal/eventbus"
	"litepan/internal/file"
	"litepan/internal/settings"
)

type AccountLookup interface {
	LookupUploadAccount(ctx context.Context, accountID int64) (name, driverType string, err error)
}

type Options struct {
	Exec     *driverexec.Executor
	Files    *file.Service
	Accounts AccountLookup
	Repo     domain.UploadTaskRepository
	Settings *settings.Service
	Bus      *eventbus.Bus
	DataDir  string
	Log      *slog.Logger
}

type Manager struct {
	exec     *driverexec.Executor
	files    *file.Service
	accounts AccountLookup
	repo     domain.UploadTaskRepository
	settings *settings.Service
	bus      *eventbus.Bus
	dataDir  string
	log      *slog.Logger

	mu           sync.Mutex
	tasks        map[string]*taskState
	queueOrder   int
	limit        int
	running      int
	runCond      sync.Cond
	subs         map[chan []byte]struct{}
	subMu        sync.Mutex
	tempRegistry *TempRegistry

	resumePersistMu sync.Mutex
	resumePersist   map[string]*time.Timer
}

func NewManager(opts Options) *Manager {
	m := &Manager{
		exec:     opts.Exec,
		files:    opts.Files,
		accounts: opts.Accounts,
		repo:     opts.Repo,
		settings: opts.Settings,
		bus:      opts.Bus,
		dataDir:  opts.DataDir,
		log:      opts.Log,
		tasks:    make(map[string]*taskState),
		limit:    defaultLimit,
		subs:     make(map[chan []byte]struct{}),
	}
	m.runCond.L = &m.mu
	if m.log == nil {
		m.log = slog.Default()
	}
	m.tempRegistry = NewTempRegistry()
	_ = m.RefreshConcurrencyLimit(context.Background())
	m.restoreTasks()
	m.initTempCleanup()
	return m
}

func (m *Manager) TempDir() string {
	return TempDir(m.dataDir)
}

func (m *Manager) Create(ctx context.Context, p CreateParams) (*Task, error) {
	if p.TotalBytes < 0 {
		return nil, domain.Errorf(domain.CodeValidation, "上传文件大小非法")
	}
	if m.accounts == nil {
		return nil, domain.Errorf(domain.CodeInternal, "上传服务未配置账号查询")
	}
	accountName, driverType, err := m.accounts.LookupUploadAccount(ctx, p.AccountID)
	if err != nil {
		return nil, err
	}
	name := p.DisplayName
	if name == "" {
		name = p.FileName
	}
	now := time.Now()
	m.mu.Lock()
	m.queueOrder++
	order := m.queueOrder
	id := newTaskID()
	st := &taskState{
		Task: Task{
			TaskID:            id,
			ClientTaskID:      p.ClientTaskID,
			AccountID:         p.AccountID,
			AccountName:       accountName,
			DriverType:        driverType,
			FileName:          name,
			TargetPath:        p.TargetPath,
			TargetDisplayPath: p.TargetDisplayPath,
			Status:            StatusPending,
			Message:           "等待上传",
			TotalBytes:        p.TotalBytes,
			QueueOrder:        order,
			CreatedAt:         unixFloat(now),
			UpdatedAt:         unixFloat(now),
		},
		localPath:      p.LocalPath,
		conflictPolicy: p.ConflictPolicy,
		runDone:        make(chan struct{}),
	}
	m.tasks[id] = st
	task := m.snapshot(st)
	m.mu.Unlock()
	m.persistTask(st)
	m.broadcast()
	go m.runTask(id)
	_ = ctx
	return task, nil
}

func (m *Manager) List(_ context.Context, accountID int64) []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Task, 0, len(m.tasks))
	for _, st := range m.tasks {
		if accountID > 0 && st.AccountID != accountID {
			continue
		}
		out = append(out, *m.snapshot(st))
	}
	sortTasksDesc(out)
	return out
}

func (m *Manager) Get(_ context.Context, taskID string) (*Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.tasks[taskID]
	if !ok {
		return nil, false
	}
	t := m.snapshot(st)
	return t, true
}
