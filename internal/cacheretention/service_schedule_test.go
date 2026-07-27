package cacheretention

import (
	"context"
	"sync"
	"testing"
	"time"
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
