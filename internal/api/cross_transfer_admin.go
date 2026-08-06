package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"litepan/internal/crosstransfer"
	"litepan/internal/domain"
	"litepan/internal/upload"
)

func (h *Handler) crossTransferRoutes(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.crossTransfer != nil) {
		return
	}
	routes := crosstransfer.BuildRoutes()
	writeOK(w, routes)
}

type crossTransferScanReq struct {
	SourceAccountID   int64                     `json:"source_account_id"`
	SourceParentID    string                    `json:"source_parent_id"`
	Method            string                    `json:"method"`
	SourceDisplayPath string                    `json:"source_display_path"`
	Sources           []crossTransferScanSource `json:"sources"`
}

type crossTransferScanSource struct {
	ParentID    string   `json:"parent_id"`
	DisplayPath string   `json:"display_path"`
	AncestorIDs []string `json:"ancestor_ids"`
}

func (r crossTransferScanReq) roots() []crosstransfer.ScanRoot {
	if len(r.Sources) == 0 {
		return []crosstransfer.ScanRoot{{
			ParentID:    r.SourceParentID,
			DisplayPath: r.SourceDisplayPath,
		}}
	}
	roots := make([]crosstransfer.ScanRoot, 0, len(r.Sources))
	for _, source := range r.Sources {
		roots = append(roots, crosstransfer.ScanRoot{
			ParentID:    source.ParentID,
			DisplayPath: source.DisplayPath,
			AncestorIDs: source.AncestorIDs,
		})
	}
	return roots
}

func (h *Handler) crossTransferScan(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.crossTransfer != nil) {
		return
	}
	var req crossTransferScanReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	result, err := h.crossTransfer.ScanSources(r.Context(), req.SourceAccountID, req.roots(), req.Method)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, result)
}

func (h *Handler) crossTransferScanStream(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.crossTransfer != nil) {
		return
	}
	var req crossTransferScanReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	h.streamCrossTransferNDJSON(w, r, func(emit func(crosstransfer.StreamEvent) error) error {
		return h.crossTransfer.ScanSourcesStream(
			r.Context(),
			req.SourceAccountID,
			req.roots(),
			req.Method,
			emit,
		)
	})
}

type crossTransferProbeFile struct {
	SourceFileID string `json:"source_file_id"`
	RelPath      string `json:"rel_path"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	Hash         string `json:"hash"`
}

type crossTransferProbeReq struct {
	SourceAccountID int64                    `json:"source_account_id"`
	TargetAccountID int64                    `json:"target_account_id"`
	TargetParentID  string                   `json:"target_parent_id"`
	Method          string                   `json:"method"`
	Files           []crossTransferProbeFile `json:"files"`
}

func (h *Handler) crossTransferProbe(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.crossTransfer != nil) {
		return
	}
	var req crossTransferProbeReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	h.streamCrossTransferNDJSON(w, r, func(emit func(crosstransfer.StreamEvent) error) error {
		files := make([]crosstransfer.TransferFile, 0, len(req.Files))
		for _, f := range req.Files {
			files = append(files, crosstransfer.TransferFile{
				SourceFileID: f.SourceFileID,
				RelPath:      f.RelPath,
				Name:         f.Name,
				Size:         f.Size,
				Hash:         f.Hash,
			})
		}
		return h.crossTransfer.ProbeStream(r.Context(), req.SourceAccountID, req.TargetAccountID, req.TargetParentID, req.Method, files, emit)
	})
}

type crossTransferTransferFile struct {
	SourceFileID string `json:"source_file_id"`
	RelPath      string `json:"rel_path"`
	RelDir       string `json:"rel_dir"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	Hash         string `json:"hash"`
}

type crossTransferExecuteReq struct {
	SourceAccountID   int64                       `json:"source_account_id"`
	SourceAccountName string                      `json:"source_account_name"`
	SourceDriverType  string                      `json:"source_driver_type"`
	TargetAccountID   int64                       `json:"target_account_id"`
	TargetAccountName string                      `json:"target_account_name"`
	TargetDriverType  string                      `json:"target_driver_type"`
	TargetParentID    string                      `json:"target_parent_id"`
	TargetDisplayPath string                      `json:"target_display_path"`
	Method            string                      `json:"method"`
	Files             []crossTransferTransferFile `json:"files"`
	Conflict          string                      `json:"conflict"`
	Fallback          bool                        `json:"fallback"`
}

func (h *Handler) crossTransferExecute(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.crossTransfer != nil) {
		return
	}
	var req crossTransferExecuteReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	files := make([]crosstransfer.TransferFile, 0, len(req.Files))
	for _, f := range req.Files {
		files = append(files, crosstransfer.TransferFile{
			SourceFileID: f.SourceFileID,
			RelPath:      f.RelPath,
			RelDir:       f.RelDir,
			Name:         f.Name,
			Size:         f.Size,
			Hash:         f.Hash,
		})
	}
	h.streamCrossTransferNDJSON(w, r, func(emit func(crosstransfer.StreamEvent) error) error {
		return h.crossTransfer.ExecuteStream(r.Context(), crosstransfer.ExecuteInput{
			SourceAccountID:   req.SourceAccountID,
			SourceAccountName: req.SourceAccountName,
			SourceDriverType:  req.SourceDriverType,
			TargetAccountID:   req.TargetAccountID,
			TargetAccountName: req.TargetAccountName,
			TargetDriverType:  req.TargetDriverType,
			TargetParentID:    req.TargetParentID,
			TargetDisplayPath: req.TargetDisplayPath,
			MethodID:          req.Method,
			Files:             files,
			Conflict:          req.Conflict,
			Fallback:          req.Fallback,
		}, emit)
	})
}

func (h *Handler) streamCrossTransferNDJSON(w http.ResponseWriter, r *http.Request, fn func(func(crosstransfer.StreamEvent) error) error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, domain.Errorf(domain.CodeInternal, "不支持流式响应"))
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	writeLine := func(event crosstransfer.StreamEvent) error {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s\n", raw); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := fn(writeLine); err != nil {
		_ = writeLine(crosstransfer.StreamEvent{"event": "error", "message": err.Error()})
	}
}

func (h *Handler) crossTransferRelayTasks(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.uploads != nil) {
		return
	}
	writeOK(w, h.listRelayTasks(r.Context()))
}

func (h *Handler) crossTransferRelayStream(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.uploads != nil) {
		return
	}
	s, err := newSSEWriter(w)
	if err != nil {
		writeErr(w, err)
		return
	}
	initial, _ := json.Marshal(map[string]any{"tasks": h.listRelayTasks(r.Context())})
	s.writeEvent("tasks", string(initial))
	ch := h.uploads.Subscribe()
	defer h.uploads.Unsubscribe(ch)
	ticker := time.NewTicker(defaultSSEPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case _, ok := <-ch:
			if !ok {
				return
			}
			payload, _ := json.Marshal(map[string]any{"tasks": h.listRelayTasks(r.Context())})
			s.writeEvent("tasks", string(payload))
		case <-ticker.C:
			s.writeEvent("ping", "{}")
		}
	}
}

type crossTransferRelayDeleteReq struct {
	TaskIDs []string `json:"task_ids"`
}

func (h *Handler) crossTransferRelayBatchDelete(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.uploads != nil) {
		return
	}
	var req crossTransferRelayDeleteReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	removed := h.deleteRelayTasks(r.Context(), req.TaskIDs)
	writeOK(w, map[string]any{"removed": removed})
}

type relayTaskDTO struct {
	TaskID              string         `json:"task_id"`
	SourceAccountID     int64          `json:"source_account_id"`
	SourceAccountName   string         `json:"source_account_name"`
	SourceDriverType    string         `json:"source_driver_type"`
	TargetAccountID     int64          `json:"target_account_id"`
	TargetAccountName   string         `json:"target_account_name"`
	TargetDriverType    string         `json:"target_driver_type"`
	SourceFileID        string         `json:"source_file_id"`
	FileName            string         `json:"file_name"`
	RelPath             string         `json:"rel_path"`
	RelDir              string         `json:"rel_dir"`
	TargetParentID      string         `json:"target_parent_id"`
	TargetDisplayPath   string         `json:"target_display_path"`
	TotalBytes          int64          `json:"total_bytes"`
	Status              string         `json:"status"`
	Phase               string         `json:"phase"`
	Progress            int            `json:"progress"`
	DownloadedBytes     int64          `json:"downloaded_bytes"`
	UploadedBytes       int64          `json:"uploaded_bytes"`
	SpeedBytesPerSecond float64        `json:"speed_bytes_per_second"`
	Message             string         `json:"message"`
	Error               string         `json:"error"`
	Result              map[string]any `json:"result,omitempty"`
	QueueOrder          int            `json:"queue_order,omitempty"`
	CreatedAt           float64        `json:"created_at"`
	UpdatedAt           float64        `json:"updated_at"`
}

func (h *Handler) listRelayTasks(ctx context.Context) []relayTaskDTO {
	if h.uploads == nil {
		return nil
	}
	tasks := h.uploads.List(ctx, 0)
	out := make([]relayTaskDTO, 0, len(tasks))
	for _, task := range tasks {
		if task.SourceType != upload.SourceTypeCrossTransfer || task.Phase != upload.PhaseDownloading {
			continue
		}
		out = append(out, toRelayTaskDTO(task))
	}
	return out
}

func (h *Handler) deleteRelayTasks(ctx context.Context, taskIDs []string) int {
	removed := 0
	for _, id := range taskIDs {
		task, ok := h.uploads.Get(ctx, id)
		if !ok || task.SourceType != upload.SourceTypeCrossTransfer || task.Phase != upload.PhaseDownloading {
			continue
		}
		found, err := h.uploads.Delete(ctx, id, false)
		if found && err == nil {
			removed++
		}
	}
	return removed
}

func toRelayTaskDTO(task upload.Task) relayTaskDTO {
	return relayTaskDTO{
		TaskID:              task.TaskID,
		SourceAccountID:     task.SourceAccountID,
		SourceAccountName:   task.SourceAccountName,
		SourceDriverType:    task.SourceDriverType,
		TargetAccountID:     task.AccountID,
		TargetAccountName:   task.AccountName,
		TargetDriverType:    task.DriverType,
		SourceFileID:        task.SourceFileID,
		FileName:            task.FileName,
		RelPath:             task.RelPath,
		RelDir:              task.RelDir,
		TargetParentID:      task.TargetPath,
		TargetDisplayPath:   task.TargetDisplayPath,
		TotalBytes:          task.TotalBytes,
		Status:              task.Status,
		Phase:               task.Phase,
		Progress:            task.Progress,
		DownloadedBytes:     task.DownloadedBytes,
		UploadedBytes:       task.UploadedBytes,
		SpeedBytesPerSecond: task.SpeedBytesPerSecond,
		Message:             task.Message,
		Error:               task.Error,
		Result:              task.Result,
		QueueOrder:          task.QueueOrder,
		CreatedAt:           task.CreatedAt,
		UpdatedAt:           task.UpdatedAt,
	}
}
