package api

import (
	"context"
	"net/http"
	"strconv"

	"litepan/internal/domain"
	"litepan/internal/settings"
)

const uploadConcurrencyMin = 1
const uploadConcurrencyMax = 5

func (h *Handler) uploadConcurrencyLimit(ctx context.Context) int {
	if h.settings == nil {
		return 3
	}
	v := h.settings.Int(settings.KeyUploadTaskConcurrency)
	if v < uploadConcurrencyMin {
		return 3
	}
	return v
}

func (h *Handler) getUploadRuntime(w http.ResponseWriter, r *http.Request) {
	writeOK(w, map[string]any{
		"concurrency":     h.uploadConcurrencyLimit(r.Context()),
		"concurrency_min": uploadConcurrencyMin,
		"concurrency_max": uploadConcurrencyMax,
	})
}

type updateUploadRuntimeReq struct {
	Concurrency int `json:"concurrency"`
}

func (h *Handler) updateUploadRuntime(w http.ResponseWriter, r *http.Request) {
	var req updateUploadRuntimeReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Concurrency < uploadConcurrencyMin || req.Concurrency > uploadConcurrencyMax {
		writeErr(w, domain.Errorf(domain.CodeValidation, "上传任务并发数必须是 %d-%d 之间的整数", uploadConcurrencyMin, uploadConcurrencyMax))
		return
	}
	h.applyUploadConcurrencyHotReload(r.Context(), &req.Concurrency)
	writeOK(w, map[string]any{"concurrency": req.Concurrency})
}

func (h *Handler) applyUploadConcurrencyHotReload(ctx context.Context, value *int) {
	if value == nil || h.uploads == nil {
		return
	}
	if h.settings != nil {
		_ = h.settings.Update(ctx, map[string]string{
			settings.KeyUploadTaskConcurrency: strconv.Itoa(*value),
		})
	}
	h.uploads.RefreshConcurrencyLimit(ctx)
}

func (h *Handler) applyUploadConcurrencyFromSettings(ctx context.Context, in map[string]string) {
	if _, ok := in[settings.KeyUploadTaskConcurrency]; !ok || h.uploads == nil {
		return
	}
	h.uploads.RefreshConcurrencyLimit(ctx)
}
