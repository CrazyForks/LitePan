package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"litepan/internal/domain"
	"litepan/internal/playback"
	"litepan/internal/strm"
)

func (h *Handler) strmPlay(w http.ResponseWriter, r *http.Request) {
	if h.strm == nil || h.playback == nil {
		writeErr(w, domain.Errf(domain.CodeNotImplement))
		return
	}
	accountID, err := parsePathInt64(r, "account_id")
	if err != nil {
		writeErr(w, err)
		return
	}
	fileID, err := strm.DecodeFileKey(chi.URLParam(r, "file_key"))
	if err != nil {
		writeErr(w, domain.Errorf(domain.CodeValidation, "非法 file_key"))
		return
	}
	if err := h.authorizeSTRMPlay(r); err != nil {
		writeErr(w, err)
		return
	}
	fileName, _ := url.PathUnescape(chi.URLParam(r, "filename"))
	if err := h.playback.ServeHTTP(w, r, playback.Request{
		AccountID: accountID,
		FileID:    fileID,
	}, playback.Intent{FileName: fileName}); err != nil {
		writeErr(w, err)
	}
}

func (h *Handler) strmPathPlay(w http.ResponseWriter, r *http.Request) {
	if h.strm == nil || h.playback == nil || h.files == nil {
		writeErr(w, domain.Errf(domain.CodeNotImplement))
		return
	}
	accountID, err := parsePathInt64(r, "account_id")
	if err != nil {
		writeErr(w, err)
		return
	}
	rootID, err := strm.DecodePathKey(chi.URLParam(r, "root_key"))
	if err != nil {
		writeErr(w, domain.Errorf(domain.CodeValidation, "非法 root_key"))
		return
	}
	relativePath, err := strm.DecodePathKey(chi.URLParam(r, "path_key"))
	if err != nil {
		writeErr(w, domain.Errorf(domain.CodeValidation, "非法 path_key"))
		return
	}
	if err := h.authorizeSTRMPlay(r); err != nil {
		writeErr(w, err)
		return
	}
	item, err := h.files.ResolvePath(r.Context(), accountID, rootID, relativePath)
	if err != nil {
		writeErr(w, err)
		return
	}
	fileName, _ := url.PathUnescape(chi.URLParam(r, "filename"))
	if fileName == "" {
		fileName = item.Name
	}
	if err := h.playback.ServeHTTP(w, r, playback.Request{AccountID: accountID, FileID: item.ID}, playback.Intent{FileName: fileName}); err != nil {
		writeErr(w, err)
	}
}

func (h *Handler) authorizeSTRMPlay(r *http.Request) error {
	ok, err := h.strm.MatchToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		return err
	}
	if !ok {
		return domain.Errf(domain.CodePermissionDenied)
	}
	signature := chi.URLParam(r, "signature")
	if h.strm.SignatureEnabled() {
		if signature == "" {
			return domain.Errf(domain.CodePermissionDenied)
		}
		unsignedPath := strings.TrimSuffix(r.URL.EscapedPath(), "/s/"+signature)
		if !h.strm.VerifySignature(unsignedPath, signature) {
			return domain.Errf(domain.CodePermissionDenied)
		}
	}
	return nil
}
