package cacheretention

import (
	"context"
	"time"

	"litepan/internal/auth"
	"litepan/internal/domain"
	"litepan/internal/driver"
)

func (s *Service) schedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scheduleOnce(ctx)
		}
	}
}

func (s *Service) scheduleOnce(ctx context.Context) {
	if s.StartupRemaining() > 0 {
		return
	}
	if !s.cacheGloballyEnabled(ctx) {
		return
	}
	now := time.Now()
	tasks, err := s.repo.List(ctx)
	if err != nil {
		return
	}

	strmBusy := s.snapshotBusyAccounts()

	s.mu.Lock()
	if len(s.runningAccounts) > 0 {
		s.mu.Unlock()
		return
	}
	var pick *domain.CacheRetentionTask
	for _, t := range tasks {
		if t.Status != domain.RetentionStatusRunning {
			continue
		}
		if s.running[t.ID] {
			continue
		}
		if !IsInTimeWindow(t, now) {
			continue
		}
		if accountBusy(strmBusy, t.AccountID) {
			continue
		}
		if s.accountCacheDisabled(ctx, t.AccountID) {
			continue
		}
		ttl := s.accountCacheTTL(ctx, t.AccountID)
		_, pending := s.pendingRun[t.ID]
		if !pending {
			if last, ok := s.accountLastDone[t.AccountID]; ok && ttl > 0 && now.Sub(last) < ttl {
				continue
			}
		}
		next, hasNext := s.nextRun[t.ID]
		if !pending && (!hasNext || next.After(now)) {
			continue
		}
		pick = t
		break
	}
	if pick == nil {
		s.mu.Unlock()
		return
	}
	delete(s.pendingRun, pick.ID)
	s.mu.Unlock()
	s.runTaskAsync(pick)
}

func (s *Service) isAccountBusy(accountID int64) bool {
	return accountBusy(s.snapshotBusyAccounts(), accountID)
}

func (s *Service) snapshotBusyAccounts() map[int64]struct{} {
	set := snapshotRunningAccountIDs(s.strmBusy)
	for id := range snapshotRunningAccountIDs(s.organizeBusy) {
		if set == nil {
			set = make(map[int64]struct{})
		}
		set[id] = struct{}{}
	}
	return set
}

func snapshotRunningAccountIDs(lister RunningAccountLister) map[int64]struct{} {
	if lister == nil {
		return nil
	}
	ids := lister.GetRunningAccountIDs()
	if len(ids) == 0 {
		return nil
	}
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

func accountBusy(set map[int64]struct{}, accountID int64) bool {
	if set == nil || accountID <= 0 {
		return false
	}
	_, ok := set[accountID]
	return ok
}

func (s *Service) runTaskAsync(task *domain.CacheRetentionTask) {
	if task == nil {
		return
	}
	s.mu.Lock()
	if s.running[task.ID] {
		s.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(s.appCtx)
	s.running[task.ID] = true
	s.runningAccounts[task.AccountID] = struct{}{}
	s.runningTaskAcct[task.ID] = task.AccountID
	s.taskCancels[task.ID] = cancel
	s.liveStats[task.ID] = scanStats{}
	s.mu.Unlock()
	s.log.Info("缓存任务开始执行",
		"task_id", task.ID,
		"account_id", task.AccountID,
		"account_name", task.AccountName,
		"path", task.Path,
		"refresh_interval_minutes", task.RefreshInterval,
	)

	go func() {
		defer cancel()
		started := time.Now()
		ctx := driver.WithExtraAPIDelay(runCtx, task.ApiInterval)
		progress := func(st scanStats) {
			if st.StartedAt.IsZero() {
				st.StartedAt = started
			}
			s.mu.Lock()
			s.liveStats[task.ID] = st
			s.mu.Unlock()
		}
		stats, err := s.scan.refreshTask(ctx, task, progress)
		s.mu.Lock()
		delete(s.running, task.ID)
		delete(s.taskCancels, task.ID)
		delete(s.runningAccounts, task.AccountID)
		delete(s.runningTaskAcct, task.ID)
		s.accountLastDone[task.AccountID] = time.Now()
		delete(s.liveStats, task.ID)
		s.nextRun[task.ID] = time.Now().Add(time.Duration(task.RefreshInterval) * time.Minute)
		s.mu.Unlock()

		durationMS := int(time.Since(started).Milliseconds())
		if err != nil {
			s.log.Warn("缓存任务执行失败", "task_id", task.ID, "account_id", task.AccountID, "err", err)
			s.handleRunError(context.Background(), task, err, durationMS)
			return
		}
		s.log.Info("缓存任务执行完成",
			"task_id", task.ID,
			"account_id", task.AccountID,
			"scanned_dirs", stats.ScannedDirs,
			"scanned_files", stats.ScannedFiles,
			"api_calls", stats.APICalls,
			"duration_ms", durationMS,
		)
		_ = s.repo.UpdateRunStats(context.Background(), task.ID, domain.RetentionRunStats{
			FileCount:         stats.ScannedFiles,
			LastRefresh:       time.Now(),
			LastRefreshStatus: "success",
			LastDurationMS:    durationMS,
			LastAPICalls:      stats.APICalls,
			LastSkipCalls:     stats.SkipCalls,
			LastScannedDirs:   stats.ScannedDirs,
			LastRunConfigFP:   taskConfigFingerprint(task),
			ErrorMessage:      "",
		})
		s.notifyLargeScope(task, stats)
	}()
}

func (s *Service) handleRunError(ctx context.Context, task *domain.CacheRetentionTask, err error, durationMS int) {
	status := "error"
	msg := err.Error()
	if auth.IsAuthError(err) {
		_, _ = s.PauseByAccount(ctx, task.AccountID, domain.PauseReasonAuthFailure, msg)
		return
	}
	_ = s.repo.UpdateRunStats(ctx, task.ID, domain.RetentionRunStats{
		LastRefresh:       time.Now(),
		LastRefreshStatus: status,
		LastDurationMS:    durationMS,
		LastRunConfigFP:   taskConfigFingerprint(task),
		ErrorMessage:      msg,
	})
}
