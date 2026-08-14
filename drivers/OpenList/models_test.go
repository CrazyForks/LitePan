package openlist

import (
	"testing"
)

func TestNormalizePathKeepsRequestsInsideRoot(t *testing.T) {
	d := &Driver{add: Addition{RootPath: "/媒体/../媒体"}}
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "根目录别名", input: "root", want: "/媒体"},
		{name: "相对路径", input: "电影/示例.mkv", want: "/媒体/电影/示例.mkv"},
		{name: "根目录内绝对路径", input: "/媒体/电影/示例.mkv", want: "/媒体/电影/示例.mkv"},
		{name: "相对路径越界", input: "../私有", wantErr: true},
		{name: "绝对路径越界", input: "/私有/示例.mkv", wantErr: true},
		{name: "清理后越界", input: "/媒体/../私有/示例.mkv", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.normalizePath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizePath(%q) 未拒绝越界路径，得到 %q", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizePath(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizePath(%q)=%q want %q", tt.input, got, tt.want)
			}
		})
	}
}
