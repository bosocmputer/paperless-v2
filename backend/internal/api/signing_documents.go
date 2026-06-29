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
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

const (
	signingLegalTextVersion = "thai-eta-2544-v2"
	signingLegalText        = "เอกสารนี้จัดทำและลงนามในรูปแบบอิเล็กทรอนิกส์ตาม พ.ร.บ. ธุรกรรมทางอิเล็กทรอนิกส์ พ.ศ. 2544 ผู้ลงนามยืนยันความถูกต้องของเนื้อหาและยอมรับผลผูกพันทางกฎหมายทุกประการ"
	maxSigningEventBytes    = 8 * 1024
)

var signingUXEventNames = map[string]bool{
	"task_open":         true,
	"pdf_load_success":  true,
	"pdf_load_error":    true,
	"signature_started": true,
	"signature_cleared": true,
	"sign_attempt":      true,
	"sign_success":      true,
	"sign_error":        true,
	"reject_success":    true,
	"attachment_upload": true,
	"blocked_not_turn":  true,
	"blocked_signed":    true,
	"blocked_rejected":  true,
}

func (s *Server) listSigningDocuments(w http.ResponseWriter, r *http.Request) {
	documents, err := s.store.ListSigningDocuments(r.Context())
	if err != nil {
		s.logger.Error("list signing documents failed", "error", err)
		writeError(w, http.StatusInternalServerError, "signing_documents_failed", "Cannot load signing documents right now.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": documents})
}

func (s *Server) createSigningDocument(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	maxBytes := s.cfg.MaxUploadMB * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024)
	if err := r.ParseMultipartForm(maxBytes + 1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_form", "Document form is invalid.")
		return
	}
	docFormatCode := strings.TrimSpace(r.FormValue("docFormatCode"))
	if docFormatCode == "" {
		docFormatCode = strings.TrimSpace(r.FormValue("doc_format_code"))
	}
	docNo := strings.TrimSpace(r.FormValue("docNo"))
	if docNo == "" {
		docNo = strings.TrimSpace(r.FormValue("doc_no"))
	}
	if docFormatCode == "" || docNo == "" {
		writeError(w, http.StatusBadRequest, "document_required", "doc_format_code and doc_no are required.")
		return
	}

	format, err := s.fetchSMLDocFormatByCode(r.Context(), docFormatCode)
	if err != nil {
		s.writeDocFormatValidationError(w, err)
		return
	}
	candidate, err := s.fetchSMLDocumentCandidate(r.Context(), format.Code, docNo)
	if err != nil {
		writeError(w, http.StatusBadGateway, "sml_document_validation_failed", "Cannot verify selected SML document.")
		return
	}
	if candidate.IsLockRecord == 1 && strings.TrimSpace(r.FormValue("confirmLocked")) != "1" {
		writeError(w, http.StatusConflict, "sml_document_locked", "SML document is already locked. Confirm before creating a PaperLess document.")
		return
	}

	screenCode := normalizeScreenCode(format.ScreenCode)
	configs, err := s.store.ListDocumentConfigSteps(r.Context(), screenCode, format.Code)
	if err != nil || len(configs) == 0 {
		writeError(w, http.StatusBadRequest, "document_config_required", "Document config is required before sending for signature.")
		return
	}
	if missing := s.missingActiveConfigUsers(r.Context(), configs); len(missing) > 0 {
		writeError(w, http.StatusBadRequest, "signer_user_inactive", "Every configured signer must exist and be active: "+strings.Join(missing, ", "))
		return
	}
	_, active, err := s.store.GetSignatureTemplateState(r.Context(), screenCode, format.Code)
	if err != nil || active == nil {
		writeError(w, http.StatusBadRequest, "signature_template_required", "Signature template is required before sending for signature.")
		return
	}
	if issues := validateSignatureTemplate(*active, configs, s.cfg.MaxTemplatePages); len(issues) > 0 {
		writeValidationIssues(w, http.StatusBadRequest, "signature_template_invalid", issues)
		return
	}

	uploaded, err := s.readAndStorePDFUpload(w, r, "file", actor.ID, "document.pdf")
	if err != nil {
		return
	}
	if active.SampleFile != nil && active.SampleFile.PageCount > 0 && uploaded.PageCount != active.SampleFile.PageCount {
		writeError(w, http.StatusBadRequest, "pdf_page_count_mismatch", fmt.Sprintf("Uploaded PDF has %d pages but template has %d pages.", uploaded.PageCount, active.SampleFile.PageCount))
		return
	}

	document, err := s.store.CreateSigningDocument(r.Context(), store.CreateSigningDocumentInput{
		ScreenCode: screenCode,
		Format:     format,
		Candidate:  candidate,
		Template:   *active,
		Configs:    configs,
		File:       uploaded,
		ActorID:    actor.ID,
		IPAddress:  clientIP(r),
		UserAgent:  r.UserAgent(),
	})
	if errors.Is(err, store.ErrSigningDocumentDuplicate) {
		writeError(w, http.StatusConflict, "signing_document_duplicate", "This SML document is already in an active PaperLess workflow.")
		return
	}
	if err != nil {
		s.logger.Error("create signing document failed", "error", err)
		writeError(w, http.StatusInternalServerError, "signing_document_create_failed", "Cannot create signing document right now.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"document": s.withExternalURLs(r, document)})
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
	writeJSON(w, http.StatusOK, map[string]any{"document": s.withExternalURLs(r, document)})
}

func (s *Server) getSigningDocumentPDF(w http.ResponseWriter, r *http.Request) {
	document, err := s.store.FindSigningDocumentByID(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if errors.Is(err, store.ErrSigningDocumentNotFound) {
		writeError(w, http.StatusNotFound, "signing_document_not_found", "Signing document was not found.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	user, _ := currentUser(r)
	if user.Role != "admin" && !documentHasSigner(document, user.Username) {
		writeError(w, http.StatusForbidden, "forbidden", "You cannot view this document.")
		return
	}
	file := document.CurrentFile
	switch strings.TrimSpace(r.URL.Query().Get("version")) {
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
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", file.OriginalName))
	http.ServeFile(w, r, file.StoragePath)
}

func (s *Server) retrySigningDocumentLock(w http.ResponseWriter, r *http.Request) {
	document, err := s.store.FindSigningDocumentByID(r.Context(), strings.TrimSpace(r.PathValue("id")))
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
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "lock": metadata})
}

func (s *Server) retrySigningDocumentFinalPDF(w http.ResponseWriter, r *http.Request) {
	documentID := strings.TrimSpace(r.PathValue("id"))
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
	finalOK, lockOK := s.finalizeCompletedDocument(r.Context(), documentID, clientIP(r), r.UserAgent())
	if !finalOK {
		writeError(w, http.StatusBadGateway, "final_pdf_failed", "Final PDF evidence generation failed. You can retry again.")
		return
	}
	updated, _ := s.store.FindSigningDocumentByID(r.Context(), documentID)
	writeJSON(w, http.StatusOK, map[string]any{"document": s.withExternalURLs(r, updated), "lockOk": lockOK})
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

	printedAt := time.Now()
	deviceIDHash := shortHash(req.DeviceID)
	printedBy := strings.TrimSpace(actor.DisplayName)
	if printedBy == "" {
		printedBy = actor.Username
	}
	printed, err := createPrintCopyPDF(document.FinalFile.StoragePath, document.FinalFile.PageCount, printEvidencePage{
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
	pageCount := document.FinalFile.PageCount + 1
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
		writeError(w, http.StatusInternalServerError, "print_event_failed", "Cannot record print event right now.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"printCopyId": printEvent.ID,
		"fileUrl":     fmt.Sprintf("/api/signing-documents/%s/print-copies/%s/pdf", document.ID, printEvent.ID),
		"printEvent":  printEvent,
	})
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
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", printEvent.File.OriginalName))
	http.ServeFile(w, r, printEvent.File.StoragePath)
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
	documents, err := s.store.ListPendingSigningTasksForUser(r.Context(), user.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_tasks_failed", "Cannot load signing tasks right now.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": documents})
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
	writeJSON(w, http.StatusOK, map[string]any{"document": document, "task": signer, "legal": signingLegalPayload()})
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
	uploaded, note, err := s.readAndStoreSigningAttachment(w, r, user.ID)
	if err != nil {
		return
	}
	if err := s.store.AddSigningAttachment(r.Context(), signer.DocumentID, signer.ID, uploaded.ID, note, user.ID); err != nil {
		s.logger.Error("add signing attachment failed", "error", err, "signerID", signer.ID)
		writeError(w, http.StatusInternalServerError, "attachment_upload_failed", "Cannot attach file right now.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"file": uploaded})
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
	result, err := s.store.SignInternalTask(r.Context(), taskID, user.Username, uploaded.ID, req.DeviceID, clientIP(r), r.UserAgent(), signingLegalTextVersion)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"session": models.VerifyExternalOTPResponse{SessionToken: sessionToken, ExpiresAt: expiresAt},
		"task":    signer,
	})
}

func (s *Server) getPublicSigningDocument(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "signing_document_failed", "Cannot load signing document right now.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": document, "task": signer, "legal": signingLegalPayload()})
}

func (s *Server) getPublicSigningPDF(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
	document, err := s.store.FindSigningDocumentByID(r.Context(), signer.DocumentID)
	if err != nil || document.CurrentFile == nil {
		writeError(w, http.StatusNotFound, "pdf_not_found", "PDF was not found.")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	http.ServeFile(w, r, document.CurrentFile.StoragePath)
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

func (s *Server) uploadPublicSigningTaskAttachment(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
	if signer.Status != "pending" {
		writeError(w, http.StatusConflict, taskUnavailableCode(signer.Status), taskUnavailableMessage(signer.Status))
		return
	}
	uploaded, note, err := s.readAndStoreSigningAttachment(w, r, "")
	if err != nil {
		return
	}
	if err := s.store.AddSigningAttachment(r.Context(), signer.DocumentID, signer.ID, uploaded.ID, note, ""); err != nil {
		s.logger.Error("add public signing attachment failed", "error", err, "signerID", signer.ID)
		writeError(w, http.StatusInternalServerError, "attachment_upload_failed", "Cannot attach file right now.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"file": uploaded})
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
	if signer.Status != "pending" {
		writeError(w, http.StatusConflict, taskUnavailableCode(signer.Status), taskUnavailableMessage(signer.Status))
		return
	}
	scope := "public-sign:" + signer.ID
	if s.replayIdempotentResponse(w, r, scope, "") {
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
	result, err := s.store.SignExternalTask(r.Context(), signer.ID, uploaded.ID, req.DeviceID, clientIP(r), r.UserAgent(), signingLegalTextVersion)
	s.writeTaskMutationResult(w, r, scope, "", result, err)
}

func (s *Server) rejectPublicSigningTask(w http.ResponseWriter, r *http.Request) {
	signer, ok := s.externalSignerFromRequest(w, r)
	if !ok {
		return
	}
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
	scope := "public-reject:" + signer.ID
	if s.replayIdempotentResponse(w, r, scope, "") {
		return
	}
	documentID, err := s.store.RejectExternalTask(r.Context(), signer.ID, req.Reason, req.DeviceID, clientIP(r), r.UserAgent())
	s.writeRejectResult(w, r, scope, "", documentID, err)
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
		s.finalizeCompletedDocument(r.Context(), result.DocumentID, clientIP(r), r.UserAgent())
	}
	document, _ := s.store.FindSigningDocumentByID(r.Context(), result.DocumentID)
	payload := map[string]any{"document": document}
	s.completeIdempotency(idempotencyScope, actorUserID, r, http.StatusOK, payload)
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
	metadata, err := s.lockSMLDocument(lockCtx, docNo)
	if err != nil {
		metadata = map[string]any{"error": err.Error(), "docNo": docNo}
		_ = s.store.MarkDocumentLockResult(context.Background(), documentID, false, metadata)
		return false, metadata
	}
	_ = s.store.MarkDocumentLockResult(context.Background(), documentID, true, metadata)
	return true, metadata
}

func (s *Server) finalizeCompletedDocument(ctx context.Context, documentID, ipAddress, userAgent string) (bool, bool) {
	start := time.Now()
	if err := s.refreshStampedPDF(ctx, documentID, true); err != nil {
		s.logger.Error("final pdf evidence failed", "error", err, "documentID", documentID)
		_ = s.store.MarkDocumentEvidenceFailed(context.Background(), documentID, map[string]any{
			"error":     truncateForMetadata(err.Error(), 500),
			"elapsedMs": time.Since(start).Milliseconds(),
		})
		return false, false
	}
	ok, _ := s.lockCompletedDocument(ctx, documentID, "")
	_ = s.store.AddSigningEvent(context.Background(), documentID, "", "", "final_pdf_metrics", "บันทึก metric การสร้าง final PDF", ipAddress, userAgent, map[string]any{
		"elapsedMs": time.Since(start).Milliseconds(),
		"lockOk":    ok,
	})
	return true, ok
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
	return s.storeUploadedBytes(r.Context(), data, filepath.Base(header.Filename), fallbackName, "application/pdf", ".pdf", pageCount, actorID)
}

func (s *Server) storeSignatureImage(ctx context.Context, dataURL, actorID string) (models.UploadedFile, error) {
	data, contentType, ext, err := parseSignatureDataURL(dataURL)
	if err != nil {
		return models.UploadedFile{}, err
	}
	if int64(len(data)) > 2*1024*1024 {
		return models.UploadedFile{}, fmt.Errorf("signature image must be 2 MB or smaller")
	}
	return s.storeUploadedBytes(ctx, data, "signature"+ext, "signature"+ext, contentType, ext, 0, actorID)
}

func (s *Server) readAndStoreSigningAttachment(w http.ResponseWriter, r *http.Request, actorID string) (models.UploadedFile, string, error) {
	maxBytes := s.cfg.MaxUploadMB * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024)
	if err := r.ParseMultipartForm(maxBytes + 1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_form", "Attachment form is invalid.")
		return models.UploadedFile{}, "", err
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
