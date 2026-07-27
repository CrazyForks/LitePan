package strm

import (
	"context"

	"litepan/internal/domain"
	"litepan/internal/eventbus"
)

func (s *Service) notifyScanFailures(task *domain.StrmTask, failures []ScanFailure) {
	if s == nil || s.bus == nil || task == nil || len(failures) == 0 {
		return
	}
	summary := scanFailureSummary(task.Name, failures)
	s.bus.Publish(context.Background(), eventbus.NotificationCreated{
		Level:     "warning",
		Category:  domain.NotificationCategoryStrmScanWarn,
		Title:     "STRM 扫描部分失败",
		Message:   EncodeScanFailureMessage(summary, failures),
		AccountID: task.AccountID,
		RefID:     task.ID,
	})
}
