package strmscrape

import (
	"path/filepath"
	"testing"

	"litepan/internal/mediaorganize/rules"
)

func TestHanzawaCasePrefixedSeasonDirCollapsesToShowWorkRoot(t *testing.T) {
	root := t.TempDir()
	show := filepath.Join(root, "半泽直树 {tmdb-555}")
	s1 := filepath.Join(show, "半泽直树 Season 1")
	s2 := filepath.Join(show, "半泽直树 Season 2")
	mustMkdir(t, s1)
	mustMkdir(t, s2)
	mustWrite(t, filepath.Join(s1, "Hanzawa.Naoki.S01E01.一旦被整必定加倍奉还！.strm"), "x")
	mustWrite(t, filepath.Join(s2, "Hanzawa.Naoki.S02E01.十倍奉还开始！.strm"), "x")

	if !rules.IsSeasonDirName("半泽直树 Season 1") {
		t.Fatal("带剧名的季目录应被识别为季目录")
	}

	works, err := scanWorks(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(works) != 1 {
		t.Fatalf("works=%d，期望季目录折叠后合并为 1 组；groups=%+v", len(works), works)
	}

	if got := workDisplayName(works[0]); got != "半泽直树 {tmdb-555}" {
		t.Fatalf("work folder=%q，期望半泽直树 {tmdb-555}", got)
	}
	item := buildItem(1, root, works[0])
	if item.Title != "半泽直树" {
		t.Fatalf("item.title=%q，期望半泽直树", item.Title)
	}
	if item.TMDBID != "555" {
		t.Fatalf("item.tmdb_id=%q，期望 555", item.TMDBID)
	}
	if item.MediaType != MediaTypeTV {
		t.Fatalf("item.media_type=%q，期望 tv", item.MediaType)
	}
	if item.FileCount != 2 {
		t.Fatalf("item.file_count=%d，期望 2", item.FileCount)
	}
}
