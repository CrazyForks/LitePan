package planner_test

import (
	"fmt"
	"testing"

	"litepan/internal/domain"
	"litepan/internal/mediaorganize/moplan"
	"litepan/internal/mediaorganize/planner"
	"litepan/internal/mediaorganize/rules"
)

func TestHanzawaCaseOrganizeUsesShowRootForPrefixedSeasonDirs(t *testing.T) {
	fs := &mockFS{dirs: map[string][]domain.FileItem{
		"root": {
			{ID: "show", Name: "半泽直树 {tmdb-555}", IsDir: true},
		},
		"show": {
			{ID: "season1", Name: "半泽直树 Season 1", IsDir: true},
			{ID: "season2", Name: "半泽直树 Season 2", IsDir: true},
		},
		"season1": {
			{ID: "s1e1", Name: "Hanzawa.Naoki.S01E01.一旦被整必定加倍奉还！.mkv"},
			{ID: "s1e2", Name: "Hanzawa.Naoki.S01E02.抖落上司的冤罪！要恶人加倍奉还.mkv"},
		},
		"season2": {
			{ID: "s2e1", Name: "Hanzawa.Naoki.S02E01.十倍奉还开始！.mkv"},
		},
	}}
	tmdb := &mockTMDB{
		lookupFn: func(id string) map[string]any {
			if id != "555" {
				return nil
			}
			return map[string]any{
				"id":             555,
				"name":           "半泽直树",
				"original_name":  "Hanzawa Naoki",
				"first_air_date": "2013-07-07",
			}
		},
	}

	p := newTestPlanner(fs, tmdb, "root")
	plan, err := p.Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Skipped) != 0 {
		t.Fatalf("skipped=%+v，期望修复后不再产生季目录拆组提示", plan.Skipped)
	}

	dirRenames := map[string]string{}
	seasonRenames := map[string]string{}
	fileMeta := map[string]map[string]any{}
	for _, action := range plan.Actions {
		switch action.Kind {
		case moplan.ActionKindRelocate:
			switch action.Metadata["kind_label"] {
			case "dir_rename":
				dirRenames[action.SourceID] = action.TargetName
			case "season_dir_rename":
				seasonRenames[action.SourceID] = action.TargetName
			default:
				fileMeta[action.SourceID] = action.Metadata
			}
		}
	}
	if got := dirRenames["show"]; got != "半泽直树 (2013) {tmdb-555}" {
		t.Fatalf("show dir rename=%q，期望半泽直树 (2013) {tmdb-555}；all=%v", got, dirRenames)
	}
	if got := seasonRenames["season1"]; got != "Season 01" {
		t.Fatalf("season1 dir rename=%q，期望 Season 01；all=%v", got, seasonRenames)
	}
	if got := seasonRenames["season2"]; got != "Season 02" {
		t.Fatalf("season2 dir rename=%q，期望 Season 02；all=%v", got, seasonRenames)
	}
	if len(fileMeta) != 3 {
		t.Fatalf("file relocates=%d，期望 3；actions=%+v", len(fileMeta), plan.Actions)
	}

	for _, fileID := range []string{"s1e1", "s1e2", "s2e1"} {
		meta := fileMeta[fileID]
		if meta == nil {
			t.Fatalf("%s 缺少 rename 动作，actions=%+v", fileID, plan.Actions)
		}
		if got := fmt.Sprint(meta["title"]); got != "半泽直树" {
			t.Fatalf("%s title=%q，期望半泽直树；meta=%+v", fileID, got, meta)
		}
		if got := fmt.Sprint(meta["tmdb_id"]); got != "555" {
			t.Fatalf("%s tmdb_id=%q，期望 555；meta=%+v", fileID, got, meta)
		}
	}
}

func TestHanzawaCaseGroupingMergesPrefixedSeasonDir(t *testing.T) {
	p := newTestPlanner(nil, nil, "root")
	showAnc := []rules.Ancestor{
		{ID: "show", Name: "半泽直树 {tmdb-555}"},
	}
	s1Anc := append(append([]rules.Ancestor(nil), showAnc...), rules.Ancestor{ID: "season1", Name: "半泽直树 Season 1"})
	s2Anc := append(append([]rules.Ancestor(nil), showAnc...), rules.Ancestor{ID: "season2", Name: "半泽直树 Season 2"})

	entries := []planner.BatchEntryForTest{
		{
			Item:      domain.FileItem{ID: "s1e1", Name: "Hanzawa.Naoki.S01E01.一旦被整必定加倍奉还！.mkv"},
			Ancestors: s1Anc,
		},
		{
			Item:      domain.FileItem{ID: "s2e1", Name: "Hanzawa.Naoki.S02E01.十倍奉还开始！.mkv"},
			Ancestors: s2Anc,
		},
	}

	groups, skips := planner.GroupEntriesForTestExport(p, entries)
	if len(skips) != 0 {
		t.Fatalf("skips=%+v", skips)
	}
	if len(groups) != 1 {
		t.Fatalf("groups=%v，期望带剧名的季目录合并到同一部作品", groups)
	}
	for key, count := range groups {
		if key.DirID != "show" || key.DirName != "半泽直树 {tmdb-555}" || key.Title != "半泽直树" || count != 2 {
			t.Fatalf("group=%+v count=%d，不符合预期", key, count)
		}
	}
}
