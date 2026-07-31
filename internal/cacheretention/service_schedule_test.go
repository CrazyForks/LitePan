package cacheretention

import (
	"context"
	"sync"
	"testing"
	"time"

	"litepan/internal/domain"
	"litepan/internal/eventbus"
)

type reciprocalBusy struct {
	other RunningAccountLister
}

func (r reciprocalBusy) GetRunningAccountIDs() []int64 {
	if r.other != nil {
		_ = r.other.GetRunningAccountIDs()
	}
	return []int64{42}
}

type stubBusyAccounts struct {
	ids []int64
}

func (s stubBusyAccounts) GetRunningAccountIDs() []int64 {
	return s.ids
}

func TestSnapshotBusyAccountsMergesStrmAndOrganize(t *testing.T) {
	svc := &Service{}
	svc.strmBusy = stubBusyAccounts{ids: []int64{7}}
	svc.organizeBusy = stubBusyAccounts{ids: []int64{9}}
	set := svc.snapshotBusyAccounts()
	if !accountBusy(set, 7) || !accountBusy(set, 9) {
		t.Fatalf("set=%v", set)
	}
	if accountBusy(set, 8) {
		t.Fatal("unexpected busy account")
	}
}

func TestScheduleOnceCrossBusyCheckNoDeadlock(t *testing.T) {
	svc := &Service{
		running:         make(map[int64]bool),
		runningAccounts: make(map[int64]struct{}),
		runningTaskAcct: make(map[int64]int64),
		taskCancels:     make(map[int64]context.CancelFunc),
		nextRun:         make(map[int64]time.Time),
		accountLastDone: make(map[int64]time.Time),
		pendingRun:      make(map[int64]struct{}),
		liveStats:       make(map[int64]scanStats),
	}
	svc.strmBusy = reciprocalBusy{other: svc}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		svc.mu.Lock()
		time.Sleep(200 * time.Millisecond)
		svc.mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		svc.isAccountBusy(42)
	}()
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cross busy check deadlocked")
	}
}

func TestNotifyLargeScopeThresholdAndMessage(t *testing.T) {
	bus := eventbus.New(nil)
	defer func() {
		_ = bus.Close(context.Background())
	}()

	got := make(chan eventbus.NotificationCreated, 1)
	eventbus.Subscribe(bus, func(ctx context.Context, evt eventbus.NotificationCreated) {
		got <- evt
	})

	svc := &Service{bus: bus}
	task := &domain.CacheRetentionTask{ID: 7, AccountID: 18, Path: "/电影"}

	svc.notifyLargeScope(task, scanStats{APICalls: 499, SkipCalls: 0})
	select {
	case evt := <-got:
		t.Fatalf("499 次 API 调用不应触发提醒，got=%+v", evt)
	case <-time.After(100 * time.Millisecond):
	}

	svc.notifyLargeScope(task, scanStats{APICalls: 500, SkipCalls: 0})
	select {
	case evt := <-got:
		if evt.Category != domain.NotificationCategoryCacheScopeWarn {
			t.Fatalf("category=%q", evt.Category)
		}
		if evt.Title != "缓存保持任务范围过大" {
			t.Fatalf("title=%q", evt.Title)
		}
		want := "该任务扫描范围过大，继续执行意义不大，还可能增加网盘访问压力，建议尽快改为常用子目录。"
		if evt.Message != want {
			t.Fatalf("message=%q", evt.Message)
		}
	case <-time.After(time.Second):
		t.Fatal("500 次 API 调用应触发提醒")
	}
}
