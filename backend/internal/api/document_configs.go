package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

func (s *Server) listDocumentConfigSteps(w http.ResponseWriter, r *http.Request) {
	screenCode := normalizeScreenCode(r.URL.Query().Get("screen_code"))
	docFormatCode := strings.TrimSpace(r.URL.Query().Get("doc_format_code"))
	if screenCode != "" && !isValidScreenCode(screenCode) {
		writeError(w, http.StatusBadRequest, "invalid_screen_code", "screen_code is invalid.")
		return
	}

	steps, err := s.store.ListDocumentConfigSteps(r.Context(), screenCode, docFormatCode)
	if err != nil {
		s.logger.Error("list document config steps failed", "error", err)
		writeError(w, http.StatusInternalServerError, "document_configs_failed", "Cannot load document config right now.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"configs": steps})
}

func (s *Server) createDocumentConfigStep(w http.ResponseWriter, r *http.Request) {
	var req models.DocumentConfigStepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	req = normalizeDocumentConfigStep(req)
	if message := validateDocumentConfigStep(req); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_document_config", message)
		return
	}
	if err := s.ensureSMLDocFormatExists(r.Context(), req.ScreenCode, req.DocFormatCode); err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}

	step, err := s.store.CreateDocumentConfigStep(r.Context(), req)
	if errors.Is(err, store.ErrDocumentConfigDuplicate) {
		writeError(w, http.StatusConflict, "document_config_duplicate", "Position code already exists for this document format.")
		return
	}
	if err != nil {
		s.logger.Error("create document config step failed", "error", err)
		writeError(w, http.StatusInternalServerError, "document_config_create_failed", "Cannot create document config right now.")
		return
	}

	actor, _ := currentUser(r)
	if err := s.store.WriteAudit(r.Context(), actor.ID, "document_config.create", "document_config_step", step.ID, clientIP(r), r.UserAgent()); err != nil {
		s.logger.Warn("write document config create audit failed", "error", err, "configID", step.ID)
	}

	writeJSON(w, http.StatusCreated, map[string]any{"config": step})
}

func (s *Server) updateDocumentConfigStep(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_document_config_id", "Document config id is required.")
		return
	}

	var req models.DocumentConfigStepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	req = normalizeDocumentConfigStep(req)
	if message := validateDocumentConfigStep(req); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_document_config", message)
		return
	}
	if err := s.ensureSMLDocFormatExists(r.Context(), req.ScreenCode, req.DocFormatCode); err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}

	step, err := s.store.UpdateDocumentConfigStep(r.Context(), id, req)
	if errors.Is(err, store.ErrDocumentConfigNotFound) {
		writeError(w, http.StatusNotFound, "document_config_not_found", "Document config was not found.")
		return
	}
	if errors.Is(err, store.ErrDocumentConfigDuplicate) {
		writeError(w, http.StatusConflict, "document_config_duplicate", "Position code already exists for this document format.")
		return
	}
	if err != nil {
		s.logger.Error("update document config step failed", "error", err, "configID", id)
		writeError(w, http.StatusInternalServerError, "document_config_update_failed", "Cannot update document config right now.")
		return
	}

	actor, _ := currentUser(r)
	if err := s.store.WriteAudit(r.Context(), actor.ID, "document_config.update", "document_config_step", step.ID, clientIP(r), r.UserAgent()); err != nil {
		s.logger.Warn("write document config update audit failed", "error", err, "configID", step.ID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"config": step})
}

func (s *Server) deleteDocumentConfigStep(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_document_config_id", "Document config id is required.")
		return
	}

	if err := s.store.DeleteDocumentConfigStep(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrDocumentConfigNotFound) {
			writeError(w, http.StatusNotFound, "document_config_not_found", "Document config was not found.")
			return
		}
		s.logger.Error("delete document config step failed", "error", err, "configID", id)
		writeError(w, http.StatusInternalServerError, "document_config_delete_failed", "Cannot delete document config right now.")
		return
	}

	actor, _ := currentUser(r)
	if err := s.store.WriteAudit(r.Context(), actor.ID, "document_config.delete", "document_config_step", id, clientIP(r), r.UserAgent()); err != nil {
		s.logger.Warn("write document config delete audit failed", "error", err, "configID", id)
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) writeDocFormatValidationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errSMLConfigMissing):
		writeError(w, http.StatusServiceUnavailable, "sml_not_configured", "SML Paperless API is not configured.")
	case errors.Is(err, errDocFormatNotFound):
		writeError(w, http.StatusBadRequest, "doc_format_not_found", "Doc format code was not found in SML erp_doc_format.")
	default:
		s.logger.Warn("validate document format against sml failed", "error", err)
		writeError(w, http.StatusBadGateway, "sml_doc_formats_failed", "Cannot verify document format with SML right now.")
	}
}

func normalizeDocumentConfigStep(req models.DocumentConfigStepRequest) models.DocumentConfigStepRequest {
	req.ScreenCode = normalizeScreenCode(req.ScreenCode)
	req.DocFormatCode = strings.TrimSpace(req.DocFormatCode)
	req.PositionCode = strings.TrimSpace(req.PositionCode)
	req.PositionName = strings.TrimSpace(req.PositionName)
	req.User01 = strings.TrimSpace(req.User01)
	req.User02 = strings.TrimSpace(req.User02)
	req.User03 = strings.TrimSpace(req.User03)
	return req
}

func validateDocumentConfigStep(req models.DocumentConfigStepRequest) string {
	if !isValidScreenCode(req.ScreenCode) {
		return "screen_code is invalid."
	}
	if req.DocFormatCode == "" {
		return "Doc format code is required."
	}
	if req.PositionCode == "" {
		return "Position code is required."
	}
	if req.PositionName == "" {
		return "Position name is required."
	}
	if req.User01 == "" {
		return "User01 is required."
	}
	if req.SequenceNo <= 0 {
		return "Sequence must be greater than 0."
	}
	if req.ConditionType != 1 && req.ConditionType != 2 && req.ConditionType != 3 {
		return "Condition must be 1, 2, or 3."
	}
	return ""
}

func normalizeScreenCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func isValidScreenCode(value string) bool {
	if value == "" || len(value) > 40 {
		return false
	}
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
