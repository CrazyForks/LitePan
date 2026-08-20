package api

import (
	"net/http"
)

// getAnnouncement 返回当前公告。未配置（enabled=false）或拉取失败/无内容时 item 为 null，
// 前端据此不弹窗；本接口本身不报错，保证公告不可用时后台无感。
func (h *Handler) getAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.announcement != nil) {
		return
	}
	enabled := h.announcement.Enabled()
	if !enabled {
		writeOK(w, map[string]any{"enabled": false, "item": nil})
		return
	}
	item, err := h.announcement.Fetch(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]any{"enabled": true, "item": item})
}
