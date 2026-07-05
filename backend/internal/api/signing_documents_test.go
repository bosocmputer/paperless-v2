package api

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/phpdave11/gofpdf"
)

func TestNormalizeSigningTaskEventMetadataKeepsOnlySafeFields(t *testing.T) {
	document := models.SigningDocument{DocFormatCode: "PO", DocNo: "PO26060001"}
	signer := models.SigningDocumentSigner{
		ID:            "signer-1",
		PositionCode:  "2",
		SignerName:    "201:นาย ก",
		SignerUser:    "201",
		SignerType:    "any",
		Status:        "pending",
		ConditionType: 1,
	}
	req := models.SigningTaskEventRequest{
		Event:         "sign_success",
		SessionID:     strings.Repeat("a", 120),
		ElapsedMS:     12_000,
		PDFPage:       1,
		PDFPageCount:  3,
		AttachmentCnt: 1,
		ErrorCode:     strings.Repeat("e", 120),
		Viewport:      models.SignatureDesignerViewport{Width: 390, Height: 844},
	}

	metadata, err := normalizeSigningTaskEventMetadata(req, document, signer)
	if err != nil {
		t.Fatalf("expected metadata, got %v", err)
	}
	if metadata["event"] != "sign_success" || metadata["docFormatCode"] != "PO" || metadata["positionCode"] != "2" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	if got := metadata["sessionId"].(string); len(got) != 80 {
		t.Fatalf("expected truncated session id, got %d", len(got))
	}
	if got := metadata["errorCode"].(string); len(got) != 80 {
		t.Fatalf("expected truncated error code, got %d", len(got))
	}
	if _, ok := metadata["signerName"]; ok {
		t.Fatal("metadata must not include signer name")
	}
	if _, ok := metadata["signerUser"]; ok {
		t.Fatal("metadata must not include signer user")
	}
	if _, ok := metadata["docNo"]; ok {
		t.Fatal("metadata must not include document number")
	}
}

func TestNormalizeSigningTaskEventMetadataRejectsInvalidEvent(t *testing.T) {
	_, err := normalizeSigningTaskEventMetadata(models.SigningTaskEventRequest{Event: "mousemove"}, models.SigningDocument{}, models.SigningDocumentSigner{})
	if err == nil {
		t.Fatal("expected invalid event error")
	}
}

func TestNormalizeSigningTaskEventMetadataAcceptsQueueEvents(t *testing.T) {
	for _, event := range []string{"ready_task_open", "waiting_queue_seen", "waiting_task_open", "history_open", "history_detail_open", "history_pdf_open", "related_documents_open", "related_documents_load_success", "related_documents_load_error", "related_document_click"} {
		t.Run(event, func(t *testing.T) {
			metadata, err := normalizeSigningTaskEventMetadata(models.SigningTaskEventRequest{Event: event}, models.SigningDocument{DocFormatCode: "PO"}, models.SigningDocumentSigner{Status: "waiting"})
			if err != nil {
				t.Fatalf("expected queue event to be accepted, got %v", err)
			}
			if metadata["event"] != event {
				t.Fatalf("unexpected event metadata %#v", metadata)
			}
		})
	}
}

func TestSanitizeRelatedDocumentsForSignerRemovesOpenIds(t *testing.T) {
	graph := models.SMLRelatedDocumentsGraph{
		Root: models.SMLRelatedDocumentNode{
			DocNo:               "PO26060001",
			DocFormatCode:       "PO",
			PaperlessDocumentID: "doc-1",
			PaperlessStatus:     "in_progress",
			CanOpenPaperless:    true,
			CanViewCurrentPDF:   true,
			CanViewSignedPDF:    true,
			CurrentPDFURL:       "/api/signing-documents/doc-1/pdf?version=current",
			SignedPDFURL:        "/api/signing-documents/doc-1/pdf?version=final",
			PaperlessMatches: []models.SigningDocumentReference{{
				ID:               "doc-1",
				CanOpenPaperless: true,
				CurrentPDFURL:    "/api/signing-documents/doc-1/pdf?version=current",
			}},
		},
		Nodes: []models.SMLRelatedDocumentNode{{
			DocNo:               "PA26060001",
			DocFormatCode:       "PA",
			PaperlessDocumentID: "doc-2",
			PaperlessStatus:     "completed",
			CanOpenPaperless:    true,
			CanViewCurrentPDF:   true,
			CanViewSignedPDF:    true,
			CurrentPDFURL:       "/api/signing-documents/doc-2/pdf?version=current",
			SignedPDFURL:        "/api/signing-documents/doc-2/pdf?version=final",
			PaperlessMatches: []models.SigningDocumentReference{{
				ID:               "doc-2",
				CanOpenPaperless: true,
				SignedPDFURL:     "/api/signing-documents/doc-2/pdf?version=final",
			}},
		}},
	}
	sanitized := sanitizeRelatedDocumentsForSigner(graph)
	if sanitized.Root.PaperlessDocumentID != "" || sanitized.Root.CanOpenPaperless {
		t.Fatalf("root should not expose open id: %#v", sanitized.Root)
	}
	if sanitized.Nodes[0].PaperlessDocumentID != "" || sanitized.Nodes[0].CanOpenPaperless {
		t.Fatalf("node should not expose open id: %#v", sanitized.Nodes[0])
	}
	if sanitized.Root.CurrentPDFURL != "" || sanitized.Root.SignedPDFURL != "" || sanitized.Root.CanViewCurrentPDF || sanitized.Root.CanViewSignedPDF || sanitized.Root.PaperlessMatches != nil {
		t.Fatalf("root should not expose pdf urls or matches: %#v", sanitized.Root)
	}
	if sanitized.Nodes[0].CurrentPDFURL != "" || sanitized.Nodes[0].SignedPDFURL != "" || sanitized.Nodes[0].CanViewCurrentPDF || sanitized.Nodes[0].CanViewSignedPDF || sanitized.Nodes[0].PaperlessMatches != nil {
		t.Fatalf("node should not expose pdf urls or matches: %#v", sanitized.Nodes[0])
	}
	if sanitized.Nodes[0].PaperlessStatus != "completed" {
		t.Fatal("status metadata should remain visible")
	}
}

func TestSigningDocumentPDFURLIncludesStableCacheKey(t *testing.T) {
	updatedAt := time.Date(2026, 7, 1, 11, 9, 34, 305444000, time.FixedZone("ICT", 7*60*60))
	got := signingDocumentPDFURL("doc-1", "current", updatedAt)
	if !strings.HasPrefix(got, "/api/signing-documents/doc-1/pdf?version=current&v=") {
		t.Fatalf("pdf url = %q, want cache-keyed current PDF URL", got)
	}

	withoutTimestamp := signingDocumentPDFURL("doc-1", "final", time.Time{})
	if withoutTimestamp != "/api/signing-documents/doc-1/pdf?version=final" {
		t.Fatalf("pdf url without timestamp = %q", withoutTimestamp)
	}
}

func TestCanRetrySigningDocumentImagesStatusIncludesCompletedRepair(t *testing.T) {
	for _, status := range []string{"completed_image_failed", "completed"} {
		if !canRetrySigningDocumentImagesStatus(status) {
			t.Fatalf("status %q should be retryable for SML image repair", status)
		}
	}
	for _, status := range []string{"pending_confirm", "completed_lock_failed", "completed_evidence_failed", "draft"} {
		if canRetrySigningDocumentImagesStatus(status) {
			t.Fatalf("status %q should not be retryable for SML image repair", status)
		}
	}
}

func TestSMLImagesHTTPErrorExplainsMissingImageDatabase(t *testing.T) {
	status, code, message := smlImagesHTTPError(map[string]any{
		"errorCode": "tenant_image_database_missing",
		"errorDetails": map[string]any{
			"imageDatabase": "stpt_images",
		},
	})

	if status != http.StatusFailedDependency {
		t.Fatalf("status = %d, want %d", status, http.StatusFailedDependency)
	}
	if code != "tenant_image_database_missing" {
		t.Fatalf("code = %q, want tenant_image_database_missing", code)
	}
	if !strings.Contains(message, "stpt_images") {
		t.Fatalf("message = %q, want image database name", message)
	}
}

func TestTenantReadinessLoginMessageExplainsMissingImageDatabase(t *testing.T) {
	message := tenantReadinessLoginMessage(models.SMLTenantReadiness{
		Status:        "image_db_missing",
		ImageDatabase: "silk_images",
	})
	if !strings.Contains(message, "silk_images") {
		t.Fatalf("message = %q, want image database name", message)
	}
	if strings.Contains(strings.ToLower(message), "failed") {
		t.Fatalf("message should be user-facing Thai copy, got %q", message)
	}
	if !strings.Contains(message, "ตั้งค่า image DB") {
		t.Fatalf("message = %q, want self-service action copy", message)
	}
}

func TestTenantReadinessCanSelfProvisionOnlyMissingImageDatabase(t *testing.T) {
	if !tenantReadinessCanSelfProvision(models.SMLTenantReadiness{Status: "image_db_missing", Tenant: "silk"}) {
		t.Fatal("image_db_missing should be self-provisionable")
	}
	if tenantReadinessCanSelfProvision(models.SMLTenantReadiness{Status: "main_db_missing", Tenant: "silk"}) {
		t.Fatal("main_db_missing should not be self-provisionable")
	}
	if !tenantReadinessCanSelfProvision(models.SMLTenantReadiness{Status: "doc_images_table_missing", Tenant: "silk"}) {
		t.Fatal("doc_images_table_missing should be self-provisionable")
	}
	if tenantReadinessCanSelfProvision(models.SMLTenantReadiness{Status: "schema_mismatch", Tenant: "silk"}) {
		t.Fatal("schema_mismatch should not be self-provisionable")
	}
	if tenantReadinessCanSelfProvision(models.SMLTenantReadiness{Status: "image_db_missing"}) {
		t.Fatal("missing tenant should not be self-provisionable")
	}
}

func TestSelectSigningHistoryPDFFileDefaultsToCurrent(t *testing.T) {
	current := &models.UploadedFile{ID: "current-file"}
	final := &models.UploadedFile{ID: "final-file"}
	document := models.SigningDocument{CurrentFile: current, FinalFile: final}

	if got := selectSigningHistoryPDFFile(document, ""); got != current {
		t.Fatalf("default history PDF = %#v, want current", got)
	}
	if got := selectSigningHistoryPDFFile(document, "current"); got != current {
		t.Fatalf("current history PDF = %#v, want current", got)
	}
	if got := selectSigningHistoryPDFFile(document, "final"); got != final {
		t.Fatalf("final history PDF = %#v, want final", got)
	}
	if got := selectSigningHistoryPDFFile(models.SigningDocument{CurrentFile: current}, "final"); got != nil {
		t.Fatalf("explicit final without final file = %#v, want nil", got)
	}
}

func TestNormalizeDocumentFlowEventMetadata(t *testing.T) {
	metadata, err := normalizeDocumentFlowEventMetadata(documentFlowEventRequest{
		Event:         "document_flow_load_success",
		SessionID:     strings.Repeat("s", 120),
		DocFormatCode: "po",
		ElapsedMS:     -1,
		NodeCount:     99,
		ErrorCode:     strings.Repeat("e", 120),
	})
	if err != nil {
		t.Fatalf("expected document flow event metadata, got %v", err)
	}
	if metadata["event"] != "document_flow_load_success" || metadata["docFormatCode"] != "PO" {
		t.Fatalf("unexpected metadata %#v", metadata)
	}
	if got := metadata["sessionId"].(string); len(got) != 80 {
		t.Fatalf("session id should be truncated, got %d", len(got))
	}
	if metadata["elapsedMs"] != int64(0) || metadata["nodeCount"] != 30 {
		t.Fatalf("metadata bounds were not clamped: %#v", metadata)
	}
	if _, ok := metadata["docNo"]; ok {
		t.Fatal("metadata must not include document number")
	}
}

func TestSMLLookupErrorViewHidesRawErrors(t *testing.T) {
	code, status, message := smlLookupErrorView(errors.New("Cannot load related documents from SML: no active document found for doc_no: BAD-NOT-FOUND"))
	if code != "sml_document_not_found" || status != 404 || message != "ไม่พบเลขเอกสารนี้ใน SML" {
		t.Fatalf("unexpected not found view: %s %d %s", code, status, message)
	}

	code, status, message = smlLookupErrorView(errors.New("dial tcp 192.168.2.109:8200: connection refused"))
	if code != "sml_unavailable" || status != 502 || strings.Contains(message, "connection refused") {
		t.Fatalf("raw SML error leaked: %s %d %s", code, status, message)
	}
}

func TestSanitizeSigningDocumentForSignerRemovesSensitiveEvidence(t *testing.T) {
	document := models.SigningDocument{
		OriginalFile: &models.UploadedFile{ID: "original", SHA256: "hash"},
		CurrentFile:  &models.UploadedFile{ID: "current", SHA256: "hash"},
		FinalFile:    &models.UploadedFile{ID: "final", SHA256: "hash"},
		Signers: []models.SigningDocumentSigner{{
			ID:              "signer-1",
			SignerUser:      "201",
			Status:          "waiting",
			SignatureFileID: "signature-file",
			DeviceID:        "device-secret",
			IPAddress:       "127.0.0.1",
			UserAgent:       "browser",
			ExternalTokenID: "token-id",
			ExternalURL:     "https://secret.example",
		}},
		Events: []models.SigningDocumentEvent{{
			ID:        "event-1",
			IPAddress: "127.0.0.1",
			UserAgent: "browser",
			Metadata:  map[string]any{"token": "secret"},
		}},
		Attachments: []models.SigningDocumentAttachment{{ID: "attachment-1"}},
		PrintEvents: []models.SigningDocumentPrintEvent{{ID: "print-1"}},
	}

	sanitized := sanitizeSigningDocumentForSigner(document)
	if sanitized.OriginalFile != nil || sanitized.CurrentFile != nil || sanitized.FinalFile != nil {
		t.Fatal("file metadata must be removed from signer document payload")
	}
	if len(sanitized.Attachments) != 0 || len(sanitized.PrintEvents) != 0 {
		t.Fatal("attachments and print events must be removed from signer document payload")
	}
	signer := sanitized.Signers[0]
	if signer.SignatureFileID != "" || signer.DeviceID != "" || signer.IPAddress != "" || signer.UserAgent != "" || signer.ExternalTokenID != "" || signer.ExternalURL != "" {
		t.Fatalf("signer sensitive fields were not cleared: %#v", signer)
	}
	if sanitized.Events[0].IPAddress != "" || sanitized.Events[0].UserAgent != "" || sanitized.Events[0].Metadata != nil {
		t.Fatalf("event sensitive fields were not cleared: %#v", sanitized.Events[0])
	}
}

func TestSanitizeSigningDocumentForExternalKeepsOnlySignableDocumentContext(t *testing.T) {
	document := models.SigningDocument{
		DocNo:         "PO26060001",
		DocFormatCode: "PO",
		OriginalFile:  &models.UploadedFile{ID: "original", SHA256: "hash"},
		CurrentFile:   &models.UploadedFile{ID: "current", SHA256: "hash"},
		FinalFile:     &models.UploadedFile{ID: "final", SHA256: "hash"},
		Signers: []models.SigningDocumentSigner{{
			ID:              "signer-1",
			SignatureFileID: "signature-file",
			DeviceID:        "device-secret",
			IPAddress:       "127.0.0.1",
			UserAgent:       "browser",
			ExternalTokenID: "token-id",
			ExternalURL:     "https://secret.example",
		}},
		Events:      []models.SigningDocumentEvent{{ID: "event-1", IPAddress: "127.0.0.1", UserAgent: "browser", Metadata: map[string]any{"token": "secret"}}},
		Attachments: []models.SigningDocumentAttachment{{ID: "attachment-1"}},
		PrintEvents: []models.SigningDocumentPrintEvent{{ID: "print-1"}},
	}

	sanitized := sanitizeSigningDocumentForExternal(document)
	if sanitized.DocNo != "PO26060001" || sanitized.DocFormatCode != "PO" {
		t.Fatalf("document context should remain: %#v", sanitized)
	}
	if sanitized.OriginalFile != nil || sanitized.CurrentFile != nil || sanitized.FinalFile != nil {
		t.Fatal("external document payload must not expose file metadata")
	}
	if sanitized.Signers != nil || sanitized.Events != nil {
		t.Fatalf("external document payload must not expose workflow signers/events: %#v", sanitized)
	}
	if len(sanitized.Attachments) != 0 || len(sanitized.PrintEvents) != 0 {
		t.Fatal("external document payload must not expose attachments or print events")
	}
}

func TestTaskUnavailableCodeMapsExternalTerminalStates(t *testing.T) {
	tests := map[string]string{
		"signed":   "already_signed",
		"rejected": "already_rejected",
		"waiting":  "signing_task_not_turn",
		"skipped":  "signing_task_skipped",
		"pending":  "signing_task_unavailable",
	}
	for status, want := range tests {
		if got := taskUnavailableCode(status); got != want {
			t.Fatalf("taskUnavailableCode(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestCurrentPDFNeedsSignatureTransparencyRefresh(t *testing.T) {
	signedAt := time.Now()
	document := models.SigningDocument{
		Signers: []models.SigningDocumentSigner{{
			ID:              "signer-1",
			Status:          "signed",
			SignatureFileID: "signature-file",
			SignedAt:        &signedAt,
		}},
	}
	if !currentPDFNeedsSignatureTransparencyRefresh(document) {
		t.Fatal("signed document without transparency metadata should refresh")
	}
	document.Events = []models.SigningDocumentEvent{{
		Action: "pdf_stamped",
		Metadata: map[string]any{
			"signatureTransparencyVersion": signatureTransparencyVersion,
		},
	}}
	if currentPDFNeedsSignatureTransparencyRefresh(document) {
		t.Fatal("document with current transparency metadata should not refresh")
	}
	document.Signers = nil
	if currentPDFNeedsSignatureTransparencyRefresh(document) {
		t.Fatal("document without signed signatures should not refresh")
	}
}

func TestNormalizeSigningCreateEventMetadata(t *testing.T) {
	metadata, err := normalizeSigningCreateEventMetadata(signingCreateEventRequest{
		Event:                "create_submit_success",
		SessionID:            strings.Repeat("s", 120),
		DocFormatCode:        "PO",
		ElapsedMS:            15_000,
		BoxCount:             3,
		ValidationIssueCount: 0,
		Viewport:             models.SignatureDesignerViewport{Width: 390, Height: 844},
	})
	if err != nil {
		t.Fatalf("expected metadata, got %v", err)
	}
	if metadata["event"] != "create_submit_success" || metadata["docFormatCode"] != "PO" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	if got := metadata["sessionId"].(string); len(got) != 80 {
		t.Fatalf("expected truncated session id, got %d", len(got))
	}
	if _, ok := metadata["docNo"]; ok {
		t.Fatal("metadata must not include document number")
	}
}

func TestNormalizeSigningCreateEventMetadataRejectsInvalidEvent(t *testing.T) {
	_, err := normalizeSigningCreateEventMetadata(signingCreateEventRequest{Event: "mousemove"})
	if err == nil {
		t.Fatal("expected invalid event error")
	}
}

func TestDecodeSigningTaskEventPayloadRejectsUnknownField(t *testing.T) {
	body := `{"event":"task_open","token":"secret"}`
	if _, err := decodeSigningTaskEventPayload(strings.NewReader(body), maxSigningEventBytes); err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

func TestDetectSigningAttachmentType(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		contentType string
		ext         string
		wantErr     bool
	}{
		{name: "png", data: []byte("\x89PNG\r\n\x1a\nextra"), contentType: "image/png", ext: ".png"},
		{name: "jpeg", data: []byte{0xff, 0xd8, 0xff, 0x00}, contentType: "image/jpeg", ext: ".jpg"},
		{name: "invalid", data: []byte("hello"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentType, ext, _, err := detectSigningAttachmentType(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected valid attachment, got %v", err)
			}
			if contentType != tt.contentType || ext != tt.ext {
				t.Fatalf("unexpected type %s %s", contentType, ext)
			}
		})
	}
}

func TestDetectSigningAttachmentTypePDF(t *testing.T) {
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.AddPage()
	var buffer bytes.Buffer
	if err := pdf.Output(&buffer); err != nil {
		t.Fatalf("create pdf: %v", err)
	}
	contentType, ext, pageCount, err := detectSigningAttachmentType(buffer.Bytes())
	if err != nil {
		t.Fatalf("expected PDF attachment, got %v", err)
	}
	if contentType != "application/pdf" || ext != ".pdf" || pageCount != 1 {
		t.Fatalf("unexpected PDF metadata %s %s %d", contentType, ext, pageCount)
	}
}

func TestNormalizePrintCopyRequestWebDefaultsPrinterName(t *testing.T) {
	req := normalizePrintCopyRequest(models.CreatePrintCopyRequest{
		Channel:        "WEB",
		DeviceID:       strings.Repeat("d", 200),
		ClientTimezone: "Asia/Bangkok",
	})
	if req.Channel != "web" {
		t.Fatalf("expected web channel, got %q", req.Channel)
	}
	if req.PrinterName != "not_available_web_browser" {
		t.Fatalf("expected web printer fallback, got %q", req.PrinterName)
	}
	if len(req.DeviceID) != 160 {
		t.Fatalf("expected bounded device id, got %d", len(req.DeviceID))
	}
}

func TestValidateSigningDocumentLayoutAllowsPartialPositions(t *testing.T) {
	configs := []models.DocumentConfigStep{
		{PositionCode: "1", PositionName: "ผู้จัดทำ", User01: "001:น.ส X", ConditionType: 1, SequenceNo: 1},
		{PositionCode: "2", PositionName: "ผู้ตรวจสอบ", User01: "201:นาย ก", ConditionType: 1, SequenceNo: 2},
		{PositionCode: "3", PositionName: "ผู้อนุมัติ", User01: "901:นาย A", User02: "902:นาย B", ConditionType: 2, SequenceNo: 3},
	}
	boxes := []models.SignatureTemplateBoxRequest{
		{PositionCode: "1", SignerType: "any", PageNo: 1, XRatio: 0.1, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		{PositionCode: "3", SignerType: "internal", SignerUser: "901", PageNo: 1, XRatio: 0.55, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		{PositionCode: "3", SignerType: "internal", SignerUser: "902", PageNo: 1, XRatio: 0.75, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
	}

	normalized, selected, placements, issues := validateSigningDocumentLayout(boxes, configs, 1)
	if len(issues) != 0 {
		t.Fatalf("expected partial layout to be valid, got %#v", issues)
	}
	if len(selected) != 2 {
		t.Fatalf("expected only boxed positions to be selected, got %d", len(selected))
	}
	if len(normalized) != 3 {
		t.Fatalf("expected three task boxes, got %d", len(normalized))
	}
	if len(placements) != 3 {
		t.Fatalf("expected three placements, got %d", len(placements))
	}
	if normalized[1].SignerUser != "901:นาย A" {
		t.Fatalf("expected signer user to normalize to configured label, got %q", normalized[1].SignerUser)
	}
}

func TestValidateSigningDocumentLayoutAllowsMultiplePlacementsWithoutDuplicateTasks(t *testing.T) {
	configs := []models.DocumentConfigStep{
		{PositionCode: "1", PositionName: "ผู้จัดทำ", User01: "001:น.ส X", ConditionType: 1, SequenceNo: 1},
		{PositionCode: "2", PositionName: "ผู้อนุมัติ", User01: "901:นาย A", User02: "902:นาย B", ConditionType: 2, SequenceNo: 2},
	}
	boxes := []models.SignatureTemplateBoxRequest{
		{PositionCode: "1", SignerType: "any", PageNo: 1, XRatio: 0.1, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		{PositionCode: "1", SignerType: "any", PageNo: 2, XRatio: 0.1, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		{PositionCode: "2", SignerType: "internal", SignerUser: "901", PageNo: 1, XRatio: 0.45, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		{PositionCode: "2", SignerType: "internal", SignerUser: "901", PageNo: 2, XRatio: 0.45, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		{PositionCode: "2", SignerType: "internal", SignerUser: "902", PageNo: 1, XRatio: 0.7, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		{PositionCode: "2", SignerType: "internal", SignerUser: "902", PageNo: 2, XRatio: 0.7, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
	}

	taskBoxes, selected, placements, issues := validateSigningDocumentLayout(boxes, configs, 2)
	if len(issues) != 0 {
		t.Fatalf("expected multi-placement layout to be valid, got %#v", issues)
	}
	if len(selected) != 2 {
		t.Fatalf("expected two selected workflow steps, got %d", len(selected))
	}
	if len(taskBoxes) != 3 {
		t.Fatalf("expected one any task and two all-user tasks, got %d", len(taskBoxes))
	}
	if len(placements) != 6 {
		t.Fatalf("expected all six stamp placements, got %d", len(placements))
	}
}

func TestValidateSigningDocumentLayoutRejectsZeroBoxes(t *testing.T) {
	_, _, _, issues := validateSigningDocumentLayout(nil, []models.DocumentConfigStep{
		{PositionCode: "1", PositionName: "ผู้จัดทำ", User01: "001:น.ส X", ConditionType: 1},
	}, 1)
	if !hasSignatureIssue(issues, "layout_box_required") {
		t.Fatalf("expected layout_box_required, got %#v", issues)
	}
}

func TestValidateLegalNoticeBoxRequiredAndBounds(t *testing.T) {
	_, issues := normalizeAndValidateLegalNoticeBox(nil, 1, true)
	if !hasSignatureIssue(issues, "legal_notice_box_required") {
		t.Fatalf("expected legal_notice_box_required, got %#v", issues)
	}

	box, issues := normalizeAndValidateLegalNoticeBox(&models.LegalNoticeBoxRequest{
		PageNo:      1,
		XRatio:      0.2,
		YRatio:      0.62,
		WidthRatio:  0.6,
		HeightRatio: 0.08,
	}, 1, true)
	if len(issues) != 0 {
		t.Fatalf("expected valid legal notice box, got %#v", issues)
	}
	if box == nil || box.Label != "ข้อความกฎหมาย" {
		t.Fatalf("expected default legal notice label, got %#v", box)
	}

	_, issues = normalizeAndValidateLegalNoticeBox(&models.LegalNoticeBoxRequest{
		PageNo:      2,
		XRatio:      0.9,
		YRatio:      0.9,
		WidthRatio:  0.2,
		HeightRatio: 0.2,
	}, 1, true)
	if !hasSignatureIssue(issues, "legal_notice_page_invalid") || !hasSignatureIssue(issues, "legal_notice_bounds_invalid") {
		t.Fatalf("expected page and bounds issues, got %#v", issues)
	}
}

func TestValidateLegalNoticeBoxesAllowsMultiplePages(t *testing.T) {
	boxes, issues := normalizeAndValidateLegalNoticeBoxes([]models.LegalNoticeBoxRequest{
		{PageNo: 1, XRatio: 0.2, YRatio: 0.62, WidthRatio: 0.6, HeightRatio: 0.08},
		{PageNo: 2, XRatio: 0.2, YRatio: 0.62, WidthRatio: 0.6, HeightRatio: 0.08},
	}, nil, 2, true)
	if len(issues) != 0 {
		t.Fatalf("expected multiple legal notice boxes to be valid, got %#v", issues)
	}
	if len(boxes) != 2 {
		t.Fatalf("expected two legal notice boxes, got %d", len(boxes))
	}
}

func TestCurrentPDFNeedsLegalNoticeRefresh(t *testing.T) {
	doc := models.SigningDocument{
		OriginalFileID:      "file-original",
		CurrentFileID:       "file-original",
		LegalNoticeSnapshot: &models.LegalNoticeSnapshot{Text: signingLegalText, PageNo: 1, WidthRatio: 0.6, HeightRatio: 0.08},
	}
	if !currentPDFNeedsLegalNoticeRefresh(doc) {
		t.Fatalf("expected original current PDF to need legal notice refresh")
	}

	doc.CurrentFileID = "file-current"
	doc.Events = []models.SigningDocumentEvent{{
		Action:   "pdf_stamped",
		Metadata: map[string]any{"legalNoticeStamped": true, "legalNoticeDisplayVersion": signingLegalNoticePDFDisplayVersion},
	}}
	if currentPDFNeedsLegalNoticeRefresh(doc) {
		t.Fatalf("expected legal stamped current PDF to skip refresh")
	}

	doc.Events = []models.SigningDocumentEvent{{
		Action:   "pdf_stamped",
		Metadata: map[string]any{"legalNoticeStamped": false},
	}}
	if !currentPDFNeedsLegalNoticeRefresh(doc) {
		t.Fatalf("expected old current PDF without legal notice marker to need refresh")
	}

	doc.Events = []models.SigningDocumentEvent{{
		Action:   "pdf_stamped",
		Metadata: map[string]any{"legalNoticeStamped": true},
	}}
	if !currentPDFNeedsLegalNoticeRefresh(doc) {
		t.Fatalf("expected old current PDF without display version to need refresh")
	}
}

func TestValidateSigningDocumentLayoutRejectsMissingConditionTwoUser(t *testing.T) {
	configs := []models.DocumentConfigStep{
		{PositionCode: "3", PositionName: "ผู้อนุมัติ", User01: "901:นาย A", User02: "902:นาย B", ConditionType: 2},
	}
	boxes := []models.SignatureTemplateBoxRequest{
		{PositionCode: "3", SignerType: "internal", SignerUser: "901", PageNo: 1, XRatio: 0.1, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		{PositionCode: "3", SignerType: "internal", SignerUser: "901:นาย A", PageNo: 1, XRatio: 0.4, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
	}

	_, _, _, issues := validateSigningDocumentLayout(boxes, configs, 1)
	if !hasSignatureIssue(issues, "condition_all_missing_user_box") {
		t.Fatalf("expected missing user issue, got %#v", issues)
	}
}
