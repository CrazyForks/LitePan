package cacheretention

import (
	"time"

	"litepan/internal/domain"
)

func (s *Service) queuePendingRun(id int64) {
	s.mu.Lock()
	s.pendingRun[id] = struct{}{}
	s.nextRun[id] = time.Now()
	s.mu.Unlock()
}

func (s *Service) clearPendingRun(id int64) {
	s.mu.Lock()
	delete(s.pendingRun, id)
	s.mu.Unlock()
}

func (s *Service) IsPending(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.pendingRun[id]
	return ok
}

func (s *Service) pendingIDs() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int64, 0, len(s.pendingRun))
	for id := range s.pendingRun {
		out = append(out, id)
	}
	return out
}

// startTaskImmediate 手动触发时立刻开跑；返回 false 表示已在跑或同账号占用。
func (s *Service) startTaskImmediate(task *domain.CacheRetentionTask) bool {
	if task == nil {
		return false
	}
	s.mu.Lock()
	if s.running[task.ID] {
		s.mu.Unlock()
		return false
	}
	if _, ok := s.runningAccounts[task.AccountID]; ok {
		s.mu.Unlock()
		return false
	}
	delete(s.pendingRun, task.ID)
	s.mu.Unlock()
	s.runTaskAsync(task)
	return true
}
