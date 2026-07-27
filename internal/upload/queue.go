package upload

import (
	"context"
	"time"

	"litepan/internal/settings"
)

func (m *Manager) RefreshConcurrencyLimit(ctx context.Context) int {
	limit := defaultLimit
	if m.settings != nil {
		v := m.settings.Int(settings.KeyUploadTaskConcurrency)
		if v > 0 {
			limit = v
		}
	}
	m.mu.Lock()
	m.limit = limit
	m.mu.Unlock()
	m.runCond.Broadcast()
	return limit
}

func (m *Manager) runTask(taskID string) {
	m.mu.Lock()
	st, ok := m.tasks[taskID]
	if !ok {
		m.mu.Unlock()
		return
	}
	done := st.runDone
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		if current, exists := m.tasks[taskID]; exists && current.runDone == done {
			current.cancel = nil
		}
		m.mu.Unlock()
		if done != nil {
			close(done)
		}
	}()

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.mu.Lock()
	for {
		st, ok = m.tasks[taskID]
		if !ok || st.runDone != done || st.Status != StatusPending {
			m.mu.Unlock()
			return
		}
		if m.running < m.limit {
			break
		}
		m.runCond.Wait()
	}
	m.running++
	st.cancel = cancel
	st.Message = "排队中"
	st.UpdatedAt = unixFloat(time.Now())
	m.mu.Unlock()
	m.broadcast()

	defer func() {
		m.mu.Lock()
		m.running--
		m.runCond.Signal()
		m.mu.Unlock()
	}()

	m.executeUpload(runCtx, taskID)
}
