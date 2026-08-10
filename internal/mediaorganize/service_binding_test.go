package mediaorganize

import (
	"strings"
	"testing"

	"litepan/internal/mediaorganize/moplan"
	"litepan/internal/mediaorganize/rules"
)

func TestApplyBindingToPlanActions(t *testing.T) {
	uid := "movie|d1|Unknowable 2020|Unknowable"
	year := 1999
	plan := &Plan{
		TaskID: "t1",
		Actions: []moplan.PlanAction{
			{
				ID:         "a1",
				Kind:       moplan.ActionKindRelocate,
				SourceID:   "d1",
				SourceName: "Unknowable 2020",
				TargetName: "Unknowable (2020) {tmdb-}",
				Metadata: map[string]any{
					"kind_label": "dir_rename",
					"group_uid":  uid,
					"tmdb_id":    "",
					"media_kind": "movie",
				},
			},
			{
				ID:         "a2",
				Kind:       moplan.ActionKindRelocate,
				SourceID:   "f1",
				SourceName: "Unknowable.2020.1080p.mkv",
				TargetName: "Unknowable (2020) 1080p.mkv",
				Metadata: map[string]any{
					"group_uid":  uid,
					"tmdb_id":    "",
					"media_kind": "movie",
					"season":     nil,
					"episode":    nil,
				},
			},
			{
				ID:         "a3",
				Kind:       moplan.ActionKindRelocate,
				SourceID:   "f2",
				SourceName: "Other.movie.mkv",
				TargetName: "Other.movie.mkv",
				Metadata: map[string]any{
					"group_uid": "movie|d2|Other|Other",
					"tmdb_id":   "",
				},
			},
		},
		Diagnostics: map[string]any{
			"needs_match": []any{
				map[string]any{"group_uid": uid, "title": "Unknowable"},
				map[string]any{"group_uid": "tv|x|Y|Z", "title": "Z"},
			},
		},
	}

	applyBindingToPlanActions(plan, uid, "603", "The Matrix", "The Matrix", &year, "tmdb", rules.DefaultMediaTagOrder, "zh-CN")

	dir := plan.Actions[0]
	if dir.TargetName != "The Matrix (1999) {tmdb-603}" {
		t.Fatalf("目录目标名错误: %q", dir.TargetName)
	}
	if dir.Metadata["tmdb_id"] != "603" {
		t.Fatalf("目录 tmdb_id 未更新: %v", dir.Metadata["tmdb_id"])
	}
	if dir.Metadata["group_new_dir_name"] != "The Matrix (1999) {tmdb-603}" {
		t.Fatalf("group_new_dir_name 未更新: %v", dir.Metadata["group_new_dir_name"])
	}
	if dir.Confidence != 0.99 {
		t.Fatalf("置信度应为 0.99: %v", dir.Confidence)
	}

	file := plan.Actions[1]
	if !strings.Contains(file.TargetName, "The Matrix (1999) {tmdb-603}") || !strings.HasSuffix(file.TargetName, "[1080p].mkv") {
		t.Fatalf("文件目标名错误: %q", file.TargetName)
	}
	if file.Metadata["tmdb_id"] != "603" {
		t.Fatalf("文件 tmdb_id 未更新: %v", file.Metadata["tmdb_id"])
	}

	if plan.Actions[2].TargetName != "Other.movie.mkv" {
		t.Fatalf("其他组动作被误改: %+v", plan.Actions[2])
	}

	needs, ok := plan.Diagnostics["needs_match"].([]map[string]any)
	if !ok || len(needs) != 1 || needs[0]["group_uid"] != "tv|x|Y|Z" {
		t.Fatalf("needs_match 未正确过滤: %#v (ok=%v)", plan.Diagnostics["needs_match"], ok)
	}
}

func TestApplyBindingToPlanActionsTVEpisode(t *testing.T) {
	uid := "tv|d1|灵笼 第2季|灵笼"
	year := 2019
	plan := &Plan{
		TaskID: "t2",
		Actions: []moplan.PlanAction{
			{
				ID:         "b1",
				Kind:       moplan.ActionKindRelocate,
				SourceID:   "f1",
				SourceName: "[BeanSub&FZSD][灵笼][01][1080P][x264].mkv",
				TargetName: "灵笼 1080P.mkv",
				Metadata: map[string]any{
					"group_uid":  uid,
					"media_kind": "tv",
					"tmdb_id":    "",
					"season":     float64(2),
					"episode":    float64(1),
				},
			},
		},
	}

	applyBindingToPlanActions(plan, uid, "91557", "灵笼", "灵笼 - Ling Long", &year, "tmdb", rules.DefaultMediaTagOrder, "zh-CN")

	ep := plan.Actions[0]
	if !strings.Contains(ep.TargetName, "灵笼 - Ling Long (2019) {tmdb-91557} S02E01") {
		t.Fatalf("剧集目标名错误: %q", ep.TargetName)
	}
	if ep.Metadata["tmdb_id"] != "91557" {
		t.Fatalf("剧集 tmdb_id 未更新: %v", ep.Metadata["tmdb_id"])
	}
}
