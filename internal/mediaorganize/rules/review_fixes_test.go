package rules

import (
	"strings"
	"testing"
)

func TestBareNumericGuard(t *testing.T) {
	tests := []struct {
		input       string
		wantEpisode bool
	}{
		{"01.mkv", true},
		{"12.mkv", true},
		{"999.mkv", true},
		{"720.mkv", false},
		{"1080.mkv", false},
		{"2160.mkv", false},
		{"2012.mkv", false},
		{"1917.mkv", false},
		{"2001.mkv", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeParsedMedia(ParseFilenameStrict(tt.input))
			hasEp := got.Episode != nil
			if hasEp != tt.wantEpisode {
				t.Fatalf("episode = %v, wantEpisode=%v (full=%+v)", got.Episode, tt.wantEpisode, got)
			}
		})
	}
}

func TestBracketEpisodeNonAnime(t *testing.T) {
	got := NormalizeParsedMedia(ParseFilenameStrict("Breaking.Bad.[01].mkv"))
	if got.Episode == nil || *got.Episode != 1 {
		t.Fatalf("episode = %v, want 1 (full=%+v)", got.Episode, got)
	}
	if got.Season == nil || *got.Season != 1 {
		t.Fatalf("season = %v, want 1 (full=%+v)", got.Season, got)
	}
	if !strings.Contains(got.Title, "Breaking Bad") {
		t.Fatalf("title = %q, want contains Breaking Bad", got.Title)
	}
	for _, name := range []string{"Some.Movie.[1080].mkv", "Some.Movie.[2023].mkv"} {
		got := NormalizeParsedMedia(ParseFilenameStrict(name))
		if got.Episode != nil {
			t.Fatalf("%s: episode = %v, want nil", name, got.Episode)
		}
	}
}

func TestSeasonZeroEpisode(t *testing.T) {
	got := NormalizeParsedMedia(ParseFilenameStrict("Show.Name.S00E01.1080p.WEB-DL.mkv"))
	if got.Episode == nil || *got.Episode != 1 {
		t.Fatalf("episode = %v, want 1 (full=%+v)", got.Episode, got)
	}
	if got.Season == nil || *got.Season != 0 {
		t.Fatalf("season = %v, want 0 (full=%+v)", got.Season, got)
	}
}

func TestChineseEditionStrip(t *testing.T) {
	for _, tt := range []struct {
		input string
		deny  string
	}{
		{"误杀 加长版", "加长版"},
		{"银翼杀手 导演剪辑版", "导演剪辑"},
		{"泰坦尼克号 未删减版", "未删减"},
		{"天空之城 特典映像", "特典"},
	} {
		got := StripChineseQualityTags(tt.input)
		if strings.Contains(got, tt.deny) {
			t.Fatalf("StripChineseQualityTags(%q) = %q, 应剥离 %q", tt.input, got, tt.deny)
		}
	}
	if label := ExtractSpecialLabel("天空之城 特典01"); label == "" {
		t.Fatal("特典 应识别为特殊内容标签")
	}
}

func TestEnglishQualityTagBoundaries(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
	}{
		{input: "Paddington DD 5.1", want: "Paddington"},
		{input: "Submarine Subs", want: "Submarine"},
		{input: "Dredd 1080p BluRay DD+", want: "Dredd"},
		{input: "Uncut Gems", want: "Uncut Gems"},
		{input: "Extended Family", want: "Extended Family"},
		{input: "Movie Extended 1080p", want: "Movie"},
		{input: "Movie.Director's.Cut.BluRay", want: "Movie"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			if got := StripChineseQualityTags(tt.input); got != tt.want {
				t.Fatalf("StripChineseQualityTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEnglishMovieTitlesSurviveFilenameParsing(t *testing.T) {
	for _, tt := range []struct {
		filename string
		title    string
		year     int
	}{
		{filename: "Paddington.2014.1080p.BluRay.DD.5.1.mkv", title: "Paddington", year: 2014},
		{filename: "Submarine.2010.1080p.WEB-DL.Subs.mkv", title: "Submarine", year: 2010},
		{filename: "Dredd.2012.2160p.REMUX.DDP.7.1.mkv", title: "Dredd", year: 2012},
		{filename: "Uncut.Gems.2019.1080p.BluRay.DTS.mkv", title: "Uncut Gems", year: 2019},
		{filename: "Extended.Family.2023.S01E01.1080p.WEB-DL.mkv", title: "Extended Family", year: 2023},
	} {
		t.Run(tt.filename, func(t *testing.T) {
			got := NormalizeParsedMedia(ParseFilenameStrict(tt.filename))
			if got.Title != tt.title || got.Year == nil || *got.Year != tt.year {
				t.Fatalf("解析结果=%+v，期望 title=%q year=%d", got, tt.title, tt.year)
			}
		})
	}
}

func TestDTSHDMAScan(t *testing.T) {
	out := map[string]any{}
	EnrichMediaTagsFromFilename("Movie.2020.1080p.BluRay.DTS-HD.MA.5.1.x264-GROUP.mkv", out)
	if got, _ := out["audio_codec"].(string); got != "DTS-HD MA" {
		t.Fatalf("audio_codec = %q, want DTS-HD MA (full=%v)", got, out)
	}
}

func TestExplicitIdentityYearFormats(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		dir         bool
		wantTitle   string
		wantYear    int
		wantSeason  int
		wantEpisode int
	}{
		{name: "括号点分信息", input: "电影名(1980.中国.国语.剧情).mkv", wantTitle: "电影名", wantYear: 1980},
		{name: "中文括号点分信息", input: "电影名（1980.中国.国语.剧情）.mkv", wantTitle: "电影名", wantYear: 1980},
		{name: "括号空格信息", input: "电影名 (1980 中国 国语 剧情).mkv", wantTitle: "电影名", wantYear: 1980},
		{name: "十八世纪年份", input: "早期电影(1895.法国.默片.纪录).mkv", wantTitle: "早期电影", wantYear: 1895},
		{name: "纯年份片名另有年份", input: "2012(2019.美国.英语.灾难).mkv", wantTitle: "2012", wantYear: 2019},
		{name: "纯年份片名", input: "2012.mkv", wantTitle: "2012"},
		{name: "片名粘连年份数字", input: "赌侠1999.mkv", wantTitle: "赌侠1999"},
		{name: "片名粘连数字另有年份", input: "赌侠1999(1998.中国.粤语.喜剧).mkv", wantTitle: "赌侠1999", wantYear: 1998},
		{name: "片名开头年份另有年份", input: "2001太空漫游(1968.美国.英语.科幻).mkv", wantTitle: "2001太空漫游", wantYear: 1968},
		{name: "点分双年份", input: "1917.2019.1080p.BluRay.mkv", wantTitle: "1917", wantYear: 2019},
		{name: "点分扩展信息", input: "电影名.1980.中国.国语.剧情.mkv", wantTitle: "电影名", wantYear: 1980},
		{name: "电视剧单集", input: "剧名(2019.中国.国语.剧情).S01E02.mkv", wantTitle: "剧名", wantYear: 2019, wantSeason: 1, wantEpisode: 2},
		{name: "目录扩展信息", input: "电影名(1980.中国.国语.剧情)", dir: true, wantTitle: "电影名", wantYear: 1980},
		{name: "电视剧季度目录", input: "剧名 第2季(2024.中国.国语.剧情)", dir: true, wantTitle: "剧名", wantYear: 2024, wantSeason: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ParsedMedia
			if tt.dir {
				got = NormalizeParsedMedia(ParseDirName(tt.input))
			} else {
				got = NormalizeParsedMedia(ParseFilenameStrict(tt.input))
			}
			if got.Title != tt.wantTitle || intValue(got.Year) != tt.wantYear ||
				intValue(got.Season) != tt.wantSeason || intValue(got.Episode) != tt.wantEpisode {
				t.Fatalf("解析结果 = %+v，期望 title=%q year=%d season=%d episode=%d", got, tt.wantTitle, tt.wantYear, tt.wantSeason, tt.wantEpisode)
			}
		})
	}
}

func TestYirenRootScatterAmbiguous(t *testing.T) {
	showAnc := []Ancestor{{ID: "show", Name: "一人之下"}}
	catAnc := append(append([]Ancestor(nil), showAnc...), Ancestor{ID: "cat", Name: "前五季+番外+剧场版"})
	s1Anc := append(append([]Ancestor(nil), catAnc...), Ancestor{ID: "s1", Name: "第1季（2016）4K"})
	s2Anc := append(append([]Ancestor(nil), catAnc...), Ancestor{ID: "s2", Name: "第2季（2017）4K"})

	entries := []ScanEntry{
		{FileName: "01 4K.mp4", Ancestors: showAnc},
		{FileName: "02 4K.mp4", Ancestors: showAnc},
		{FileName: "01.mp4", Ancestors: s1Anc},
		{FileName: "01.mp4", Ancestors: s2Anc},
	}
	layout := AnalyzeTVTreeLayout(entries)
	if !layout["show"].HasMultiSeason {
		t.Fatalf("layout should detect multi season: %+v", layout["show"])
	}

	fp := PrepareTVFileParsed(NormalizeParsedMedia(ParseFilenameStrict("01 4K.mp4")), showAnc)
	if !IsBareEpisodeLikeFilename("01 4K.mp4", fp) {
		t.Fatal("01 4K.mp4 should look like bare episode file")
	}
	if !IsAmbiguousRootTVScatter(showAnc, layout, "show") {
		t.Fatal("root scatter should be ambiguous")
	}

	s1fp := PrepareTVFileParsed(NormalizeParsedMedia(ParseFilenameStrict("01.mp4")), s1Anc)
	if s1fp.Episode == nil || *s1fp.Episode != 1 {
		t.Fatalf("season folder 01.mp4 episode = %v", s1fp.Episode)
	}
	if s1fp.Season == nil || *s1fp.Season != 1 {
		t.Fatalf("season folder 01.mp4 season = %v", s1fp.Season)
	}
	if IsAmbiguousRootTVScatter(s1Anc, layout, "show") {
		t.Fatal("file inside season folder should not be ambiguous scatter")
	}
}

func intValue(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
