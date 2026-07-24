package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

const internalDocumentScreenCode = "INTERNAL"

var (
	internalMasterCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,19}$`)
	internalPrefixPattern     = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,19}$`)
	internalRunningPattern    = regexp.MustCompile(`(?i)^@?(?:YYYY|YY|MM|DD|[-_.])*#{1,9}(?:YYYY|YY|MM|DD|[-_.])*$`)
)

func (s *Server) internalDocumentsAvailable(w http.ResponseWriter) bool {
	if !s.cfg.InternalDocuments {
		writeError(w, http.StatusNotFound, "feature_not_enabled", "Internal documents are not enabled.")
		return false
	}
	return true
}

func (s *Server) listDocumentTypes(w http.ResponseWriter, r *http.Request) {
	actor, _ := currentUser(r)
	masters := []models.InternalDocumentMaster{}
	if s.cfg.InternalDocuments {
		if err := s.store.EnsureDefaultInternalDocumentMasters(r.Context(), actor.ID); err != nil {
			s.logger.Error("seed internal document masters failed", "error", err)
			writeError(w, http.StatusInternalServerError, "document_types_failed", "Cannot load document types right now.")
			return
		}
		var err error
		masters, err = s.store.ListInternalDocumentMasters(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "document_types_failed", "Cannot load document types right now.")
			return
		}
		s.ensureInternalMasterSamples(r.Context(), masters, actor.ID)
		masters, err = s.store.ListInternalDocumentMasters(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "document_types_failed", "Cannot load document types right now.")
			return
		}
	}
	items := make([]models.DocumentType, 0, len(masters)+64)
	smlWarning := ""
	formats, smlErr := s.fetchSMLDocFormats(r.Context(), "")
	if smlErr != nil {
		smlWarning = "โหลดชนิดเอกสารจาก SML ไม่สำเร็จ แต่ยังตั้งค่าเอกสารภายในได้"
	} else {
		for _, format := range formats {
			items = append(items, models.DocumentType{Code: format.Code, Name1: format.Name1, Name2: format.Name2, ScreenCode: format.ScreenCode, Source: "sml", Active: true})
		}
	}
	for _, master := range masters {
		items = append(items, models.DocumentType{Code: master.Code, Name1: master.Name, ScreenCode: internalDocumentScreenCode, Source: "internal", Active: master.Status == "active"})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Source == items[j].Source {
			return items[i].Code < items[j].Code
		}
		return items[i].Source < items[j].Source
	})
	writeJSON(w, http.StatusOK, map[string]any{"documentTypes": items, "smlWarning": smlWarning})
}

func (s *Server) listInternalDocumentMasters(w http.ResponseWriter, r *http.Request) {
	if !s.internalDocumentsAvailable(w) {
		return
	}
	actor, _ := currentUser(r)
	if err := s.store.EnsureDefaultInternalDocumentMasters(r.Context(), actor.ID); err != nil {
		s.logger.Error("seed internal document masters failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_masters_failed", "Cannot load internal document masters right now.")
		return
	}
	items, err := s.store.ListInternalDocumentMasters(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_masters_failed", "Cannot load internal document masters right now.")
		return
	}
	s.ensureInternalMasterSamples(r.Context(), items, actor.ID)
	items, err = s.store.ListInternalDocumentMasters(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_masters_failed", "Cannot load internal document masters right now.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"masters": items})
}

func (s *Server) createInternalDocumentMaster(w http.ResponseWriter, r *http.Request) {
	if !s.internalDocumentsAvailable(w) {
		return
	}
	actor, _ := currentUser(r)
	var req models.InternalDocumentMasterRequest
	if err := decodeLimitedJSON(w, r, 32<<10, &req); err != nil {
		return
	}
	req, issues := normalizeInternalMasterRequest(req)
	if len(issues) > 0 {
		writeValidationMessages(w, issues)
		return
	}
	collides, collisionErr := s.internalMasterCodeCollidesWithSML(r.Context(), req.Code)
	if collisionErr != nil {
		writeError(w, http.StatusBadGateway, "sml_document_type_check_failed", "ตรวจสอบรหัสกับ SML ไม่สำเร็จ กรุณาลองใหม่")
		return
	}
	if collides {
		writeError(w, http.StatusConflict, "internal_master_code_conflict", "รหัสเอกสารซ้ำกับชนิดเอกสารใน SML")
		return
	}
	if req.Status == "active" {
		writeError(w, http.StatusBadRequest, "internal_master_activation_requires_config", "กรุณาสร้าง Master แล้วตั้งค่า Workflow ก่อนเปิดใช้งาน")
		return
	}
	master, err := s.store.CreateInternalDocumentMaster(r.Context(), req, actor.ID)
	if errors.Is(err, store.ErrInternalMasterDuplicate) {
		writeError(w, http.StatusConflict, "internal_master_duplicate", "รหัส Master นี้มีอยู่แล้ว")
		return
	}
	if err != nil {
		s.logger.Error("create internal document master failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_master_create_failed", "Cannot create internal document master right now.")
		return
	}
	if err := s.ensureInternalMasterSample(r.Context(), master, actor.ID); err != nil {
		s.logger.Error("create internal master sample PDF failed", "error", err, "masterId", master.ID)
	}
	_ = s.store.WriteAudit(r.Context(), actor.ID, "internal_master.create", "internal_document_master", master.ID, clientIP(r), r.UserAgent())
	writeJSON(w, http.StatusCreated, map[string]any{"master": master})
}

func (s *Server) updateInternalDocumentMaster(w http.ResponseWriter, r *http.Request) {
	if !s.internalDocumentsAvailable(w) {
		return
	}
	actor, _ := currentUser(r)
	var req models.InternalDocumentMasterRequest
	if err := decodeLimitedJSON(w, r, 32<<10, &req); err != nil {
		return
	}
	req, issues := normalizeInternalMasterRequest(req)
	if req.Revision < 1 {
		issues = append(issues, "revision ต้องมากกว่า 0")
	}
	if len(issues) > 0 {
		writeValidationMessages(w, issues)
		return
	}
	current, err := s.store.FindInternalDocumentMaster(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeInternalMasterError(w, err)
		return
	}
	collides, collisionErr := s.internalMasterCodeCollidesWithSML(r.Context(), req.Code)
	if collisionErr != nil {
		writeError(w, http.StatusBadGateway, "sml_document_type_check_failed", "ตรวจสอบรหัสกับ SML ไม่สำเร็จ กรุณาลองใหม่")
		return
	}
	if !strings.EqualFold(current.Code, req.Code) && collides {
		writeError(w, http.StatusConflict, "internal_master_code_conflict", "รหัสเอกสารซ้ำกับชนิดเอกสารใน SML")
		return
	}
	if req.Status == "active" {
		if err := s.validateInternalMasterActivation(r.Context(), current.ID, req.Code); err != nil {
			writeError(w, http.StatusConflict, "internal_master_not_ready", err.Error())
			return
		}
	}
	master, err := s.store.UpdateInternalDocumentMaster(r.Context(), current.ID, req)
	if err != nil {
		writeInternalMasterError(w, err)
		return
	}
	_ = s.store.WriteAudit(r.Context(), actor.ID, "internal_master.update", "internal_document_master", master.ID, clientIP(r), r.UserAgent())
	writeJSON(w, http.StatusOK, map[string]any{"master": master})
}

func (s *Server) deleteInternalDocumentMaster(w http.ResponseWriter, r *http.Request) {
	if !s.internalDocumentsAvailable(w) {
		return
	}
	actor, _ := currentUser(r)
	id := strings.TrimSpace(r.PathValue("id"))
	if err := s.store.DeleteInternalDocumentMaster(r.Context(), id); err != nil {
		writeInternalMasterError(w, err)
		return
	}
	_ = s.store.WriteAudit(r.Context(), actor.ID, "internal_master.delete", "internal_document_master", id, clientIP(r), r.UserAgent())
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createInternalDocument(w http.ResponseWriter, r *http.Request) {
	if !s.internalDocumentsAvailable(w) {
		return
	}
	actor, _ := currentUser(r)
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required.")
		return
	}
	var req models.InternalDocumentCreateRequest
	if err := decodeLimitedJSON(w, r, 256<<10, &req); err != nil {
		return
	}
	normalized, totalCents, issues := normalizeInternalDocumentCreateRequest(req)
	if len(issues) > 0 {
		writeValidationMessages(w, issues)
		return
	}
	master, err := s.store.FindInternalDocumentMaster(r.Context(), normalized.MasterID)
	if err != nil {
		writeInternalMasterError(w, err)
		return
	}
	if master.Status != "active" {
		writeError(w, http.StatusConflict, "internal_master_inactive", "Master เอกสารนี้ยังไม่เปิดใช้งาน")
		return
	}
	configs, err := s.internalWorkflowContext(r.Context(), master.Code)
	if err != nil {
		writeError(w, http.StatusConflict, "internal_master_not_ready", err.Error())
		return
	}
	company, err := s.fetchSMLCompanyProfile(r.Context())
	if err != nil {
		var smlErr *smlRequestError
		if errors.As(err, &smlErr) && smlErr.Code != "" {
			writeError(w, http.StatusBadGateway, smlErr.Code, "สร้างเอกสารไม่ได้: ข้อมูลบริษัทใน SML ไม่พร้อม กรุณาแจ้งผู้ดูแล SML")
		} else {
			writeError(w, http.StatusBadGateway, "company_profile_unavailable", "สร้างเอกสารไม่ได้: อ่านข้อมูลบริษัทจาก SML ไม่สำเร็จ")
		}
		return
	}
	documentDate, _ := time.Parse("2006-01-02", normalized.DocumentDate)
	requiredDate, _ := time.Parse("2006-01-02", normalized.RequiredDate)
	document, existed, err := s.store.ReserveInternalDocument(r.Context(), store.ReserveInternalDocumentInput{
		MasterID: master.ID, DocumentDate: documentDate, RequiredDate: requiredDate,
		RequesterName: normalized.RequesterName, PositionName: normalized.PositionName,
		DepartmentName: normalized.DepartmentName, Purpose: normalized.Purpose,
		TotalAmount: centsToAmount(totalCents), Items: normalized.Items,
		Company: internalCompanySnapshot(company), IdempotencyKey: idempotencyKey, ActorID: actor.ID,
	})
	if err != nil {
		s.writeInternalDocumentError(w, err)
		return
	}
	if existed {
		switch document.Status {
		case "draft":
			writeJSON(w, http.StatusOK, map[string]any{"internalDocument": document, "signingDocumentId": document.SigningDocumentID, "idempotentReplay": true})
			return
		case "generating":
			if time.Since(document.UpdatedAt) < 2*time.Minute {
				writeError(w, http.StatusConflict, "internal_document_generation_in_progress", "ระบบกำลังสร้างเอกสารจากคำขอนี้ กรุณารอสักครู่")
				return
			}
			_ = s.store.MarkInternalDocumentGenerationFailed(r.Context(), document.ID)
			document.Status = "generation_failed"
		}
		if document.Status == "generation_failed" {
			if recovered, recoverErr := s.recoverInternalDocumentCreate(r.Context(), document, actor.ID); recoverErr == nil {
				writeJSON(w, http.StatusOK, map[string]any{"internalDocument": recovered, "signingDocumentId": recovered.SigningDocumentID, "idempotentReplay": true, "recovered": true})
				return
			} else if !errors.Is(recoverErr, store.ErrSigningDocumentNotFound) {
				s.logger.Error("recover internal document create failed", "error", recoverErr, "internalDocumentId", document.ID)
				writeError(w, http.StatusInternalServerError, "internal_document_recovery_failed", "กู้คืน Draft ที่สร้างค้างไม่สำเร็จ กรุณาลองใหม่")
				return
			}
		}
	}
	data, pageCount, err := renderInternalDocumentPDF(document)
	if err != nil {
		_ = s.store.MarkInternalDocumentGenerationFailed(context.Background(), document.ID)
		s.logger.Error("render internal document PDF failed", "error", err, "internalDocumentId", document.ID)
		writeError(w, http.StatusInternalServerError, "internal_pdf_failed", "สร้าง PDF เอกสารภายในไม่สำเร็จ")
		return
	}
	uploaded, err := s.storeUploadedBytes(r.Context(), data, document.DocumentNo+".pdf", "internal-document.pdf", "application/pdf", ".pdf", pageCount, actor.ID)
	if err != nil {
		_ = s.store.MarkInternalDocumentGenerationFailed(context.Background(), document.ID)
		writeError(w, http.StatusInternalServerError, "internal_pdf_storage_failed", "บันทึก PDF เอกสารภายในไม่สำเร็จ")
		return
	}
	if err := s.store.CreateSigningDocumentUpload(r.Context(), uploaded.ID, actor.ID); err != nil {
		s.cleanupUploadedFileBestEffort(uploaded, "internal_upload_stage_failed")
		_ = s.store.MarkInternalDocumentGenerationFailed(context.Background(), document.ID)
		writeError(w, http.StatusInternalServerError, "internal_pdf_storage_failed", "เตรียม PDF เอกสารภายในไม่สำเร็จ")
		return
	}
	session, _ := currentSession(r)
	format := models.SMLDocFormat{Code: master.Code, Name1: master.Name, ScreenCode: internalDocumentScreenCode}
	candidate := models.SMLDocumentCandidate{DocNo: document.DocumentNo, DocDate: document.DocumentDate, TotalAmount: float64(totalCents) / 100, PartyName: document.RequesterName}
	signingDocument, err := s.store.CreateSigningDocument(r.Context(), store.CreateSigningDocumentInput{
		DocumentSource: "internal", InternalDocumentID: document.ID,
		ScreenCode: internalDocumentScreenCode, Format: format, Candidate: candidate,
		SMLDataGroup: session.SMLDataGroup, SMLDataCode: session.SMLDataCode,
		TemplateSnapshot: map[string]any{"source": "internal_draft_layout_required", "pageCount": pageCount},
		Configs:          configs, File: uploaded, AllowEmptyDraftLayout: true,
		ActorID: actor.ID, IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		if errors.Is(err, store.ErrSigningDocumentDuplicate) {
			if recovered, recoverErr := s.recoverInternalDocumentCreate(r.Context(), document, actor.ID); recoverErr == nil {
				s.cleanupUploadedFileBestEffort(uploaded, "internal_duplicate_recovered")
				writeJSON(w, http.StatusOK, map[string]any{"internalDocument": recovered, "signingDocumentId": recovered.SigningDocumentID, "idempotentReplay": true, "recovered": true})
				return
			}
		}
		s.cleanupUploadedFileBestEffort(uploaded, "internal_create_failed")
		_ = s.store.MarkInternalDocumentGenerationFailed(context.Background(), document.ID)
		s.writeInternalDocumentError(w, err)
		return
	}
	document, err = s.store.CompleteInternalDocumentCreate(r.Context(), document.ID, signingDocument.ID, uploaded, actor.ID)
	if err != nil {
		s.logger.Error("complete internal document create failed", "error", err, "internalDocumentId", document.ID)
		writeError(w, http.StatusInternalServerError, "internal_document_create_incomplete", "สร้าง Draft แล้วแต่บันทึกข้อมูลแบบฟอร์มไม่สมบูรณ์ กรุณาลองใหม่ด้วย request เดิม")
		return
	}
	_ = s.store.WriteAudit(r.Context(), actor.ID, "internal_document.create", "internal_document", document.ID, clientIP(r), r.UserAgent())
	writeJSON(w, http.StatusCreated, map[string]any{"internalDocument": document, "signingDocument": signingDocument})
}

func (s *Server) recoverInternalDocumentCreate(ctx context.Context, document models.InternalDocument, actorID string) (models.InternalDocument, error) {
	signingDocument, err := s.store.FindSigningDocumentByInternalDocumentID(ctx, document.ID)
	if err != nil {
		return models.InternalDocument{}, err
	}
	if signingDocument.OriginalFile == nil || signingDocument.OriginalFile.ID == "" {
		return models.InternalDocument{}, fmt.Errorf("existing signing document has no original file")
	}
	return s.store.CompleteInternalDocumentCreate(ctx, document.ID, signingDocument.ID, *signingDocument.OriginalFile, actorID)
}

func (s *Server) getInternalDocument(w http.ResponseWriter, r *http.Request) {
	if !s.internalDocumentsAvailable(w) {
		return
	}
	document, err := s.store.FindInternalDocumentByID(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.writeInternalDocumentError(w, err)
		return
	}
	actor, _ := currentUser(r)
	if document.Status == "draft" && document.CreatedBy != actor.ID {
		writeError(w, http.StatusNotFound, "internal_document_not_found", "Internal document was not found.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"internalDocument": document})
}

func (s *Server) updateInternalDocument(w http.ResponseWriter, r *http.Request) {
	if !s.internalDocumentsAvailable(w) {
		return
	}
	actor, _ := currentUser(r)
	var req models.InternalDocumentUpdateRequest
	if err := decodeLimitedJSON(w, r, 256<<10, &req); err != nil {
		return
	}
	current, err := s.store.FindInternalDocumentByID(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.writeInternalDocumentError(w, err)
		return
	}
	if current.CreatedBy != actor.ID || current.Status != "draft" {
		writeError(w, http.StatusNotFound, "internal_document_not_found", "Internal document was not found.")
		return
	}
	createReq := models.InternalDocumentCreateRequest{MasterID: current.MasterID, DocumentDate: current.DocumentDate, RequiredDate: req.RequiredDate, RequesterName: req.RequesterName, PositionName: req.PositionName, DepartmentName: req.DepartmentName, Purpose: req.Purpose, Items: req.Items}
	normalized, totalCents, issues := normalizeInternalDocumentCreateRequest(createReq)
	if req.Revision < 1 {
		issues = append(issues, "revision ต้องมากกว่า 0")
	}
	if len(issues) > 0 {
		writeValidationMessages(w, issues)
		return
	}
	candidate := current
	candidate.RequiredDate = normalized.RequiredDate
	candidate.RequesterName = normalized.RequesterName
	candidate.PositionName = normalized.PositionName
	candidate.DepartmentName = normalized.DepartmentName
	candidate.Purpose = normalized.Purpose
	candidate.Items = normalized.Items
	candidate.TotalAmount = centsToAmount(totalCents)
	candidate.Revision = req.Revision + 1
	data, pages, err := renderInternalDocumentPDF(candidate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_pdf_failed", "สร้าง PDF revision ใหม่ไม่สำเร็จ")
		return
	}
	original, err := s.storeUploadedBytes(r.Context(), data, current.DocumentNo+fmt.Sprintf("-r%d.pdf", candidate.Revision), "internal-document.pdf", "application/pdf", ".pdf", pages, actor.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_pdf_storage_failed", "บันทึก PDF revision ใหม่ไม่สำเร็จ")
		return
	}
	requiredDate, _ := time.Parse("2006-01-02", normalized.RequiredDate)
	document, err := s.store.UpdateInternalDocumentRevision(r.Context(), store.UpdateInternalDocumentRevisionInput{
		InternalID: current.ID, ExpectedRevision: req.Revision, RequiredDate: requiredDate,
		RequesterName: normalized.RequesterName, PositionName: normalized.PositionName,
		DepartmentName: normalized.DepartmentName, Purpose: normalized.Purpose,
		TotalAmount: centsToAmount(totalCents), Items: normalized.Items,
		OriginalFile: original, CurrentFile: original, ActorID: actor.ID,
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		s.cleanupUploadedFileBestEffort(original, "internal_revision_failed")
		s.writeInternalDocumentError(w, err)
		return
	}
	_ = s.store.WriteAudit(r.Context(), actor.ID, "internal_document.update", "internal_document", document.ID, clientIP(r), r.UserAgent())
	writeJSON(w, http.StatusOK, map[string]any{"internalDocument": document})
}

func (s *Server) printInternalDocument(w http.ResponseWriter, r *http.Request) {
	if !s.internalDocumentsAvailable(w) {
		return
	}
	actor, _ := currentUser(r)
	document, file, err := s.store.CreateInternalDocumentPrintEvent(r.Context(), strings.TrimSpace(r.PathValue("id")), actor.ID, clientIP(r), r.UserAgent(), actor.Role == "superadmin")
	if err != nil {
		if errors.Is(err, store.ErrSigningDocumentLayoutRequired) {
			writeError(w, http.StatusConflict, "internal_document_layout_required", "กรุณาจัดวางกรอบลายเซ็นและข้อความกฎหมายบน PDF ก่อนพิมพ์")
			return
		}
		s.writeInternalDocumentError(w, err)
		return
	}
	_ = s.store.WriteAudit(r.Context(), actor.ID, "internal_document.print", "internal_document", document.ID, clientIP(r), r.UserAgent())
	writeJSON(w, http.StatusOK, map[string]any{"internalDocument": document, "pdfUrl": fmt.Sprintf("/api/internal-documents/%s/pdf?v=%d", document.ID, document.Revision), "sha256": file.SHA256})
}

func (s *Server) getInternalDocumentPDF(w http.ResponseWriter, r *http.Request) {
	if !s.internalDocumentsAvailable(w) {
		return
	}
	document, err := s.store.FindInternalDocumentByID(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.writeInternalDocumentError(w, err)
		return
	}
	actor, _ := currentUser(r)
	if document.Status == "draft" && document.CreatedBy != actor.ID && actor.Role != "superadmin" {
		writeError(w, http.StatusNotFound, "internal_document_not_found", "Internal document was not found.")
		return
	}
	signingDocument, signingErr := s.store.FindSigningDocumentByInternalDocumentID(r.Context(), document.ID)
	if signingErr != nil || signingDocument.CurrentFile == nil {
		writeError(w, http.StatusNotFound, "internal_pdf_not_found", "PDF was not found.")
		return
	}
	file, err := s.store.FindUploadedFileByID(r.Context(), signingDocument.CurrentFile.ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "internal_pdf_not_found", "PDF was not found.")
		return
	}
	serveInlinePDF(w, r, file)
}

func (s *Server) internalWorkflowContext(ctx context.Context, code string) ([]models.DocumentConfigStep, error) {
	configs, err := s.store.ListDocumentConfigSteps(ctx, internalDocumentScreenCode, code)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, fmt.Errorf("กรุณาตั้งค่า Workflow ของเอกสารนี้ก่อน")
	}
	return configs, nil
}

func (s *Server) resolveConfigDocumentFormat(ctx context.Context, code string) (models.SMLDocFormat, error) {
	code = strings.TrimSpace(code)
	if s.cfg.InternalDocuments {
		master, err := s.store.FindInternalDocumentMasterByCode(ctx, code)
		if err == nil {
			return models.SMLDocFormat{Code: master.Code, Name1: master.Name, Format: master.RunningPattern, ScreenCode: internalDocumentScreenCode, Source: "internal"}, nil
		}
		if !errors.Is(err, store.ErrInternalMasterNotFound) {
			return models.SMLDocFormat{}, err
		}
	}
	format, err := s.fetchSMLDocFormatByCode(ctx, code)
	if err == nil {
		format.Source = "sml"
	}
	return format, err
}

func internalActiveTemplateLayout(active *models.SignatureTemplate, configs []models.DocumentConfigStep, pageCount int) ([]models.SignatureTemplateBoxRequest, []models.DocumentConfigStep, []models.SignaturePlacementSnapshot, []models.LegalNoticeBoxRequest, []models.LegalNoticeSnapshot, []models.SignatureValidationIssue) {
	boxes := boxRequestsFromTemplate(active.Boxes)
	for i := range boxes {
		boxes[i].PageNo = pageCount
	}
	layout, selected, placements, issues := validateSigningDocumentLayout(boxes, configs, pageCount)
	legal := legalNoticeBoxRequestFromTemplate(active.LegalNoticeBox)
	if legal == nil {
		issues = append(issues, signatureIssue("signature_template_legal_notice_required", "", "Active Template ต้องมีกรอบข้อความกฎหมาย"))
		return layout, selected, placements, nil, nil, issues
	}
	legal.PageNo = pageCount
	legalBoxes, legalIssues := normalizeAndValidateLegalNoticeBoxes([]models.LegalNoticeBoxRequest{*legal}, legal, pageCount, true)
	issues = append(issues, legalIssues...)
	snapshots := make([]models.LegalNoticeSnapshot, 0, len(legalBoxes))
	for _, box := range legalBoxes {
		snapshots = append(snapshots, legalNoticeSnapshotFromBox(box, "preset"))
	}
	return layout, selected, placements, legalBoxes, snapshots, issues
}

func (s *Server) ensureInternalMasterSample(ctx context.Context, master models.InternalDocumentMaster, actorID string) error {
	_, active, err := s.store.GetSignatureTemplateState(ctx, internalDocumentScreenCode, master.Code)
	if err != nil || active != nil {
		return err
	}
	doc := models.InternalDocument{MasterName: master.Name, DocumentNo: master.Prefix + "260101-001", DocumentDate: "2026-01-01", RequiredDate: "2026-01-15", RequesterName: "ตัวอย่างผู้ขอเบิก", PositionName: "ตำแหน่ง", DepartmentName: "ส่วนงาน/ฝ่าย/แผนก", Purpose: "ตัวอย่างวัตถุประสงค์ของเอกสาร", TotalAmount: "1000.00", CompanySnapshot: models.InternalDocumentCompanySnapshot{DisplayName: "ชื่อบริษัทจากระบบ SML", Address1: "ที่อยู่บริษัท", TelephoneNumber: "00-0000-0000", TaxNumber: "0000000000000"}, Items: []models.InternalDocumentItem{{SequenceNo: 1, Description: "ตัวอย่างรายการ", Amount: "1000.00"}}}
	data, pages, err := renderInternalDocumentPDF(doc)
	if err != nil {
		return err
	}
	file, err := s.storeUploadedBytes(ctx, data, "internal-"+master.Code+"-sample.pdf", "internal-sample.pdf", "application/pdf", ".pdf", pages, actorID)
	if err != nil {
		return err
	}
	_, err = s.store.UpsertActiveSignatureTemplateSample(ctx, internalDocumentScreenCode, master.Code, file.ID, actorID)
	return err
}

func (s *Server) ensureInternalMasterSamples(ctx context.Context, masters []models.InternalDocumentMaster, actorID string) {
	for _, master := range masters {
		if err := s.ensureInternalMasterSample(ctx, master, actorID); err != nil {
			s.logger.Warn("ensure internal master sample failed", "masterId", master.ID, "error", err)
		}
	}
}

func normalizeInternalMasterRequest(req models.InternalDocumentMasterRequest) (models.InternalDocumentMasterRequest, []string) {
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	req.Name = strings.TrimSpace(req.Name)
	req.Prefix = strings.ToUpper(strings.TrimSpace(req.Prefix))
	req.RunningPattern = strings.ToUpper(strings.TrimSpace(req.RunningPattern))
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	if req.Status == "" {
		req.Status = "inactive"
	}
	issues := []string{}
	if !internalMasterCodePattern.MatchString(req.Code) {
		issues = append(issues, "รหัส Master ต้องเป็น A-Z, 0-9, _ หรือ - และยาวไม่เกิน 20 ตัว")
	}
	if req.Name == "" || len([]rune(req.Name)) > 120 {
		issues = append(issues, "ชื่อเอกสารต้องมี 1-120 ตัวอักษร")
	}
	if !internalPrefixPattern.MatchString(req.Prefix) {
		issues = append(issues, "Prefix ต้องเป็น A-Z, 0-9, _ หรือ - และยาวไม่เกิน 20 ตัว")
	}
	if !internalRunningPattern.MatchString(req.RunningPattern) {
		issues = append(issues, "Running pattern ต้องใช้ YYYY, YY, MM, DD และ # หนึ่งชุด")
	}
	if req.Status != "active" && req.Status != "inactive" {
		issues = append(issues, "สถานะไม่ถูกต้อง")
	}
	if len(req.Prefix)+len(strings.TrimPrefix(req.RunningPattern, "@")) > 40 {
		issues = append(issues, "เลขที่เอกสารที่ได้ต้องยาวไม่เกิน 40 ตัวอักษร")
	}
	return req, issues
}

func normalizeInternalDocumentCreateRequest(req models.InternalDocumentCreateRequest) (models.InternalDocumentCreateRequest, int64, []string) {
	req.MasterID = strings.TrimSpace(req.MasterID)
	req.DocumentDate = strings.TrimSpace(req.DocumentDate)
	req.RequiredDate = strings.TrimSpace(req.RequiredDate)
	req.RequesterName = strings.TrimSpace(req.RequesterName)
	req.PositionName = strings.TrimSpace(req.PositionName)
	req.DepartmentName = strings.TrimSpace(req.DepartmentName)
	req.Purpose = strings.TrimSpace(req.Purpose)
	issues := []string{}
	if !isUUIDText(req.MasterID) {
		issues = append(issues, "กรุณาเลือก Master เอกสาร")
	}
	docDate, err := time.Parse("2006-01-02", req.DocumentDate)
	if err != nil {
		issues = append(issues, "วันที่เอกสารไม่ถูกต้อง")
	}
	required, err2 := time.Parse("2006-01-02", req.RequiredDate)
	if err2 != nil {
		issues = append(issues, "วันที่ต้องการใช้เงินไม่ถูกต้อง")
	}
	if err == nil && err2 == nil && required.Before(docDate) {
		issues = append(issues, "วันที่ต้องการใช้เงินต้องไม่ก่อนวันที่เอกสาร")
	}
	if req.RequesterName == "" || len([]rune(req.RequesterName)) > 160 {
		issues = append(issues, "กรุณากรอกชื่อผู้ขอเบิกไม่เกิน 160 ตัวอักษร")
	}
	if len([]rune(req.PositionName)) > 120 || len([]rune(req.DepartmentName)) > 160 {
		issues = append(issues, "ตำแหน่งหรือส่วนงานยาวเกินกำหนด")
	}
	if req.Purpose == "" || len([]rune(req.Purpose)) > 1000 {
		issues = append(issues, "กรุณากรอกวัตถุประสงค์ไม่เกิน 1,000 ตัวอักษร")
	}
	if len(req.Items) < 1 || len(req.Items) > 100 {
		issues = append(issues, "ต้องมีรายการ 1-100 รายการ")
	}
	total := int64(0)
	for i := range req.Items {
		req.Items[i].SequenceNo = i + 1
		req.Items[i].Description = strings.TrimSpace(req.Items[i].Description)
		req.Items[i].Amount = strings.TrimSpace(req.Items[i].Amount)
		if req.Items[i].Description == "" || len([]rune(req.Items[i].Description)) > 500 {
			issues = append(issues, fmt.Sprintf("รายการที่ %d ต้องมีรายละเอียดไม่เกิน 500 ตัวอักษร", i+1))
		}
		cents, e := store.ParseInternalAmount(req.Items[i].Amount)
		if e != nil || cents <= 0 {
			issues = append(issues, fmt.Sprintf("จำนวนเงินรายการที่ %d ต้องมากกว่า 0 และมีทศนิยมไม่เกิน 2 ตำแหน่ง", i+1))
		} else if total > 999999999999999999-cents {
			issues = append(issues, "ยอดรวมสูงเกินกำหนด")
		} else {
			total += cents
			req.Items[i].Amount = centsToAmount(cents)
		}
	}
	return req, total, issues
}

func centsToAmount(cents int64) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }

func internalCompanySnapshot(p models.SMLCompanyProfile) models.InternalDocumentCompanySnapshot {
	return models.InternalDocumentCompanySnapshot{DisplayName: p.DisplayName, CompanyName1: p.CompanyName1, BusinessName1: p.BusinessName1, Address1: p.Address1, Address2: p.Address2, TelephoneNumber: p.TelephoneNumber, FaxNumber: p.FaxNumber, TaxNumber: p.TaxNumber, BranchStatus: p.BranchStatus, BranchType: p.BranchType, BranchCode: p.BranchCode}
}

func (s *Server) validateInternalMasterActivation(ctx context.Context, id, code string) error {
	_, err := s.internalWorkflowContext(ctx, code)
	return err
}

func (s *Server) internalMasterCodeCollidesWithSML(ctx context.Context, code string) (bool, error) {
	_, err := s.fetchSMLDocFormatByCode(ctx, code)
	if err == nil || errors.Is(err, errDocFormatAmbiguous) {
		return true, nil
	}
	if errors.Is(err, errDocFormatNotFound) {
		return false, nil
	}
	return false, err
}

func writeValidationMessages(w http.ResponseWriter, issues []string) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": "validation_failed", "message": "กรุณาตรวจสอบข้อมูล", "issues": issues})
}

func writeInternalMasterError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInternalMasterNotFound):
		writeError(w, http.StatusNotFound, "internal_master_not_found", "ไม่พบ Master เอกสารภายใน")
	case errors.Is(err, store.ErrInternalMasterDuplicate):
		writeError(w, http.StatusConflict, "internal_master_duplicate", "รหัส Master นี้มีอยู่แล้ว")
	case errors.Is(err, store.ErrInternalMasterInUse):
		writeError(w, http.StatusConflict, "internal_master_in_use", "Master นี้ถูกใช้งานแล้ว ปิดใช้งานแทนการลบหรือเปลี่ยนรหัส")
	case errors.Is(err, store.ErrInternalMasterRevisionConflict):
		writeError(w, http.StatusConflict, "revision_conflict", "ข้อมูลถูกแก้ไขจากหน้าจออื่น กรุณาโหลดใหม่")
	default:
		writeError(w, http.StatusInternalServerError, "internal_master_failed", "Cannot save internal document master right now.")
	}
}

func (s *Server) writeInternalDocumentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInternalDocumentNotFound):
		writeError(w, http.StatusNotFound, "internal_document_not_found", "ไม่พบเอกสารภายใน")
	case errors.Is(err, store.ErrInternalDocumentInvalidStatus):
		writeError(w, http.StatusConflict, "internal_document_status_invalid", "สถานะเอกสารไม่อนุญาตให้ทำรายการนี้")
	case errors.Is(err, store.ErrInternalDocumentRevisionConflict):
		writeError(w, http.StatusConflict, "revision_conflict", "เอกสารถูกแก้ไขจากหน้าจออื่น กรุณาโหลดใหม่")
	case errors.Is(err, store.ErrSigningDocumentDuplicate):
		writeError(w, http.StatusConflict, "signing_document_duplicate", "เลขที่เอกสารนี้มีอยู่ใน PaperLess แล้ว")
	default:
		s.logger.Error("internal document operation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_document_failed", "Cannot process internal document right now.")
	}
}

func decodeLimitedJSON(w http.ResponseWriter, r *http.Request, limit int64, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return err
	}
	return nil
}
