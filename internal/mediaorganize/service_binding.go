package mediaorganize

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/mediaorganize/rules"
)

// ApplyBindingToPlan 用用户选择的 TMDB 影片就地更新当前计划中该组的动作，
// 不落库、不整树重新生成；重新生成计划时恢复自动识别。
func (s *Service) ApplyBindingToPlan(ctx context.Context, taskID, groupUID, tmdbID string) (*Plan, error) {
	plan, err := s.loadPlan(taskID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, nil
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.loadTaskConfig(task)
	if err != nil {
		return nil, err
	}
	marker := stringFromAny(cfg["rename_marker"])
	settingsDict := SettingsDict(s.settings)
	tagOrder := bindingTagOrder(settingsDict)
	tmdbLang := stringFromAny(settingsDict["mo_tmdb_language"])

	var indexes []int
	mediaKind := ""
	for i := range plan.Actions {
		md := plan.Actions[i].Metadata
		if md == nil {
			continue
		}
		if fmt.Sprint(md["group_uid"]) != groupUID {
			continue
		}
		indexes = append(indexes, i)
		if mediaKind == "" {
			mediaKind = fmt.Sprint(md["media_kind"])
		}
	}
	if len(indexes) == 0 {
		return nil, domain.Errorf(domain.CodeValidation, "当前计划中未找到该作品组")
	}

	results, err := s.SearchTMDB(ctx, tmdbID, nil, "", mediaKind)
	if err != nil || len(results) == 0 {
		return nil, domain.Errorf(domain.CodeDriverError, "未找到 TMDB 影片 %s", tmdbID)
	}
	var raw map[string]any
	if err := json.Unmarshal(results[0], &raw); err != nil || len(raw) == 0 {
		return nil, domain.Errorf(domain.CodeDriverError, "TMDB 影片信息解析失败")
	}
	id, tTitle, tOriginal, tYear := rules.ExtractTMDBDisplayFields(raw, mediaKind)
	if id == "" {
		return nil, domain.Errorf(domain.CodeDriverError, "TMDB 影片信息缺失")
	}
	tmdbID = id
	shortTitle := tTitle
	if shortTitle == "" {
		shortTitle = tOriginal
	}
	if shortTitle == "" {
		shortTitle = "未命名"
	}
	displayTitle := rules.BuildDisplayTitle(tTitle, tOriginal, shortTitle)

	applyBindingToPlanActions(plan, groupUID, tmdbID, shortTitle, displayTitle, tYear, marker, tagOrder, tmdbLang)
	if err := s.savePlan(taskID, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

// applyBindingToPlanActions 就地更新计划中指定组的动作（纯计算，便于测试）。
func applyBindingToPlanActions(plan *Plan, groupUID, tmdbID, shortTitle, displayTitle string, tYear *int, marker string, tagOrder []string, tmdbLang string) {
	var indexes []int
	for i := range plan.Actions {
		md := plan.Actions[i].Metadata
		if md == nil {
			continue
		}
		if fmt.Sprint(md["group_uid"]) != groupUID {
			continue
		}
		indexes = append(indexes, i)
	}
	for _, i := range indexes {
		a := &plan.Actions[i]
		md := a.Metadata
		md["tmdb_id"] = tmdbID
		md["title"] = shortTitle
		season := bindingMetaInt(md["season"])
		episode := bindingMetaInt(md["episode"])

		if fmt.Sprint(md["kind_label"]) == "dir_rename" {
			folderName := rules.SanitizeFilename(rules.BuildFolderName(rules.ParsedMedia{
				Title: shortTitle,
				Year:  tYear,
			}, tmdbID))
			if folderName != "" {
				a.TargetName = folderName
				md["group_new_dir_name"] = folderName
			}
		} else {
			parsed := rules.NormalizeParsedMedia(rules.ParseFilenameStrict(a.SourceName))
			mediaTag := rules.BuildMediaInfoTags(parsed, tagOrder)
			ext := rules.FileExtension(a.SourceName)
			base := rules.SanitizeFilename(rules.BuildTargetFilename(rules.ParsedMedia{
				Title:   displayTitle,
				Year:    tYear,
				Season:  season,
				Episode: episode,
			}, marker, tmdbID))
			if special := rules.ExtractSpecialLabel(a.SourceName); special != "" && !strings.Contains(displayTitle, special) {
				base += " " + special
			}
			if part := rules.ExtractPartLabel(a.SourceName); part != "" {
				base += " " + part
			}
			if name := bindingComposeFilename(base, mediaTag, ext); name != "" {
				a.TargetName = rules.FitFilenameBytes(name, tmdbLang)
			}
		}
		a.Reason = fmt.Sprintf("手动匹配 | %s -> %s | tmdb-%s", a.SourceName, a.TargetName, tmdbID)
		a.Confidence = 0.99
	}

	// 从 needs_match 移除已绑定的组。
	switch needs := plan.Diagnostics["needs_match"].(type) {
	case []map[string]any:
		out := needs[:0]
		for _, entry := range needs {
			if fmt.Sprint(entry["group_uid"]) != groupUID {
				out = append(out, entry)
			}
		}
		plan.Diagnostics["needs_match"] = out
	case []any:
		out := make([]map[string]any, 0, len(needs))
		for _, item := range needs {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if fmt.Sprint(entry["group_uid"]) != groupUID {
				out = append(out, entry)
			}
		}
		plan.Diagnostics["needs_match"] = out
	}
}

func bindingTagOrder(settingsDict map[string]any) []string {
	raw := stringFromAny(settingsDict["mo_media_tag_order"])
	if raw == "" {
		return rules.DefaultMediaTagOrder
	}
	var order []string
	if err := json.Unmarshal([]byte(raw), &order); err != nil {
		return rules.DefaultMediaTagOrder
	}
	return order
}

func bindingMetaInt(v any) *int {
	if v == nil {
		return nil
	}
	switch n := v.(type) {
	case float64:
		if n == 0 {
			return nil
		}
		iv := int(n)
		return &iv
	case int:
		if n == 0 {
			return nil
		}
		return &n
	case string:
		if n == "" || n == "0" {
			return nil
		}
		var iv int
		if _, err := fmt.Sscanf(n, "%d", &iv); err == nil && iv != 0 {
			return &iv
		}
	}
	return nil
}

func bindingComposeFilename(base, mediaTag, ext string) string {
	if mediaTag != "" {
		if ext != "" {
			return base + " " + mediaTag + "." + ext
		}
		return base + " " + mediaTag
	}
	if ext != "" {
		return base + "." + ext
	}
	return base
}
