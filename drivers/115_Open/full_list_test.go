package pan115open

import (
	"testing"

	"litepan/internal/driver"
)

func TestBuildDirPathAppendsSelfName(t *testing.T) {
	// 115 get_info 的 paths 是父目录链，不含目录自身；目录名来自 data.file_name。
	got := buildDirPath([]dirPathEntry{
		{FileID: flexString("0"), FileName: "根目录"},
		{FileID: flexString("100"), FileName: "库"},
		{FileID: flexString("200"), FileName: "电影"},
	}, "阿凡达")
	if got != "库/电影/阿凡达" {
		t.Fatalf("路径应包含目录自身，实际=%q", got)
	}
}

func TestBuildDirPathEdgeCases(t *testing.T) {
	if got := buildDirPath(nil, "电影"); got != "电影" {
		t.Fatalf("无父链时只应返回自身名，实际=%q", got)
	}
	if got := buildDirPath(nil, ""); got != "" {
		t.Fatalf("全空时应返回空串，实际=%q", got)
	}
	if got := buildDirPath([]dirPathEntry{{FileID: flexString("0"), FileName: "根目录"}}, "电影"); got != "电影" {
		t.Fatalf("根段应被跳过，实际=%q", got)
	}
}

var _ = driver.FullListLister((*Driver)(nil))
