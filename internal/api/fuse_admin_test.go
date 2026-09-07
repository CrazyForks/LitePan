package api

import (
	"testing"
)

// fromFuseMountDTO 必须透传 ReadOnly/Enabled —— 曾经是 `|| true` 恒真，
// 导致"只读/启用"永远无法保存 false。
func TestFromFuseMountDTOPreservesFlags(t *testing.T) {
	m, err := fromFuseMountDTO(fuseMountDTO{
		Name:      "test",
		AccountID: 7,
		ReadOnly:  false,
		Enabled:   false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.ReadOnly {
		t.Fatal("ReadOnly=false 被改写为 true")
	}
	if m.Enabled {
		t.Fatal("Enabled=false 被改写为 true")
	}
	if m.Name != "test" || m.AccountID != 7 {
		t.Fatalf("字段透传异常: %+v", m)
	}

	// 真值也应保持
	m2, err := fromFuseMountDTO(fuseMountDTO{Name: "test2", ReadOnly: true, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !m2.ReadOnly || !m2.Enabled {
		t.Fatalf("true 值被改写: %+v", m2)
	}
}
