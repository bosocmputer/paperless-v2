package api

import (
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
	writeJSON(w, http.StatusOK, map[string]any{"document": document, "task": signer})
}

func (s *Server) signMySigningTask(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	var req models.SignTaskRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	uploaded, err := s.storeSignatureImage(r.Context(), req.SignatureDataURL, user.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_signature", err.Error())
		return
	}
	result, err := s.store.SignInternalTask(r.Context(), taskID, user.Username, uploaded.ID, req.DeviceID, clientIP(r), r.UserAgent())
	s.writeTaskMutationResult(w, r, result, err)
}

func (s *Server) rejectMySigningTask(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	taskID := strings.TrimSpace(r.PathValue("taskId"))
	var req models.RejectTaskRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	documentID, err := s.store.RejectInternalTask(r.Context(), taskID, user.Username, req.Reason, req.DeviceID, clientIP(r), r.UserAgent())
	s.writeRejectResult(w, r, documentID, err)
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
	writeJSON(w, http.StatusOK, map[string]any{"document": document, "task": signer})
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
	uploaded, err := s.storeSignatureImage(r.Context(), req.SignatureDataURL, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_signature", err.Error())
		return
	}
	result, err := s.store.SignExternalTask(r.Context(), signer.ID, uploaded.ID, req.DeviceID, clientIP(r), r.UserAgent())
	s.writeTaskMutationResult(w, r, result, err)
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
	documentID, err := s.store.RejectExternalTask(r.Context(), signer.ID, req.Reason, req.DeviceID, clientIP(r), r.UserAgent())
	s.writeRejectResult(w, r, documentID, err)
}

func (s *Server) writeTaskMutationResult(w http.ResponseWriter, r *http.Request, result store.SignTaskResult, err error) {
	if errors.Is(err, store.ErrSigningTaskNotFound) {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return
	}
	if errors.Is(err, store.ErrSigningTaskUnavailable) {
		writeError(w, http.StatusConflict, "signing_task_unavailable", "This signing task is not available.")
		return
	}
	if err != nil {
		s.logger.Error("sign task failed", "error", err)
		writeError(w, http.StatusInternalServerError, "signing_task_failed", "Cannot sign document right now.")
		return
	}
	if err := s.refreshStampedPDF(r.Context(), result.DocumentID, result.Completed); err != nil {
		s.logger.Error("stamp signing document pdf failed", "error", err, "documentID", result.DocumentID)
		_ = s.store.AddSigningEvent(context.Background(), result.DocumentID, "", "", "pdf_stamp_failed", "สร้าง PDF พร้อมลายเซ็นไม่สำเร็จ", clientIP(r), r.UserAgent(), map[string]any{
			"error": err.Error(),
		})
	}
	if result.Completed {
		s.lockCompletedDocument(r.Context(), result.DocumentID, "")
	}
	document, _ := s.store.FindSigningDocumentByID(r.Context(), result.DocumentID)
	writeJSON(w, http.StatusOK, map[string]any{"document": document})
}

func (s *Server) writeRejectResult(w http.ResponseWriter, r *http.Request, documentID string, err error) {
	if errors.Is(err, store.ErrSigningTaskNotFound) {
		writeError(w, http.StatusNotFound, "signing_task_not_found", "Signing task was not found.")
		return
	}
	if errors.Is(err, store.ErrSigningTaskUnavailable) {
		writeError(w, http.StatusConflict, "signing_task_unavailable", "This signing task is not available.")
		return
	}
	if err != nil {
		s.logger.Error("reject task failed", "error", err)
		writeError(w, http.StatusInternalServerError, "reject_task_failed", "Cannot reject document right now.")
		return
	}
	document, _ := s.store.FindSigningDocumentByID(r.Context(), documentID)
	writeJSON(w, http.StatusOK, map[string]any{"document": document})
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
