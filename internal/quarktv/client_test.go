package quarktv

import (
	"testing"

	"litepan/internal/domain"
)

func TestPickStreamingCandidatePrefersDolbyOnlyWhenEnabled(t *testing.T) {
	infos := []streamingVideoInfo{
		{Resolution: "4k", Accessable: 1, Format: "mp4", URL: "https://example/4k"},
		{Resolution: "dolby_vision", Accessable: 1, Format: "matroska,webm", URL: "https://example/dv"},
	}

	gotOff, ok := pickStreamingCandidate("fid", infos, StreamingPreference{
		PreferredResolution: domain.QuarkTVResolutionAuto,
		AllowDolby:          false,
	}, nil)
	if !ok {
		t.Fatal("dolby 关闭时未选出候选")
	}
	if gotOff.Resolution != "4k" {
		t.Fatalf("dolby 关闭时应选 4k，实际为 %q", gotOff.Resolution)
	}

	gotOn, ok := pickStreamingCandidate("fid", infos, StreamingPreference{
		PreferredResolution: domain.QuarkTVResolutionAuto,
		AllowDolby:          true,
	}, nil)
	if !ok {
		t.Fatal("dolby 开启时未选出候选")
	}
	if gotOn.Resolution != "dolby_vision" {
		t.Fatalf("dolby 开启时应选 dolby_vision，实际为 %q", gotOn.Resolution)
	}
}

func TestPickStreamingCandidateRespectsResolutionCap(t *testing.T) {
	infos := []streamingVideoInfo{
		{Resolution: "4k", Accessable: 1, Format: "mp4", URL: "https://example/4k"},
		{Resolution: "super", Accessable: 1, Format: "mp4", URL: "https://example/super"},
		{Resolution: "high", Accessable: 1, Format: "mp4", URL: "https://example/high"},
	}

	got, ok := pickStreamingCandidate("fid", infos, StreamingPreference{
		PreferredResolution: domain.QuarkTVResolutionHigh,
		AllowDolby:          false,
	}, nil)
	if !ok {
		t.Fatal("受限清晰度下未选出候选")
	}
	if got.Resolution != "high" {
		t.Fatalf("清晰度上限为 high 时应选 high，实际为 %q", got.Resolution)
	}
}
