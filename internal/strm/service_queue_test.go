package strm

import (
	"litepan/internal/domain"
	"testing"
	"time"
)

func TestQueuedTasksSixTasksDoNotStarve(t *testing.T) {
	s, _ := testService(t)
	now := time.Now()
	tasks := make([]*domain.StrmTask, 6)
	for i := range tasks {
		tasks[i] = &domain.StrmTask{ID: int64(i + 1), AccountID: 7, Status: domain.StrmStatusActive}
	}
	for want := int64(1); want <= 6; want++ {
		ready := s.queuedTasks(tasks, now)
		if len(ready) == 0 || ready[0].ID != want {
			t.Fatalf("等待任务应为 %d，实际 %v", want, ready)
		}
		s.mu.Lock()
		if !s.canStartTaskLocked(ready[0], 3) {
			t.Fatal("队首无法启动")
		}
		if len(ready) > 1 && s.canStartTaskLocked(ready[1], 3) {
			t.Fatal("后来的任务不能插队")
		}
		delete(s.waitingRuns, want)
		s.running[want] = true
		s.runningAccounts[7] = struct{}{}
		if len(ready) > 1 && s.canStartTaskLocked(ready[1], 3) {
			t.Fatal("同账号不能并行")
		}
		s.clearTaskRunState(want, 7)
		s.mu.Unlock()
		// 保持已完成任务仍然到期，模拟短间隔，不能抢占剩余任务。
	}
	if got := s.queuedTasks(tasks, now)[0].ID; got != 1 {
		t.Fatalf("第二轮队首=%d", got)
	}
}

func TestQueuedManualRunIgnoresIntervalAndWindow(t *testing.T) {
	s, _ := testService(t)
	task := &domain.StrmTask{ID: 1, AccountID: 7, Status: domain.StrmStatusActive, ScheduleMode: domain.StrmScheduleManual, LastScan: time.Now()}
	s.pendingRun[1] = domain.StrmRunModeFull
	s.enqueueRunLocked(task)
	for i := 0; i < 3; i++ {
		if got := s.queuedTasks([]*domain.StrmTask{task}, time.Now()); len(got) != 1 {
			t.Fatal("手动排队不应受扫描间隔限制")
		}
	}
	if len(s.waitingRuns) != 1 {
		t.Fatal("重复调度不应重复入队")
	}
	task.Status = domain.StrmStatusPaused
	if len(s.queuedTasks([]*domain.StrmTask{task}, time.Now())) != 0 || len(s.pendingRun) != 0 {
		t.Fatal("暂停应清除等待请求")
	}
}

func TestQueuedTasksWindowAndOtherAccounts(t *testing.T) {
	s, _ := testService(t)
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.Local)
	a := &domain.StrmTask{ID: 1, AccountID: 7, Status: domain.StrmStatusActive, TimeWindowEnabled: true, TimeStart: "11:00", TimeEnd: "12:00"}
	b := &domain.StrmTask{ID: 2, AccountID: 8, Status: domain.StrmStatusActive}
	s.queuedTasks([]*domain.StrmTask{a, b}, now)
	s.runningAccounts[7] = struct{}{}
	if !s.canStartTaskLocked(b, 3) {
		t.Fatal("其他账号不应被阻塞")
	}
	ready := s.queuedTasks([]*domain.StrmTask{a, b}, now.Add(time.Hour))
	if len(ready) != 1 || ready[0].ID != 2 {
		t.Fatal("自动任务不能在窗口外启动")
	}
	s.queuedTasks(nil, now)
	if len(s.waitingRuns) != 0 {
		t.Fatal("删除任务应清理排队状态")
	}
}
