package automation

import (
	"context"
	"strings"
	"time"

	"litepan/internal/domain"
)

const schedulerInterval = 10 * time.Second

func (s *Service) Start(ctx context.Context) {
	if s == nil || s.rules == nil {
		return
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.appCtx = ctx
	s.mu.Unlock()
	go s.schedulerLoop(ctx)
}

func (s *Service) schedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(schedulerInterval)
	defer ticker.Stop()
	s.scheduleOnce(ctx)
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
	rules, err := s.rules.List(ctx, false)
	if err != nil {
		s.log.Warn("automation scheduler list failed", "err", err)
		return
	}
	now := time.Now()
	for _, rule := range rules {
		if rule == nil || rule.Status != domain.AutomationStatusRunning {
			continue
		}
		computed := computeNextRun(rule.TriggerType, decodeMap(rule.TriggerConfig), now)
		nextRun := computed
		if !rule.NextRunAt.IsZero() {
			nextRun = rule.NextRunAt
		}
		if nextRun.IsZero() || nextRun.After(now) {
			continue
		}
		rule.NextRunAt = computed
		if err := s.rules.Update(ctx, rule); err != nil {
			s.log.Warn("automation schedule update next run failed", "rule_id", rule.ID, "err", err)
			continue
		}
		s.submitRun(rule.ID, "schedule", true)
	}
}

func computeNextRun(triggerType string, cfg map[string]any, base time.Time) time.Time {
	switch triggerType {
	case domain.AutomationTriggerDaily:
		h, m := parseClock(anyString(cfg["time"]))
		next := time.Date(base.Year(), base.Month(), base.Day(), h, m, 0, 0, base.Location())
		if !next.After(base) {
			next = next.Add(24 * time.Hour)
		}
		return next
	case domain.AutomationTriggerInterval:
		h, m := parseClock(anyString(cfg["start_time"]))
		interval := clampInt(anyInt(cfg["interval_hours"]), 1, 24*365)
		next := time.Date(base.Year(), base.Month(), base.Day(), h, m, 0, 0, base.Location())
		for !next.After(base) {
			next = next.Add(time.Duration(interval) * time.Hour)
		}
		return next
	default:
		return time.Time{}
	}
}

func parseClock(text string) (int, int) {
	parts := strings.Split(strings.TrimSpace(text), ":")
	if len(parts) != 2 {
		return 0, 0
	}
	return clampInt(anyInt(parts[0]), 0, 23), clampInt(anyInt(parts[1]), 0, 59)
}
