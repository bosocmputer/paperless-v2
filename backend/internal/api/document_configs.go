package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

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

func (s *Server) listDocumentConfigWorkflows(w http.ResponseWriter, r *http.Request) {
	steps, err := s.store.ListDocumentConfigSteps(r.Context(), "", "")
	if err != nil {
		s.logger.Error("list document config workflows failed", "error", err)
		writeError(w, http.StatusInternalServerError, "document_config_workflows_failed", "Cannot load document workflows right now.")
		return
	}

	formatsByCode := map[string]models.SMLDocFormat{}
	var smlWarning string
	formats, err := s.fetchSMLDocFormats(r.Context(), "")
	if err != nil {
		smlWarning = err.Error()
		s.logger.Warn("load SML doc formats for workflow summary failed", "error", err)
	} else {
		for _, format := range formats {
			formatsByCode[strings.ToUpper(strings.TrimSpace(format.Code))] = format
		}
	}

	grouped := map[string][]models.DocumentConfigStep{}
	for _, step := range steps {
		key := strings.ToUpper(strings.TrimSpace(step.DocFormatCode))
		if key == "" {
			continue
		}
		grouped[key] = append(grouped[key], step)
	}

	summaries := make([]models.DocumentConfigWorkflowSummary, 0, len(grouped))
	for key, group := range grouped {
		format := formatsByCode[key]
		if strings.TrimSpace(format.Code) == "" {
			format.Code = group[0].DocFormatCode
			format.ScreenCode = group[0].ScreenCode
		}
		summary := documentConfigWorkflowSummary(format, group)
		warnings, err := s.documentConfigPresetWarnings(r.Context(), summary.ScreenCode, summary.DocFormatCode, group)
		if err != nil {
			s.logger.Warn("load preset warnings for workflow summary failed", "error", err, "docFormatCode", summary.DocFormatCode)
		}
		summary.WarningCount = len(warnings)
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		return strings.ToUpper(summaries[i].DocFormatCode) < strings.ToUpper(summaries[j].DocFormatCode)
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"workflows":  summaries,
		"smlWarning": smlWarning,
	})
}

func (s *Server) getDocumentConfigWorkflow(w http.ResponseWriter, r *http.Request) {
	docFormatCode := strings.TrimSpace(r.PathValue("docFormatCode"))
	workflow, err := s.loadDocumentConfigWorkflow(r.Context(), docFormatCode)
	if err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflow": workflow})
}

func (s *Server) saveDocumentConfigWorkflow(w http.ResponseWriter, r *http.Request) {
	docFormatCode := strings.TrimSpace(r.PathValue("docFormatCode"))
	var req models.DocumentConfigWorkflowSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}

	format, err := s.fetchSMLDocFormatByCode(r.Context(), docFormatCode)
	if err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}
	screenCode := normalizeScreenCode(format.ScreenCode)
	if !isValidScreenCode(screenCode) {
		writeError(w, http.StatusBadGateway, "doc_format_invalid_screen_code", "Doc format code has no valid screen_code in SML erp_doc_format.")
		return
	}

	steps, validationMessages := normalizeDocumentConfigWorkflowSteps(format, req.Steps)
	userMessages, err := s.validateDocumentConfigWorkflowUsers(r.Context(), steps)
	if err != nil {
		s.logger.Error("validate workflow users failed", "error", err, "docFormatCode", docFormatCode)
		writeError(w, http.StatusInternalServerError, "workflow_user_validation_failed", "Cannot validate workflow users right now.")
		return
	}
	validationMessages = append(validationMessages, userMessages...)
	if len(validationMessages) > 0 {
		writeError(w, http.StatusBadRequest, "invalid_document_config_workflow", strings.Join(validationMessages, " "))
		return
	}

	updatedSteps, err := s.store.ReplaceDocumentConfigWorkflow(r.Context(), screenCode, strings.TrimSpace(format.Code), req.Revision, steps)
	if errors.Is(err, store.ErrDocumentConfigRevisionConflict) {
		actor, _ := currentUser(r)
		_ = s.store.WriteAuditWithMetadata(r.Context(), actor.ID, "document_config.workflow_revision_conflict", "document_config_workflow", strings.TrimSpace(format.Code), clientIP(r), r.UserAgent(), map[string]any{
			"docFormatCode": strings.TrimSpace(format.Code),
		})
		writeError(w, http.StatusConflict, "workflow_revision_conflict", "Workflow was changed from another tab. Refresh before saving again.")
		return
	}
	if errors.Is(err, store.ErrDocumentConfigDuplicate) {
		writeError(w, http.StatusConflict, "document_config_duplicate", "Position code already exists for this document format.")
		return
	}
	if err != nil {
		s.logger.Error("save document config workflow failed", "error", err, "docFormatCode", docFormatCode)
		writeError(w, http.StatusInternalServerError, "document_config_workflow_save_failed", "Cannot save document workflow right now.")
		return
	}

	workflow, err := s.workflowFromSteps(r.Context(), format, updatedSteps)
	if err != nil {
		s.logger.Error("reload document config workflow after save failed", "error", err, "docFormatCode", docFormatCode)
		writeError(w, http.StatusInternalServerError, "document_config_workflow_reload_failed", "Workflow was saved, but cannot reload it right now.")
		return
	}

	actor, _ := currentUser(r)
	if err := s.store.WriteAuditWithMetadata(r.Context(), actor.ID, "document_config.workflow_save", "document_config_workflow", workflow.DocFormat.Code, clientIP(r), r.UserAgent(), map[string]any{
		"docFormatCode": workflow.DocFormat.Code,
		"screenCode":    workflow.DocFormat.ScreenCode,
		"stepCount":     len(workflow.Steps),
		"warningCount":  len(workflow.PresetWarnings),
	}); err != nil {
		s.logger.Warn("write workflow save audit failed", "error", err, "docFormatCode", workflow.DocFormat.Code)
	}

	writeJSON(w, http.StatusOK, map[string]any{"workflow": workflow})
}

func (s *Server) copyDocumentConfigWorkflow(w http.ResponseWriter, r *http.Request) {
	targetDocFormatCode := strings.TrimSpace(r.PathValue("docFormatCode"))
	var req models.DocumentConfigWorkflowCopyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	sourceDocFormatCode := strings.TrimSpace(req.SourceDocFormatCode)
	if sourceDocFormatCode == "" {
		writeError(w, http.StatusBadRequest, "source_doc_format_required", "Source doc format is required.")
		return
	}
	if strings.EqualFold(sourceDocFormatCode, targetDocFormatCode) {
		writeError(w, http.StatusBadRequest, "copy_same_workflow", "Source and target doc format must be different.")
		return
	}

	sourceFormat, err := s.fetchSMLDocFormatByCode(r.Context(), sourceDocFormatCode)
	if err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}
	targetFormat, err := s.fetchSMLDocFormatByCode(r.Context(), targetDocFormatCode)
	if err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}
	sourceSteps, err := s.store.ListDocumentConfigSteps(r.Context(), normalizeScreenCode(sourceFormat.ScreenCode), strings.TrimSpace(sourceFormat.Code))
	if err != nil {
		s.logger.Error("load source workflow for copy failed", "error", err, "sourceDocFormatCode", sourceDocFormatCode)
		writeError(w, http.StatusInternalServerError, "workflow_copy_load_failed", "Cannot load source workflow right now.")
		return
	}
	if len(sourceSteps) == 0 {
		writeError(w, http.StatusBadRequest, "source_workflow_empty", "Source workflow has no steps to copy.")
		return
	}

	copySteps := make([]models.DocumentConfigStepRequest, 0, len(sourceSteps))
	for _, step := range sourceSteps {
		copySteps = append(copySteps, models.DocumentConfigStepRequest{
			ScreenCode:             normalizeScreenCode(targetFormat.ScreenCode),
			DocFormatCode:          strings.TrimSpace(targetFormat.Code),
			PositionCode:           step.PositionCode,
			PositionName:           step.PositionName,
			User01:                 step.User01,
			User02:                 step.User02,
			User03:                 step.User03,
			SequenceNo:             step.SequenceNo,
			ConditionType:          step.ConditionType,
			AttachmentRequirements: step.AttachmentRequirements,
		})
	}
	normalizedSteps, validationMessages := normalizeDocumentConfigWorkflowSteps(targetFormat, copySteps)
	userMessages, err := s.validateDocumentConfigWorkflowUsers(r.Context(), normalizedSteps)
	if err != nil {
		s.logger.Error("validate copied workflow users failed", "error", err, "targetDocFormatCode", targetDocFormatCode)
		writeError(w, http.StatusInternalServerError, "workflow_user_validation_failed", "Cannot validate workflow users right now.")
		return
	}
	validationMessages = append(validationMessages, userMessages...)
	if len(validationMessages) > 0 {
		writeError(w, http.StatusBadRequest, "invalid_document_config_workflow", strings.Join(validationMessages, " "))
		return
	}

	targetScreenCode := normalizeScreenCode(targetFormat.ScreenCode)
	updatedSteps, err := s.store.ReplaceDocumentConfigWorkflow(r.Context(), targetScreenCode, strings.TrimSpace(targetFormat.Code), req.Revision, normalizedSteps)
	if errors.Is(err, store.ErrDocumentConfigRevisionConflict) {
		writeError(w, http.StatusConflict, "workflow_revision_conflict", "Target workflow was changed from another tab. Refresh before copying again.")
		return
	}
	if err != nil {
		s.logger.Error("copy document config workflow failed", "error", err, "sourceDocFormatCode", sourceDocFormatCode, "targetDocFormatCode", targetDocFormatCode)
		writeError(w, http.StatusInternalServerError, "workflow_copy_failed", "Cannot copy workflow right now.")
		return
	}

	workflow, err := s.workflowFromSteps(r.Context(), targetFormat, updatedSteps)
	if err != nil {
		s.logger.Error("reload copied workflow failed", "error", err, "targetDocFormatCode", targetDocFormatCode)
		writeError(w, http.StatusInternalServerError, "workflow_copy_reload_failed", "Workflow was copied, but cannot reload it right now.")
		return
	}

	actor, _ := currentUser(r)
	if err := s.store.WriteAuditWithMetadata(r.Context(), actor.ID, "document_config.workflow_copy", "document_config_workflow", workflow.DocFormat.Code, clientIP(r), r.UserAgent(), map[string]any{
		"sourceDocFormatCode": strings.TrimSpace(sourceFormat.Code),
		"targetDocFormatCode": workflow.DocFormat.Code,
		"stepCount":           len(workflow.Steps),
	}); err != nil {
		s.logger.Warn("write workflow copy audit failed", "error", err, "targetDocFormatCode", workflow.DocFormat.Code)
	}

	writeJSON(w, http.StatusOK, map[string]any{"workflow": workflow})
}

func (s *Server) recordDocumentConfigWorkflowEvent(w http.ResponseWriter, r *http.Request) {
	docFormatCode := strings.TrimSpace(r.PathValue("docFormatCode"))
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req models.DocumentConfigWorkflowEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	event := strings.TrimSpace(req.Event)
	if !allowedDocumentConfigWorkflowEvent(event) {
		writeError(w, http.StatusBadRequest, "invalid_workflow_event", "Workflow event is invalid.")
		return
	}

	actor, _ := currentUser(r)
	metadata := map[string]any{
		"event":                event,
		"docFormatCode":        docFormatCode,
		"sessionId":            truncateForMetadata(req.SessionID, 80),
		"stepCount":            clampInt(req.StepCount, 0, 30),
		"validationIssueCount": clampInt(req.ValidationIssueCount, 0, 1000),
		"elapsedMs":            clampInt64(req.ElapsedMs, 0, 24*60*60*1000),
	}
	if err := s.store.WriteAuditWithMetadata(r.Context(), actor.ID, "document_config.workflow_event", "document_config_workflow", docFormatCode, clientIP(r), r.UserAgent(), metadata); err != nil {
		s.logger.Warn("write workflow ux event failed", "error", err, "event", event, "docFormatCode", docFormatCode)
	}
	w.WriteHeader(http.StatusNoContent)
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

func (s *Server) loadDocumentConfigWorkflow(ctx context.Context, docFormatCode string) (models.DocumentConfigWorkflow, error) {
	format, err := s.fetchSMLDocFormatByCode(ctx, docFormatCode)
	if err != nil {
		return models.DocumentConfigWorkflow{}, err
	}
	screenCode := normalizeScreenCode(format.ScreenCode)
	if !isValidScreenCode(screenCode) {
		return models.DocumentConfigWorkflow{}, errDocFormatInvalidScreenCode
	}
	format.ScreenCode = screenCode
	steps, err := s.store.ListDocumentConfigSteps(ctx, screenCode, strings.TrimSpace(format.Code))
	if err != nil {
		return models.DocumentConfigWorkflow{}, err
	}
	return s.workflowFromSteps(ctx, format, steps)
}

func (s *Server) workflowFromSteps(ctx context.Context, format models.SMLDocFormat, steps []models.DocumentConfigStep) (models.DocumentConfigWorkflow, error) {
	format.Code = strings.TrimSpace(format.Code)
	format.ScreenCode = normalizeScreenCode(format.ScreenCode)
	warnings, err := s.documentConfigPresetWarnings(ctx, format.ScreenCode, format.Code, steps)
	if err != nil {
		return models.DocumentConfigWorkflow{}, err
	}
	return models.DocumentConfigWorkflow{
		DocFormat:      format,
		Steps:          steps,
		Revision:       store.ComputeDocumentConfigWorkflowRevision(steps),
		PresetWarnings: warnings,
	}, nil
}

func documentConfigWorkflowSummary(format models.SMLDocFormat, steps []models.DocumentConfigStep) models.DocumentConfigWorkflowSummary {
	var updatedAt *time.Time
	conditionCounts := map[string]int{"1": 0, "2": 0, "3": 0}
	users := map[string]bool{}
	screenCode := normalizeScreenCode(format.ScreenCode)
	docFormatCode := strings.TrimSpace(format.Code)
	if docFormatCode == "" && len(steps) > 0 {
		docFormatCode = steps[0].DocFormatCode
	}
	if screenCode == "" && len(steps) > 0 {
		screenCode = steps[0].ScreenCode
	}
	format.Code = docFormatCode
	format.ScreenCode = screenCode

	for i := range steps {
		step := steps[i]
		if updatedAt == nil || step.UpdatedAt.After(*updatedAt) {
			value := step.UpdatedAt
			updatedAt = &value
		}
		conditionCounts[fmt.Sprintf("%d", step.ConditionType)]++
		for _, user := range documentConfigStepUsers(step.User01, step.User02, step.User03) {
			users[strings.ToLower(user)] = true
		}
	}

	return models.DocumentConfigWorkflowSummary{
		DocFormatCode:   docFormatCode,
		ScreenCode:      screenCode,
		DocFormat:       format,
		StepCount:       len(steps),
		UserCount:       len(users),
		ConditionCounts: conditionCounts,
		UpdatedAt:       updatedAt,
		Revision:        store.ComputeDocumentConfigWorkflowRevision(steps),
	}
}

func (s *Server) documentConfigPresetWarnings(ctx context.Context, screenCode, docFormatCode string, steps []models.DocumentConfigStep) ([]models.DocumentConfigPresetWarning, error) {
	counts, err := s.store.ListSignatureTemplateBoxPositionCounts(ctx, screenCode, docFormatCode)
	if err != nil {
		return nil, err
	}
	if len(counts) == 0 {
		return []models.DocumentConfigPresetWarning{}, nil
	}
	knownPositions := map[string]bool{}
	for _, step := range steps {
		knownPositions[strings.ToLower(strings.TrimSpace(step.PositionCode))] = true
	}

	positionCodes := make([]string, 0, len(counts))
	for positionCode := range counts {
		positionCodes = append(positionCodes, positionCode)
	}
	sort.Slice(positionCodes, func(i, j int) bool {
		return strings.ToLower(positionCodes[i]) < strings.ToLower(positionCodes[j])
	})

	warnings := []models.DocumentConfigPresetWarning{}
	for _, positionCode := range positionCodes {
		if knownPositions[strings.ToLower(strings.TrimSpace(positionCode))] {
			continue
		}
		count := counts[positionCode]
		warnings = append(warnings, models.DocumentConfigPresetWarning{
			Code:         "preset_position_missing",
			PositionCode: positionCode,
			BoxCount:     count,
			Message:      fmt.Sprintf("Preset มี %d กรอบใน Position %s แต่ Workflow ปัจจุบันไม่มี Position นี้แล้ว", count, positionCode),
		})
	}
	return warnings, nil
}

func normalizeDocumentConfigStep(req models.DocumentConfigStepRequest) models.DocumentConfigStepRequest {
	req.ScreenCode = normalizeScreenCode(req.ScreenCode)
	req.DocFormatCode = strings.TrimSpace(req.DocFormatCode)
	req.PositionCode = strings.TrimSpace(req.PositionCode)
	req.PositionName = strings.TrimSpace(req.PositionName)
	req.User01 = strings.TrimSpace(req.User01)
	req.User02 = strings.TrimSpace(req.User02)
	req.User03 = strings.TrimSpace(req.User03)
	req.AttachmentRequirements, _ = normalizeAttachmentRequirementsForStep(req)
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
	if req.SequenceNo <= 0 {
		return "Sequence must be greater than 0."
	}
	if req.ConditionType != 1 && req.ConditionType != 2 && req.ConditionType != 3 {
		return "Condition must be 1, 2, or 3."
	}
	if (req.ConditionType == 1 || req.ConditionType == 2) && len(documentConfigStepUsers(req.User01, req.User02, req.User03)) == 0 {
		return "Condition 1 or 2 requires at least one user."
	}
	if _, messages := normalizeAttachmentRequirementsForStep(req); len(messages) > 0 {
		return strings.Join(messages, " ")
	}
	return ""
}

func normalizeDocumentConfigWorkflowSteps(format models.SMLDocFormat, rawSteps []models.DocumentConfigStepRequest) ([]models.DocumentConfigStepRequest, []string) {
	if len(rawSteps) > 30 {
		return nil, []string{"Workflow supports at most 30 steps."}
	}

	screenCode := normalizeScreenCode(format.ScreenCode)
	docFormatCode := strings.TrimSpace(format.Code)
	seen := map[string]bool{}
	steps := make([]models.DocumentConfigStepRequest, 0, len(rawSteps))
	messages := []string{}

	for index, raw := range rawSteps {
		step := normalizeDocumentConfigStep(raw)
		step.ScreenCode = screenCode
		step.DocFormatCode = docFormatCode
		if step.SequenceNo <= 0 {
			step.SequenceNo = float64(index + 1)
		}
		label := fmt.Sprintf("Step %d", index+1)
		if step.PositionCode != "" {
			label = fmt.Sprintf("Position %s", step.PositionCode)
		}
		if step.PositionCode == "" {
			messages = append(messages, fmt.Sprintf("%s: Position code is required.", label))
		}
		if step.PositionName == "" {
			messages = append(messages, fmt.Sprintf("%s: Position name is required.", label))
		}
		if step.SequenceNo <= 0 {
			messages = append(messages, fmt.Sprintf("%s: Sequence must be greater than 0.", label))
		}
		if step.ConditionType != 1 && step.ConditionType != 2 && step.ConditionType != 3 {
			messages = append(messages, fmt.Sprintf("%s: Condition must be 1, 2, or 3.", label))
		}
		if (step.ConditionType == 1 || step.ConditionType == 2) && len(documentConfigStepUsers(step.User01, step.User02, step.User03)) == 0 {
			messages = append(messages, fmt.Sprintf("%s: Condition 1 or 2 requires at least one active user slot.", label))
		}
		normalizedRequirements, requirementMessages := normalizeAttachmentRequirementsForStep(step)
		step.AttachmentRequirements = normalizedRequirements
		for _, message := range requirementMessages {
			messages = append(messages, fmt.Sprintf("%s: %s", label, message))
		}
		duplicateKey := strings.ToLower(step.PositionCode)
		if duplicateKey != "" {
			if seen[duplicateKey] {
				messages = append(messages, fmt.Sprintf("%s: Position code is duplicated.", label))
			}
			seen[duplicateKey] = true
		}
		steps = append(steps, step)
	}
	return steps, messages
}

func normalizeAttachmentRequirementsForStep(step models.DocumentConfigStepRequest) ([]models.AttachmentRequirement, []string) {
	const maxRequirementsPerStep = 12
	const maxRequirementLabelLength = 80

	raw := step.AttachmentRequirements
	messages := []string{}
	if len(raw) > maxRequirementsPerStep {
		messages = append(messages, fmt.Sprintf("Required attachments support at most %d items per step.", maxRequirementsPerStep))
		raw = raw[:maxRequirementsPerStep]
	}
	userSlotCount := len(documentConfigStepUsers(step.User01, step.User02, step.User03))
	if step.ConditionType == 3 {
		userSlotCount = 1
	}
	if userSlotCount <= 0 {
		userSlotCount = 1
	}

	seen := map[string]bool{}
	out := make([]models.AttachmentRequirement, 0, len(raw))
	for index, requirement := range raw {
		label := strings.TrimSpace(requirement.Label)
		if label == "" {
			messages = append(messages, "Required attachment name is required.")
			continue
		}
		if len([]rune(label)) > maxRequirementLabelLength {
			messages = append(messages, fmt.Sprintf("Required attachment %q is longer than %d characters.", label, maxRequirementLabelLength))
			label = truncateRunes(label, maxRequirementLabelLength)
		}
		slot := requirement.SignerSlot
		if slot <= 0 {
			slot = 1
		}
		if slot > userSlotCount {
			messages = append(messages, fmt.Sprintf("Required attachment %q points to signer slot %d, but this step has only %d signer slot(s).", label, slot, userSlotCount))
			continue
		}
		key := normalizeRequirementKey(requirement.Key)
		if key == "" {
			key = fmt.Sprintf("slot-%d-%d", slot, index+1)
		}
		duplicateKey := fmt.Sprintf("%d:%s", slot, strings.ToLower(label))
		if seen[duplicateKey] {
			messages = append(messages, fmt.Sprintf("Required attachment %q is duplicated for signer slot %d.", label, slot))
			continue
		}
		seen[duplicateKey] = true
		out = append(out, models.AttachmentRequirement{
			Key:        key,
			Label:      label,
			SignerSlot: slot,
		})
	}
	return out, messages
}

func normalizeRequirementKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
		if builder.Len() >= 80 {
			break
		}
	}
	return builder.String()
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func documentConfigStepUsers(values ...string) []string {
	users := []string{}
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized != "" {
			users = append(users, normalized)
		}
	}
	return users
}

func (s *Server) validateDocumentConfigWorkflowUsers(ctx context.Context, steps []models.DocumentConfigStepRequest) ([]string, error) {
	hasInternalUsers := false
	for _, step := range steps {
		if step.ConditionType == 1 || step.ConditionType == 2 {
			hasInternalUsers = true
			break
		}
	}
	if !hasInternalUsers {
		return nil, nil
	}

	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	activeUsernames := map[string]bool{}
	for _, user := range users {
		if strings.EqualFold(user.Status, "active") {
			activeUsernames[strings.ToLower(strings.TrimSpace(user.Username))] = true
		}
	}

	messages := []string{}
	for _, step := range steps {
		if step.ConditionType != 1 && step.ConditionType != 2 {
			continue
		}
		for _, value := range documentConfigStepUsers(step.User01, step.User02, step.User03) {
			username := strings.ToLower(strings.TrimSpace(strings.Split(value, ":")[0]))
			if username == "" {
				continue
			}
			if !activeUsernames[username] {
				messages = append(messages, fmt.Sprintf("Position %s: user %s is not active or does not exist.", step.PositionCode, username))
			}
		}
	}
	return messages, nil
}

func allowedDocumentConfigWorkflowEvent(event string) bool {
	switch event {
	case "workflow_open",
		"workflow_save_attempt",
		"workflow_save_success",
		"workflow_save_error",
		"workflow_revision_conflict",
		"workflow_copy",
		"workflow_reorder":
		return true
	default:
		return false
	}
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
