package api

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
	"github.com/ledongthuc/pdf"
)

func (s *Server) getSignatureTemplateState(w http.ResponseWriter, r *http.Request) {
	docFormatCode := strings.TrimSpace(r.URL.Query().Get("doc_format_code"))
	if docFormatCode == "" {
		writeError(w, http.StatusBadRequest, "doc_format_code_required", "doc_format_code is required.")
		return
	}

	format, err := s.fetchSMLDocFormatByCode(r.Context(), docFormatCode)
	if err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}
	screenCode := normalizeScreenCode(format.ScreenCode)

	configs, err := s.store.ListDocumentConfigSteps(r.Context(), screenCode, format.Code)
	if err != nil {
		s.logger.Error("list document configs for signature template failed", "error", err, "docFormatCode", format.Code)
		writeError(w, http.StatusInternalServerError, "signature_template_configs_failed", "Cannot load document config right now.")
		return
	}

	draft, active, err := s.store.GetSignatureTemplateState(r.Context(), screenCode, format.Code)
	if err != nil {
		s.logger.Error("load signature template state failed", "error", err, "docFormatCode", format.Code)
		writeError(w, http.StatusInternalServerError, "signature_template_failed", "Cannot load signature template right now.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"docFormat":        format,
		"configs":          configs,
		"draft":            draft,
		"active":           active,
		"maxTemplatePages": s.cfg.MaxTemplatePages,
		"draftIssues":      validationIssuesForTemplate(draft, configs, s.cfg.MaxTemplatePages),
		"activeIssues":     validationIssuesForTemplate(active, configs, s.cfg.MaxTemplatePages),
	})
}

func (s *Server) uploadSignatureTemplateSamplePDF(w http.ResponseWriter, r *http.Request) {
	docFormatCode := strings.TrimSpace(r.URL.Query().Get("doc_format_code"))
	if docFormatCode == "" {
		writeError(w, http.StatusBadRequest, "doc_format_code_required", "doc_format_code is required.")
		return
	}

	format, err := s.fetchSMLDocFormatByCode(r.Context(), docFormatCode)
	if err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}
	screenCode := normalizeScreenCode(format.ScreenCode)

	maxBytes := s.cfg.MaxUploadMB * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "pdf_file_required", "PDF file is required.")
		return
	}
	defer file.Close()

	contentType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if contentType != "" && !strings.Contains(contentType, "pdf") {
		writeError(w, http.StatusBadRequest, "invalid_pdf_content_type", "Uploaded file content type must be PDF.")
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "pdf_read_failed", "Cannot read uploaded PDF.")
		return
	}
	if int64(len(data)) > maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "pdf_too_large", fmt.Sprintf("PDF must be %d MB or smaller.", s.cfg.MaxUploadMB))
		return
	}
	if !isPDFBytes(data) {
		writeError(w, http.StatusBadRequest, "invalid_pdf", "Uploaded file must be a valid PDF.")
		return
	}
	pageCount, err := readPDFPageCount(data)
	if err != nil || pageCount <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_pdf", "Uploaded file must be a readable PDF.")
		return
	}
	if pageCount > s.cfg.MaxTemplatePages {
		writeError(w, http.StatusBadRequest, "pdf_too_many_pages", fmt.Sprintf("PDF must be %d pages or fewer.", s.cfg.MaxTemplatePages))
		return
	}
	originalName := filepath.Base(strings.TrimSpace(header.Filename))
	if originalName == "." || originalName == string(os.PathSeparator) {
		originalName = "sample.pdf"
	}

	if err := os.MkdirAll(s.cfg.FileStorageDir, 0o750); err != nil {
		s.logger.Error("create upload directory failed", "error", err, "dir", s.cfg.FileStorageDir)
		writeError(w, http.StatusInternalServerError, "upload_storage_failed", "Cannot prepare file storage right now.")
		return
	}

	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	storedName := fmt.Sprintf("%s-%s.pdf", sha[:16], randomHex(8))
	storagePath := filepath.Join(s.cfg.FileStorageDir, storedName)
	if err := os.WriteFile(storagePath, data, 0o640); err != nil {
		s.logger.Error("write uploaded pdf failed", "error", err, "path", storagePath)
		writeError(w, http.StatusInternalServerError, "upload_write_failed", "Cannot save uploaded PDF right now.")
		return
	}

	actor, _ := currentUser(r)
	uploaded, err := s.store.CreateUploadedFile(r.Context(), models.UploadedFile{
		OriginalName: originalName,
		StoredName:   storedName,
		StoragePath:  storagePath,
		ContentType:  "application/pdf",
		SizeBytes:    int64(len(data)),
		PageCount:    pageCount,
		SHA256:       sha,
		CreatedBy:    actor.ID,
	})
	if err != nil {
		_ = os.Remove(storagePath)
		s.logger.Error("create uploaded file record failed", "error", err)
		writeError(w, http.StatusInternalServerError, "upload_record_failed", "Cannot save uploaded PDF right now.")
		return
	}

	template, err := s.store.UpsertDraftSignatureTemplateSample(r.Context(), screenCode, format.Code, uploaded.ID, actor.ID)
	if err != nil {
		s.logger.Error("upsert signature template sample failed", "error", err, "docFormatCode", format.Code)
		writeError(w, http.StatusInternalServerError, "signature_template_sample_failed", "Cannot attach PDF to template right now.")
		return
	}

	if err := s.store.WriteAudit(r.Context(), actor.ID, "signature_template.sample_upload", "signature_template", template.ID, clientIP(r), r.UserAgent()); err != nil {
		s.logger.Warn("write signature template upload audit failed", "error", err, "templateID", template.ID)
	}

	writeJSON(w, http.StatusCreated, map[string]any{"template": template})
}

func (s *Server) getSignatureTemplateSamplePDF(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_template_id", "Template id is required.")
		return
	}

	template, err := s.store.FindSignatureTemplateByID(r.Context(), id)
	if errors.Is(err, store.ErrSignatureTemplateNotFound) {
		writeError(w, http.StatusNotFound, "signature_template_not_found", "Signature template was not found.")
		return
	}
	if err != nil {
		s.logger.Error("load signature template PDF failed", "error", err, "templateID", id)
		writeError(w, http.StatusInternalServerError, "signature_template_failed", "Cannot load signature template right now.")
		return
	}
	if template.SampleFile == nil || template.SampleFile.StoragePath == "" {
		writeError(w, http.StatusNotFound, "sample_pdf_not_found", "Sample PDF was not found.")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", template.SampleFile.OriginalName))
	http.ServeFile(w, r, template.SampleFile.StoragePath)
}

func (s *Server) saveSignatureTemplateBoxes(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_template_id", "Template id is required.")
		return
	}

	var req models.SaveSignatureBoxesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	boxes, issues := normalizeAndValidateBoxRequests(req.Boxes, s.cfg.MaxTemplatePages)
	if len(issues) > 0 {
		writeValidationIssues(w, http.StatusBadRequest, "invalid_signature_boxes", issues)
		return
	}

	template, err := s.store.ReplaceSignatureTemplateBoxes(r.Context(), id, req.Revision, boxes)
	if errors.Is(err, store.ErrSignatureTemplateNotFound) {
		writeError(w, http.StatusNotFound, "signature_template_not_found", "Signature template was not found.")
		return
	}
	if errors.Is(err, store.ErrSignatureTemplateNotDraft) {
		writeError(w, http.StatusBadRequest, "signature_template_not_draft", "Only draft templates can be edited.")
		return
	}
	if errors.Is(err, store.ErrSignatureRevisionConflict) {
		writeError(w, http.StatusConflict, "signature_template_revision_conflict", "Template was changed from another tab. Please refresh and try again.")
		return
	}
	if err != nil {
		s.logger.Error("save signature template boxes failed", "error", err, "templateID", id)
		writeError(w, http.StatusInternalServerError, "signature_template_save_failed", "Cannot save signature boxes right now.")
		return
	}

	actor, _ := currentUser(r)
	if err := s.store.WriteAudit(r.Context(), actor.ID, "signature_template.boxes_save", "signature_template", template.ID, clientIP(r), r.UserAgent()); err != nil {
		s.logger.Warn("write signature template boxes audit failed", "error", err, "templateID", template.ID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"template": template})
}

func (s *Server) publishSignatureTemplate(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_template_id", "Template id is required.")
		return
	}

	template, err := s.store.FindSignatureTemplateByID(r.Context(), id)
	if errors.Is(err, store.ErrSignatureTemplateNotFound) {
		writeError(w, http.StatusNotFound, "signature_template_not_found", "Signature template was not found.")
		return
	}
	if err != nil {
		s.logger.Error("load signature template before publish failed", "error", err, "templateID", id)
		writeError(w, http.StatusInternalServerError, "signature_template_failed", "Cannot load signature template right now.")
		return
	}
	if template.Status != "draft" {
		writeError(w, http.StatusBadRequest, "signature_template_not_draft", "Only draft templates can be published.")
		return
	}

	configs, err := s.store.ListDocumentConfigSteps(r.Context(), template.ScreenCode, template.DocFormatCode)
	if err != nil {
		s.logger.Error("list document configs before publish failed", "error", err, "templateID", id)
		writeError(w, http.StatusInternalServerError, "signature_template_configs_failed", "Cannot validate template right now.")
		return
	}
	issues := validateSignatureTemplate(template, configs, s.cfg.MaxTemplatePages)
	if len(issues) > 0 {
		writeValidationIssues(w, http.StatusBadRequest, "signature_template_invalid", issues)
		return
	}

	actor, _ := currentUser(r)
	published, err := s.store.PublishSignatureTemplate(r.Context(), id, actor.ID)
	if errors.Is(err, store.ErrSignatureTemplateNotFound) {
		writeError(w, http.StatusNotFound, "signature_template_not_found", "Signature template was not found.")
		return
	}
	if errors.Is(err, store.ErrSignatureTemplateNotDraft) {
		writeError(w, http.StatusBadRequest, "signature_template_not_draft", "Only draft templates can be published.")
		return
	}
	if err != nil {
		s.logger.Error("publish signature template failed", "error", err, "templateID", id)
		writeError(w, http.StatusInternalServerError, "signature_template_publish_failed", "Cannot publish signature template right now.")
		return
	}

	if err := s.store.WriteAudit(r.Context(), actor.ID, "signature_template.publish", "signature_template", published.ID, clientIP(r), r.UserAgent()); err != nil {
		s.logger.Warn("write signature template publish audit failed", "error", err, "templateID", published.ID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"template": published})
}

func isPDFBytes(data []byte) bool {
	return len(data) >= 5 && string(data[:5]) == "%PDF-"
}

func readPDFPageCount(data []byte) (int, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, err
	}
	return reader.NumPage(), nil
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "random"
	}
	return hex.EncodeToString(buf)
}

func normalizeAndValidateBoxRequests(boxes []models.SignatureTemplateBoxRequest, maxPages int) ([]models.SignatureTemplateBoxRequest, []models.SignatureValidationIssue) {
	normalized := make([]models.SignatureTemplateBoxRequest, 0, len(boxes))
	issues := []models.SignatureValidationIssue{}
	usedSlots := map[string]bool{}
	for index, box := range boxes {
		box.PositionCode = strings.TrimSpace(box.PositionCode)
		box.SignerType = strings.ToLower(strings.TrimSpace(box.SignerType))
		box.SignerUser = strings.TrimSpace(box.SignerUser)
		box.Label = strings.TrimSpace(box.Label)
		if box.SignerType == "" {
			box.SignerType = "any"
		}
		if box.SignerSlot == 0 {
			box.SignerSlot = index + 1
		}
		if box.PositionCode == "" {
			issues = append(issues, signatureIssue("box_position_required", "", "Every signature box must choose a position."))
		}
		if box.SignerType != "any" && box.SignerType != "internal" && box.SignerType != "external" {
			issues = append(issues, signatureIssue("box_signer_type_invalid", box.PositionCode, "Signer type must be any, internal, or external."))
		}
		if box.SignerSlot <= 0 {
			issues = append(issues, signatureIssue("box_signer_slot_invalid", box.PositionCode, "Signer slot must be greater than 0."))
		}
		if box.PositionCode != "" && box.SignerSlot > 0 {
			slotKey := fmt.Sprintf("%s:%d", strings.ToLower(box.PositionCode), box.SignerSlot)
			if usedSlots[slotKey] {
				issues = append(issues, signatureIssue("box_signer_slot_duplicate", box.PositionCode, "Signer slot must be unique inside the same position."))
			}
			usedSlots[slotKey] = true
		}
		if box.PageNo <= 0 || box.PageNo > maxPages {
			issues = append(issues, signatureIssue("box_page_invalid", box.PositionCode, fmt.Sprintf("Page must be between 1 and %d.", maxPages)))
		}
		if box.XRatio < 0 || box.YRatio < 0 || box.WidthRatio <= 0 || box.HeightRatio <= 0 || box.XRatio+box.WidthRatio > 1 || box.YRatio+box.HeightRatio > 1 {
			issues = append(issues, signatureIssue("box_bounds_invalid", box.PositionCode, "Signature box must stay inside the PDF page."))
		}
		normalized = append(normalized, box)
	}
	return normalized, issues
}

func validationIssuesForTemplate(template *models.SignatureTemplate, configs []models.DocumentConfigStep, maxPages int) []models.SignatureValidationIssue {
	if template == nil {
		return []models.SignatureValidationIssue{}
	}
	return validateSignatureTemplate(*template, configs, maxPages)
}

func validateSignatureTemplate(template models.SignatureTemplate, configs []models.DocumentConfigStep, maxPages int) []models.SignatureValidationIssue {
	issues := []models.SignatureValidationIssue{}
	if template.SampleFileID == "" {
		issues = append(issues, signatureIssue("sample_pdf_required", "", "Upload a sample PDF before publishing."))
	}
	if template.SampleFile != nil && template.SampleFile.PageCount > maxPages {
		issues = append(issues, signatureIssue("pdf_too_many_pages", "", fmt.Sprintf("Sample PDF must be %d pages or fewer.", maxPages)))
	}
	if len(configs) == 0 {
		issues = append(issues, signatureIssue("document_config_required", "", "Document config is required before publishing a signature template."))
	}

	normalizedBoxes, boxIssues := normalizeAndValidateBoxRequests(boxRequestsFromTemplate(template.Boxes), maxPages)
	issues = append(issues, boxIssues...)

	stepsByPosition := map[string]models.DocumentConfigStep{}
	for _, step := range configs {
		stepsByPosition[strings.ToLower(step.PositionCode)] = step
	}

	boxesByPosition := map[string][]models.SignatureTemplateBoxRequest{}
	for _, box := range normalizedBoxes {
		key := strings.ToLower(box.PositionCode)
		boxesByPosition[key] = append(boxesByPosition[key], box)
		if _, ok := stepsByPosition[key]; !ok && box.PositionCode != "" {
			issues = append(issues, signatureIssue("box_position_unknown", box.PositionCode, "Signature box uses a position that is not in document config."))
		}
	}

	for _, step := range configs {
		positionBoxes := boxesByPosition[strings.ToLower(step.PositionCode)]
		switch step.ConditionType {
		case 1:
			if !hasSignerType(positionBoxes, "any") {
				issues = append(issues, signatureIssue("condition_any_box_required", step.PositionCode, fmt.Sprintf("%s needs at least one any-signer box.", step.PositionName)))
			}
			for _, box := range positionBoxes {
				if box.SignerType != "any" || box.SignerUser != "" {
					issues = append(issues, signatureIssue("condition_any_type_invalid", step.PositionCode, fmt.Sprintf("%s must use any-signer boxes without fixed users.", step.PositionName)))
					break
				}
			}
		case 2:
			required := stepUsers(step)
			if len(required) == 0 {
				issues = append(issues, signatureIssue("condition_all_users_required", step.PositionCode, fmt.Sprintf("%s needs at least one configured user.", step.PositionName)))
				continue
			}
			if len(positionBoxes) != len(required) {
				issues = append(issues, signatureIssue("condition_all_box_count_invalid", step.PositionCode, fmt.Sprintf("%s needs exactly %d signature boxes.", step.PositionName, len(required))))
			}
			seen := map[string]int{}
			for _, box := range positionBoxes {
				if box.SignerType != "internal" {
					issues = append(issues, signatureIssue("condition_all_type_invalid", step.PositionCode, fmt.Sprintf("%s must use internal signer boxes.", step.PositionName)))
					continue
				}
				if box.SignerUser == "" {
					issues = append(issues, signatureIssue("condition_all_user_required", step.PositionCode, fmt.Sprintf("%s requires a signer user on every box.", step.PositionName)))
					continue
				}
				if box.SignerType == "internal" && box.SignerUser != "" {
					seen[box.SignerUser]++
				}
			}
			for _, user := range required {
				if seen[user] == 0 {
					issues = append(issues, signatureIssue("condition_all_missing_user_box", step.PositionCode, fmt.Sprintf("%s needs a signature box for %s.", step.PositionName, user)))
				}
				if seen[user] > 1 {
					issues = append(issues, signatureIssue("condition_all_duplicate_user_box", step.PositionCode, fmt.Sprintf("%s has duplicate boxes for %s.", step.PositionName, user)))
				}
			}
			for user := range seen {
				if !containsString(required, user) {
					issues = append(issues, signatureIssue("condition_all_unknown_user_box", step.PositionCode, fmt.Sprintf("%s has a box for a user outside this position: %s.", step.PositionName, user)))
				}
			}
		case 3:
			if !hasSignerType(positionBoxes, "external") {
				issues = append(issues, signatureIssue("condition_external_box_required", step.PositionCode, fmt.Sprintf("%s needs at least one external signer box.", step.PositionName)))
			}
			for _, box := range positionBoxes {
				if box.SignerType != "external" || box.SignerUser != "" {
					issues = append(issues, signatureIssue("condition_external_type_invalid", step.PositionCode, fmt.Sprintf("%s must use external signer boxes without internal users.", step.PositionName)))
					break
				}
			}
		}
	}

	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].PositionCode < issues[j].PositionCode
	})
	return issues
}

func boxRequestsFromTemplate(boxes []models.SignatureTemplateBox) []models.SignatureTemplateBoxRequest {
	out := make([]models.SignatureTemplateBoxRequest, 0, len(boxes))
	for _, box := range boxes {
		out = append(out, models.SignatureTemplateBoxRequest{
			PositionCode: box.PositionCode,
			SignerSlot:   box.SignerSlot,
			SignerType:   box.SignerType,
			SignerUser:   box.SignerUser,
			PageNo:       box.PageNo,
			XRatio:       box.XRatio,
			YRatio:       box.YRatio,
			WidthRatio:   box.WidthRatio,
			HeightRatio:  box.HeightRatio,
			Label:        box.Label,
		})
	}
	return out
}

func stepUsers(step models.DocumentConfigStep) []string {
	users := []string{}
	for _, user := range []string{step.User01, step.User02, step.User03} {
		user = strings.TrimSpace(user)
		if user != "" {
			users = append(users, user)
		}
	}
	return users
}

func hasSignerType(boxes []models.SignatureTemplateBoxRequest, signerType string) bool {
	for _, box := range boxes {
		if box.SignerType == signerType {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func signatureIssue(code, positionCode, message string) models.SignatureValidationIssue {
	return models.SignatureValidationIssue{Code: code, PositionCode: positionCode, Message: message}
}

func writeValidationIssues(w http.ResponseWriter, status int, code string, issues []models.SignatureValidationIssue) {
	writeJSON(w, status, map[string]any{
		"error":   code,
		"message": "Signature template validation failed.",
		"issues":  issues,
	})
}
