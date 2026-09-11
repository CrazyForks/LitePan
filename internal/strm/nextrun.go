package strm

import (
	"time"

	"litepan/internal/domain"
)

// NextRunAt 计算任务下一次自动扫描的计划时间。
//
// ok=false 表示该任务当下不参与自动调度：手动调度、未启用，或正在执行（执行中不应再排入下一轮）。
// 返回值保证不早于 now：已到期且窗口内 → now（等下一次调度轮询）；窗口外 → 下一个窗口起点。
//
// 口径与调度器（service_schedule.go）保持一致：到期时间 = 上次扫描时间 + 有效间隔；
// 从未扫描过则视为立即到期；开了时间窗口且到期时间落在窗口外时，顺延到下一个窗口起点。
func (s *Service) NextRunAt(task *domain.StrmTask, now time.Time) (time.Time, bool) {
	if task == nil || !ShouldAutoSchedule(task) {
		return time.Time{}, false
	}
	if task.Status != domain.StrmStatusActive {
		return time.Time{}, false
	}

	interval := s.effectiveScanIntervalMinutes(task)
	if interval <= 0 {
		interval = defaultScanIntervalMinutes
	}

	due := now
	if !task.LastScan.IsZero() {
		due = task.LastScan.Add(time.Duration(interval) * time.Minute)
	}
	candidate := nextWithinTimeWindow(task, due)

	// 不返回已经错过的时间：窗口内已到期视为立即执行，窗口外顺延到下一次开窗。
	if !candidate.After(now) {
		if IsInTimeWindow(task, now) {
			return now, true
		}
		return nextWithinTimeWindow(task, now), true
	}
	return candidate, true
}

// nextWithinTimeWindow 在开了时间窗口且候选时间落在窗口外时，顺延到下一个窗口起点。
func nextWithinTimeWindow(task *domain.StrmTask, candidate time.Time) time.Time {
	if task == nil || !task.TimeWindowEnabled {
		return candidate
	}
	sh, sm, ok1 := parseClock(task.TimeStart)
	eh, em, ok2 := parseClock(task.TimeEnd)
	if !ok1 || !ok2 {
		return candidate
	}

	startMin := sh*60 + sm
	endMin := eh*60 + em
	curMin := candidate.Hour()*60 + candidate.Minute()
	inside := false
	if startMin <= endMin {
		inside = curMin >= startMin && curMin <= endMin
	} else {
		// 跨午夜窗口，例如 22:00-06:00
		inside = curMin >= startMin || curMin <= endMin
	}
	if inside {
		return candidate
	}

	next := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), sh, sm, 0, 0, candidate.Location())
	if !next.After(candidate) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
