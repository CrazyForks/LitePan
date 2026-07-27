package mediaorganize

func NormalizeTaskConfig(config map[string]any) map[string]any {
	defaults := map[string]any{
		"task_name":              "",
		"account_id":             "",
		"target_directory":       "",
		"target_directory_id":    "",
		"file_extensions":        "mkv;mp4;avi;ts;mov;wmv;iso;m2ts;rmvb;flv;m4v;webm",
		"metadata_extensions":    "nfo;ass;ssa;srt;sub;idx;sup;vtt;jpg;jpeg;png;webp;bmp",
		"action_type":            "move",
		"target_root":            "",
		"target_root_id":         "",
		"media_type":             "auto",
		"rename_marker":          "",
		"movie_template":         "{title} ({year}) {tmdb-{tmdb_id}} [{video_info}]",
		"episode_template":       "{title} ({year}) {tmdb-{tmdb_id}} S{season:02d}E{episode:02d} [{video_info}]",
		"season_folder_template": "Season {season:02d}",
		"use_tmdb":               true,
		"overwrite_existing":     false,
		"recursive":              true,
	}
	if config == nil {
		return defaults
	}
	for key := range defaults {
		if val, ok := config[key]; ok {
			defaults[key] = val
		}
	}
	return defaults
}
