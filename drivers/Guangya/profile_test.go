package guangya

import "testing"

func TestGuangyaMembership(t *testing.T) {
	tests := []struct {
		name       string
		vip, svip  int
		membership string
	}{
		{name: "普通账号", membership: ""},
		{name: "有效 VIP", vip: 2, membership: "VIP"},
		{name: "有效 SVIP 状态", svip: 2, membership: "VIP"},
		{name: "已过期", vip: 3, membership: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := guangyaMembership(tt.vip, tt.svip); got != tt.membership {
				t.Fatalf("guangyaMembership(%d, %d) = %q, want %q", tt.vip, tt.svip, got, tt.membership)
			}
		})
	}
}
