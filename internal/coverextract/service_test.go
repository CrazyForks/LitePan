package coverextract

import "testing"

func TestExtractionTimes(t *testing.T) {
	tests := []struct {
		name     string
		req      ExtractRequest
		duration int64
		want     int
	}{
		{name: "默认均匀五帧", req: ExtractRequest{Mode: "uniform"}, duration: 60_000, want: 5},
		{name: "片头片尾各取一帧", req: ExtractRequest{Mode: "head_tail"}, duration: 60_000, want: 2},
		{name: "极短视频避免首尾重复", req: ExtractRequest{Mode: "head_tail"}, duration: 1000, want: 1},
		{name: "精确时间", req: ExtractRequest{Mode: "timestamp", TimestampMS: 900}, duration: 1000, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractionTimes(tt.req, tt.duration)
			if err != nil {
				t.Fatalf("extractionTimes() error = %v", err)
			}
			if len(got) != tt.want {
				t.Fatalf("len = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestDefaultTargetUsesMediaOrganizeSeasonRules(t *testing.T) {
	for _, season := range []string{"Season 1", "Season 01", "S01", "第一季"} {
		id, path := defaultTarget("season-id", []DirectoryRef{
			{ID: "root-id", Name: "根目录"},
			{ID: "show-id", Name: "胜与败 (2025)"},
			{ID: "season-id", Name: season},
		})
		if id != "show-id" || path != "/胜与败 (2025)" {
			t.Fatalf("%s: id=%q path=%q", season, id, path)
		}
	}
	id, path := defaultTarget("movie-id", []DirectoryRef{{ID: "movie-id", Name: "电影"}})
	if id != "movie-id" || path != "/电影" {
		t.Fatalf("非季目录应保存到视频同目录: id=%q path=%q", id, path)
	}
}

func TestRemoveAlsoDropsCandidateImages(t *testing.T) {
	s := &Service{
		files: map[string]*SessionFile{
			"session-id": {ID: "session-id", Frames: []Frame{{ID: "frame-1"}, {ID: "frame-2"}}},
		},
		frames: map[string]*imageEntry{
			"frame-1": {Data: []byte("one")},
			"frame-2": {Data: []byte("two")},
		},
		imageLen: 6,
	}
	s.Remove("session-id")
	if len(s.files) != 0 || len(s.frames) != 0 || s.imageLen != 0 {
		t.Fatalf("移除视频后候选数据未清空: files=%d frames=%d bytes=%d", len(s.files), len(s.frames), s.imageLen)
	}
}

func TestCloneFileKeepsEmptyFramesAsArray(t *testing.T) {
	cloned := cloneFile(&SessionFile{})
	if cloned.Frames == nil {
		t.Fatal("空候选图必须保持为空数组，不能序列化为 null")
	}
}

func TestFrameMustBelongToSessionFile(t *testing.T) {
	file := &SessionFile{Frames: []Frame{{ID: "frame-1"}}}
	if !frameBelongsToFile(file, "frame-1") {
		t.Fatal("应识别归属当前视频的候选帧")
	}
	if frameBelongsToFile(file, "frame-2") {
		t.Fatal("不能用其它视频的候选帧保存海报")
	}
}

func TestSaveComposedRejectsInvalidSizeBeforeIO(t *testing.T) {
	s := &Service{}
	if _, err := s.SaveComposed(t.Context(), SaveRequest{}, nil); err == nil {
		t.Fatal("空合成海报应被拒绝")
	}
	tooLarge := make([]byte, MaxPosterBytes+1)
	if _, err := s.SaveComposed(t.Context(), SaveRequest{}, tooLarge); err == nil {
		t.Fatal("超过上限的合成海报应被拒绝")
	}
}

func TestExtractionTimesRejectsInvalidInput(t *testing.T) {
	if _, err := extractionTimes(ExtractRequest{Mode: "uniform", Count: 10}, 60_000); err == nil {
		t.Fatal("帧数超限应拒绝")
	}
	if _, err := extractionTimes(ExtractRequest{Mode: "timestamp", TimestampMS: 60_000}, 60_000); err == nil {
		t.Fatal("超过时长的时间点应拒绝")
	}
}

func TestLoopbackGuard(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:1234", "[::1]:1234"} {
		if !isLoopback(addr) {
			t.Fatalf("%s 应识别为回环", addr)
		}
	}
	if isLoopback("192.168.1.2:1234") {
		t.Fatal("局域网地址不应通过回环检查")
	}
}

func TestSupportedVideoExtensions(t *testing.T) {
	for _, name := range []string{"a.mp4", "A.MKV", "a.mov", "a.webm"} {
		if !IsSupported(name) {
			t.Fatalf("%s 应支持", name)
		}
	}
	if IsSupported("a.avi") {
		t.Fatal("AVI 不在首版产品范围")
	}
}

func TestDurationPattern(t *testing.T) {
	match := durationPattern.FindStringSubmatch("Duration: 01:02:03.45, start: 0.000000")
	if len(match) != 4 || match[1] != "01" || match[2] != "02" || match[3] != "03.45" {
		t.Fatalf("时长解析不正确: %#v", match)
	}
}
