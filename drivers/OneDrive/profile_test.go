package onedrive

import (
	"encoding/json"
	"testing"
)

func TestAccountProfilePayload(t *testing.T) {
	var drive struct {
		Owner struct {
			User struct {
				ID          string `json:"id"`
				DisplayName string `json:"displayName"`
			} `json:"user"`
		} `json:"owner"`
		Quota struct {
			Total int64 `json:"total"`
			Used  int64 `json:"used"`
		} `json:"quota"`
	}
	if err := json.Unmarshal([]byte(`{"owner":{"user":{"id":"u1","displayName":"OneDrive 用户"}},"quota":{"total":100,"used":40}}`), &drive); err != nil {
		t.Fatal(err)
	}
	if drive.Owner.User.DisplayName != "OneDrive 用户" || drive.Quota.Total != 100 || drive.Quota.Used != 40 {
		t.Fatalf("账号资料解析错误: %+v", drive)
	}
}
