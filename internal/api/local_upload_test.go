package api

import (
	"os"
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

func TestResolveLocalUploadSourceRejectsEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside.mkv")
	if err := os.WriteFile(inside, []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "outside.mkv")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "outside-link.mkv")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveLocalUploadSource(inside, root)
	if err != nil {
		t.Fatalf("映射目录内文件被错误拒绝: %v", err)
	}
	want, err := filepath.EvalSymlinks(inside)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != want {
		t.Fatalf("inside resolved=%q want %q", resolved, want)
	}
	if _, err := resolveLocalUploadSource(link, root); err == nil {
		t.Fatal("指向映射目录外的链接未被拒绝")
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
