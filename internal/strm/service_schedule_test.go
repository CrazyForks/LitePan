package strm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"litepan/internal/domain"
	"litepan/internal/settings"
	"litepan/internal/store"
)

func testService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, store.Options{Memory: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	st := store.New(db)
	settingsSvc, err := settings.New(ctx, st.Configs)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(ServiceOptions{
		Repo:     st.StrmTasks,
		Settings: settingsSvc,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return svc, st
}

type reciprocalRetentionBusy struct {
	other RunningAccountLister
}

func (r reciprocalRetentionBusy) GetRunningAccountIDs() []int64 {
	if r.other != nil {
		_ = r.other.GetRunningAccountIDs()
	}
	return []int64{7}
}

func TestShouldRunCrossBusyCheckNoDeadlock(t *testing.T) {
	svc, _ := testService(t)
	svc.SetRetentionBusyChecker(reciprocalRetentionBusy{other: svc})

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
		task := &domain.StrmTask{ID: 1, AccountID: 7, LastScan: time.Now().Add(-2 * time.Hour)}
		svc.shouldRun(task, time.Now())
	}()
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shouldRun cross busy check deadlocked")
	}
}

func TestTaskRunContextHasNoFixedDeadline(t *testing.T) {
	ctx, cancel := taskRunContext(context.Background())
	if _, ok := ctx.Deadline(); ok {
		cancel()
		t.Fatal("STRM 任务不应有固定执行期限")
	}

	cancel()
	<-ctx.Done()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("取消任务后错误 = %v，期望 context.Canceled", ctx.Err())
	}
}

func TestTaskStartLimitMatchesLegacyScheduler(t *testing.T) {
	svc, _ := testService(t)

	svc.mu.Lock()
	svc.running[1] = true
	svc.runningAccounts[7] = struct{}{}
	if svc.canStartTaskLocked(&domain.StrmTask{ID: 2, AccountID: 7}, 3) {
		t.Fatal("同一账号的 STRM 任务应串行")
	}
	if !svc.canStartTaskLocked(&domain.StrmTask{ID: 2, AccountID: 8}, 3) {
		t.Fatal("不同账号且未达到全局上限时应允许并发")
	}
	svc.running[2] = true
	svc.running[3] = true
	if svc.canStartTaskLocked(&domain.StrmTask{ID: 4, AccountID: 9}, 3) {
		t.Fatal("达到全局任务并发上限后应等待")
	}
	svc.mu.Unlock()
}
