package api

import (
	"testing"
	"time"

	"litepan/internal/domain"
	"litepan/internal/strm"
)

// 仪表带的「运行计划」依赖后端下发的 next_run_at，这里固定 DTO 的输出格式与空值语义。
func TestStrmTaskDTONextRunAt(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, loc)
	h := &Handler{strm: &strm.Service{}}

	task := &domain.StrmTask{
		ID:           1,
		Name:         "电影 / 115 主库",
		Status:       domain.StrmStatusActive,
		ScheduleMode: domain.StrmScheduleWindow,
		ScanInterval: 360,
		LastScan:     time.Date(2026, 9, 11, 10, 30, 0, 0, loc),
	}

	next := nextRunAtString(h, task, now)
	if next == "" {
		t.Fatalf("启用的自动任务应返回 next_run_at")
	}
	want := FormatAPITime(time.Date(2026, 9, 11, 16, 30, 0, 0, loc))
	if next != want {
		t.Fatalf("期望 %s，实际 %s", want, next)
	}

	dto := toStrmTaskDTO(task, strm.TaskListMeta{}, false, next)
	if dto.NextRunAt != want {
		t.Fatalf("DTO 未带上 next_run_at：%q", dto.NextRunAt)
	}

	// 手动任务应输出空值，前端据此跳过
	task.ScheduleMode = domain.StrmScheduleManual
	if manual := nextRunAtString(h, task, now); manual != "" {
		t.Fatalf("手动任务不应有 next_run_at，实际 %q", manual)
	}
	if dtoManual := toStrmTaskDTO(task, strm.TaskListMeta{}, false, ""); dtoManual.NextRunAt != "" {
		t.Fatalf("手动任务的 DTO 应为空，实际 %q", dtoManual.NextRunAt)
	}
}
