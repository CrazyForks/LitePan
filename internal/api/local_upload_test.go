package api

import (
	"path/filepath"
	"testing"
)

func TestCleanRelativePath(t *testing.T) {
	cases := map[string]string{
		"媒体库/电影.mkv":       "媒体库/电影.mkv",
		"/媒体库/电影.mkv":      "媒体库/电影.mkv",
		"媒体库\\电影.mkv":      "媒体库/电影.mkv",
		"../秘密/偷跑.mkv":     "",
		"../../etc/passwd": "",
		"媒体库/../电影":        "电影",
		"":                 "",
		"/":                "",
	}
	for in, want := range cases {
		if got := cleanRelativePath(in); got != want {
			t.Errorf("cleanRelativePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsWithinRoot(t *testing.T) {
	root := filepath.Clean("/app/data/updatefiles")
	ok := []string{
		"/app/data/updatefiles",
		"/app/data/updatefiles/媒体库/电影.mkv",
	}
	for _, p := range ok {
		if !isWithinRoot(p, root) {
			t.Errorf("应在映射根内: %s", p)
		}
	}
	bad := []string{
		"/app/data/updatefiles2/电影.mkv",
		"/app/data/other/电影.mkv",
		"/etc/passwd",
	}
	for _, p := range bad {
		if isWithinRoot(p, root) {
			t.Errorf("不应在映射根内: %s", p)
		}
	}
}

func TestSystemJunkFilter(t *testing.T) {
	junkFiles := []string{".DS_Store", ".localized", "Thumbs.db", "desktop.ini", "._photo.jpg", "._.DS_Store"}
	for _, f := range junkFiles {
		if !isSystemJunkFile(f) {
			t.Errorf("应判定为系统垃圾文件: %s", f)
		}
	}
	if isSystemJunkFile("movie.mkv") || isSystemJunkFile("poster.jpg") {
		t.Error("普通文件不应被判定为垃圾文件")
	}
	junkDirs := []string{"__MACOSX", ".Spotlight-V100", ".Trashes", ".fseventsd", "$RECYCLE.BIN", "System Volume Information", ".Trash-1000"}
	for _, d := range junkDirs {
		if !isSystemJunkDir(d) {
			t.Errorf("应判定为系统垃圾目录: %s", d)
		}
	}
	if isSystemJunkDir("电影") || isSystemJunkDir("Season 1") {
		t.Error("普通目录不应被判定为垃圾目录")
	}
}
