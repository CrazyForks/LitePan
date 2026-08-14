package cloud189

import "testing"

func TestSessionProfile(t *testing.T) {
	d := &Driver{loginName: "user@189.cn"}
	if profile := d.sessionProfile(); profile.Nickname != "user@189.cn" {
		t.Fatalf("登录名回退错误: %+v", profile)
	}
}
