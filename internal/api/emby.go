package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"litepan/internal/domain"
	"litepan/internal/embyproxy"
	"litepan/internal/settings"
)

func (h *Handler) getEmbyConfig(w http.ResponseWriter, r *http.Request) {
	if h.embyProxy == nil {
		writeErr(w, domain.Errf(domain.CodeNotImplement))
		return
	}
	writeOK(w, h.embyProxy.Snapshot(r))
}

func (h *Handler) updateEmbyConfig(w http.ResponseWriter, r *http.Request) {
	if h.embyProxy == nil {
		writeErr(w, domain.Errf(domain.CodeNotImplement))
		return
	}
	var in embyproxy.UpdateRequest
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.embyProxy.Update(r.Context(), in); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, h.embyProxy.Snapshot(r))
}

func (h *Handler) testEmbyConfig(w http.ResponseWriter, r *http.Request) {
	if h.embyProxy == nil {
		writeErr(w, domain.Errf(domain.CodeNotImplement))
		return
	}
	var in embyproxy.UpdateRequest
	err := json.NewDecoder(r.Body).Decode(&in)
	if err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, domain.Errorf(domain.CodeValidation, "请求体格式错误"))
		return
	}
	if err == nil {
		err = h.embyProxy.TestUpdate(r.Context(), in)
	} else {
		err = h.embyProxy.Test(r.Context())
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (h *Handler) refreshEmbyLibrary(w http.ResponseWriter, r *http.Request) {
	if h.embyProxy == nil {
		writeErr(w, domain.Errf(domain.CodeNotImplement))
		return
	}
	result, err := h.embyProxy.RefreshLibrary(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, result)
}

func embySettingsTouched(changed map[string]string) bool {
	for _, key := range []string{
		settings.KeyEmbyEnabled,
		settings.KeyEmbyURL,
		settings.KeyEmbyAPIKey,
		settings.KeyEmbyProxyPort,
	} {
		if _, ok := changed[key]; ok {
			return true
		}
	}
	return false
}
