package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

const (
	signingLegalTextVersion             = "thai-eta-2544-v2"
	signingLegalNoticePDFDisplayVersion = "thai-safe-v4"
	signatureTransparencyVersion        = "transparent-v1"
	signingLegalText                    = "เอกสารนี้จัดทำและลงนามในรูปแบบอิเล็กทรอนิกส์ตาม พ.ร.บ. ธุรกรรมทางอิเล็กทรอนิกส์ พ.ศ. 2544 ผู้ลงนามยืนยันความถูกต้องของเนื้อหาและยอมรับผลผูกพันทางกฎหมายทุกประการ"
	maxSigningEventBytes                = 8 * 1024
	maxRuntimeSignNoteBoxes             = 30
	maxRuntimeSignNoteChars             = 500
	maxRuntimeSignNotePayloadBytes      = 64 * 1024
	minRuntimeSignNoteBoxWidthRatio     = 0.04
	minRuntimeSignNoteBoxHeightRatio    = 0.015
	defaultRuntimeSignNoteFontSizePt    = 10.0
	minRuntimeSignNoteFontSizePt        = 8.0
	maxRuntimeSignNoteFontSizePt        = 18.0
	defaultRuntimeSignNotePaddingPt     = 2.0
	minRuntimeSignNotePaddingPt         = 1.0
	maxRuntimeSignNotePaddingPt         = 6.0
)

var signingUXEventNames = map[string]bool{
	"task_open":                      true,
	"pdf_load_success":               true,
	"pdf_load_error":                 true,
	"signature_started":              true,
	"signature_cleared":              true,
	"sign_note_box_add":              true,
	"sign_note_box_delete":           true,
	"sign_attempt":                   true,
	"sign_success":                   true,
	"sign_error":                     true,
	"reject_success":                 true,
	"attachment_upload":              true,
	"ready_task_open":                true,
	"waiting_queue_seen":             true,
	"waiting_task_open":              true,
	"history_open":                   true,
	"history_detail_open":            true,
	"history_pdf_open":               true,
	"blocked_not_turn":               true,
	"blocked_signed":                 true,
	"blocked_rejected":               true,
	"related_documents_open":         true,
	"related_documents_load_success": true,
	"related_documents_load_error":   true,
	"related_document_click":         true,
}

var signingCreateEventNames = map[string]bool{
	"create_layout_open":           true,
	"wizard_open":                  true,
	"step_complete":                true,
	"pdf_upload_success":           true,
	"pdf_upload_error":             true,
	"preset_applied":               true,
	"box_add":                      true,
	"box_delete":                   true,
	"legal_notice_box_add":         true,
	"legal_notice_box_delete":      true,
	"legal_notice_missing_blocked": true,
	"layout_validation_error":      true,
	"validation_blocked":           true,
	"create_submit_success":        true,
	"create_submit_error":          true,
	"create_success":               true,
	"create_error":                 true,
	"pdf_render_error":             true,
}

type createSigningDocumentRequest struct {
	DocFormatCode       string                               `json:"docFormatCode"`
	DocNo               string                               `json:"docNo"`
	FileID              string                               `json:"fileId"`
	SignatureTemplateID string                               `json:"signatureTemplateId"`
	ConfirmLocked       bool                                 `json:"confirmLocked"`
	LayoutBoxes         []models.SignatureTemplateBoxRequest `json:"layoutBoxes"`
	SignNoteBoxes       []models.SignatureTemplateBoxRequest `json:"signNoteBoxes"`
	LegalNoticeBox      *models.LegalNoticeBoxRequest        `json:"legalNoticeBox"`
	LegalNoticeBoxes    []models.LegalNoticeBoxRequest       `json:"legalNoticeBoxes"`
	ContextVersion      string                               `json:"contextVersion,omitempty"`
}

type signingDocumentBatchValidationRequest struct {
	DocFormatCode string   `json:"docFormatCode"`
	FileIDs       []string `json:"fileIds"`
}

type signingDocumentBatchEventRequest struct {
	Event         string `json:"event"`
	DocFormatCode string `json:"docFormatCode"`
	Total         int    `json:"total"`
	Created       int    `json:"created"`
	Failed        int    `json:"failed"`
}

type signingDocumentBatchValidationIssue struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

type signingDocumentBatchValidationItem struct {
	FileID       string                                     `json:"fileId"`
	OriginalName string                                     `json:"originalName"`
	DocNo        string                                     `json:"docNo,omitempty"`
	PageCount    int                                        `json:"pageCount,omitempty"`
	SizeBytes    int64                                      `json:"sizeBytes,omitempty"`
	Status       string                                     `json:"status"`
	Candidate    *models.SMLDocumentCandidate               `json:"candidate,omitempty"`
	Issues       []signingDocumentBatchValidationIssue      `json:"issues"`
	Duplicate    *store.SigningDocumentDuplicateCheckResult `json:"duplicate,omitempty"`
}

func documentNumberFromPDFName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("ชื่อไฟล์ไม่ถูกต้อง")
	}
	ext := filepath.Ext(name)
	if !strings.EqualFold(ext, ".pdf") {
		return "", fmt.Errorf("รองรับเฉพาะไฟล์ PDF ที่ตั้งชื่อตามเลขเอกสาร")
	}
	docNo := strings.ToUpper(strings.TrimSpace(strings.TrimSuffix(name, ext)))
	if docNo == "" {
		return "", fmt.Errorf("ชื่อไฟล์ต้องมีเลขเอกสารก่อน .pdf")
	}
	if len([]rune(docNo)) > 25 {
		return "", fmt.Errorf("เลขเอกสารจากชื่อไฟล์ต้องไม่เกิน 25 ตัวอักษร")
	}
	if strings.IndexFunc(docNo, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("ชื่อไฟล์มีอักขระที่ไม่รองรับ")
	}
	return docNo, nil
}

func signingDocumentBatchContextVersion(configs []models.DocumentConfigStep, template models.SignatureTemplate) string {
	payload := struct {
		Configs  []models.DocumentConfigStep `json:"configs"`
		Template struct {
			ID        string    `json:"id"`
			Version   int       `json:"version"`
			Revision  int       `json:"revision"`
			UpdatedAt time.Time `json:"updatedAt"`
		} `json:"template"`
	}{Configs: configs}
	payload.Template.ID = template.ID
	payload.Template.Version = template.Version
	payload.Template.Revision = template.Revision
	payload.Template.UpdatedAt = template.UpdatedAt
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type signingCreateEventRequest struct {
	Event                string                           `json:"event"`
	SessionID            string                           `json:"sessionId"`
	DocFormatCode        string                           `json:"docFormatCode"`
	ElapsedMS            int64                            `json:"elapsedMs"`
	BoxCount             int                              `json:"boxCount"`
	ValidationIssueCount int                              `json:"validationIssueCount"`
	Viewport             models.SignatureDesignerViewport `json:"viewport"`
}

func (s *Server) listSigningDocuments(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	size := parsePositiveQueryInt(r, "size", 100)
	if size > 100 {
		size = 100
	}
	queue := r.URL.Query().Get("queue")
	createdByUserID := ""
	if strings.EqualFold(strings.TrimSpace(queue), "draft") {
		createdByUserID = actor.ID
	}
	result, err := s.store.ListSigningDocuments(r.Context(), store.SigningDocumentListQuery{
		Queue:           queue,
		Search:          r.URL.Query().Get("search"),
		Page:            parsePositiveQueryInt(r, "page", 1),
		Size:            size,
		CreatedByUserID: createdByUserID,
	})
	if err != nil {
		s.logger.Error("list signing documents failed", "error", err)
		writeError(w, http.StatusInternalServerError, "signing_documents_failed", "Cannot load signing documents right now.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) checkSigningDocumentDuplicate(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	docFormatCode := strings.TrimSpace(r.URL.Query().Get("doc_format_code"))
	docNo := strings.TrimSpace(r.URL.Query().Get("doc_no"))
	if docFormatCode == "" || docNo == "" {
		writeError(w, http.StatusBadRequest, "document_required", "กรุณาระบุชนิดเอกสารและเลขที่เอกสาร")
		return
	}
	result, err := s.store.CheckSigningDocumentDuplicate(r.Context(), docFormatCode, docNo)
	if err != nil {
		s.logger.Error("check signing document duplicate failed", "error", err)
		writeError(w, http.StatusInternalServerError, "duplicate_check_failed", "ตรวจสอบเอกสารซ้ำไม่สำเร็จ")
		return
	}
	result = prepareSigningDocumentDuplicateResponse(result, actor.ID)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getAdminDashboard(w http.ResponseWriter, r *http.Request) {
	dashboard, err := s.store.GetAdminDashboard(r.Context())
	if err != nil {
		s.logger.Error("admin dashboard failed", "error", err)
		writeError(w, http.StatusInternalServerError, "admin_dashboard_failed", "Cannot load admin dashboard right now.")
		return
	}
	writeJSON(w, http.StatusOK, dashboard)
}

func (s *Server) uploadSigningDocumentPDF(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	maxBytes := s.cfg.MaxUploadMB * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024)
	uploaded, err := s.readAndStorePDFUpload(w, r, "file", actor.ID, "document.pdf")
	if err != nil {
		return
	}
	if err := s.store.CreateSigningDocumentUpload(r.Context(), uploaded.ID, actor.ID); err != nil {
		s.logger.Error("create signing document upload session failed", "error", err)
		_ = os.Remove(uploaded.StoragePath)
		writeError(w, http.StatusInternalServerError, "upload_session_failed", "Cannot prepare document upload right now.")
		return
	}
	go s.cleanupExpiredSigningUploads()
	writeJSON(w, http.StatusCreated, map[string]any{
		"file":    uploaded,
		"fileUrl": fmt.Sprintf("/api/signing-documents/uploads/%s/pdf", uploaded.ID),
	})
}

func (s *Server) getSigningDocumentUploadPDF(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	file, err := s.store.FindSigningDocumentUploadFile(r.Context(), strings.TrimSpace(r.PathValue("fileId")), actor.ID)
	if errors.Is(err, store.ErrSigningDocumentUploadNotFound) {
		writeError(w, http.StatusNotFound, "upload_not_found", "Uploaded PDF was not found or has expired.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "upload_failed", "Cannot load uploaded PDF right now.")
		return
	}
	serveInlinePDF(w, r, file)
}

func (s *Server) deleteSigningDocumentUpload(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	fileID := strings.TrimSpace(r.PathValue("fileId"))
	if !isUUIDText(fileID) {
		writeError(w, http.StatusBadRequest, "file_id_invalid", "Uploaded file id is invalid.")
		return
	}
	storagePath, deleted, err := s.store.DeleteSigningDocumentUpload(r.Context(), fileID, actor.ID)
	if err != nil {
		s.logger.Error("delete signing document upload failed", "error", err, "fileId", fileID)
		writeError(w, http.StatusInternalServerError, "upload_delete_failed", "Cannot discard uploaded PDF right now.")
		return
	}
	if deleted && strings.TrimSpace(storagePath) != "" {
		if err := os.Remove(storagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logger.Warn("remove discarded signing document upload failed", "error", err, "fileId", fileID)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) validateSigningDocumentBatch(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	startedAt := time.Now()
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req signingDocumentBatchValidationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	docFormatCode := strings.ToUpper(strings.TrimSpace(req.DocFormatCode))
	fileIDs, err := normalizeBatchFileIDs(req.FileIDs)
	if err != nil {
		writeError(w, http.StatusBadRequest, "batch_files_invalid", err.Error())
		return
	}
	format, err := s.fetchSMLDocFormatByCode(r.Context(), docFormatCode)
	if err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}
	screenCode := normalizeScreenCode(format.ScreenCode)
	configs, err := s.store.ListDocumentConfigSteps(r.Context(), screenCode, format.Code)
	if err != nil || len(configs) == 0 {
		writeError(w, http.StatusBadRequest, "document_config_required", "กรุณาตั้งค่า Workflow ของชนิดเอกสารนี้ก่อนนำเข้า")
		return
	}
	_, active, err := s.store.GetSignatureTemplateState(r.Context(), screenCode, format.Code)
	if err != nil {
		s.logger.Error("load batch active template failed", "error", err, "docFormatCode", format.Code)
		writeError(w, http.StatusInternalServerError, "signature_template_failed", "Cannot load active signature template right now.")
		return
	}
	_, baseLayout, baseLegalBoxes, templateIssues := activeTemplateCreateLayout(active, 1)
	if len(templateIssues) == 0 {
		var normalizedLayout []models.SignatureTemplateBoxRequest
		var selectedConfigs []models.DocumentConfigStep
		normalizedLayout, selectedConfigs, _, templateIssues = validateSigningDocumentLayout(baseLayout, configs, 1)
		if len(templateIssues) == 0 {
			templateIssues = append(templateIssues, s.inactiveSigningLayoutUserIssues(r.Context(), selectedConfigs, normalizedLayout)...)
		}
		_, legalIssues := normalizeAndValidateLegalNoticeBoxes(baseLegalBoxes, nil, 1, true)
		templateIssues = append(templateIssues, legalIssues...)
	}
	if len(templateIssues) > 0 {
		writeValidationIssues(w, http.StatusBadRequest, "signature_template_required", templateIssues)
		return
	}

	uploads, err := s.store.FindSigningDocumentUploadFiles(r.Context(), fileIDs, actor.ID)
	if err != nil {
		s.logger.Error("load batch uploads failed", "error", err)
		writeError(w, http.StatusInternalServerError, "batch_uploads_failed", "Cannot load uploaded PDFs right now.")
		return
	}
	items := make([]signingDocumentBatchValidationItem, 0, len(fileIDs))
	docNoCounts := make(map[string]int, len(fileIDs))
	docNos := make([]string, 0, len(fileIDs))
	totalPageCount := 0
	totalBytes := int64(0)
	for _, fileID := range fileIDs {
		item := signingDocumentBatchValidationItem{FileID: fileID, Status: "invalid", Issues: []signingDocumentBatchValidationIssue{}}
		uploaded, ok := uploads[fileID]
		if !ok {
			item.Issues = append(item.Issues, batchValidationIssue("upload_not_found", "ไม่พบไฟล์ที่อัปโหลดหรือไฟล์หมดอายุ", false))
			items = append(items, item)
			continue
		}
		item.OriginalName = uploaded.OriginalName
		item.PageCount = uploaded.PageCount
		item.SizeBytes = uploaded.SizeBytes
		totalBytes += uploaded.SizeBytes
		if uploaded.PageCount > 0 {
			totalPageCount += uploaded.PageCount
		}
		docNo, parseErr := documentNumberFromPDFName(uploaded.OriginalName)
		if parseErr != nil {
			item.Issues = append(item.Issues, batchValidationIssue("filename_invalid", parseErr.Error(), false))
		} else {
			item.DocNo = docNo
			docNoCounts[docNo]++
			docNos = append(docNos, docNo)
		}
		if uploaded.PageCount <= 0 {
			item.Issues = append(item.Issues, batchValidationIssue("pdf_invalid", "PDF ไม่สามารถอ่านจำนวนหน้าได้", false))
		}
		items = append(items, item)
	}
	applyBatchPageLimit(items, totalPageCount)

	candidateByDocNo := map[string]models.SMLDocumentCandidate{}
	if len(docNos) > 0 {
		payload, fetchErr := s.fetchSMLDocumentCandidatesBatch(r.Context(), format.Code, docNos)
		if fetchErr != nil {
			s.logger.Warn("fetch SML batch candidates failed", "error", fetchErr, "docFormatCode", format.Code, "count", len(docNos))
			writeError(w, http.StatusBadGateway, "sml_batch_validation_failed", "ตรวจสอบรายการเอกสารกับ SML ไม่สำเร็จ กรุณาลองใหม่")
			return
		}
		for _, candidate := range payload.Data {
			candidateByDocNo[strings.ToUpper(strings.TrimSpace(candidate.DocNo))] = candidate
		}
	}
	duplicates, err := s.store.CheckSigningDocumentDuplicates(r.Context(), format.Code, docNos)
	if err != nil {
		s.logger.Error("check signing batch duplicates failed", "error", err)
		writeError(w, http.StatusInternalServerError, "duplicate_check_failed", "ตรวจสอบเอกสารซ้ำไม่สำเร็จ")
		return
	}

	summary := map[string]int{"total": len(items), "ready": 0, "warning": 0, "invalid": 0}
	for index := range items {
		item := &items[index]
		if item.DocNo != "" && docNoCounts[item.DocNo] > 1 {
			item.Issues = append(item.Issues, batchValidationIssue("duplicate_in_batch", "มีชื่อเอกสารซ้ำภายในชุด กรุณาลบให้เหลือหนึ่งไฟล์", false))
		}
		candidate, found := candidateByDocNo[item.DocNo]
		if item.DocNo != "" && !found {
			item.Issues = append(item.Issues, batchValidationIssue("sml_document_not_found", "ไม่พบเลขเอกสารนี้ใน SML สำหรับชนิดเอกสารที่เลือก", false))
		}
		if found {
			candidateCopy := candidate
			item.Candidate = &candidateCopy
			duplicate := duplicates[item.DocNo]
			duplicate = prepareSigningDocumentDuplicateResponse(duplicate, actor.ID)
			if !duplicate.CanCreate {
				duplicateCopy := duplicate
				item.Duplicate = &duplicateCopy
				item.Issues = append(item.Issues, batchValidationIssue("signing_document_duplicate", duplicate.Message, false))
			}
			if candidate.IsLockRecord == 1 {
				item.Issues = append(item.Issues, batchValidationIssue("sml_document_locked", "เอกสารนี้ถูก Lock ใน SML ต้องยืนยันก่อนนำเข้า", false))
			}
		}
		if batchIssuesExcept(item.Issues, "sml_document_locked") == 0 && item.PageCount > 0 {
			_, layout, legalBoxes, layoutIssues := activeTemplateCreateLayout(active, item.PageCount)
			if len(layoutIssues) == 0 {
				_, _, _, layoutIssues = validateSigningDocumentLayout(layout, configs, item.PageCount)
				_, legalIssues := normalizeAndValidateLegalNoticeBoxes(legalBoxes, nil, item.PageCount, true)
				layoutIssues = append(layoutIssues, legalIssues...)
			}
			for _, issue := range layoutIssues {
				item.Issues = append(item.Issues, batchValidationIssue(issue.Code, issue.Message, false))
			}
		}
		if hasBatchIssue(item.Issues, "sml_document_locked") && len(item.Issues) == 1 {
			item.Status = "warning"
			summary["warning"]++
		} else if len(item.Issues) == 0 {
			item.Status = "ready"
			summary["ready"]++
		} else {
			item.Status = "invalid"
			summary["invalid"]++
		}
	}
	contextVersion := signingDocumentBatchContextVersion(configs, *active)
	_ = s.store.WriteAuditWithMetadata(r.Context(), actor.ID, "signing_document.batch_validate", "signing_document_batch", format.Code, clientIP(r), r.UserAgent(), map[string]any{
		"docFormatCode": format.Code,
		"total":         summary["total"], "ready": summary["ready"], "warning": summary["warning"], "invalid": summary["invalid"],
		"pageCount": totalPageCount,
		"bytes":     totalBytes,
		"elapsedMs": time.Since(startedAt).Milliseconds(),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"docFormatCode":  format.Code,
		"contextVersion": contextVersion,
		"items":          items,
		"summary":        summary,
	})
}

func (s *Server) recordSigningDocumentBatchEvent(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var req signingDocumentBatchEventRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_batch_event", "Batch event payload is invalid.")
		return
	}
	req.Event = strings.TrimSpace(req.Event)
	if req.Event != "batch_retry" && req.Event != "batch_discard" {
		writeError(w, http.StatusBadRequest, "invalid_batch_event", "Batch event is invalid.")
		return
	}
	metadata := map[string]any{
		"docFormatCode": truncateForMetadata(strings.ToUpper(strings.TrimSpace(req.DocFormatCode)), 25),
		"total":         clampInt(req.Total, 0, 30),
		"created":       clampInt(req.Created, 0, 30),
		"failed":        clampInt(req.Failed, 0, 30),
	}
	if err := s.store.WriteAuditWithMetadata(r.Context(), actor.ID, "signing_document."+req.Event, "signing_document_batch", "", clientIP(r), r.UserAgent(), metadata); err != nil {
		s.logger.Warn("write signing document batch event failed", "error", err, "event", req.Event)
		writeError(w, http.StatusInternalServerError, "batch_event_failed", "Cannot record batch event right now.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func batchValidationIssue(code, message string, retryable bool) signingDocumentBatchValidationIssue {
	return signingDocumentBatchValidationIssue{Code: code, Message: message, Retryable: retryable}
}

func applyBatchPageLimit(items []signingDocumentBatchValidationItem, totalPageCount int) {
	if totalPageCount <= 100 {
		return
	}
	message := fmt.Sprintf("ชุดนี้มี PDF รวม %d หน้า เกินขีดจำกัด 100 หน้า กรุณาลบบางรายการ", totalPageCount)
	for index := range items {
		items[index].Issues = append(items[index].Issues, batchValidationIssue("batch_page_limit", message, false))
	}
}

func hasBatchIssue(issues []signingDocumentBatchValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func batchIssuesExcept(issues []signingDocumentBatchValidationIssue, ignoredCode string) int {
	count := 0
	for _, issue := range issues {
		if issue.Code != ignoredCode {
			count++
		}
	}
	return count
}

func normalizeBatchFileIDs(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 30 {
		return nil, fmt.Errorf("กรุณาเลือกไฟล์ตั้งแต่ 1 ถึง 30 ไฟล์")
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if !isUUIDText(value) {
			return nil, fmt.Errorf("พบรหัสไฟล์ที่ไม่ถูกต้อง")
		}
		if seen[value] {
			return nil, fmt.Errorf("พบไฟล์ซ้ำในคำขอตรวจสอบ")
		}
		seen[value] = true
		result = append(result, value)
	}
	return result, nil
}

func isUUIDText(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, ch := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if ch != '-' {
				return false
			}
			continue
		}
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}

func serveInlinePDF(w http.ResponseWriter, r *http.Request, file models.UploadedFile) {
	setNoStoreHeaders(w)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", file.OriginalName))
	http.ServeFile(w, r, file.StoragePath)
}

func serveInlineUploadedFile(w http.ResponseWriter, r *http.Request, file models.UploadedFile) {
	contentType := strings.TrimSpace(file.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	setNoStoreHeaders(w)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", file.OriginalName))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, file.StoragePath)
}

func (s *Server) recordSigningDocumentCreateEvent(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	req, err := decodeSigningCreateEventPayload(r.Body, maxSigningEventBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_create_event", "Create event payload is invalid.")
		return
	}
	metadata, err := normalizeSigningCreateEventMetadata(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_create_event", "Create event payload is invalid.")
		return
	}
	if err := s.store.WriteAuditWithMetadata(r.Context(), user.ID, "signing_document.create_ux_event", "signing_document_create", "", clientIP(r), r.UserAgent(), metadata); err != nil {
		s.logger.Warn("write signing create event failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_event_failed", "Cannot record create event right now.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createSigningDocument(w http.ResponseWriter, r *http.Request) {
	s.createSigningDocumentWithMode(w, r, false)
}

func (s *Server) createSigningDocumentBatchItem(w http.ResponseWriter, r *http.Request) {
	s.createSigningDocumentWithMode(w, r, true)
}

func (s *Server) createSigningDocumentWithMode(w http.ResponseWriter, r *http.Request, batchMode bool) {
	actor, _ := currentUser(r)
	startedAt := time.Now()
	if strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
		writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required when creating a signing document.")
		return
	}
	idempotencyScope := "signing_document_create"
	if batchMode {
		idempotencyScope = "signing_document_batch_item_create"
	}
	if s.replayIdempotentResponse(w, r, idempotencyScope, actor.ID) {
		return
	}
	idempotencyCompleted := false
	defer func() {
		if !idempotencyCompleted {
			s.releaseIdempotency(idempotencyScope, actor.ID, r)
		}
	}()

	req, ok := s.decodeCreateSigningDocumentRequest(w, r)
	if !ok {
		return
	}
	req.DocFormatCode = strings.TrimSpace(req.DocFormatCode)
	req.DocNo = strings.TrimSpace(req.DocNo)
	req.FileID = strings.TrimSpace(req.FileID)
	req.SignatureTemplateID = strings.TrimSpace(req.SignatureTemplateID)
	req.ContextVersion = strings.TrimSpace(req.ContextVersion)
	if req.DocFormatCode == "" || (!batchMode && req.DocNo == "") {
		writeError(w, http.StatusBadRequest, "document_required", "doc_format_code and doc_no are required.")
		return
	}
	if req.FileID == "" {
		writeError(w, http.StatusBadRequest, "document_pdf_required", "Uploaded PDF fileId is required.")
		return
	}

	var uploaded models.UploadedFile
	var err error
	if batchMode {
		uploaded, err = s.store.FindSigningDocumentUploadFile(r.Context(), req.FileID, actor.ID)
		if errors.Is(err, store.ErrSigningDocumentUploadNotFound) {
			writeError(w, http.StatusNotFound, "upload_not_found", "Uploaded PDF was not found or has expired. Upload the PDF again.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "upload_failed", "Cannot load uploaded PDF right now.")
			return
		}
		if uploaded.PageCount > 100 {
			writeError(w, http.StatusBadRequest, "pdf_page_limit", "A single PDF in batch import cannot exceed 100 pages. Create this document separately.")
			return
		}
		req.DocNo, err = documentNumberFromPDFName(uploaded.OriginalName)
		if err != nil {
			writeError(w, http.StatusBadRequest, "filename_invalid", err.Error())
			return
		}
		if req.ContextVersion == "" {
			writeError(w, http.StatusBadRequest, "context_version_required", "Batch validation must be completed before importing documents.")
			return
		}
	}

	format, err := s.fetchSMLDocFormatByCode(r.Context(), req.DocFormatCode)
	if err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}
	candidate, err := s.fetchSMLDocumentCandidate(r.Context(), format.Code, req.DocNo)
	if err != nil {
		writeError(w, http.StatusBadGateway, "sml_document_validation_failed", "Cannot verify selected SML document.")
		return
	}
	if candidate.IsLockRecord == 1 && !req.ConfirmLocked {
		writeError(w, http.StatusConflict, "sml_document_locked", "SML document is already locked. Confirm before creating a PaperLess document.")
		return
	}
	duplicateCheck, err := s.store.CheckSigningDocumentDuplicate(r.Context(), format.Code, candidate.DocNo)
	if err != nil {
		s.logger.Error("check signing document duplicate failed", "error", err)
		writeError(w, http.StatusInternalServerError, "duplicate_check_failed", "ตรวจสอบเอกสารซ้ำไม่สำเร็จ")
		return
	}
	if !duplicateCheck.CanCreate {
		s.writeSigningDocumentDuplicateConflict(w, duplicateCheck, actor.ID)
		return
	}
	if !batchMode {
		uploaded, err = s.store.FindSigningDocumentUploadFile(r.Context(), req.FileID, actor.ID)
		if errors.Is(err, store.ErrSigningDocumentUploadNotFound) {
			writeError(w, http.StatusNotFound, "upload_not_found", "Uploaded PDF was not found or has expired. Upload the PDF again.")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "upload_failed", "Cannot load uploaded PDF right now.")
			return
		}
	}

	screenCode := normalizeScreenCode(format.ScreenCode)
	configs, err := s.store.ListDocumentConfigSteps(r.Context(), screenCode, format.Code)
	if err != nil || len(configs) == 0 {
		writeError(w, http.StatusBadRequest, "document_config_required", "Document config is required before sending for signature.")
		return
	}
	layoutSource := "per_document_upload"
	req.SignNoteBoxes = nil
	if actor.Role == "admin" || batchMode {
		_, active, templateErr := s.store.GetSignatureTemplateState(r.Context(), screenCode, format.Code)
		if templateErr != nil {
			s.logger.Error("load admin create template failed", "error", templateErr, "docFormatCode", format.Code)
			writeError(w, http.StatusInternalServerError, "signature_template_failed", "Cannot load active signature template right now.")
			return
		}
		if batchMode && active != nil && signingDocumentBatchContextVersion(configs, *active) != req.ContextVersion {
			writeError(w, http.StatusConflict, "batch_context_changed", "Workflow or Active Template changed. Please validate the batch again.")
			return
		}
		templateID, layout, legalBoxes, issues := activeTemplateCreateLayout(active, uploaded.PageCount)
		if len(issues) > 0 {
			writeValidationIssues(w, http.StatusBadRequest, "signature_template_required", issues)
			return
		}
		req.SignatureTemplateID = templateID
		req.LayoutBoxes = layout
		req.LegalNoticeBoxes = legalBoxes
		req.LegalNoticeBox = &legalBoxes[0]
		layoutSource = "active_template_locked"
	}
	layoutBoxes, selectedConfigs, signaturePlacements, issues := validateSigningDocumentLayout(req.LayoutBoxes, configs, uploaded.PageCount)
	if len(issues) == 0 {
		issues = append(issues, s.inactiveSigningLayoutUserIssues(r.Context(), selectedConfigs, layoutBoxes)...)
	}
	signNoteBoxes := []models.SignatureTemplateBoxRequest{}
	signNotePlacements := []models.SignNotePlacementSnapshot{}
	legalNoticeBoxes, legalNoticeIssues := normalizeAndValidateLegalNoticeBoxes(req.LegalNoticeBoxes, req.LegalNoticeBox, uploaded.PageCount, true)
	issues = append(issues, legalNoticeIssues...)
	if len(issues) > 0 {
		writeValidationIssues(w, http.StatusBadRequest, "signature_layout_invalid", issues)
		return
	}
	legalNoticeSnapshots := make([]models.LegalNoticeSnapshot, 0, len(legalNoticeBoxes))
	for _, box := range legalNoticeBoxes {
		legalNoticeSource := "per_document"
		if box.Source == "preset" || (box.Source == "" && req.SignatureTemplateID != "") {
			legalNoticeSource = "preset"
		}
		legalNoticeSnapshots = append(legalNoticeSnapshots, legalNoticeSnapshotFromBox(box, legalNoticeSource))
	}
	legalNoticeSnapshot := legalNoticeSnapshots[0]
	currentFile, ok := s.createInitialLegalNoticePDF(w, r, uploaded, legalNoticeSnapshots, actor.ID)
	if !ok {
		return
	}

	layoutSnapshot := map[string]any{
		"source":              layoutSource,
		"signatureTemplateId": req.SignatureTemplateID,
		"pageCount":           uploaded.PageCount,
		"boxes":               layoutBoxes,
		"signaturePlacements": signaturePlacements,
		"legalNoticeBox":      legalNoticeBoxes[0],
		"legalNoticeBoxes":    legalNoticeBoxes,
		"signNoteBoxes":       signNoteBoxes,
		"signNotePlacements":  signNotePlacements,
	}
	session, _ := currentSession(r)
	document, err := s.store.CreateSigningDocument(r.Context(), store.CreateSigningDocumentInput{
		ScreenCode:          screenCode,
		Format:              format,
		Candidate:           candidate,
		SMLDataGroup:        session.SMLDataGroup,
		SMLDataCode:         session.SMLDataCode,
		SignatureTemplateID: req.SignatureTemplateID,
		TemplateSnapshot:    layoutSnapshot,
		LegalNoticeSnapshot: legalNoticeSnapshot,
		LegalNoticeBoxes:    legalNoticeSnapshots,
		SignaturePlacements: signaturePlacements,
		SignNotePlacements:  signNotePlacements,
		LayoutBoxes:         layoutBoxes,
		Configs:             selectedConfigs,
		File:                uploaded,
		CurrentFile:         &currentFile,
		CurrentLegalVersion: signingLegalNoticePDFDisplayVersion,
		ActorID:             actor.ID,
		IPAddress:           clientIP(r),
		UserAgent:           r.UserAgent(),
	})
	if err != nil && currentFile.ID != "" && currentFile.ID != uploaded.ID {
		s.cleanupUploadedFileBestEffort(currentFile, "create_signing_document_failed")
	}
	if errors.Is(err, store.ErrSigningDocumentDuplicate) {
		duplicateCheck, duplicateErr := s.store.CheckSigningDocumentDuplicate(r.Context(), format.Code, candidate.DocNo)
		if duplicateErr != nil || duplicateCheck.CanCreate {
			duplicateCheck = store.SigningDocumentDuplicateCheckResult{
				CanCreate: false,
				Message:   "เอกสารนี้มีอยู่ใน PaperLess แล้ว กรุณาเปิดเอกสารเดิมแทนการสร้างซ้ำ",
			}
		}
		s.writeSigningDocumentDuplicateConflict(w, duplicateCheck, actor.ID)
		return
	}
	if errors.Is(err, store.ErrSigningDocumentUploadNotFound) {
		writeError(w, http.StatusConflict, "upload_already_used", "Uploaded PDF was already used or expired. Upload the PDF again.")
		return
	}
	if err != nil {
		s.logger.Error("create signing document failed", "error", err)
		writeError(w, http.StatusInternalServerError, "signing_document_create_failed", "Cannot create signing document right now.")
		return
	}
	payload := map[string]any{"document": s.withExternalURLs(r, document)}
	if batchMode {
		_ = s.store.WriteAuditWithMetadata(r.Context(), actor.ID, "signing_document.batch_import", "signing_document", document.ID, clientIP(r), r.UserAgent(), map[string]any{
			"docFormatCode": document.DocFormatCode,
			"pageCount":     uploaded.PageCount,
			"bytes":         uploaded.SizeBytes,
			"elapsedMs":     time.Since(startedAt).Milliseconds(),
		})
	}
	s.completeIdempotency(idempotencyScope, actor.ID, r, http.StatusCreated, payload)
	idempotencyCompleted = true
	writeJSON(w, http.StatusCreated, payload)
}

func (s *Server) writeSigningDocumentDuplicateConflict(w http.ResponseWriter, result store.SigningDocumentDuplicateCheckResult, actorID string) {
	result.CanCreate = false
	result = prepareSigningDocumentDuplicateResponse(result, actorID)
	message := strings.TrimSpace(result.Message)
	if message == "" {
		message = "เอกสารนี้มีอยู่ใน PaperLess แล้ว กรุณาเปิดเอกสารเดิมแทนการสร้างซ้ำ"
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":             "signing_document_duplicate",
		"message":           message,
		"canCreate":         false,
		"blockingDocument":  result.BlockingDocument,
		"previousDocuments": result.PreviousDocuments,
	})
}

func prepareSigningDocumentDuplicateResponse(result store.SigningDocumentDuplicateCheckResult, actorID string) store.SigningDocumentDuplicateCheckResult {
	if result.BlockingDocument != nil && isOtherUserDraftReference(*result.BlockingDocument, actorID) {
		ref := *result.BlockingDocument
		result.BlockingDocument = &models.SigningDocumentReference{
			DocNo:         ref.DocNo,
			DocFormatCode: ref.DocFormatCode,
			Status:        ref.Status,
			CreatedAt:     ref.CreatedAt,
			UpdatedAt:     ref.UpdatedAt,
		}
		result.Message = "เอกสารนี้มีอยู่ใน PaperLess แล้วและอยู่ระหว่างเตรียมส่ง กรุณาตรวจสอบกับผู้สร้างเอกสารหรือผู้ดูแลระบบ"
	} else {
		result.BlockingDocument = enrichSigningDocumentReferenceForAdmin(result.BlockingDocument)
	}
	for i := range result.PreviousDocuments {
		result.PreviousDocuments[i] = enrichSigningDocumentReferenceForAdminValue(result.PreviousDocuments[i])
	}
	return result
}

func isOtherUserDraftReference(ref models.SigningDocumentReference, actorID string) bool {
	return strings.EqualFold(strings.TrimSpace(ref.Status), "draft") &&
		(strings.TrimSpace(ref.CreatedBy) == "" || strings.TrimSpace(ref.CreatedBy) != strings.TrimSpace(actorID))
}

func enrichSigningDocumentReferenceForAdmin(ref *models.SigningDocumentReference) *models.SigningDocumentReference {
	if ref == nil {
		return nil
	}
	item := enrichSigningDocumentReferenceForAdminValue(*ref)
	return &item
}

func enrichSigningDocumentReferenceForAdminValue(ref models.SigningDocumentReference) models.SigningDocumentReference {
	ref.CanOpenPaperless = strings.TrimSpace(ref.ID) != ""
	ref.CanViewCurrentPDF = ref.CanOpenPaperless && ref.HasCurrentPDF
	ref.CanViewSignedPDF = ref.CanOpenPaperless && ref.HasFinalPDF
	if ref.CanViewCurrentPDF {
		ref.CurrentPDFURL = signingDocumentPDFURL(ref.ID, "current", ref.UpdatedAt)
	}
	if ref.CanViewSignedPDF {
		ref.SignedPDFURL = signingDocumentPDFURL(ref.ID, "final", ref.UpdatedAt)
	}
	return ref
}

func signingDocumentPDFURL(documentID, version string, updatedAt time.Time) string {
	url := fmt.Sprintf("/api/signing-documents/%s/pdf?version=%s", documentID, version)
	if !updatedAt.IsZero() {
		url = fmt.Sprintf("%s&v=%d", url, updatedAt.UTC().UnixNano())
	}
	return url
}

func (s *Server) sendSigningDocument(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	documentID := strings.TrimSpace(r.PathValue("id"))
	scope := "signing_document_send:" + documentID
	if strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
		writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required when sending a signing document.")
		return
	}
	if s.replayIdempotentResponse(w, r, scope, actor.ID) {
		return
	}
	idempotencyCompleted := false
	defer func() {
		if !idempotencyCompleted {
			s.releaseIdempotency(scope, actor.ID, r)
		}
	}()

	document, err := s.store.SendSigningDocument(r.Context(), documentID, actor.ID, clientIP(r), r.UserAgent())
	if s.writeSigningDocumentTransitionError(w, err, "send_signing_document_failed", "Cannot send signing document right now.") {
		return
	}
	payload := map[string]any{"document": s.withExternalURLs(r, document)}
	s.completeIdempotency(scope, actor.ID, r, http.StatusOK, payload)
	idempotencyCompleted = true
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) confirmSigningDocument(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	documentID := strings.TrimSpace(r.PathValue("id"))
	scope := "signing_document_confirm:" + documentID
	if strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
		writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required when confirming a signing document.")
		return
	}
	if s.replayIdempotentResponse(w, r, scope, actor.ID) {
		return
	}
	idempotencyCompleted := false
	defer func() {
		if !idempotencyCompleted {
			s.releaseIdempotency(scope, actor.ID, r)
		}
	}()

	if _, err := s.store.PrepareSigningDocumentConfirmation(r.Context(), documentID, actor.ID, clientIP(r), r.UserAgent()); s.writeSigningDocumentTransitionError(w, err, "confirm_signing_document_failed", "Cannot confirm signing document right now.") {
		return
	}
	result := s.finalizeCompletedDocument(r.Context(), documentID, clientIP(r), r.UserAgent())
	updated, _ := s.store.FindSigningDocumentByID(r.Context(), documentID)
	payload := map[string]any{
		"document": s.withExternalURLs(r, updated),
		"finalOk":  result.FinalOK,
		"imageOk":  result.ImageOK,
		"lockOk":   result.LockOK,
		"image":    result.ImageMetadata,
		"lock":     result.LockMetadata,
	}
	if result.LockOK {
		_ = s.store.AddSigningEvent(context.Background(), documentID, actor.ID, "", "document_confirmed", "ผู้ดูแลยืนยันเอกสารเรียบร้อยแล้ว", clientIP(r), r.UserAgent(), nil)
		updated, _ = s.store.FindSigningDocumentByID(r.Context(), documentID)
		payload["document"] = s.withExternalURLs(r, updated)
	}
	s.completeIdempotency(scope, actor.ID, r, http.StatusOK, payload)
	idempotencyCompleted = true
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) cancelSigningDocument(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	documentID := strings.TrimSpace(r.PathValue("id"))
	scope := "signing_document_cancel:" + documentID
	if strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
		writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required when cancelling a signing document.")
		return
	}
	if s.replayIdempotentResponse(w, r, scope, actor.ID) {
		return
	}
	idempotencyCompleted := false
	defer func() {
		if !idempotencyCompleted {
			s.releaseIdempotency(scope, actor.ID, r)
		}
	}()

	document, err := s.store.CancelSigningDocument(r.Context(), documentID, actor.ID, clientIP(r), r.UserAgent())
	if s.writeSigningDocumentTransitionError(w, err, "cancel_signing_document_failed", "Cannot cancel signing document right now.") {
		return
	}
	payload := map[string]any{"document": s.withExternalURLs(r, document)}
	s.completeIdempotency(scope, actor.ID, r, http.StatusOK, payload)
	idempotencyCompleted = true
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) writeSigningDocumentTransitionError(w http.ResponseWriter, err error, code, message string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrSigningDocumentNotFound) {
		writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
		return true
	}
	if errors.Is(err, store.ErrSigningDocumentInvalidStatus) {
		writeError(w, http.StatusConflict, "signing_document_status_invalid", "Document status does not allow this action.")
		return true
	}
	s.logger.Error(message, "error", err)
	writeError(w, http.StatusInternalServerError, code, message)
	return true
}

func (s *Server) decodeCreateSigningDocumentRequest(w http.ResponseWriter, r *http.Request) (createSigningDocumentRequest, bool) {
	var req createSigningDocumentRequest
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.Contains(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_form", "Document form is invalid.")
			return req, false
		}
		req.DocFormatCode = firstNonEmpty(r.FormValue("docFormatCode"), r.FormValue("doc_format_code"))
		req.DocNo = firstNonEmpty(r.FormValue("docNo"), r.FormValue("doc_no"))
		req.FileID = firstNonEmpty(r.FormValue("fileId"), r.FormValue("file_id"))
		req.SignatureTemplateID = firstNonEmpty(r.FormValue("signatureTemplateId"), r.FormValue("signature_template_id"))
		req.ConfirmLocked = strings.TrimSpace(r.FormValue("confirmLocked")) == "1" || strings.EqualFold(strings.TrimSpace(r.FormValue("confirmLocked")), "true")
		rawBoxes := firstNonEmpty(r.FormValue("layoutBoxes"), r.FormValue("layout_boxes"))
		if rawBoxes != "" {
			if err := json.Unmarshal([]byte(rawBoxes), &req.LayoutBoxes); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_layout_boxes", "layout_boxes must be valid JSON.")
				return req, false
			}
		}
		rawSignNoteBoxes := firstNonEmpty(r.FormValue("signNoteBoxes"), r.FormValue("sign_note_boxes"))
		if rawSignNoteBoxes != "" {
			if err := json.Unmarshal([]byte(rawSignNoteBoxes), &req.SignNoteBoxes); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_sign_note_boxes", "sign_note_boxes must be valid JSON.")
				return req, false
			}
		}
		rawLegalNoticeBox := firstNonEmpty(r.FormValue("legalNoticeBox"), r.FormValue("legal_notice_box"))
		if rawLegalNoticeBox != "" {
			if err := json.Unmarshal([]byte(rawLegalNoticeBox), &req.LegalNoticeBox); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_legal_notice_box", "legal_notice_box must be valid JSON.")
				return req, false
			}
		}
		rawLegalNoticeBoxes := firstNonEmpty(r.FormValue("legalNoticeBoxes"), r.FormValue("legal_notice_boxes"))
		if rawLegalNoticeBoxes != "" {
			if err := json.Unmarshal([]byte(rawLegalNoticeBoxes), &req.LegalNoticeBoxes); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_legal_notice_boxes", "legal_notice_boxes must be valid JSON.")
				return req, false
			}
		}
		return req, true
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return req, false
	}
	return req, true
}

func (s *Server) createInitialLegalNoticePDF(w http.ResponseWriter, r *http.Request, uploaded models.UploadedFile, legalNotices []models.LegalNoticeSnapshot, actorID string) (models.UploadedFile, bool) {
	if uploaded.StoragePath == "" || uploaded.PageCount <= 0 {
		writeError(w, http.StatusBadRequest, "document_pdf_invalid", "Uploaded PDF is invalid.")
		return models.UploadedFile{}, false
	}
	stamped, err := stampPDFWithSignaturePlacementsAndLegalNotices(uploaded.StoragePath, uploaded.PageCount, nil, nil, nil, legalNotices, nil)
	if err != nil {
		s.logger.Error("create initial legal notice pdf failed", "error", err, "fileID", uploaded.ID)
		writeError(w, http.StatusInternalServerError, "pdf_legal_notice_failed", "Cannot add legal notice to the PDF right now.")
		return models.UploadedFile{}, false
	}
	pageCount := uploaded.PageCount
	if count, err := readPDFPageCount(stamped); err == nil && count > 0 {
		pageCount = count
	}
	name := fmt.Sprintf("%s-legal-notice.pdf", strings.TrimSuffix(filepath.Base(uploaded.OriginalName), filepath.Ext(uploaded.OriginalName)))
	currentFile, err := s.storeUploadedBytes(r.Context(), stamped, name, "legal-notice-document.pdf", "application/pdf", ".pdf", pageCount, actorID)
	if err != nil {
		s.logger.Error("store initial legal notice pdf failed", "error", err, "fileID", uploaded.ID)
		writeError(w, http.StatusInternalServerError, "pdf_legal_notice_store_failed", "Cannot store legal notice PDF right now.")
		return models.UploadedFile{}, false
	}
	return currentFile, true
}

func currentPDFNeedsLegalNoticeRefresh(document models.SigningDocument) bool {
	if len(documentLegalNotices(document)) == 0 {
		return false
	}
	for _, event := range document.Events {
		if event.Action != "pdf_stamped" && event.Action != "final_pdf_ready" {
			continue
		}
		if metadataBool(event.Metadata, "legalNoticeStamped") && metadataString(event.Metadata, "legalNoticeDisplayVersion") == signingLegalNoticePDFDisplayVersion {
			return false
		}
	}
	return true
}

func currentPDFNeedsSignatureTransparencyRefresh(document models.SigningDocument) bool {
	if len(signedDocumentSigners(document.Signers)) == 0 {
		return false
	}
	for _, event := range document.Events {
		if event.Action != "pdf_stamped" && event.Action != "final_pdf_ready" {
			continue
		}
		if metadataString(event.Metadata, "signatureTransparencyVersion") == signatureTransparencyVersion {
			return false
		}
	}
	return true
}

func metadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	value, ok := metadata[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func decodeSigningCreateEventPayload(body io.Reader, maxBytes int64) (signingCreateEventRequest, error) {
	var req signingCreateEventRequest
	decoder := json.NewDecoder(io.LimitReader(body, maxBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

func normalizeSigningCreateEventMetadata(req signingCreateEventRequest) (map[string]any, error) {
	req.Event = strings.TrimSpace(req.Event)
	if !signingCreateEventNames[req.Event] {
		return nil, fmt.Errorf("invalid event")
	}
	metadata := map[string]any{
		"event":                req.Event,
		"sessionId":            truncateForMetadata(req.SessionID, 80),
		"docFormatCode":        truncateForMetadata(req.DocFormatCode, 40),
		"elapsedMs":            clampInt64(req.ElapsedMS, 0, 24*60*60*1000),
		"boxCount":             clampInt(req.BoxCount, 0, 1000),
		"validationIssueCount": clampInt(req.ValidationIssueCount, 0, 1000),
		"viewport": map[string]any{
			"width":  clampInt(req.Viewport.Width, 0, 10000),
			"height": clampInt(req.Viewport.Height, 0, 10000),
		},
	}
	return metadata, nil
}

func (s *Server) lockedAdminCreateLayout(ctx context.Context, screenCode, docFormatCode string, pageCount int) (string, []models.SignatureTemplateBoxRequest, []models.SignatureTemplateBoxRequest, []models.LegalNoticeBoxRequest, []models.SignatureValidationIssue, error) {
	_, active, err := s.store.GetSignatureTemplateState(ctx, screenCode, docFormatCode)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	templateID, layout, legalBoxes, issues := activeTemplateCreateLayout(active, pageCount)
	return templateID, layout, nil, legalBoxes, issues, nil
}

func activeTemplateCreateLayout(active *models.SignatureTemplate, pageCount int) (string, []models.SignatureTemplateBoxRequest, []models.LegalNoticeBoxRequest, []models.SignatureValidationIssue) {
	issues := []models.SignatureValidationIssue{}
	if active == nil {
		issues = append(issues, signatureIssue("signature_template_required", "", "Active signature template is required. Please contact superadmin."))
		return "", nil, nil, issues
	}
	if len(active.Boxes) == 0 {
		issues = append(issues, signatureIssue("signature_template_boxes_required", "", "Active signature template has no signature boxes. Please contact superadmin."))
	}
	legalBox := legalNoticeBoxRequestFromTemplate(active.LegalNoticeBox)
	if legalBox == nil {
		issues = append(issues, signatureIssue("signature_template_legal_notice_required", "", "Active signature template has no legal notice box. Please contact superadmin."))
	}
	if len(issues) > 0 {
		return active.ID, nil, nil, issues
	}
	samplePageCount := 0
	if active.SampleFile != nil {
		samplePageCount = active.SampleFile.PageCount
	}
	layout := expandTemplateBoxesForDocument(boxRequestsFromTemplate(active.Boxes), samplePageCount, pageCount)
	legalBoxes := expandLegalNoticeBoxesForDocument([]models.LegalNoticeBoxRequest{*legalBox}, samplePageCount, pageCount)
	return active.ID, layout, legalBoxes, nil
}

func expandTemplateBoxesForDocument(source []models.SignatureTemplateBoxRequest, samplePageCount, targetPageCount int) []models.SignatureTemplateBoxRequest {
	pages := maxInt(1, targetPageCount)
	mismatch := samplePageCount > 0 && samplePageCount != pages
	if !mismatch {
		out := make([]models.SignatureTemplateBoxRequest, 0, len(source))
		for _, box := range source {
			box.PageNo = clampPageNo(box.PageNo, pages)
			out = append(out, box)
		}
		return out
	}
	pattern := []models.SignatureTemplateBoxRequest{}
	for _, box := range source {
		if box.PageNo == 0 || box.PageNo == 1 {
			pattern = append(pattern, box)
		}
	}
	if len(pattern) == 0 {
		pattern = source
	}
	out := make([]models.SignatureTemplateBoxRequest, 0, len(pattern)*pages)
	for pageNo := 1; pageNo <= pages; pageNo++ {
		for _, box := range pattern {
			box.PageNo = pageNo
			out = append(out, box)
		}
	}
	return out
}

func expandLegalNoticeBoxesForDocument(source []models.LegalNoticeBoxRequest, samplePageCount, targetPageCount int) []models.LegalNoticeBoxRequest {
	pages := maxInt(1, targetPageCount)
	mismatch := samplePageCount > 0 && samplePageCount != pages
	if !mismatch {
		out := make([]models.LegalNoticeBoxRequest, 0, len(source))
		for _, box := range source {
			box.PageNo = clampPageNo(box.PageNo, pages)
			if strings.TrimSpace(box.Source) == "" {
				box.Source = "preset"
			}
			out = append(out, box)
		}
		return out
	}
	pattern := []models.LegalNoticeBoxRequest{}
	for _, box := range source {
		if box.PageNo == 0 || box.PageNo == 1 {
			pattern = append(pattern, box)
		}
	}
	if len(pattern) == 0 {
		pattern = source
	}
	out := make([]models.LegalNoticeBoxRequest, 0, len(pattern)*pages)
	for pageNo := 1; pageNo <= pages; pageNo++ {
		for _, box := range pattern {
			box.PageNo = pageNo
			if strings.TrimSpace(box.Source) == "" {
				box.Source = "preset"
			}
			out = append(out, box)
		}
	}
	return out
}

func clampPageNo(value, pageCount int) int {
	if value < 1 {
		return 1
	}
	if value > pageCount {
		return pageCount
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func validateSigningDocumentLayout(boxes []models.SignatureTemplateBoxRequest, configs []models.DocumentConfigStep, pageCount int) ([]models.SignatureTemplateBoxRequest, []models.DocumentConfigStep, []models.SignaturePlacementSnapshot, []models.SignatureValidationIssue) {
	normalized, issues := normalizeAndValidateSigningDocumentPlacements(boxes, pageCount)
	if len(normalized) == 0 {
		issues = append(issues, signatureIssue("layout_box_required", "", "Add at least one signature box before sending the document."))
	}

	stepsByPosition := map[string]models.DocumentConfigStep{}
	for _, step := range configs {
		stepsByPosition[strings.ToLower(strings.TrimSpace(step.PositionCode))] = step
	}
	boxesByPosition := map[string][]models.SignatureTemplateBoxRequest{}
	for _, box := range normalized {
		key := strings.ToLower(strings.TrimSpace(box.PositionCode))
		boxesByPosition[key] = append(boxesByPosition[key], box)
		if _, ok := stepsByPosition[key]; !ok && box.PositionCode != "" {
			issues = append(issues, signatureIssue("box_position_unknown", box.PositionCode, "Signature box uses a position that is not in document config."))
		}
	}

	taskBoxes := []models.SignatureTemplateBoxRequest{}
	selected := []models.DocumentConfigStep{}
	placements := []models.SignaturePlacementSnapshot{}
	for _, step := range configs {
		key := strings.ToLower(strings.TrimSpace(step.PositionCode))
		positionBoxes := boxesByPosition[key]
		if len(positionBoxes) == 0 {
			continue
		}
		sortPlacementBoxes(positionBoxes)
		selected = append(selected, step)
		switch step.ConditionType {
		case 1:
			if len(stepUsers(step)) == 0 {
				issues = append(issues, signatureIssue("condition_any_users_required", step.PositionCode, fmt.Sprintf("%s needs at least one configured user.", step.PositionName)))
			}
			for i := range positionBoxes {
				positionBoxes[i].SignerType = "any"
				positionBoxes[i].SignerUser = ""
				positionBoxes[i].SignerSlot = 1
				placements = append(placements, signaturePlacementSnapshotFromBox(step, positionBoxes[i], "any", "", "", 1))
			}
			taskBoxes = append(taskBoxes, positionBoxes[0])
		case 2:
			required := map[string]struct {
				Full    string
				Slot    int
				Display string
			}{}
			for index, user := range stepUsers(step) {
				username, display := splitSignerUser(user)
				if username != "" {
					required[strings.ToLower(username)] = struct {
						Full    string
						Slot    int
						Display string
					}{Full: user, Slot: index + 1, Display: display}
				}
			}
			if len(required) == 0 {
				issues = append(issues, signatureIssue("condition_all_users_required", step.PositionCode, fmt.Sprintf("%s needs at least one configured user.", step.PositionName)))
			}
			seen := map[string]int{}
			primaryByUser := map[string]models.SignatureTemplateBoxRequest{}
			for i := range positionBoxes {
				positionBoxes[i].SignerType = "internal"
				username, _ := splitSignerUser(positionBoxes[i].SignerUser)
				key := strings.ToLower(username)
				if key == "" {
					issues = append(issues, signatureIssue("condition_all_user_required", step.PositionCode, fmt.Sprintf("%s requires a signer user on every box.", step.PositionName)))
					continue
				}
				requiredUser, ok := required[key]
				if !ok {
					issues = append(issues, signatureIssue("condition_all_unknown_user_box", step.PositionCode, fmt.Sprintf("%s has a box for a user outside this position: %s.", step.PositionName, username)))
					continue
				}
				seen[key]++
				positionBoxes[i].SignerUser = requiredUser.Full
				positionBoxes[i].SignerSlot = requiredUser.Slot
				placements = append(placements, signaturePlacementSnapshotFromBox(step, positionBoxes[i], "internal", username, requiredUser.Display, requiredUser.Slot))
				if _, ok := primaryByUser[key]; !ok {
					primaryByUser[key] = positionBoxes[i]
				}
			}
			for _, user := range stepUsers(step) {
				username, _ := splitSignerUser(user)
				key := strings.ToLower(username)
				if key == "" {
					continue
				}
				requiredUser := required[key]
				if seen[key] == 0 {
					issues = append(issues, signatureIssue("condition_all_missing_user_box", step.PositionCode, fmt.Sprintf("%s needs a signature box for %s.", step.PositionName, requiredUser.Full)))
					continue
				}
				taskBoxes = append(taskBoxes, primaryByUser[key])
			}
		case 3:
			for i := range positionBoxes {
				positionBoxes[i].SignerType = "external"
				positionBoxes[i].SignerUser = ""
				positionBoxes[i].SignerSlot = 1
				placements = append(placements, signaturePlacementSnapshotFromBox(step, positionBoxes[i], "external", "", "บุคคลภายนอก", 1))
			}
			taskBoxes = append(taskBoxes, positionBoxes[0])
		default:
			issues = append(issues, signatureIssue("condition_type_invalid", step.PositionCode, fmt.Sprintf("%s uses unsupported condition type.", step.PositionName)))
		}
	}
	sort.SliceStable(taskBoxes, func(i, j int) bool {
		if taskBoxes[i].PositionCode == taskBoxes[j].PositionCode {
			return taskBoxes[i].SignerSlot < taskBoxes[j].SignerSlot
		}
		return taskBoxes[i].PositionCode < taskBoxes[j].PositionCode
	})
	sort.SliceStable(placements, func(i, j int) bool {
		if placements[i].SequenceNo == placements[j].SequenceNo {
			if placements[i].PositionCode == placements[j].PositionCode {
				if placements[i].SignerSlot == placements[j].SignerSlot {
					return placements[i].PageNo < placements[j].PageNo
				}
				return placements[i].SignerSlot < placements[j].SignerSlot
			}
			return placements[i].PositionCode < placements[j].PositionCode
		}
		return placements[i].SequenceNo < placements[j].SequenceNo
	})
	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].PositionCode < issues[j].PositionCode
	})
	return taskBoxes, selected, placements, issues
}

func validateSigningDocumentSignNoteLayout(boxes []models.SignatureTemplateBoxRequest, configs []models.DocumentConfigStep, pageCount int) ([]models.SignatureTemplateBoxRequest, []models.SignNotePlacementSnapshot, []models.SignatureValidationIssue) {
	normalized, issues := normalizeAndValidateSignNoteBoxRequests(boxes, pageCount)
	if len(normalized) == 0 {
		return nil, nil, issues
	}

	stepsByPosition := map[string]models.DocumentConfigStep{}
	for _, step := range configs {
		stepsByPosition[strings.ToLower(strings.TrimSpace(step.PositionCode))] = step
	}
	boxesByPosition := map[string][]models.SignatureTemplateBoxRequest{}
	for _, box := range normalized {
		key := strings.ToLower(strings.TrimSpace(box.PositionCode))
		boxesByPosition[key] = append(boxesByPosition[key], box)
		if _, ok := stepsByPosition[key]; !ok && box.PositionCode != "" {
			issues = append(issues, signatureIssue("sign_note_box_position_unknown", box.PositionCode, "Signer note box uses a position that is not in document config."))
		}
	}

	placements := []models.SignNotePlacementSnapshot{}
	for _, step := range configs {
		key := strings.ToLower(strings.TrimSpace(step.PositionCode))
		positionBoxes := boxesByPosition[key]
		if len(positionBoxes) == 0 {
			continue
		}
		sortPlacementBoxes(positionBoxes)
		switch step.ConditionType {
		case 1:
			for i := range positionBoxes {
				positionBoxes[i].SignerType = "any"
				positionBoxes[i].SignerUser = ""
				positionBoxes[i].SignerSlot = 1
				placements = append(placements, signNotePlacementSnapshotFromBox(step, positionBoxes[i], "any", "", "", 1))
			}
		case 2:
			required := map[string]struct {
				Full    string
				Slot    int
				Display string
			}{}
			for index, user := range stepUsers(step) {
				username, display := splitSignerUser(user)
				if username != "" {
					required[strings.ToLower(username)] = struct {
						Full    string
						Slot    int
						Display string
					}{Full: user, Slot: index + 1, Display: display}
				}
			}
			for i := range positionBoxes {
				positionBoxes[i].SignerType = "internal"
				username, _ := splitSignerUser(positionBoxes[i].SignerUser)
				requiredUser, ok := required[strings.ToLower(username)]
				if !ok || username == "" {
					issues = append(issues, signatureIssue("sign_note_condition_all_unknown_user", step.PositionCode, fmt.Sprintf("%s note box must choose a user in this position.", step.PositionName)))
					continue
				}
				positionBoxes[i].SignerUser = requiredUser.Full
				positionBoxes[i].SignerSlot = requiredUser.Slot
				placements = append(placements, signNotePlacementSnapshotFromBox(step, positionBoxes[i], "internal", username, requiredUser.Display, requiredUser.Slot))
			}
		case 3:
			for i := range positionBoxes {
				positionBoxes[i].SignerType = "external"
				positionBoxes[i].SignerUser = ""
				positionBoxes[i].SignerSlot = 1
				placements = append(placements, signNotePlacementSnapshotFromBox(step, positionBoxes[i], "external", "", "บุคคลภายนอก", 1))
			}
		default:
			issues = append(issues, signatureIssue("condition_type_invalid", step.PositionCode, fmt.Sprintf("%s uses unsupported condition type.", step.PositionName)))
		}
	}
	sort.SliceStable(placements, func(i, j int) bool {
		if placements[i].SequenceNo == placements[j].SequenceNo {
			if placements[i].PositionCode == placements[j].PositionCode {
				if placements[i].SignerSlot == placements[j].SignerSlot {
					return placements[i].PageNo < placements[j].PageNo
				}
				return placements[i].SignerSlot < placements[j].SignerSlot
			}
			return placements[i].PositionCode < placements[j].PositionCode
		}
		return placements[i].SequenceNo < placements[j].SequenceNo
	})
	sort.SliceStable(issues, func(i, j int) bool {
		return issues[i].PositionCode < issues[j].PositionCode
	})
	return normalized, placements, issues
}

const (
	minSignatureBoxWidthRatio  = 0.03
	minSignatureBoxHeightRatio = 0.03
)

func normalizeAndValidateSigningDocumentPlacements(boxes []models.SignatureTemplateBoxRequest, maxPages int) ([]models.SignatureTemplateBoxRequest, []models.SignatureValidationIssue) {
	normalized := make([]models.SignatureTemplateBoxRequest, 0, len(boxes))
	issues := []models.SignatureValidationIssue{}
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
		if box.PageNo <= 0 || box.PageNo > maxPages {
			issues = append(issues, signatureIssue("box_page_invalid", box.PositionCode, fmt.Sprintf("Page must be between 1 and %d.", maxPages)))
		}
		if box.XRatio < 0 || box.YRatio < 0 || box.WidthRatio <= 0 || box.HeightRatio <= 0 || box.XRatio+box.WidthRatio > 1 || box.YRatio+box.HeightRatio > 1 {
			issues = append(issues, signatureIssue("box_bounds_invalid", box.PositionCode, "Signature box must stay inside the PDF page."))
		}
		if box.WidthRatio < minSignatureBoxWidthRatio || box.HeightRatio < minSignatureBoxHeightRatio {
			issues = append(issues, signatureIssue("box_too_small", box.PositionCode, "Signature box is too small to sign clearly."))
		}
		normalized = append(normalized, box)
	}
	return normalized, issues
}

func sortPlacementBoxes(boxes []models.SignatureTemplateBoxRequest) {
	sort.SliceStable(boxes, func(i, j int) bool {
		if boxes[i].PageNo == boxes[j].PageNo {
			if boxes[i].SignerSlot == boxes[j].SignerSlot {
				if boxes[i].YRatio == boxes[j].YRatio {
					return boxes[i].XRatio < boxes[j].XRatio
				}
				return boxes[i].YRatio < boxes[j].YRatio
			}
			return boxes[i].SignerSlot < boxes[j].SignerSlot
		}
		return boxes[i].PageNo < boxes[j].PageNo
	})
}

func signaturePlacementSnapshotFromBox(step models.DocumentConfigStep, box models.SignatureTemplateBoxRequest, signerType, signerUser, signerName string, signerSlot int) models.SignaturePlacementSnapshot {
	if strings.TrimSpace(signerName) == "" {
		_, signerName = splitSignerUser(box.SignerUser)
	}
	return models.SignaturePlacementSnapshot{
		PositionCode:  step.PositionCode,
		PositionName:  step.PositionName,
		SequenceNo:    step.SequenceNo,
		ConditionType: step.ConditionType,
		SignerSlot:    signerSlot,
		SignerType:    signerType,
		SignerUser:    strings.TrimSpace(signerUser),
		SignerName:    strings.TrimSpace(signerName),
		PageNo:        box.PageNo,
		XRatio:        box.XRatio,
		YRatio:        box.YRatio,
		WidthRatio:    box.WidthRatio,
		HeightRatio:   box.HeightRatio,
		Label:         box.Label,
	}
}

func signNotePlacementSnapshotFromBox(step models.DocumentConfigStep, box models.SignatureTemplateBoxRequest, signerType, signerUser, signerName string, signerSlot int) models.SignNotePlacementSnapshot {
	if strings.TrimSpace(signerName) == "" {
		_, signerName = splitSignerUser(box.SignerUser)
	}
	return models.SignNotePlacementSnapshot{
		PositionCode:  step.PositionCode,
		PositionName:  step.PositionName,
		SequenceNo:    step.SequenceNo,
		ConditionType: step.ConditionType,
		SignerSlot:    signerSlot,
		SignerType:    signerType,
		SignerUser:    strings.TrimSpace(signerUser),
		SignerName:    strings.TrimSpace(signerName),
		PageNo:        box.PageNo,
		XRatio:        box.XRatio,
		YRatio:        box.YRatio,
		WidthRatio:    box.WidthRatio,
		HeightRatio:   box.HeightRatio,
		Label:         firstNonEmpty(box.Label, "หมายเหตุผู้เซ็น"),
	}
}

func (s *Server) inactiveSigningLayoutUserIssues(ctx context.Context, configs []models.DocumentConfigStep, boxes []models.SignatureTemplateBoxRequest) []models.SignatureValidationIssue {
	issues := []models.SignatureValidationIssue{}
	boxesByPosition := map[string][]models.SignatureTemplateBoxRequest{}
	for _, box := range boxes {
		boxesByPosition[strings.ToLower(strings.TrimSpace(box.PositionCode))] = append(boxesByPosition[strings.ToLower(strings.TrimSpace(box.PositionCode))], box)
	}
	seen := map[string]bool{}
	for _, step := range configs {
		if step.ConditionType == 3 {
			continue
		}
		users := []string{}
		if step.ConditionType == 1 {
			users = stepUsers(step)
		} else {
			for _, box := range boxesByPosition[strings.ToLower(strings.TrimSpace(step.PositionCode))] {
				if box.SignerUser != "" {
					users = append(users, box.SignerUser)
				}
			}
		}
		for _, value := range users {
			username, _ := splitSignerUser(value)
			key := strings.ToLower(strings.TrimSpace(username))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			user, err := s.store.FindUserByUsername(ctx, username)
			if err != nil || user.Status != "active" {
				issues = append(issues, signatureIssue("signer_user_inactive", step.PositionCode, fmt.Sprintf("Signer user %s must exist and be active.", username)))
			}
		}
	}
	return issues
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func splitSignerUser(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	parts := strings.SplitN(value, ":", 2)
	username := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return username, username
	}
	display := strings.TrimSpace(parts[1])
	if display == "" {
		display = username
	}
	return username, display
}

func (s *Server) cleanupExpiredSigningUploads() {
	paths, err := s.store.CleanupExpiredSigningDocumentUploads(context.Background(), time.Now().Add(-24*time.Hour))
	if err != nil {
		s.logger.Warn("cleanup expired signing uploads failed", "error", err)
		return
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.logger.Warn("remove expired signing upload failed", "error", err, "path", path)
		}
	}
}

func (s *Server) cleanupUploadedFileBestEffort(file models.UploadedFile, reason string) {
	fileID := strings.TrimSpace(file.ID)
	if fileID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	storagePath, deleted, err := s.store.DeleteUploadedFileIfUnreferenced(ctx, fileID)
	if err != nil {
		s.logger.Warn("cleanup generated upload failed", "error", err, "fileID", fileID, "reason", reason)
		return
	}
	if !deleted {
		return
	}
	if strings.TrimSpace(storagePath) == "" {
		storagePath = file.StoragePath
	}
	if strings.TrimSpace(storagePath) == "" {
		return
	}
	if err := os.Remove(storagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.logger.Warn("remove generated upload failed", "error", err, "fileID", fileID, "reason", reason)
	}
}

func (s *Server) getSigningDocument(w http.ResponseWriter, r *http.Request) {
	document, err := s.store.FindSigningDocumentByID(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if errors.Is(err, store.ErrSigningDocumentNotFound) {
		writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	actor, _ := currentUser(r)
	if !canAccessSigningDocumentAsAdmin(document, actor) {
		writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": s.withExternalURLs(r, document)})
}

func (s *Server) getSigningDocumentPDF(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	document, err := s.store.FindSigningDocumentByID(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if errors.Is(err, store.ErrSigningDocumentNotFound) {
		writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	if isAdminRole(user.Role) {
		if !canAccessSigningDocumentAsAdmin(document, user) {
			writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
			return
		}
	} else {
		if document.Status != "in_progress" || !documentHasSigner(document, user.Username) {
			writeError(w, http.StatusForbidden, "forbidden", "You cannot view this document.")
			return
		}
	}
	version := strings.TrimSpace(r.URL.Query().Get("version"))
	needsCurrentPDF := version == "" || version == "current"
	canRefreshCurrentPDF := document.Status == "in_progress" || document.Status == "pending_confirm" || document.Status == "auto_confirming"
	if needsCurrentPDF && canRefreshCurrentPDF && (currentPDFNeedsLegalNoticeRefresh(document) || currentPDFNeedsSignatureTransparencyRefresh(document)) {
		if err := s.refreshStampedPDF(r.Context(), document.ID, false); err != nil {
			s.logger.Error("refresh legal notice pdf failed", "error", err, "documentID", document.ID)
			writeError(w, http.StatusInternalServerError, "pdf_legal_notice_failed", "Cannot prepare PDF legal notice right now.")
			return
		}
		updated, err := s.store.FindSigningDocumentByID(r.Context(), document.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
			return
		}
		document = updated
	}
	file := document.CurrentFile
	switch version {
	case "original":
		file = document.OriginalFile
	case "final":
		if document.FinalFile != nil {
			file = document.FinalFile
		}
	}
	if file == nil || file.StoragePath == "" {
		writeError(w, http.StatusNotFound, "pdf_not_found", "PDF was not found.")
		return
	}
	serveInlinePDF(w, r, *file)
}

func canAccessSigningDocumentAsAdmin(document models.SigningDocument, actor models.User) bool {
	if !strings.EqualFold(strings.TrimSpace(document.Status), "draft") {
		return true
	}
	return strings.TrimSpace(document.CreatedBy) != "" && strings.TrimSpace(document.CreatedBy) == strings.TrimSpace(actor.ID)
}

func (s *Server) retrySigningDocumentLock(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	documentID := strings.TrimSpace(r.PathValue("id"))
	scope := "signing_document_retry_lock:" + documentID
	if s.replayIdempotentResponse(w, r, scope, actor.ID) {
		return
	}
	idempotencyCompleted := false
	defer func() {
		if !idempotencyCompleted {
			s.releaseIdempotency(scope, actor.ID, r)
		}
	}()

	document, err := s.store.FindSigningDocumentByID(r.Context(), documentID)
	if errors.Is(err, store.ErrSigningDocumentNotFound) {
		writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	if document.Status != "completed_lock_failed" && document.Status != "completed" {
		writeError(w, http.StatusBadRequest, "document_not_completed", "Document is not ready for SML lock retry.")
		return
	}
	ok, metadata := s.lockCompletedDocument(r.Context(), document.ID, document.DocNo)
	if !ok {
		writeError(w, http.StatusBadGateway, "sml_lock_failed", "SML lock failed. You can retry again.")
		return
	}
	payload := map[string]any{"status": "ok", "lock": metadata}
	s.completeIdempotency(scope, actor.ID, r, http.StatusOK, payload)
	idempotencyCompleted = true
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) retrySigningDocumentImages(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	documentID := strings.TrimSpace(r.PathValue("id"))
	scope := "signing_document_retry_images:" + documentID
	if s.replayIdempotentResponse(w, r, scope, actor.ID) {
		return
	}
	idempotencyCompleted := false
	defer func() {
		if !idempotencyCompleted {
			s.releaseIdempotency(scope, actor.ID, r)
		}
	}()

	document, err := s.store.FindSigningDocumentByID(r.Context(), documentID)
	if errors.Is(err, store.ErrSigningDocumentNotFound) {
		writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	if !canRetrySigningDocumentImagesStatus(document.Status) {
		writeError(w, http.StatusBadRequest, "document_not_image_retryable", "Document is not ready for SML image retry.")
		return
	}
	imageOK, imageMetadata := s.uploadCompletedDocumentImages(r.Context(), document.ID)
	if !imageOK {
		status, code, message := smlImagesHTTPError(imageMetadata)
		writeError(w, status, code, message)
		return
	}
	lockOK, lockMetadata := s.lockCompletedDocument(r.Context(), document.ID, document.DocNo)
	updated, _ := s.store.FindSigningDocumentByID(r.Context(), documentID)
	payload := map[string]any{
		"document": s.withExternalURLs(r, updated),
		"imageOk":  imageOK,
		"lockOk":   lockOK,
		"image":    imageMetadata,
		"lock":     lockMetadata,
	}
	s.completeIdempotency(scope, actor.ID, r, http.StatusOK, payload)
	idempotencyCompleted = true
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) retrySigningDocumentFinalPDF(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	documentID := strings.TrimSpace(r.PathValue("id"))
	scope := "signing_document_retry_final_pdf:" + documentID
	if s.replayIdempotentResponse(w, r, scope, actor.ID) {
		return
	}
	idempotencyCompleted := false
	defer func() {
		if !idempotencyCompleted {
			s.releaseIdempotency(scope, actor.ID, r)
		}
	}()

	document, err := s.store.FindSigningDocumentByID(r.Context(), documentID)
	if errors.Is(err, store.ErrSigningDocumentNotFound) {
		writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	if document.Status != "completed_evidence_failed" {
		writeError(w, http.StatusBadRequest, "document_not_evidence_failed", "Document is not waiting for final PDF retry.")
		return
	}
	result := s.finalizeCompletedDocument(r.Context(), documentID, clientIP(r), r.UserAgent())
	if !result.FinalOK {
		writeError(w, http.StatusBadGateway, "final_pdf_failed", "Final PDF evidence generation failed. You can retry again.")
		return
	}
	updated, _ := s.store.FindSigningDocumentByID(r.Context(), documentID)
	payload := map[string]any{
		"document": s.withExternalURLs(r, updated),
		"finalOk":  result.FinalOK,
		"imageOk":  result.ImageOK,
		"lockOk":   result.LockOK,
		"image":    result.ImageMetadata,
		"lock":     result.LockMetadata,
	}
	s.completeIdempotency(scope, actor.ID, r, http.StatusOK, payload)
	idempotencyCompleted = true
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) createSigningDocumentPrintCopy(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	documentID := strings.TrimSpace(r.PathValue("id"))
	var req models.CreatePrintCopyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	req = normalizePrintCopyRequest(req)
	scope := "signing_document_print_copy:" + documentID
	if s.replayIdempotentResponse(w, r, scope, actor.ID) {
		return
	}
	idempotencyCompleted := false
	defer func() {
		if !idempotencyCompleted {
			s.releaseIdempotency(scope, actor.ID, r)
		}
	}()

	document, err := s.store.FindSigningDocumentByID(r.Context(), documentID)
	if errors.Is(err, store.ErrSigningDocumentNotFound) {
		writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	if document.Status != "completed" {
		switch document.Status {
		case "completed_evidence_failed":
			writeError(w, http.StatusConflict, "final_evidence_required", "Final PDF evidence is not ready. Retry Final PDF first.")
		case "completed_image_failed":
			writeError(w, http.StatusConflict, "sml_images_required", "SML document images are not complete. Retry SML Images before printing the official copy.")
		case "completed_lock_failed":
			writeError(w, http.StatusConflict, "sml_lock_required", "SML lock is not complete. Retry SML Lock before printing the official copy.")
		default:
			writeError(w, http.StatusConflict, "document_not_completed", "Document must be completed before printing the official copy.")
		}
		return
	}
	if document.FinalFile == nil || document.FinalFile.StoragePath == "" {
		writeError(w, http.StatusConflict, "final_pdf_required", "Final PDF was not found.")
		return
	}
	if document.OriginalFile == nil || document.OriginalFile.PageCount <= 0 {
		writeError(w, http.StatusConflict, "original_pdf_required", "Original PDF page count was not found.")
		return
	}

	printedAt := time.Now()
	deviceIDHash := shortHash(req.DeviceID)
	printedBy := strings.TrimSpace(actor.DisplayName)
	if printedBy == "" {
		printedBy = actor.Username
	}
	printablePageCount := document.OriginalFile.PageCount
	printed, err := createPrintCopyPDF(document.FinalFile.StoragePath, printablePageCount, printEvidencePage{
		Document:        document,
		PrintedAt:       printedAt,
		PrintedBy:       printedBy,
		Channel:         req.Channel,
		PrinterName:     req.PrinterName,
		DeviceIDHash:    deviceIDHash,
		ClientTimezone:  req.ClientTimezone,
		IPAddress:       clientIP(r),
		UserAgent:       r.UserAgent(),
		FinalFileSHA256: document.FinalFile.SHA256,
	})
	if err != nil {
		s.logger.Error("create print copy pdf failed", "error", err, "documentID", document.ID)
		writeError(w, http.StatusInternalServerError, "print_copy_failed", "Cannot create print copy right now.")
		return
	}
	pageCount := printablePageCount + 1
	if count, err := readPDFPageCount(printed); err == nil && count > 0 {
		pageCount = count
	}
	name := fmt.Sprintf("%s-print-copy-%s.pdf", strings.TrimSuffix(filepath.Base(document.FinalFile.OriginalName), filepath.Ext(document.FinalFile.OriginalName)), printedAt.Format("20060102150405"))
	uploaded, err := s.storeUploadedBytes(r.Context(), printed, name, "print-copy.pdf", "application/pdf", ".pdf", pageCount, actor.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "print_copy_store_failed", "Cannot store print copy right now.")
		return
	}
	printEvent, err := s.store.CreateSigningDocumentPrintEvent(r.Context(), store.CreatePrintEventInput{
		DocumentID:      document.ID,
		FileID:          uploaded.ID,
		Channel:         req.Channel,
		PrinterName:     req.PrinterName,
		DeviceIDHash:    deviceIDHash,
		ClientTimezone:  req.ClientTimezone,
		FinalFileSHA256: document.FinalFile.SHA256,
		PrintedBy:       actor.ID,
		PrintedByLabel:  printedBy,
		IPAddress:       clientIP(r),
		UserAgent:       r.UserAgent(),
	})
	if err != nil {
		s.logger.Error("record print copy failed", "error", err, "documentID", document.ID)
		s.cleanupUploadedFileBestEffort(uploaded, "record_print_copy_failed")
		writeError(w, http.StatusInternalServerError, "print_event_failed", "Cannot record print event right now.")
		return
	}
	payload := map[string]any{
		"printCopyId": printEvent.ID,
		"fileUrl":     fmt.Sprintf("/api/signing-documents/%s/print-copies/%s/pdf", document.ID, printEvent.ID),
		"printEvent":  printEvent,
	}
	s.completeIdempotency(scope, actor.ID, r, http.StatusCreated, payload)
	idempotencyCompleted = true
	writeJSON(w, http.StatusCreated, payload)
}

func (s *Server) getSigningDocumentPrintCopyPDF(w http.ResponseWriter, r *http.Request) {
	documentID := strings.TrimSpace(r.PathValue("id"))
	printCopyID := strings.TrimSpace(r.PathValue("printCopyId"))
	printEvent, err := s.store.FindSigningDocumentPrintEvent(r.Context(), documentID, printCopyID)
	if errors.Is(err, store.ErrSigningDocumentNotFound) {
		writeError(w, http.StatusNotFound, "print_copy_not_found", "Print copy was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "print_copy_failed", "Cannot load print copy right now.")
		return
	}
	if printEvent.File.StoragePath == "" {
		writeError(w, http.StatusNotFound, "pdf_not_found", "PDF was not found.")
		return
	}
	serveInlinePDF(w, r, printEvent.File)
}

func (s *Server) regenerateExternalToken(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	signerID := strings.TrimSpace(r.PathValue("signerId"))
	if signerID == "" {
		writeError(w, http.StatusBadRequest, "signer_id_required", "signer id is required.")
		return
	}
	rawToken := randomHex(32)
	otp := randomNumericOTP(6)
	expiresAt := time.Now().Add(24 * time.Hour)
	_, err := s.store.RegenerateExternalToken(r.Context(), signerID, hashSecret(rawToken), hashSecret(otp), actor.ID, expiresAt)
	if errors.Is(err, store.ErrSigningTaskNotFound) {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "External signing task was not found.")
		return
	}
	if errors.Is(err, store.ErrExternalSignerNotTurn) {
		writeError(w, http.StatusConflict, "external_signer_not_turn", "External signer is not the current signing step.")
		return
	}
	if errors.Is(err, store.ErrExternalSignerUnavailable) {
		writeError(w, http.StatusConflict, "external_signer_unavailable", "External signer is not available for link generation.")
		return
	}
	if err != nil {
		s.logger.Error("regenerate external token failed", "error", err)
		writeError(w, http.StatusInternalServerError, "external_token_failed", "Cannot generate external link right now.")
		return
	}
	url := s.externalURL(r, rawToken)
	writeJSON(w, http.StatusOK, map[string]any{"external": models.RegenerateExternalTokenResponse{
		SignerID:  signerID,
		URL:       url,
		OTP:       otp,
		ExpiresAt: expiresAt,
	}})
}

func (s *Server) listMySigningTasks(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	readyPage := parsePositiveQueryInt(r, "readyPage", 1)
	waitingPage := parsePositiveQueryInt(r, "waitingPage", 1)
	size := parsePositiveQueryInt(r, "size", 20)
	if size > 50 {
		size = 50
	}
	queue, err := s.store.ListMySigningTaskQueue(r.Context(), user.Username, readyPage, waitingPage, size)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_tasks_failed", "Cannot load signing tasks right now.")
		return
	}
	writeJSON(w, http.StatusOK, queue)
}

func (s *Server) listMySigningHistory(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	size := parsePositiveQueryInt(r, "size", 20)
	if size > 50 {
		size = 50
	}
	result, err := s.store.ListMySigningHistory(r.Context(), user.Username, r.URL.Query().Get("search"), parsePositiveQueryInt(r, "page", 1), size)
	if err != nil {
		s.logger.Error("list signing history failed", "error", err)
		writeError(w, http.StatusInternalServerError, "signing_history_failed", "Cannot load signing history right now.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getMySigningTask(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	signer, err := s.store.FindSigningTaskByID(r.Context(), taskID)
	if errors.Is(err, store.ErrSigningTaskNotFound) {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_task_failed", "Cannot load signing task right now.")
		return
	}
	if !strings.EqualFold(signer.SignerUser, user.Username) {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	if document.Status != "in_progress" {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": sanitizeSigningDocumentForSigner(document), "task": sanitizeSigningTaskForUser(signer), "legal": signingLegalPayload()})
}

func (s *Server) getMySigningHistory(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.mySigningHistorySigner(w, r)
	if !ok {
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": sanitizeSigningDocumentForSigner(document), "task": sanitizeSigningTaskForUser(signer), "legal": signingLegalPayload()})
}

func (s *Server) getMySigningHistoryPDF(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.mySigningHistorySigner(w, r)
	if !ok {
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	version := strings.TrimSpace(r.URL.Query().Get("version"))
	if version != "" && version != "current" && version != "final" {
		writeError(w, http.StatusBadRequest, "invalid_pdf_version", "PDF version is invalid.")
		return
	}
	needsCurrent := version == "" || version == "current"
	if needsCurrent && document.Status == "in_progress" && currentPDFNeedsLegalNoticeRefresh(document) {
		if err := s.refreshStampedPDF(r.Context(), document.ID, false); err != nil {
			s.logger.Error("refresh history legal notice pdf failed", "error", err, "documentID", document.ID)
			writeError(w, http.StatusInternalServerError, "pdf_legal_notice_failed", "Cannot prepare PDF legal notice right now.")
			return
		}
		document, err = s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
			return
		}
	}
	file := selectSigningHistoryPDFFile(document, version)
	if file == nil || file.StoragePath == "" {
		writeError(w, http.StatusNotFound, "pdf_not_found", "PDF was not found.")
		return
	}
	serveInlinePDF(w, r, *file)
}

func canRetrySigningDocumentImagesStatus(status string) bool {
	return status == "completed_image_failed" || status == "completed"
}

func smlImagesHTTPError(metadata map[string]any) (int, string, string) {
	code, _ := metadata["errorCode"].(string)
	if code == "tenant_image_database_missing" {
		imageDatabase := imageDatabaseFromSMLImageMetadata(metadata)
		if imageDatabase != "" {
			return http.StatusFailedDependency, code, "ฐานข้อมูลรูป SML ยังไม่พร้อม: " + imageDatabase + " กรุณา provision image DB แล้วกดส่งรูป SML อีกครั้ง"
		}
		return http.StatusFailedDependency, code, "ฐานข้อมูลรูป SML ยังไม่พร้อม กรุณา provision image DB แล้วกดส่งรูป SML อีกครั้ง"
	}
	return http.StatusBadGateway, "sml_images_failed", "SML image upload failed. You can retry again."
}

func imageDatabaseFromSMLImageMetadata(metadata map[string]any) string {
	details, _ := metadata["errorDetails"].(map[string]any)
	if details == nil {
		return ""
	}
	if imageDatabase, _ := details["imageDatabase"].(string); strings.TrimSpace(imageDatabase) != "" {
		return strings.TrimSpace(imageDatabase)
	}
	return ""
}

func selectSigningHistoryPDFFile(document models.SigningDocument, version string) *models.UploadedFile {
	if version == "final" {
		return document.FinalFile
	}
	return document.CurrentFile
}

func (s *Server) mySigningHistorySigner(w http.ResponseWriter, r *http.Request) (models.SigningDocumentSigner, bool) {
	user, _ := currentUser(r)
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	signer, err := s.store.FindSigningTaskByID(r.Context(), taskID)
	if errors.Is(err, store.ErrSigningTaskNotFound) || (err == nil && !strings.EqualFold(signer.SignerUser, user.Username)) {
		writeError(w, http.StatusNotFound, "signing_history_not_found", "Signing history was not found.")
		return models.SigningDocumentSigner{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_history_failed", "Cannot load signing history right now.")
		return models.SigningDocumentSigner{}, false
	}
	if signer.Status != "signed" && signer.Status != "rejected" {
		writeError(w, http.StatusNotFound, "signing_history_not_found", "Signing history was not found.")
		return models.SigningDocumentSigner{}, false
	}
	return signer, true
}

func sanitizeSigningDocumentForSigner(document models.SigningDocument) models.SigningDocument {
	document.OriginalFileID = ""
	document.CurrentFileID = ""
	document.FinalFileID = ""
	document.OriginalFile = nil
	document.CurrentFile = nil
	document.FinalFile = nil
	document.Attachments = nil
	document.PrintEvents = nil
	for i := range document.Signers {
		document.Signers[i] = sanitizeSigningTaskForUser(document.Signers[i])
	}
	for i := range document.Events {
		document.Events[i].IPAddress = ""
		document.Events[i].UserAgent = ""
		document.Events[i].Metadata = nil
	}
	return document
}

func sanitizeSigningDocumentForExternal(document models.SigningDocument) models.SigningDocument {
	document = sanitizeSigningDocumentForSigner(document)
	document.Signers = nil
	document.Events = nil
	return document
}

func sanitizeSigningTaskForUser(signer models.SigningDocumentSigner) models.SigningDocumentSigner {
	signer.SignatureFileID = ""
	signer.DeviceID = ""
	signer.IPAddress = ""
	signer.UserAgent = ""
	signer.SignNoteBoxes = nil
	signer.ExternalTokenID = ""
	signer.ExternalURL = ""
	return signer
}

type signingAttachmentFileResponse struct {
	ID           string    `json:"id"`
	OriginalName string    `json:"originalName"`
	ContentType  string    `json:"contentType"`
	SizeBytes    int64     `json:"sizeBytes"`
	PageCount    int       `json:"pageCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

type signingAttachmentResponse struct {
	ID               string                        `json:"id"`
	SignerID         string                        `json:"signerId,omitempty"`
	SignerName       string                        `json:"signerName,omitempty"`
	PositionName     string                        `json:"positionName,omitempty"`
	RequirementKey   string                        `json:"requirementKey,omitempty"`
	RequirementLabel string                        `json:"requirementLabel,omitempty"`
	Note             string                        `json:"note"`
	CreatedAt        time.Time                     `json:"createdAt"`
	File             signingAttachmentFileResponse `json:"file"`
}

func sanitizeSigningAttachmentForUser(attachment models.SigningDocumentAttachment, signers []models.SigningDocumentSigner) signingAttachmentResponse {
	response := signingAttachmentResponse{
		ID:               attachment.ID,
		SignerID:         attachment.SignerID,
		RequirementKey:   attachment.RequirementKey,
		RequirementLabel: attachment.RequirementLabel,
		Note:             attachment.Note,
		CreatedAt:        attachment.CreatedAt,
		File: signingAttachmentFileResponse{
			ID:           attachment.File.ID,
			OriginalName: attachment.File.OriginalName,
			ContentType:  attachment.File.ContentType,
			SizeBytes:    attachment.File.SizeBytes,
			PageCount:    attachment.File.PageCount,
			CreatedAt:    attachment.File.CreatedAt,
		},
	}
	for _, signer := range signers {
		if strings.TrimSpace(signer.ID) == strings.TrimSpace(attachment.SignerID) {
			response.SignerName = signer.SignerName
			if response.SignerName == "" {
				response.SignerName = signer.SignerUser
			}
			response.PositionName = signer.PositionName
			break
		}
	}
	return response
}

func sanitizeSigningAttachmentsForUser(attachments []models.SigningDocumentAttachment, signers []models.SigningDocumentSigner) []signingAttachmentResponse {
	out := make([]signingAttachmentResponse, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, sanitizeSigningAttachmentForUser(attachment, signers))
	}
	return out
}

func externalSignOnlyForbidden(w http.ResponseWriter) {
	writeError(w, http.StatusForbidden, "external_sign_only", "External signing links can only be used to sign documents.")
}

func (s *Server) listMySigningTaskAttachments(w http.ResponseWriter, r *http.Request) {
	_, _, document, ok := s.authorizeSigningTaskAttachmentAccess(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"attachments": sanitizeSigningAttachmentsForUser(document.Attachments, document.Signers),
	})
}

func (s *Server) listSigningDocumentAttachments(w http.ResponseWriter, r *http.Request) {
	_, document, ok := s.authorizeSigningDocumentAttachmentAccess(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"attachments": sanitizeSigningAttachmentsForUser(document.Attachments, document.Signers),
	})
}

func (s *Server) getMySigningTaskAttachmentFile(w http.ResponseWriter, r *http.Request) {
	user, signer, document, ok := s.authorizeSigningTaskAttachmentAccess(w, r)
	if !ok {
		return
	}
	attachmentID := strings.TrimSpace(r.PathValue("attachmentId"))
	attachment, ok := findSigningDocumentAttachment(document.Attachments, attachmentID)
	if !ok || strings.TrimSpace(attachment.File.StoragePath) == "" {
		writeError(w, http.StatusNotFound, "attachment_not_found", "Attachment was not found.")
		return
	}
	metadata := map[string]any{
		"documentId":   document.ID,
		"signerId":     signer.ID,
		"attachmentId": attachment.ID,
		"fileId":       attachment.File.ID,
		"contentType":  attachment.File.ContentType,
		"sizeBytes":    attachment.File.SizeBytes,
	}
	if err := s.store.WriteAuditWithMetadata(r.Context(), user.ID, "signing_attachment.view", "signing_attachment", attachment.ID, clientIP(r), r.UserAgent(), metadata); err != nil {
		s.logger.Warn("write signing attachment view audit failed", "error", err, "attachmentID", attachment.ID)
	}
	serveInlineUploadedFile(w, r, attachment.File)
}

func (s *Server) getSigningDocumentAttachmentFile(w http.ResponseWriter, r *http.Request) {
	user, document, ok := s.authorizeSigningDocumentAttachmentAccess(w, r)
	if !ok {
		return
	}
	attachmentID := strings.TrimSpace(r.PathValue("attachmentId"))
	attachment, ok := findSigningDocumentAttachment(document.Attachments, attachmentID)
	if !ok || strings.TrimSpace(attachment.File.StoragePath) == "" {
		writeError(w, http.StatusNotFound, "attachment_not_found", "Attachment was not found.")
		return
	}
	metadata := map[string]any{
		"documentId":   document.ID,
		"attachmentId": attachment.ID,
		"fileId":       attachment.File.ID,
		"contentType":  attachment.File.ContentType,
		"sizeBytes":    attachment.File.SizeBytes,
	}
	if err := s.store.WriteAuditWithMetadata(r.Context(), user.ID, "signing_attachment.view", "signing_attachment", attachment.ID, clientIP(r), r.UserAgent(), metadata); err != nil {
		s.logger.Warn("write signing attachment view audit failed", "error", err, "attachmentID", attachment.ID)
	}
	serveInlineUploadedFile(w, r, attachment.File)
}

func (s *Server) authorizeSigningDocumentAttachmentAccess(w http.ResponseWriter, r *http.Request) (models.User, models.SigningDocument, bool) {
	user, _ := currentUser(r)
	documentID := strings.TrimSpace(r.PathValue("id"))
	document, err := s.store.FindSigningDocumentByID(r.Context(), documentID)
	if errors.Is(err, store.ErrSigningDocumentNotFound) {
		writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
		return user, models.SigningDocument{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return user, models.SigningDocument{}, false
	}
	if !canAccessSigningDocumentAsAdmin(document, user) {
		writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
		return user, document, false
	}
	return user, document, true
}

func (s *Server) authorizeSigningTaskAttachmentAccess(w http.ResponseWriter, r *http.Request) (models.User, models.SigningDocumentSigner, models.SigningDocument, bool) {
	user, _ := currentUser(r)
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	signer, err := s.store.FindSigningTaskByID(r.Context(), taskID)
	if errors.Is(err, store.ErrSigningTaskNotFound) {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return user, models.SigningDocumentSigner{}, models.SigningDocument{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_task_failed", "Cannot load signing task right now.")
		return user, models.SigningDocumentSigner{}, models.SigningDocument{}, false
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if errors.Is(err, store.ErrSigningDocumentNotFound) {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return user, signer, models.SigningDocument{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return user, signer, models.SigningDocument{}, false
	}
	if !isAdminRole(user.Role) && !documentHasInternalSigner(document.Signers, user.Username) {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return user, signer, document, false
	}
	return user, signer, document, true
}

func documentHasInternalSigner(signers []models.SigningDocumentSigner, username string) bool {
	username = strings.TrimSpace(username)
	if username == "" {
		return false
	}
	for _, signer := range signers {
		if strings.EqualFold(signer.SignerType, "external") {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(signer.SignerUser), username) {
			return true
		}
	}
	return false
}

func findSigningDocumentAttachment(attachments []models.SigningDocumentAttachment, attachmentID string) (models.SigningDocumentAttachment, bool) {
	attachmentID = strings.TrimSpace(attachmentID)
	for _, attachment := range attachments {
		if attachment.ID == attachmentID {
			return attachment, true
		}
	}
	return models.SigningDocumentAttachment{}, false
}

func findAttachmentRequirement(signer models.SigningDocumentSigner, key string) (models.AttachmentRequirement, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return models.AttachmentRequirement{}, false
	}
	for _, requirement := range signer.AttachmentRequirementsSnapshot {
		if strings.TrimSpace(requirement.Key) == key {
			return requirement, true
		}
	}
	return models.AttachmentRequirement{}, false
}

func (s *Server) writeRequiredAttachmentsMissing(w http.ResponseWriter, missing []models.AttachmentRequirement) {
	items := make([]map[string]any, 0, len(missing))
	for _, requirement := range missing {
		items = append(items, map[string]any{
			"key":        requirement.Key,
			"label":      requirement.Label,
			"signerSlot": requirement.SignerSlot,
		})
	}
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":   "required_attachments_missing",
		"message": "กรุณาแนบเอกสารที่กำหนดให้ครบก่อนเซ็น",
		"missing": items,
	})
}

type runtimeSignNoteValidationError struct {
	code    string
	message string
}

func (err runtimeSignNoteValidationError) Error() string {
	return err.message
}

func normalizeRuntimeSignNoteBoxes(boxes []models.SignNoteBox, pageCount int) ([]models.SignNoteBox, string, error) {
	if len(boxes) == 0 {
		return nil, "", nil
	}
	if len(boxes) > maxRuntimeSignNoteBoxes {
		return nil, "", runtimeSignNoteValidationError{"sign_note_box_count_invalid", fmt.Sprintf("เพิ่มกล่องหมายเหตุได้ไม่เกิน %d กล่อง", maxRuntimeSignNoteBoxes)}
	}
	if pageCount <= 0 {
		return nil, "", runtimeSignNoteValidationError{"sign_note_pdf_page_count_missing", "ไม่สามารถตรวจสอบจำนวนหน้า PDF สำหรับกล่องหมายเหตุได้"}
	}
	if payload, err := json.Marshal(boxes); err != nil {
		return nil, "", err
	} else if len(payload) > maxRuntimeSignNotePayloadBytes {
		return nil, "", runtimeSignNoteValidationError{"sign_note_payload_too_large", "ข้อมูลกล่องหมายเหตุมีขนาดใหญ่เกินไป"}
	}
	seen := map[string]bool{}
	normalized := make([]models.SignNoteBox, 0, len(boxes))
	noteParts := []string{}
	for index, box := range boxes {
		box.ClientKey = strings.TrimSpace(box.ClientKey)
		if box.ClientKey == "" {
			box.ClientKey = fmt.Sprintf("note_box_%d", index+1)
		}
		if seen[box.ClientKey] {
			return nil, "", runtimeSignNoteValidationError{"sign_note_box_duplicate", "พบกล่องหมายเหตุซ้ำ กรุณาลองใหม่อีกครั้ง"}
		}
		seen[box.ClientKey] = true
		box.Text = strings.TrimSpace(box.Text)
		box.Label = strings.TrimSpace(box.Label)
		if box.Label == "" {
			box.Label = "หมายเหตุผู้เซ็น"
		}
		if box.Text == "" {
			return nil, "", runtimeSignNoteValidationError{"sign_note_text_required", "กรุณาระบุข้อความในกล่องหมายเหตุให้ครบก่อนเซ็น"}
		}
		if len([]rune(box.Text)) > maxRuntimeSignNoteChars {
			return nil, "", runtimeSignNoteValidationError{"sign_note_text_too_long", fmt.Sprintf("ข้อความหมายเหตุแต่ละกล่องต้องไม่เกิน %d ตัวอักษร", maxRuntimeSignNoteChars)}
		}
		if box.PageNo <= 0 || box.PageNo > pageCount {
			return nil, "", runtimeSignNoteValidationError{"sign_note_page_invalid", "กล่องหมายเหตุอยู่บนหน้า PDF ที่ไม่ถูกต้อง"}
		}
		if box.XRatio < 0 || box.YRatio < 0 || box.WidthRatio <= 0 || box.HeightRatio <= 0 || box.XRatio+box.WidthRatio > 1 || box.YRatio+box.HeightRatio > 1 {
			return nil, "", runtimeSignNoteValidationError{"sign_note_bounds_invalid", "กล่องหมายเหตุต้องอยู่ในขอบเขตหน้า PDF"}
		}
		if box.WidthRatio < minRuntimeSignNoteBoxWidthRatio || box.HeightRatio < minRuntimeSignNoteBoxHeightRatio {
			return nil, "", runtimeSignNoteValidationError{"sign_note_box_too_small", "กล่องหมายเหตุเล็กเกินไป กรุณาขยายกล่องก่อนเซ็น"}
		}
		box.FontSizePt = normalizeRuntimeSignNoteFontSize(box.FontSizePt)
		box.TextAlign = normalizeRuntimeSignNoteTextAlign(box.TextAlign)
		box.VerticalAlign = normalizeRuntimeSignNoteVerticalAlign(box.VerticalAlign)
		box.PaddingPt = normalizeRuntimeSignNotePadding(box.PaddingPt)
		normalized = append(normalized, box)
		noteParts = append(noteParts, box.Text)
	}
	return normalized, truncateForMetadata(strings.Join(noteParts, " | "), 1000), nil
}

func normalizeRuntimeSignNoteFontSize(value float64) float64 {
	if value <= 0 {
		return defaultRuntimeSignNoteFontSizePt
	}
	return clampFloat(value, minRuntimeSignNoteFontSizePt, maxRuntimeSignNoteFontSizePt)
}

func normalizeRuntimeSignNotePadding(value float64) float64 {
	if value <= 0 {
		return defaultRuntimeSignNotePaddingPt
	}
	return clampFloat(value, minRuntimeSignNotePaddingPt, maxRuntimeSignNotePaddingPt)
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func normalizeRuntimeSignNoteTextAlign(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "center", "right":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "left"
	}
}

func normalizeRuntimeSignNoteVerticalAlign(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "top", "bottom":
		return strings.ToLower(strings.TrimSpace(value))
	case "middle", "center":
		return "middle"
	default:
		return "middle"
	}
}

func normalizeSignTaskRuntimeNotes(req models.SignTaskRequest, document models.SigningDocument) ([]models.SignNoteBox, string, error) {
	pageCount := 0
	if document.OriginalFile != nil {
		pageCount = document.OriginalFile.PageCount
	}
	boxes, note, err := normalizeRuntimeSignNoteBoxes(req.SignNoteBoxes, pageCount)
	if err != nil {
		return nil, "", err
	}
	if len(boxes) > 0 {
		return boxes, note, nil
	}
	return nil, truncateForMetadata(req.SignNote, 1000), nil
}

func (s *Server) recordMySigningTaskEvent(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	signer, err := s.store.FindSigningTaskByID(r.Context(), taskID)
	if errors.Is(err, store.ErrSigningTaskNotFound) || (err == nil && !strings.EqualFold(signer.SignerUser, user.Username)) {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_task_failed", "Cannot load signing task right now.")
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	req, err := decodeSigningTaskEventPayload(r.Body, maxSigningEventBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signing_task_event_invalid", "Signing task event is invalid.")
		return
	}
	metadata, err := normalizeSigningTaskEventMetadata(req, document, signer)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signing_task_event_invalid", "Signing task event is invalid.")
		return
	}
	if err := s.store.WriteAuditWithMetadata(r.Context(), user.ID, "signing_task.ux_event", "signing_task", signer.ID, clientIP(r), r.UserAgent(), metadata); err != nil {
		s.logger.Warn("write signing task ux event failed", "error", err, "signerID", signer.ID)
		writeError(w, http.StatusInternalServerError, "signing_task_event_failed", "Cannot record signing task event right now.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) uploadMySigningTaskAttachment(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	signer, err := s.store.FindSigningTaskByID(r.Context(), taskID)
	if errors.Is(err, store.ErrSigningTaskNotFound) || (err == nil && !strings.EqualFold(signer.SignerUser, user.Username)) {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_task_failed", "Cannot load signing task right now.")
		return
	}
	if signer.Status != "pending" {
		writeError(w, http.StatusConflict, taskUnavailableCode(signer.Status), taskUnavailableMessage(signer.Status))
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil || document.Status != "in_progress" {
		writeError(w, http.StatusConflict, "signing_task_unavailable", "This signing task is not available.")
		return
	}
	requirementKey := strings.TrimSpace(r.FormValue("requirementKey"))
	requirementLabel := ""
	uploaded, note, err := s.readAndStoreSigningAttachment(w, r, user.ID, func(r *http.Request) error {
		requirementKey = strings.TrimSpace(r.FormValue("requirementKey"))
		if requirementKey != "" {
			requirement, ok := findAttachmentRequirement(signer, requirementKey)
			if !ok {
				writeError(w, http.StatusBadRequest, "attachment_requirement_invalid", "Attachment requirement is invalid.")
				return fmt.Errorf("attachment requirement is invalid")
			}
			requirementLabel = requirement.Label
		}
		return nil
	})
	if err != nil {
		return
	}
	if err := s.store.AddSigningAttachment(r.Context(), signer.DocumentID, signer.ID, uploaded.ID, requirementKey, requirementLabel, note, user.ID); err != nil {
		s.logger.Error("add signing attachment failed", "error", err, "signerID", signer.ID)
		writeError(w, http.StatusInternalServerError, "attachment_upload_failed", "Cannot attach file right now.")
		return
	}
	var attachmentResponse any
	if attachments, err := s.store.ListSigningDocumentAttachments(r.Context(), signer.DocumentID); err == nil {
		for _, attachment := range attachments {
			if attachment.FileID == uploaded.ID {
				attachmentResponse = sanitizeSigningAttachmentForUser(attachment, document.Signers)
				break
			}
		}
	} else {
		s.logger.Warn("reload signing attachment after upload failed", "error", err, "signerID", signer.ID)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"attachment": attachmentResponse})
}

func (s *Server) signMySigningTask(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	var req models.SignTaskRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	if !req.LegalAccepted {
		writeError(w, http.StatusBadRequest, "legal_acceptance_required", "Legal acceptance is required before signing.")
		return
	}
	signer, err := s.store.FindSigningTaskByID(r.Context(), taskID)
	if errors.Is(err, store.ErrSigningTaskNotFound) || (err == nil && !strings.EqualFold(signer.SignerUser, user.Username)) {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_task_failed", "Cannot load signing task right now.")
		return
	}
	if signer.Status != "pending" {
		writeError(w, http.StatusConflict, taskUnavailableCode(signer.Status), taskUnavailableMessage(signer.Status))
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	signNoteBoxes, signNote, err := normalizeSignTaskRuntimeNotes(req, document)
	if err != nil {
		var validationErr runtimeSignNoteValidationError
		if errors.As(err, &validationErr) {
			writeError(w, http.StatusBadRequest, validationErr.code, validationErr.message)
			return
		}
		writeError(w, http.StatusBadRequest, "sign_note_invalid", "Sign note boxes are invalid.")
		return
	}
	if missing, err := s.store.MissingRequiredAttachments(r.Context(), signer.ID); err != nil {
		s.logger.Error("check required attachments before sign failed", "error", err, "signerID", signer.ID)
		writeError(w, http.StatusInternalServerError, "required_attachments_check_failed", "Cannot verify required attachments right now.")
		return
	} else if len(missing) > 0 {
		s.writeRequiredAttachmentsMissing(w, missing)
		return
	}
	scope := "internal-sign:" + taskID
	if s.replayIdempotentResponse(w, r, scope, user.ID) {
		return
	}
	claimed := strings.TrimSpace(r.Header.Get("Idempotency-Key")) != ""
	uploaded, err := s.storeSignatureImage(r.Context(), req.SignatureDataURL, user.ID)
	if err != nil {
		if claimed {
			_ = s.store.ReleaseIdempotencyKey(context.Background(), scope, r.Header.Get("Idempotency-Key"), user.ID)
		}
		writeError(w, http.StatusBadRequest, "invalid_signature", err.Error())
		return
	}
	result, err := s.store.SignInternalTask(r.Context(), taskID, user.Username, uploaded.ID, req.DeviceID, clientIP(r), r.UserAgent(), signingLegalTextVersion, signNote, signNoteBoxes)
	s.writeTaskMutationResult(w, r, scope, user.ID, result, err)
}

func (s *Server) rejectMySigningTask(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	var req models.RejectTaskRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" {
		writeError(w, http.StatusBadRequest, "reject_reason_required", "Reject reason is required.")
		return
	}
	scope := "internal-reject:" + taskID
	if s.replayIdempotentResponse(w, r, scope, user.ID) {
		return
	}
	documentID, err := s.store.RejectInternalTask(r.Context(), taskID, user.Username, req.Reason, req.DeviceID, clientIP(r), r.UserAgent())
	s.writeRejectResult(w, r, scope, user.ID, documentID, err)
}

func (s *Server) verifyExternalOTP(w http.ResponseWriter, r *http.Request) {
	rawToken := strings.TrimSpace(r.PathValue("token"))
	var req models.VerifyExternalOTPRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	sessionToken := randomHex(32)
	expiresAt := time.Now().Add(2 * time.Hour)
	signer, err := s.store.VerifyExternalOTP(r.Context(), hashSecret(rawToken), hashSecret(req.OTP), hashSecret(sessionToken), expiresAt)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_otp", "OTP is invalid or expired.")
		return
	}
	if signer.Status != "pending" {
		writeError(w, http.StatusConflict, taskUnavailableCode(signer.Status), taskUnavailableMessage(signer.Status))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session": models.VerifyExternalOTPResponse{SessionToken: sessionToken, ExpiresAt: expiresAt},
		"task":    sanitizeSigningTaskForUser(signer),
	})
}

func (s *Server) getPublicSigningDocument(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
	if signer.Status != "pending" {
		writeError(w, http.StatusConflict, taskUnavailableCode(signer.Status), taskUnavailableMessage(signer.Status))
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	if document.Status != "in_progress" {
		writeError(w, http.StatusConflict, "signing_task_unavailable", "This signing task is not available.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"document": sanitizeSigningDocumentForExternal(document),
		"task":     sanitizeSigningTaskForUser(signer),
		"legal":    signingLegalPayload(),
	})
}

func (s *Server) getPublicSigningPDF(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
	if signer.Status != "pending" {
		writeError(w, http.StatusConflict, taskUnavailableCode(signer.Status), taskUnavailableMessage(signer.Status))
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil || document.CurrentFile == nil {
		writeError(w, http.StatusNotFound, "pdf_not_found", "PDF was not found.")
		return
	}
	if document.Status != "in_progress" {
		writeError(w, http.StatusConflict, "signing_task_unavailable", "This signing task is not available.")
		return
	}
	if document.Status == "in_progress" && (currentPDFNeedsLegalNoticeRefresh(document) || currentPDFNeedsSignatureTransparencyRefresh(document)) {
		if err := s.refreshStampedPDF(r.Context(), document.ID, false); err != nil {
			s.logger.Error("refresh public legal notice pdf failed", "error", err, "documentID", document.ID)
			writeError(w, http.StatusInternalServerError, "pdf_legal_notice_failed", "Cannot prepare PDF legal notice right now.")
			return
		}
		document, err = s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
		if err != nil || document.CurrentFile == nil {
			writeError(w, http.StatusNotFound, "pdf_not_found", "PDF was not found.")
			return
		}
	}
	serveInlinePDF(w, r, *document.CurrentFile)
}

func (s *Server) recordPublicSigningTaskEvent(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	req, err := decodeSigningTaskEventPayload(r.Body, maxSigningEventBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signing_task_event_invalid", "Signing task event is invalid.")
		return
	}
	metadata, err := normalizeSigningTaskEventMetadata(req, document, signer)
	if err != nil {
		writeError(w, http.StatusBadRequest, "signing_task_event_invalid", "Signing task event is invalid.")
		return
	}
	if err := s.store.WriteAuditWithMetadata(r.Context(), "", "signing_task.ux_event", "signing_task", signer.ID, clientIP(r), r.UserAgent(), metadata); err != nil {
		s.logger.Warn("write public signing task ux event failed", "error", err, "signerID", signer.ID)
		writeError(w, http.StatusInternalServerError, "signing_task_event_failed", "Cannot record signing task event right now.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listPublicSigningTaskAttachments(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
	attachments, err := s.store.ListSigningTaskAttachments(r.Context(), signer.ID)
	if err != nil {
		s.logger.Error("list public signing attachments failed", "error", err, "signerID", signer.ID)
		writeError(w, http.StatusInternalServerError, "attachments_failed", "Cannot load attachments right now.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"attachments": sanitizeSigningAttachmentsForUser(attachments, []models.SigningDocumentSigner{signer}),
	})
}

func (s *Server) getPublicSigningTaskAttachmentFile(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
	attachments, err := s.store.ListSigningTaskAttachments(r.Context(), signer.ID)
	if err != nil {
		s.logger.Error("load public signing attachments for file failed", "error", err, "signerID", signer.ID)
		writeError(w, http.StatusInternalServerError, "attachments_failed", "Cannot load attachments right now.")
		return
	}
	attachmentID := strings.TrimSpace(r.PathValue("attachmentId"))
	attachment, ok := findSigningDocumentAttachment(attachments, attachmentID)
	if !ok || strings.TrimSpace(attachment.File.StoragePath) == "" {
		writeError(w, http.StatusNotFound, "attachment_not_found", "Attachment was not found.")
		return
	}
	if err := s.store.WriteAuditWithMetadata(r.Context(), "", "signing_attachment.view", "signing_attachment", attachment.ID, clientIP(r), r.UserAgent(), map[string]any{
		"documentId":   signer.DocumentID,
		"signerId":     signer.ID,
		"attachmentId": attachment.ID,
		"fileId":       attachment.File.ID,
		"contentType":  attachment.File.ContentType,
		"sizeBytes":    attachment.File.SizeBytes,
	}); err != nil {
		s.logger.Warn("write public signing attachment view audit failed", "error", err, "attachmentID", attachment.ID)
	}
	serveInlineUploadedFile(w, r, attachment.File)
}

func (s *Server) uploadPublicSigningTaskAttachment(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
	if signer.Status != "pending" {
		writeError(w, http.StatusConflict, taskUnavailableCode(signer.Status), taskUnavailableMessage(signer.Status))
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil || document.Status != "in_progress" {
		writeError(w, http.StatusConflict, "signing_task_unavailable", "This signing task is not available.")
		return
	}
	requirementKey := strings.TrimSpace(r.FormValue("requirementKey"))
	var requirement models.AttachmentRequirement
	uploaded, note, err := s.readAndStoreSigningAttachment(w, r, "", func(r *http.Request) error {
		requirementKey = strings.TrimSpace(r.FormValue("requirementKey"))
		var ok bool
		requirement, ok = findAttachmentRequirement(signer, requirementKey)
		if requirementKey == "" || !ok {
			writeError(w, http.StatusBadRequest, "attachment_requirement_required", "Attachment requirement is required.")
			return fmt.Errorf("attachment requirement is required")
		}
		return nil
	})
	if err != nil {
		return
	}
	if err := s.store.AddSigningAttachment(r.Context(), signer.DocumentID, signer.ID, uploaded.ID, requirement.Key, requirement.Label, note, ""); err != nil {
		s.logger.Error("add public signing attachment failed", "error", err, "signerID", signer.ID)
		writeError(w, http.StatusInternalServerError, "attachment_upload_failed", "Cannot attach file right now.")
		return
	}
	var attachmentResponse any
	if attachments, err := s.store.ListSigningTaskAttachments(r.Context(), signer.ID); err == nil {
		for _, attachment := range attachments {
			if attachment.FileID == uploaded.ID {
				attachmentResponse = sanitizeSigningAttachmentForUser(attachment, []models.SigningDocumentSigner{signer})
				break
			}
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"attachment": attachmentResponse})
}

func (s *Server) signPublicSigningTask(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
	var req models.SignTaskRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	if !req.LegalAccepted {
		writeError(w, http.StatusBadRequest, "legal_acceptance_required", "Legal acceptance is required before signing.")
		return
	}
	scope := "public-sign:" + signer.ID
	if s.replayIdempotentResponse(w, r, scope, "") {
		return
	}
	if signer.Status != "pending" {
		writeError(w, http.StatusConflict, taskUnavailableCode(signer.Status), taskUnavailableMessage(signer.Status))
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	signNoteBoxes, signNote, err := normalizeSignTaskRuntimeNotes(req, document)
	if err != nil {
		var validationErr runtimeSignNoteValidationError
		if errors.As(err, &validationErr) {
			writeError(w, http.StatusBadRequest, validationErr.code, validationErr.message)
			return
		}
		writeError(w, http.StatusBadRequest, "sign_note_invalid", "Sign note boxes are invalid.")
		return
	}
	if missing, err := s.store.MissingRequiredAttachments(r.Context(), signer.ID); err != nil {
		s.logger.Error("check public required attachments before sign failed", "error", err, "signerID", signer.ID)
		writeError(w, http.StatusInternalServerError, "required_attachments_check_failed", "Cannot verify required attachments right now.")
		return
	} else if len(missing) > 0 {
		s.writeRequiredAttachmentsMissing(w, missing)
		return
	}
	claimed := strings.TrimSpace(r.Header.Get("Idempotency-Key")) != ""
	uploaded, err := s.storeSignatureImage(r.Context(), req.SignatureDataURL, "")
	if err != nil {
		if claimed {
			_ = s.store.ReleaseIdempotencyKey(context.Background(), scope, r.Header.Get("Idempotency-Key"), "")
		}
		writeError(w, http.StatusBadRequest, "invalid_signature", err.Error())
		return
	}
	result, err := s.store.SignExternalTask(r.Context(), signer.ID, uploaded.ID, req.DeviceID, clientIP(r), r.UserAgent(), signingLegalTextVersion, signNote, signNoteBoxes)
	s.writePublicTaskMutationResult(w, r, scope, signer.ID, result, err)
}

func (s *Server) rejectPublicSigningTask(w http.ResponseWriter, r *http.Request) {
	_, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
	externalSignOnlyForbidden(w)
}

func (s *Server) writeTaskMutationResult(w http.ResponseWriter, r *http.Request, idempotencyScope, actorUserID string, result store.SignTaskResult, err error) {
	if errors.Is(err, store.ErrSigningTaskNotFound) {
		s.releaseIdempotency(idempotencyScope, actorUserID, r)
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return
	}
	if errors.Is(err, store.ErrSigningTaskUnavailable) {
		s.releaseIdempotency(idempotencyScope, actorUserID, r)
		writeError(w, http.StatusConflict, "signing_task_unavailable", "This signing task is not available.")
		return
	}
	if errors.Is(err, store.ErrRequiredAttachmentsMissing) {
		s.releaseIdempotency(idempotencyScope, actorUserID, r)
		writeError(w, http.StatusConflict, "required_attachments_missing", "กรุณาแนบเอกสารที่กำหนดให้ครบก่อนเซ็น")
		return
	}
	if err != nil {
		s.releaseIdempotency(idempotencyScope, actorUserID, r)
		s.logger.Error("sign task failed", "error", err)
		writeError(w, http.StatusInternalServerError, "signing_task_failed", "Cannot sign document right now.")
		return
	}
	if err := s.refreshStampedPDF(r.Context(), result.DocumentID, false); err != nil {
		s.logger.Error("stamp signing document pdf failed", "error", err, "documentID", result.DocumentID)
		_ = s.store.AddSigningEvent(context.Background(), result.DocumentID, "", "", "pdf_stamp_failed", "สร้าง PDF พร้อมลายเซ็นไม่สำเร็จ", clientIP(r), r.UserAgent(), map[string]any{
			"error": err.Error(),
		})
	}
	if result.Completed {
		s.enqueueAutoFinalize(result.DocumentID, clientIP(r), r.UserAgent())
	}
	document, _ := s.store.FindSigningDocumentByID(r.Context(), result.DocumentID)
	payload := map[string]any{"document": document}
	s.completeIdempotency(idempotencyScope, actorUserID, r, http.StatusOK, payload)
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) writePublicTaskMutationResult(w http.ResponseWriter, r *http.Request, idempotencyScope, signerID string, result store.SignTaskResult, err error) {
	if errors.Is(err, store.ErrSigningTaskNotFound) {
		s.releaseIdempotency(idempotencyScope, "", r)
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return
	}
	if errors.Is(err, store.ErrSigningTaskUnavailable) {
		s.releaseIdempotency(idempotencyScope, "", r)
		writeError(w, http.StatusConflict, "signing_task_unavailable", "This signing task is not available.")
		return
	}
	if errors.Is(err, store.ErrRequiredAttachmentsMissing) {
		s.releaseIdempotency(idempotencyScope, "", r)
		writeError(w, http.StatusConflict, "required_attachments_missing", "กรุณาแนบเอกสารที่กำหนดให้ครบก่อนเซ็น")
		return
	}
	if err != nil {
		s.releaseIdempotency(idempotencyScope, "", r)
		s.logger.Error("public sign task failed", "error", err)
		writeError(w, http.StatusInternalServerError, "signing_task_failed", "Cannot sign document right now.")
		return
	}
	if err := s.refreshStampedPDF(r.Context(), result.DocumentID, false); err != nil {
		s.logger.Error("stamp public signing document pdf failed", "error", err, "documentID", result.DocumentID)
		_ = s.store.AddSigningEvent(context.Background(), result.DocumentID, "", "", "pdf_stamp_failed", "สร้าง PDF พร้อมลายเซ็นไม่สำเร็จ", clientIP(r), r.UserAgent(), map[string]any{
			"error": err.Error(),
		})
	}
	if result.Completed {
		s.enqueueAutoFinalize(result.DocumentID, clientIP(r), r.UserAgent())
	}
	document, _ := s.store.FindSigningDocumentByID(r.Context(), result.DocumentID)
	signer, signerErr := s.store.FindSigningTaskByID(r.Context(), signerID)
	if signerErr != nil {
		signer = models.SigningDocumentSigner{ID: signerID, DocumentID: result.DocumentID, Status: "signed"}
	}
	payload := map[string]any{
		"document":  sanitizeSigningDocumentForExternal(document),
		"task":      sanitizeSigningTaskForUser(signer),
		"legal":     signingLegalPayload(),
		"completed": result.Completed,
	}
	s.completeIdempotency(idempotencyScope, "", r, http.StatusOK, payload)
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) writeRejectResult(w http.ResponseWriter, r *http.Request, idempotencyScope, actorUserID, documentID string, err error) {
	if errors.Is(err, store.ErrSigningTaskNotFound) {
		s.releaseIdempotency(idempotencyScope, actorUserID, r)
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return
	}
	if errors.Is(err, store.ErrSigningTaskUnavailable) {
		s.releaseIdempotency(idempotencyScope, actorUserID, r)
		writeError(w, http.StatusConflict, "signing_task_unavailable", "This signing task is not available.")
		return
	}
	if err != nil {
		s.releaseIdempotency(idempotencyScope, actorUserID, r)
		s.logger.Error("reject task failed", "error", err)
		writeError(w, http.StatusInternalServerError, "reject_task_failed", "Cannot reject document right now.")
		return
	}
	document, _ := s.store.FindSigningDocumentByID(r.Context(), documentID)
	payload := map[string]any{"document": document}
	s.completeIdempotency(idempotencyScope, actorUserID, r, http.StatusOK, payload)
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) replayIdempotentResponse(w http.ResponseWriter, r *http.Request, scope, actorUserID string) bool {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		return false
	}
	claim, err := s.store.ClaimIdempotencyKey(r.Context(), scope, key, actorUserID)
	if errors.Is(err, store.ErrIdempotencyInProgress) {
		writeError(w, http.StatusConflict, "idempotency_in_progress", "The same request is still being processed.")
		return true
	}
	if err != nil {
		s.logger.Error("claim idempotency key failed", "error", err, "scope", scope)
		writeError(w, http.StatusInternalServerError, "idempotency_failed", "Cannot process duplicate request guard right now.")
		return true
	}
	if claim.Claimed {
		return false
	}
	writeRawJSON(w, claim.ResponseCode, claim.Response)
	return true
}

func (s *Server) completeIdempotency(scope, actorUserID string, r *http.Request, status int, payload any) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		return
	}
	if err := s.store.CompleteIdempotencyKey(context.Background(), scope, key, actorUserID, status, payload); err != nil {
		s.logger.Warn("complete idempotency key failed", "error", err, "scope", scope)
	}
}

func (s *Server) releaseIdempotency(scope, actorUserID string, r *http.Request) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		return
	}
	if err := s.store.ReleaseIdempotencyKey(context.Background(), scope, key, actorUserID); err != nil {
		s.logger.Warn("release idempotency key failed", "error", err, "scope", scope)
	}
}

func taskUnavailableCode(status string) string {
	switch status {
	case "signed":
		return "already_signed"
	case "rejected":
		return "already_rejected"
	case "waiting":
		return "signing_task_not_turn"
	case "skipped":
		return "signing_task_skipped"
	default:
		return "signing_task_unavailable"
	}
}

func taskUnavailableMessage(status string) string {
	switch status {
	case "signed":
		return "This signing task was already signed."
	case "rejected":
		return "This signing task was already rejected."
	case "waiting":
		return "This signing task is not available yet."
	case "skipped":
		return "This signing task was skipped by workflow condition."
	default:
		return "This signing task is not available."
	}
}

func signingLegalPayload() map[string]string {
	return map[string]string{
		"text":    signingLegalText,
		"version": signingLegalTextVersion,
	}
}

func parsePositiveQueryInt(r *http.Request, key string, fallback int) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func decodeSigningTaskEventPayload(reader io.Reader, maxBytes int64) (models.SigningTaskEventRequest, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return models.SigningTaskEventRequest{}, err
	}
	if int64(len(data)) > maxBytes {
		return models.SigningTaskEventRequest{}, fmt.Errorf("signing task event too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var req models.SigningTaskEventRequest
	if err := decoder.Decode(&req); err != nil {
		return models.SigningTaskEventRequest{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return models.SigningTaskEventRequest{}, fmt.Errorf("signing task event invalid")
	}
	return req, nil
}

func normalizeSigningTaskEventMetadata(req models.SigningTaskEventRequest, document models.SigningDocument, signer models.SigningDocumentSigner) (map[string]any, error) {
	event := strings.TrimSpace(req.Event)
	if !signingUXEventNames[event] {
		return nil, fmt.Errorf("invalid signing task event")
	}
	return map[string]any{
		"event":           event,
		"sessionId":       truncateForMetadata(req.SessionID, 80),
		"docFormatCode":   document.DocFormatCode,
		"positionCode":    signer.PositionCode,
		"conditionType":   safeConditionType(signer.ConditionType),
		"signerType":      signer.SignerType,
		"taskStatus":      signer.Status,
		"elapsedMs":       clampInt64(req.ElapsedMS, 0, 24*60*60*1000),
		"pdfPage":         clampInt(req.PDFPage, 0, 500),
		"pdfPageCount":    clampInt(req.PDFPageCount, 0, 500),
		"attachmentCount": clampInt(req.AttachmentCnt, 0, 20),
		"errorCode":       truncateForMetadata(req.ErrorCode, 80),
		"viewport": map[string]any{
			"width":  clampInt(req.Viewport.Width, 0, 10000),
			"height": clampInt(req.Viewport.Height, 0, 10000),
		},
	}, nil
}

func (s *Server) lockCompletedDocument(ctx context.Context, documentID, docNo string) (bool, map[string]any) {
	document, err := s.store.FindSigningDocumentByID(ctx, documentID)
	if err != nil {
		return false, map[string]any{"error": err.Error()}
	}
	if docNo == "" {
		docNo = document.DocNo
	}
	lockCtx, cancel := context.WithTimeout(context.Background(), s.cfg.SMLPaperlessTimeout)
	defer cancel()
	lockCtx = store.WithSMLTenant(lockCtx, document.SMLTenant)
	metadata, err := s.lockSMLDocument(lockCtx, docNo)
	if err != nil {
		metadata = map[string]any{"error": err.Error(), "docNo": docNo}
		_ = s.store.MarkDocumentLockResult(context.Background(), documentID, false, metadata)
		return false, metadata
	}
	_ = s.store.MarkDocumentLockResult(context.Background(), documentID, true, metadata)
	return true, metadata
}

type completedDocumentFinalizeResult struct {
	FinalOK       bool
	ImageOK       bool
	LockOK        bool
	ImageMetadata map[string]any
	LockMetadata  map[string]any
}

func (s *Server) uploadCompletedDocumentImages(ctx context.Context, documentID string) (bool, map[string]any) {
	start := time.Now()
	document, err := s.store.FindSigningDocumentByID(ctx, documentID)
	if err != nil {
		metadata := map[string]any{"error": truncateForMetadata(err.Error(), 500)}
		_ = s.store.MarkDocumentImageResult(context.Background(), documentID, false, metadata)
		return false, metadata
	}
	if document.FinalFile == nil || strings.TrimSpace(document.FinalFile.StoragePath) == "" {
		metadata := map[string]any{"error": "final PDF is missing", "docNo": document.DocNo}
		_ = s.store.MarkDocumentImageResult(context.Background(), documentID, false, metadata)
		return false, metadata
	}
	if document.OriginalFile == nil || document.OriginalFile.PageCount <= 0 {
		metadata := map[string]any{"error": "original PDF page count is missing", "docNo": document.DocNo}
		_ = s.store.MarkDocumentImageResult(context.Background(), documentID, false, metadata)
		return false, metadata
	}

	renderTimeout := s.cfg.SMLPaperlessTimeout
	if renderTimeout < 30*time.Second {
		renderTimeout = 30 * time.Second
	}
	renderCtx, cancel := context.WithTimeout(context.Background(), renderTimeout)
	render, err := renderSMLDocumentSnapshots(renderCtx, document.FinalFile.StoragePath, document.OriginalFile.PageCount)
	cancel()
	metadata := map[string]any{
		"docNo":      document.DocNo,
		"totalPages": document.OriginalFile.PageCount,
		"elapsedMs":  time.Since(start).Milliseconds(),
	}
	if err != nil {
		metadata["error"] = truncateForMetadata(err.Error(), 500)
		_ = s.store.MarkDocumentImageResult(context.Background(), documentID, false, metadata)
		return false, metadata
	}
	metadata["imageCount"] = render.PageCount
	metadata["truncated"] = render.Truncated
	metadata["totalBytes"] = render.TotalBytes
	metadata["dpi"] = render.Profile.DPI
	metadata["quality"] = render.Profile.Quality
	metadata["renderElapsedMs"] = render.Elapsed.Milliseconds()

	uploadTimeout := s.cfg.SMLPaperlessTimeout
	if uploadTimeout < 30*time.Second {
		uploadTimeout = 30 * time.Second
	}
	uploadCtx, uploadCancel := context.WithTimeout(context.Background(), uploadTimeout)
	uploadCtx = store.WithSMLTenant(uploadCtx, document.SMLTenant)
	smlMetadata, err := s.replaceSMLDocumentImages(uploadCtx, document.DocNo, render)
	uploadCancel()
	metadata["elapsedMs"] = time.Since(start).Milliseconds()
	if err != nil {
		metadata["error"] = truncateForMetadata(err.Error(), 500)
		var smlErr *smlRequestError
		if errors.As(err, &smlErr) {
			if strings.TrimSpace(smlErr.Code) != "" {
				metadata["errorCode"] = smlErr.Code
			}
			if smlErr.Details != nil {
				metadata["errorDetails"] = smlErr.Details
			}
		}
		_ = s.store.MarkDocumentImageResult(context.Background(), documentID, false, metadata)
		return false, metadata
	}
	for key, value := range smlMetadata {
		metadata[key] = value
	}
	_ = s.store.MarkDocumentImageResult(context.Background(), documentID, true, metadata)
	return true, metadata
}

func (s *Server) finalizeCompletedDocument(ctx context.Context, documentID, ipAddress, userAgent string) completedDocumentFinalizeResult {
	start := time.Now()
	result := completedDocumentFinalizeResult{}
	if err := s.refreshStampedPDF(ctx, documentID, true); err != nil {
		s.logger.Error("final pdf evidence failed", "error", err, "documentID", documentID)
		_ = s.store.MarkDocumentEvidenceFailed(context.Background(), documentID, map[string]any{
			"error":     truncateForMetadata(err.Error(), 500),
			"elapsedMs": time.Since(start).Milliseconds(),
		})
		return result
	}
	result.FinalOK = true
	result.ImageOK, result.ImageMetadata = s.uploadCompletedDocumentImages(ctx, documentID)
	if result.ImageOK {
		result.LockOK, result.LockMetadata = s.lockCompletedDocument(ctx, documentID, "")
	}
	_ = s.store.AddSigningEvent(context.Background(), documentID, "", "", "final_pdf_metrics", "บันทึก metric การสร้าง final PDF", ipAddress, userAgent, map[string]any{
		"elapsedMs": time.Since(start).Milliseconds(),
		"imageOk":   result.ImageOK,
		"lockOk":    result.LockOK,
	})
	return result
}

func normalizePrintCopyRequest(req models.CreatePrintCopyRequest) models.CreatePrintCopyRequest {
	req.Channel = strings.ToLower(strings.TrimSpace(req.Channel))
	if req.Channel == "" {
		req.Channel = "web"
	}
	if req.Channel != "web" && req.Channel != "app" {
		req.Channel = "web"
	}
	req.PrinterName = truncateForMetadata(req.PrinterName, 120)
	if req.PrinterName == "" {
		if req.Channel == "web" {
			req.PrinterName = "not_available_web_browser"
		} else {
			req.PrinterName = "not_provided"
		}
	}
	req.ClientTimezone = truncateForMetadata(req.ClientTimezone, 80)
	req.DeviceID = truncateForMetadata(req.DeviceID, 160)
	return req
}

func (s *Server) readAndStorePDFUpload(w http.ResponseWriter, r *http.Request, fieldName, actorID, fallbackName string) (models.UploadedFile, error) {
	maxBytes := s.cfg.MaxUploadMB * 1024 * 1024
	file, header, err := r.FormFile(fieldName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "pdf_file_required", "PDF file is required.")
		return models.UploadedFile{}, err
	}
	defer file.Close()
	contentType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if contentType != "" && !strings.Contains(contentType, "pdf") {
		writeError(w, http.StatusBadRequest, "invalid_pdf_content_type", "Uploaded file content type must be PDF.")
		return models.UploadedFile{}, fmt.Errorf("invalid pdf content type")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "pdf_too_large", fmt.Sprintf("PDF must be %d MB or smaller.", s.cfg.MaxUploadMB))
		return models.UploadedFile{}, fmt.Errorf("pdf too large")
	}
	if !isPDFBytes(data) {
		writeError(w, http.StatusBadRequest, "invalid_pdf", "Uploaded file must be a valid PDF.")
		return models.UploadedFile{}, fmt.Errorf("invalid pdf")
	}
	pageCount, err := readPDFPageCount(data)
	if err != nil || pageCount <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_pdf", "Uploaded file must be a readable PDF.")
		return models.UploadedFile{}, fmt.Errorf("invalid pdf")
	}
	uploaded, err := s.storeUploadedBytes(r.Context(), data, filepath.Base(header.Filename), fallbackName, "application/pdf", ".pdf", pageCount, actorID)
	if err != nil {
		s.logger.Error("store uploaded pdf failed", "error", err)
		writeError(w, http.StatusInternalServerError, "upload_store_failed", "Cannot save uploaded PDF right now.")
		return models.UploadedFile{}, err
	}
	return uploaded, nil
}

func (s *Server) storeSignatureImage(ctx context.Context, dataURL, actorID string) (models.UploadedFile, error) {
	data, _, _, err := parseSignatureDataURL(dataURL)
	if err != nil {
		return models.UploadedFile{}, err
	}
	if int64(len(data)) > maxSignatureImageBytes {
		return models.UploadedFile{}, fmt.Errorf("signature image must be 2 MB or smaller")
	}
	normalized, err := normalizeSignatureImage(data)
	if err != nil {
		return models.UploadedFile{}, err
	}
	if len(normalized) > maxSignatureImageBytes {
		return models.UploadedFile{}, fmt.Errorf("signature image must be 2 MB or smaller")
	}
	return s.storeUploadedBytes(ctx, normalized, "signature.png", "signature.png", "image/png", ".png", 0, actorID)
}

func (s *Server) readAndStoreSigningAttachment(w http.ResponseWriter, r *http.Request, actorID string, validate func(*http.Request) error) (models.UploadedFile, string, error) {
	maxBytes := s.cfg.MaxUploadMB * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024)
	if err := r.ParseMultipartForm(maxBytes + 1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_form", "Attachment form is invalid.")
		return models.UploadedFile{}, "", err
	}
	if validate != nil {
		if err := validate(r); err != nil {
			return models.UploadedFile{}, "", err
		}
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "attachment_file_required", "Attachment file is required.")
		return models.UploadedFile{}, "", err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "attachment_too_large", fmt.Sprintf("Attachment must be %d MB or smaller.", s.cfg.MaxUploadMB))
		return models.UploadedFile{}, "", fmt.Errorf("attachment too large")
	}
	contentType, ext, pageCount, err := detectSigningAttachmentType(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_attachment", err.Error())
		return models.UploadedFile{}, "", err
	}
	note := truncateForMetadata(r.FormValue("note"), 500)
	uploaded, err := s.storeUploadedBytes(r.Context(), data, filepath.Base(header.Filename), "attachment"+ext, contentType, ext, pageCount, actorID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "attachment_upload_failed", "Cannot store attachment right now.")
		return models.UploadedFile{}, "", err
	}
	return uploaded, note, nil
}

func detectSigningAttachmentType(data []byte) (string, string, int, error) {
	if isPDFBytes(data) {
		pageCount, err := readPDFPageCount(data)
		if err != nil || pageCount <= 0 {
			return "", "", 0, fmt.Errorf("PDF attachment must be readable")
		}
		return "application/pdf", ".pdf", pageCount, nil
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png", ".png", 0, nil
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg", ".jpg", 0, nil
	}
	return "", "", 0, fmt.Errorf("Attachment must be PDF, PNG, or JPEG")
}

func (s *Server) storeUploadedBytes(ctx context.Context, data []byte, originalName, fallbackName, contentType, ext string, pageCount int, actorID string) (models.UploadedFile, error) {
	if err := os.MkdirAll(s.cfg.FileStorageDir, 0o750); err != nil {
		return models.UploadedFile{}, err
	}
	originalName = filepath.Base(strings.TrimSpace(originalName))
	if originalName == "." || originalName == string(os.PathSeparator) || originalName == "" {
		originalName = fallbackName
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	storedName := fmt.Sprintf("%s-%s%s", sha[:16], randomHex(8), ext)
	storagePath := filepath.Join(s.cfg.FileStorageDir, storedName)
	if err := os.WriteFile(storagePath, data, 0o640); err != nil {
		return models.UploadedFile{}, err
	}
	uploaded, err := s.store.CreateUploadedFile(ctx, models.UploadedFile{
		OriginalName: originalName,
		StoredName:   storedName,
		StoragePath:  storagePath,
		ContentType:  contentType,
		SizeBytes:    int64(len(data)),
		PageCount:    pageCount,
		SHA256:       sha,
		CreatedBy:    actorID,
	})
	if err != nil {
		_ = os.Remove(storagePath)
	}
	return uploaded, err
}

func parseSignatureDataURL(value string) ([]byte, string, string, error) {
	value = strings.TrimSpace(value)
	const marker = ";base64,"
	if !strings.HasPrefix(value, "data:image/") || !strings.Contains(value, marker) {
		return nil, "", "", fmt.Errorf("signature must be an image data URL")
	}
	parts := strings.SplitN(value, marker, 2)
	contentType := strings.TrimPrefix(parts[0], "data:")
	ext := ".png"
	if contentType == "image/jpeg" || contentType == "image/jpg" {
		ext = ".jpg"
		contentType = "image/jpeg"
	} else if contentType != "image/png" {
		return nil, "", "", fmt.Errorf("signature must be PNG or JPEG")
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil || len(data) == 0 {
		return nil, "", "", fmt.Errorf("signature image is invalid")
	}
	return data, contentType, ext, nil
}

func (s *Server) missingActiveConfigUsers(ctx context.Context, configs []models.DocumentConfigStep) []string {
	missing := []string{}
	seen := map[string]bool{}
	for _, step := range configs {
		if step.ConditionType == 3 {
			continue
		}
		for _, value := range stepUsers(step) {
			username := strings.TrimSpace(strings.SplitN(value, ":", 2)[0])
			if username == "" || seen[strings.ToLower(username)] {
				continue
			}
			seen[strings.ToLower(username)] = true
			user, err := s.store.FindUserByUsername(ctx, username)
			if err != nil || user.Status != "active" {
				missing = append(missing, username)
			}
		}
	}
	return missing
}

func hashSecret(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func randomNumericOTP(length int) string {
	const digits = "0123456789"
	raw := randomHex(length)
	out := strings.Builder{}
	for _, ch := range raw {
		out.WriteByte(digits[int(ch)%len(digits)])
		if out.Len() == length {
			break
		}
	}
	return out.String()
}

func (s *Server) externalURL(r *http.Request, token string) string {
	base := s.cfg.PublicBaseURL
	if base == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return strings.TrimRight(base, "/") + "/external/sign/" + token
}

func (s *Server) withExternalURLs(r *http.Request, document models.SigningDocument) models.SigningDocument {
	document.Attachments = nil
	for i := range document.Signers {
		if document.Signers[i].SignerType == "external" && document.Signers[i].ExternalTokenID != "" {
			document.Signers[i].ExternalURL = strings.TrimRight(s.cfg.PublicBaseURL, "/") + "/external/sign/<regenerate-to-view-token>"
		}
	}
	return document
}

func (s *Server) externalSignerFromRequest(w http.ResponseWriter, r *http.Request) (models.SigningDocumentSigner, bool) {
	rawToken := strings.TrimSpace(r.PathValue("token"))
	session := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if rawToken == "" || session == "" {
		writeError(w, http.StatusUnauthorized, "external_session_required", "External signing session is required.")
		return models.SigningDocumentSigner{}, false
	}
	signer, err := s.store.FindExternalSignerBySession(r.Context(), hashSecret(rawToken), hashSecret(session))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "external_session_invalid", "External signing session is invalid or expired.")
		return models.SigningDocumentSigner{}, false
	}
	return signer, true
}

func documentHasSigner(document models.SigningDocument, username string) bool {
	for _, signer := range document.Signers {
		if strings.EqualFold(signer.SignerUser, username) {
			return true
		}
	}
	return false
}
