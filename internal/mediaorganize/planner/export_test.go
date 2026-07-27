package planner

import (
	"litepan/internal/domain"
	"litepan/internal/mediaorganize/moplan"
	"litepan/internal/mediaorganize/rules"
)

// DetectSameWorkDirConflicts 暴露冲突合并逻辑供单测使用。
func DetectSameWorkDirConflicts(p *Planner) {
	p.detectSameWorkDirConflicts()
}

// BatchEntryForTest 是 batchEntry 的测试导出形态。
type BatchEntryForTest struct {
	Item      domain.FileItem
	Ancestors []rules.Ancestor
}

// GroupEntriesForTestExport 暴露分组逻辑供单测使用。
func GroupEntriesForTestExport(p *Planner, entries []BatchEntryForTest) (map[GroupKeyForTest]int, []PendingSkipForTest) {
	internal := make([]batchEntry, len(entries))
	for i, e := range entries {
		internal[i] = batchEntry{item: e.Item, ancestors: e.Ancestors}
	}
	groups, pending := p.groupEntries(internal)
	out := make(map[GroupKeyForTest]int, len(groups))
	for key, items := range groups {
		out[GroupKeyForTest{
			MediaKind: key.mediaKind,
			DirID:     key.dirID,
			DirName:   key.dirName,
			Title:     key.title,
			Year:      key.yearPtr(),
		}] = len(items)
	}
	pendingOut := make([]PendingSkipForTest, len(pending))
	for i, ps := range pending {
		pendingOut[i] = PendingSkipForTest{Reason: ps.reason}
	}
	return out, pendingOut
}

// GroupKeyForTest 导出分组键供断言。
type GroupKeyForTest struct {
	MediaKind string
	DirID     string
	DirName   string
	Title     string
	Year      *int
}

// PendingSkipForTest 导出跳过项供断言。
type PendingSkipForTest struct {
	Reason string
}

// SetActions 供单测注入动作列表。
func (p *Planner) SetActions(actions []moplan.PlanAction) {
	p.actions = append([]moplan.PlanAction(nil), actions...)
}

// Actions 返回当前动作列表。
func (p *Planner) Actions() []moplan.PlanAction {
	return append([]moplan.PlanAction(nil), p.actions...)
}

// SetScannedDirNames 供单测注入目录名索引。
func (p *Planner) SetScannedDirNames(names map[string]string) {
	for k, v := range names {
		p.scannedDirNames[k] = v
	}
}
