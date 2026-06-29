package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	resolvedReq, err := s.resolveDocumentConfigStep(r.Context(), req)
	if err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}

	step, err := s.store.CreateDocumentConfigStep(r.Context(), resolvedReq)
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
	resolvedReq, err := s.resolveDocumentConfigStep(r.Context(), req)
	if err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}
	current, err := s.store.FindDocumentConfigStepByID(r.Context(), id)
	if errors.Is(err, store.ErrDocumentConfigNotFound) {
		writeError(w, http.StatusNotFound, "document_config_not_found", "Document config was not found.")
		return
	}
	if err != nil {
		s.logger.Error("load document config step failed", "error", err, "configID", id)
		writeError(w, http.StatusInternalServerError, "document_config_update_failed", "Cannot update document config right now.")
		return
	}
	if documentConfigTemplateBreakingChange(current, resolvedReq) {
		if ok := s.ensureDocumentConfigStepHasNoTemplateBoxes(w, r, current, "changing doc/user/condition fields"); !ok {
			return
		}
	}

	step, err := s.store.UpdateDocumentConfigStep(r.Context(), id, resolvedReq)
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
	current, err := s.store.FindDocumentConfigStepByID(r.Context(), id)
	if errors.Is(err, store.ErrDocumentConfigNotFound) {
		writeError(w, http.StatusNotFound, "document_config_not_found", "Document config was not found.")
		return
	}
	if err != nil {
		s.logger.Error("load document config step failed", "error", err, "configID", id)
		writeError(w, http.StatusInternalServerError, "document_config_delete_failed", "Cannot delete document config right now.")
		return
	}
	if ok := s.ensureDocumentConfigStepHasNoTemplateBoxes(w, r, current, "deleting this position"); !ok {
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
	case errors.Is(err, errDocFormatAmbiguous):
		writeError(w, http.StatusBadRequest, "doc_format_ambiguous", "Doc format code matches more than one SML screen code.")
	case errors.Is(err, errDocFormatInvalidScreenCode):
		writeError(w, http.StatusBadGateway, "doc_format_invalid_screen_code", "Doc format code has no valid screen_code in SML erp_doc_format.")
	default:
		s.logger.Warn("validate document format against sml failed", "error", err)
		writeError(w, http.StatusBadGateway, "sml_doc_formats_failed", "Cannot verify document format with SML right now.")
	}
}

func (s *Server) resolveDocumentConfigStep(ctx context.Context, req models.DocumentConfigStepRequest) (models.DocumentConfigStepRequest, error) {
	format, err := s.fetchSMLDocFormatByCode(ctx, req.DocFormatCode)
	if err != nil {
		return req, err
	}

	req.DocFormatCode = strings.TrimSpace(format.Code)
	req.ScreenCode = normalizeScreenCode(format.ScreenCode)
	if !isValidScreenCode(req.ScreenCode) {
		return req, errDocFormatInvalidScreenCode
	}
	return req, nil
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

func documentConfigTemplateBreakingChange(current models.DocumentConfigStep, next models.DocumentConfigStepRequest) bool {
	if !strings.EqualFold(current.ScreenCode, next.ScreenCode) {
		return true
	}
	if !strings.EqualFold(current.DocFormatCode, next.DocFormatCode) {
		return true
	}
	if !strings.EqualFold(current.PositionCode, next.PositionCode) {
		return true
	}
	if current.ConditionType != next.ConditionType {
		return true
	}
	return strings.TrimSpace(current.User01) != strings.TrimSpace(next.User01) ||
		strings.TrimSpace(current.User02) != strings.TrimSpace(next.User02) ||
		strings.TrimSpace(current.User03) != strings.TrimSpace(next.User03)
}

func (s *Server) ensureDocumentConfigStepHasNoTemplateBoxes(w http.ResponseWriter, r *http.Request, step models.DocumentConfigStep, action string) bool {
	count, err := s.store.CountSignatureTemplateBoxesForConfig(r.Context(), step.ScreenCode, step.DocFormatCode, step.PositionCode)
	if err != nil {
		s.logger.Error("count signature template boxes for document config failed", "error", err, "configID", step.ID)
		writeError(w, http.StatusInternalServerError, "document_config_reference_check_failed", "Cannot update document config right now.")
		return false
	}
	if count == 0 {
		return true
	}
	writeError(
		w,
		http.StatusConflict,
		"document_config_in_signature_template",
		fmt.Sprintf("Position %s (%s) of %s has %d signature box(es). Remove or update boxes in Signature Template before %s.", step.PositionCode, step.PositionName, step.DocFormatCode, count, action),
	)
	return false
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
