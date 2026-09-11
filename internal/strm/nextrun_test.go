package strm

import (
	"testing"
	"time"

	"litepan/internal/domain"
)

func TestNextRunAt(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, loc)
	svc := &Service{}

	base := func() *domain.StrmTask {
		return &domain.StrmTask{
			ID:           1,
			Name:         "电影 / 115 主库",
			Status:       domain.StrmStatusActive,
			ScheduleMode: domain.StrmScheduleWindow,
			ScanInterval: 360,
		}
	}

	t.Run("手动调度不参与", func(t *testing.T) {
		task := base()
		task.ScheduleMode = domain.StrmScheduleManual
		if _, ok := svc.NextRunAt(task, now); ok {
			t.Fatalf("manual 任务不应参与自动调度")
		}
	})

	t.Run("未启用不参与", func(t *testing.T) {
		task := base()
		task.Status = domain.StrmStatusPaused
		if _, ok := svc.NextRunAt(task, now); ok {
			t.Fatalf("未启用任务不应参与自动调度")
		}
	})

	t.Run("执行中的任务不排入下一轮", func(t *testing.T) {
		task := base()
		task.Status = domain.StrmStatusRunning
		if _, ok := svc.NextRunAt(task, now); ok {
			t.Fatalf("执行中的任务不应出现在下一轮计划里")
		}
	})

	t.Run("从未扫描视为立即到期", func(t *testing.T) {
		next, ok := svc.NextRunAt(base(), now)
		if !ok {
			t.Fatalf("应参与调度")
		}
		if !next.Equal(now) {
			t.Fatalf("期望 %v，实际 %v", now, next)
		}
	})

	t.Run("到期时间等于上次扫描加间隔", func(t *testing.T) {
		task := base()
		task.LastScan = time.Date(2026, 9, 11, 10, 30, 0, 0, loc)
		next, _ := svc.NextRunAt(task, now)
		want := time.Date(2026, 9, 11, 16, 30, 0, 0, loc)
		if !next.Equal(want) {
			t.Fatalf("期望 %v，实际 %v", want, next)
		}
	})

	t.Run("已到期且在窗口内时返回当前时间（不返回过去时间）", func(t *testing.T) {
		task := base()
		task.LastScan = time.Date(2026, 9, 11, 2, 0, 0, 0, loc) // 10 小时前，间隔 6 小时，已到期
		next, _ := svc.NextRunAt(task, now)
		if !next.Equal(now) {
			t.Fatalf("期望 %v，实际 %v", now, next)
		}
	})

	t.Run("错过时间窗口后顺延到下一个窗口起点（不返回过去时间）", func(t *testing.T) {
		task := base()
		task.TimeWindowEnabled = true
		task.TimeStart = "20:00"
		task.TimeEnd = "23:00"
		task.LastScan = time.Date(2026, 9, 10, 10, 0, 0, 0, loc) // 到期于昨天 16:00，早已错过
		next, _ := svc.NextRunAt(task, now)
		want := time.Date(2026, 9, 11, 20, 0, 0, 0, loc)
		if !next.Equal(want) {
			t.Fatalf("期望 %v，实际 %v", want, next)
		}
		if next.Before(now) {
			t.Fatalf("不应返回过去时间")
		}
	})

	t.Run("任务级间隔回退", func(t *testing.T) {
		task := base()
		task.ScanInterval = 60
		task.LastScan = time.Date(2026, 9, 11, 11, 30, 0, 0, loc)
		next, _ := svc.NextRunAt(task, now)
		want := time.Date(2026, 9, 11, 12, 30, 0, 0, loc)
		if !next.Equal(want) {
			t.Fatalf("期望 %v，实际 %v", want, next)
		}
	})

	t.Run("窗口外顺延到下一个窗口起点", func(t *testing.T) {
		task := base()
		task.TimeWindowEnabled = true
		task.TimeStart = "20:00"
		task.TimeEnd = "23:00"
		task.LastScan = time.Date(2026, 9, 11, 18, 0, 0, 0, loc) // 到期 24:00，落在窗口外
		next, _ := svc.NextRunAt(task, now)
		want := time.Date(2026, 9, 12, 20, 0, 0, 0, loc)
		if !next.Equal(want) {
			t.Fatalf("期望 %v，实际 %v", want, next)
		}
	})

	t.Run("跨午夜窗口顺延到当天起点", func(t *testing.T) {
		task := base()
		task.TimeWindowEnabled = true
		task.TimeStart = "22:00"
		task.TimeEnd = "06:00"
		task.LastScan = time.Date(2026, 9, 11, 2, 0, 0, 0, loc) // 到期 08:00，落在窗口外
		next, _ := svc.NextRunAt(task, now)
		want := time.Date(2026, 9, 11, 22, 0, 0, 0, loc)
		if !next.Equal(want) {
			t.Fatalf("期望 %v，实际 %v", want, next)
		}
	})

	t.Run("窗口内保持原时间", func(t *testing.T) {
		task := base()
		task.TimeWindowEnabled = true
		task.TimeStart = "00:00"
		task.TimeEnd = "23:59"
		task.LastScan = time.Date(2026, 9, 11, 16, 0, 0, 0, loc) // 到期 22:00，在窗口内
		next, _ := svc.NextRunAt(task, now)
		want := time.Date(2026, 9, 11, 22, 0, 0, 0, loc)
		if !next.Equal(want) {
			t.Fatalf("期望 %v，实际 %v", want, next)
		}
	})
}
