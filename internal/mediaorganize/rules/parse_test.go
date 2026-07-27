package rules

import "testing"

func TestParseDirAndFileNames(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		useFile   bool
		wantTitle string
		wantYear  *int
	}{
		{
			name:      "anime bracket dir",
			input:     "[4K][DBD-Raws&诸神字幕组][千与千寻][2160P][BDRip][简繁中日内封][FLAC].mkv",
			wantTitle: "千与千寻",
		},
		{
			name:      "chinese quality dir",
			input:     "千与千寻 蓝光原盘REMUX 国日双音 内封简日字幕",
			wantTitle: "千与千寻",
		},
		{
			name:      "bracket movie file",
			input:     "[爱乐之城 La La Land 2016][DIY简繁双语特效字幕][bb@HDSky][46.36GB].iso",
			useFile:   true,
			wantTitle: "爱乐之城 La La Land",
			wantYear:  intPtr(2016),
		},
		{
			name:      "simple chinese dir",
			input:     "暗战",
			wantTitle: "暗战",
		},
		{
			name:      "title with year paren",
			input:     "731 (2025)",
			wantTitle: "731",
			wantYear:  intPtr(2025),
		},
		{
			name:      "bare episode file",
			input:     "01.mp4",
			useFile:   true,
			wantTitle: "",
		},
		{
			name:      "bare episode with quality",
			input:     "01 4K.mp4",
			useFile:   true,
			wantTitle: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ParsedMedia
			if tt.useFile {
				got = NormalizeParsedMedia(ParseFilenameStrict(tt.input))
			} else {
				got = NormalizeParsedMedia(ParseDirName(tt.input))
			}
			if got.Title != tt.wantTitle {
				t.Fatalf("title = %q, want %q (full=%+v)", got.Title, tt.wantTitle, got)
			}
			if !intPtrEqual(got.Year, tt.wantYear) {
				t.Fatalf("year = %v, want %v (full=%+v)", got.Year, tt.wantYear, got)
			}
			if tt.name == "bare episode file" {
				if got.Episode == nil || *got.Episode != 1 {
					t.Fatalf("episode = %v, want 1 (full=%+v)", got.Episode, got)
				}
			}
			if tt.name == "bare episode with quality" {
				if got.Episode == nil || *got.Episode != 1 {
					t.Fatalf("episode = %v, want 1 (full=%+v)", got.Episode, got)
				}
				if got.ScreenSize != "2160p" {
					t.Fatalf("screen_size = %q, want 2160p (full=%+v)", got.ScreenSize, got)
				}
			}
		})
	}
}

func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func TestParseFilenameWithStackedEquivalentEpisodeMarkers(t *testing.T) {
	for _, tt := range []struct {
		name    string
		episode int
	}{
		{"中国奇谭 - S01E06 - 第 6 集.mkv", 6},
		{"中国奇谭 - S01E07 - 第 7 集.mkv", 7},
	} {
		got := NormalizeParsedMedia(ParseFilenameStrict(tt.name))
		if got.Title != "中国奇谭" || got.Season == nil || *got.Season != 1 || got.Episode == nil || *got.Episode != tt.episode {
			t.Fatalf("堆叠集号文件名解析不完整：name=%q got=%+v", tt.name, got)
		}
	}
}
