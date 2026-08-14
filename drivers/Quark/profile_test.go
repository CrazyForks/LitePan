package quark

import (
	"encoding/json"
	"testing"
)

func TestQuarkMembership(t *testing.T) {
	cases := map[string]string{
		"NORMAL":    "",
		"VIP":       "VIP",
		"SUPER_VIP": "SVIP",
		"Z_VIP":     "SVIP+",
		"EXP_SVIP":  "88VIP",
		"MINI_VIP":  "迷你 VIP",
		"unknown":   "",
	}
	for input, want := range cases {
		if got := quarkMembership(input); got != want {
			t.Errorf("quarkMembership(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestProfileAccountInfoEnvelope(t *testing.T) {
	var envelope struct {
		Success bool               `json:"success"`
		Data    profileAccountInfo `json:"data"`
		Code    string             `json:"code"`
	}
	data := []byte(`{"success":true,"data":{"nickname":"JN","uid":"123"},"code":"OK"}`)
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Success || envelope.Data.Nickname != "JN" || jsonID(envelope.Data.UID) != "123" {
		t.Fatalf("账号接口解析错误: %+v", envelope)
	}
}
