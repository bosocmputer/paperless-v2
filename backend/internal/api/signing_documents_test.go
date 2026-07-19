package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
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

func TestDocumentNumberFromPDFName(t *testing.T) {
	got, err := documentNumberFromPDFName(" qt26070001.PDF ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "QT26070001" {
		t.Fatalf("doc no = %q, want QT26070001", got)
	}
}

func TestDocumentNumberFromPDFNameRejectsInvalidNames(t *testing.T) {
	tests := []string{
		"document.txt",
		".pdf",
		"../QT26070001.pdf",
		`QT\\26070001.pdf`,
		strings.Repeat("A", 26) + ".pdf",
		"QT2607\n0001.pdf",
	}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := documentNumberFromPDFName(name); err == nil {
				t.Fatal("expected invalid filename")
			}
		})
	}
}

func TestSigningDocumentBatchContextVersionTracksWorkflowAndTemplate(t *testing.T) {
	configs := []models.DocumentConfigStep{{ID: "step-1", PositionCode: "1", SequenceNo: 1, UpdatedAt: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}}
	template := models.SignatureTemplate{ID: "template-1", Version: 1, Revision: 2, UpdatedAt: time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)}
	base := signingDocumentBatchContextVersion(configs, template)

	changedConfigs := append([]models.DocumentConfigStep(nil), configs...)
	changedConfigs[0].UpdatedAt = changedConfigs[0].UpdatedAt.Add(time.Second)
	if got := signingDocumentBatchContextVersion(changedConfigs, template); got == base {
		t.Fatal("workflow update must change context version")
	}
	template.Revision++
	if got := signingDocumentBatchContextVersion(configs, template); got == base {
		t.Fatal("template revision must change context version")
	}
}

func TestApplyBatchPageLimitMarksEveryItemOnlyWhenTotalExceedsLimit(t *testing.T) {
	items := []signingDocumentBatchValidationItem{{FileID: "one"}, {FileID: "two"}}
	applyBatchPageLimit(items, 101)
	for _, item := range items {
		if !hasBatchIssue(item.Issues, "batch_page_limit") {
			t.Fatalf("item %s was not marked with batch_page_limit", item.FileID)
		}
	}

	withinLimit := []signingDocumentBatchValidationItem{{FileID: "one"}}
	applyBatchPageLimit(withinLimit, 100)
	if len(withinLimit[0].Issues) != 0 {
		t.Fatalf("100 pages must remain valid: %#v", withinLimit[0].Issues)
	}
}

func TestNormalizeBatchFileIDsRejectsDuplicatesAndInvalidValues(t *testing.T) {
	valid := "4ad1a25f-7d82-4d37-8895-30bd2aa73453"
	if _, err := normalizeBatchFileIDs([]string{valid, valid}); err == nil {
		t.Fatal("duplicate file ids must be rejected")
	}
	if _, err := normalizeBatchFileIDs([]string{"not-a-uuid"}); err == nil {
		t.Fatal("invalid file ids must be rejected")
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

func TestClassifyPaperlessReferenceStatus(t *testing.T) {
	completedUpdatedAt := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		refs       []models.SigningDocumentReference
		wantStatus string
		wantID     string
	}{
		{name: "no match", refs: nil, wantStatus: "missing"},
		{
			name:       "document without current pdf is missing",
			refs:       []models.SigningDocumentReference{{ID: "doc-1", Status: "completed", HasCurrentPDF: false}},
			wantStatus: "missing",
			wantID:     "doc-1",
		},
		{
			name:       "active document with current pdf is in progress",
			refs:       []models.SigningDocumentReference{{ID: "doc-2", Status: "pending_confirm", HasCurrentPDF: true}},
			wantStatus: "in_progress",
			wantID:     "doc-2",
		},
		{
			name: "completed document wins over active",
			refs: []models.SigningDocumentReference{
				{ID: "doc-active", Status: "in_progress", HasCurrentPDF: true},
				{ID: "doc-completed", Status: "completed", HasCurrentPDF: true, UpdatedAt: completedUpdatedAt},
			},
			wantStatus: "completed",
			wantID:     "doc-completed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotRef := classifyPaperlessReferenceStatus(tc.refs)
			if gotStatus != tc.wantStatus {
				t.Fatalf("status = %q, want %q", gotStatus, tc.wantStatus)
			}
			if tc.wantID == "" {
				if gotRef != nil {
					t.Fatalf("selected = %#v, want nil", gotRef)
				}
				return
			}
			if gotRef == nil || gotRef.ID != tc.wantID {
				t.Fatalf("selected = %#v, want id %q", gotRef, tc.wantID)
			}
		})
	}
}

func TestSigningReferenceStatusFromSummary(t *testing.T) {
	tests := []struct {
		name    string
		summary models.SMLDocumentReferenceSummary
		want    string
	}{
		{
			name:    "no references",
			summary: models.SMLDocumentReferenceSummary{},
			want:    "none",
		},
		{
			name:    "all completed",
			summary: models.SMLDocumentReferenceSummary{Total: 2, Completed: 2},
			want:    "completed",
		},
		{
			name:    "missing reference",
			summary: models.SMLDocumentReferenceSummary{Total: 2, Completed: 1, Missing: 1},
			want:    "incomplete",
		},
		{
			name:    "in progress reference",
			summary: models.SMLDocumentReferenceSummary{Total: 2, Completed: 1, InProgress: 1},
			want:    "incomplete",
		},
		{
			name:    "mismatched counts fail safe",
			summary: models.SMLDocumentReferenceSummary{Total: 2, Completed: 1},
			want:    "incomplete",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := signingReferenceStatusFromSummary(tc.summary); got != tc.want {
				t.Fatalf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSigningReferenceStatusResponseDoesNotExposeDocumentDetails(t *testing.T) {
	payload := signingReferenceStatusResponse{
		Status:  "incomplete",
		Summary: models.SMLDocumentReferenceSummary{Total: 2, Missing: 1, Completed: 1},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal reference status response: %v", err)
	}
	for _, forbidden := range []string{"docNo", "paperlessDocumentId", "currentPdfUrl", "signedPdfUrl", "paperlessMatches"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("reference status response leaked %q: %s", forbidden, body)
		}
	}
}

func TestSanitizeDocumentReferenceCheckForSignerScrubsOpenFields(t *testing.T) {
	result := models.SMLDocumentReferences{
		Document: models.SMLRelatedDocumentNode{
			PaperlessDocumentID: "root-doc",
			CanOpenPaperless:    true,
			HasCurrentPDF:       true,
			HasFinalPDF:         true,
			CanViewCurrentPDF:   true,
			CanViewSignedPDF:    true,
			CurrentPDFURL:       "/api/signing-documents/root-doc/pdf",
			SignedPDFURL:        "/api/signing-documents/root-doc/pdf?version=final",
			MatchCount:          1,
			PaperlessMatches: []models.SigningDocumentReference{{
				ID: "root-doc",
			}},
		},
		Items: []models.SMLDocumentReferenceItem{{
			DocNo:               "PO26060001",
			PaperlessDocumentID: "paperless-doc",
			PaperlessStatus:     "completed",
			CanOpenPaperless:    true,
			HasCurrentPDF:       true,
			HasFinalPDF:         true,
			CanViewCurrentPDF:   true,
			CanViewSignedPDF:    true,
			CurrentPDFURL:       "/api/signing-documents/paperless-doc/pdf",
			SignedPDFURL:        "/api/signing-documents/paperless-doc/pdf?version=final",
			MatchCount:          1,
			PaperlessMatches: []models.SigningDocumentReference{{
				ID: "paperless-doc",
			}},
		}},
		Summary: models.SMLDocumentReferenceSummary{Total: 1, Completed: 1},
	}

	got := sanitizeDocumentReferenceCheckForSigner(result)
	if got.Document.PaperlessDocumentID != "" || got.Document.CanOpenPaperless || got.Document.CurrentPDFURL != "" || len(got.Document.PaperlessMatches) != 0 {
		t.Fatalf("root open fields leaked: %#v", got.Document)
	}
	item := got.Items[0]
	if item.PaperlessStatus != "completed" {
		t.Fatalf("status should remain visible, got %q", item.PaperlessStatus)
	}
	if item.PaperlessDocumentID != "" || item.CanOpenPaperless || item.CurrentPDFURL != "" || item.SignedPDFURL != "" || item.MatchCount != 0 || len(item.PaperlessMatches) != 0 {
		t.Fatalf("item open fields leaked: %#v", item)
	}
	if got.Summary.Completed != 1 {
		t.Fatalf("summary should remain visible: %#v", got.Summary)
	}
}

func TestPrepareReferenceMatchForAdminScrubsOtherUserDraft(t *testing.T) {
	ref := prepareReferenceMatchForAdmin(models.SigningDocumentReference{
		ID:            "draft-doc",
		Status:        "draft",
		CreatedBy:     "other-user",
		HasCurrentPDF: true,
		HasFinalPDF:   true,
		UpdatedAt:     time.Now(),
	}, true, "actor-user")

	if ref.ID != "" || ref.CanOpenPaperless || ref.CanViewCurrentPDF || ref.CurrentPDFURL != "" {
		t.Fatalf("other user draft should not expose open fields: %#v", ref)
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

func TestPrepareSigningDocumentDuplicateResponseScrubsOtherUserDraft(t *testing.T) {
	result := store.SigningDocumentDuplicateCheckResult{
		CanCreate: false,
		Message:   "เอกสารนี้มีอยู่ใน PaperLess แล้ว",
		BlockingDocument: &models.SigningDocumentReference{
			ID:            "secret-doc-id",
			DocNo:         "PO26060001",
			DocFormatCode: "PO",
			Status:        "draft",
			CreatedBy:     "other-user",
			HasCurrentPDF: true,
			HasFinalPDF:   true,
			UpdatedAt:     time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC),
		},
	}

	got := prepareSigningDocumentDuplicateResponse(result, "actor-user")
	if got.BlockingDocument == nil {
		t.Fatal("expected blocking document")
	}
	if got.BlockingDocument.ID != "" || got.BlockingDocument.CanOpenPaperless || got.BlockingDocument.CurrentPDFURL != "" || got.BlockingDocument.SignedPDFURL != "" {
		t.Fatalf("other user draft should not expose open fields: %#v", got.BlockingDocument)
	}
	if !strings.Contains(got.Message, "ผู้สร้างเอกสาร") {
		t.Fatalf("message = %q, want owner-safe guidance", got.Message)
	}
}

func TestPrepareSigningDocumentDuplicateResponseKeepsOwnDraftOpenable(t *testing.T) {
	updatedAt := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	result := store.SigningDocumentDuplicateCheckResult{
		CanCreate: false,
		BlockingDocument: &models.SigningDocumentReference{
			ID:            "own-doc-id",
			DocNo:         "PO26060001",
			DocFormatCode: "PO",
			Status:        "draft",
			CreatedBy:     "actor-user",
			HasCurrentPDF: true,
			UpdatedAt:     updatedAt,
		},
	}

	got := prepareSigningDocumentDuplicateResponse(result, "actor-user")
	if got.BlockingDocument == nil || !got.BlockingDocument.CanOpenPaperless || got.BlockingDocument.CurrentPDFURL == "" {
		t.Fatalf("own draft should remain openable: %#v", got.BlockingDocument)
	}
}

func TestCanAccessSigningDocumentAsAdminRestrictsDraftOwner(t *testing.T) {
	actor := models.User{ID: "actor-user", Role: "admin"}
	if !canAccessSigningDocumentAsAdmin(models.SigningDocument{Status: "draft", CreatedBy: "actor-user"}, actor) {
		t.Fatal("owner should access own draft")
	}
	if canAccessSigningDocumentAsAdmin(models.SigningDocument{Status: "draft", CreatedBy: "other-user"}, actor) {
		t.Fatal("admin should not access another user's draft")
	}
	if !canAccessSigningDocumentAsAdmin(models.SigningDocument{Status: "in_progress", CreatedBy: "other-user"}, actor) {
		t.Fatal("active document should keep admin tenant-wide visibility")
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
	if !strings.Contains(message, "ผู้ดูแลระบบ SML") {
		t.Fatalf("message = %q, want SML administrator guidance", message)
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

func TestSanitizeSigningAttachmentForUserKeepsOnlySafeMetadata(t *testing.T) {
	createdAt := time.Date(2026, 7, 7, 13, 54, 0, 0, time.UTC)
	attachment := models.SigningDocumentAttachment{
		ID:        "attachment-1",
		SignerID:  "signer-1",
		Note:      "ใบเสนอราคา",
		CreatedAt: createdAt,
		File: models.UploadedFile{
			ID:           "file-1",
			OriginalName: "ใบเสนอราคาสาขา.pdf",
			StoredName:   "stored.pdf",
			StoragePath:  "/app/uploads/secret.pdf",
			ContentType:  "application/pdf",
			SizeBytes:    123938,
			PageCount:    1,
			SHA256:       "secret-hash",
			CreatedBy:    "user-1",
			CreatedAt:    createdAt,
		},
	}

	sanitized := sanitizeSigningAttachmentForUser(attachment, []models.SigningDocumentSigner{
		{ID: "signer-1", SignerName: "นาย B", PositionName: "ผู้จัดทำ"},
	})
	payload, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatalf("marshal sanitized attachment: %v", err)
	}
	text := string(payload)
	for _, secret := range []string{"StoragePath", "storagePath", "/app/uploads", "secret-hash", "stored.pdf", "createdBy"} {
		if strings.Contains(text, secret) {
			t.Fatalf("sanitized attachment leaked %q: %s", secret, text)
		}
	}
	if sanitized.ID != "attachment-1" || sanitized.File.OriginalName != "ใบเสนอราคาสาขา.pdf" || sanitized.File.ContentType != "application/pdf" || sanitized.File.SizeBytes != 123938 {
		t.Fatalf("safe attachment metadata missing: %#v", sanitized)
	}
	if sanitized.SignerID != "signer-1" || sanitized.SignerName != "นาย B" || sanitized.PositionName != "ผู้จัดทำ" {
		t.Fatalf("signer attachment metadata missing: %#v", sanitized)
	}
}

func TestDocumentHasInternalSignerSkipsExternalSigners(t *testing.T) {
	signers := []models.SigningDocumentSigner{
		{SignerType: "external", SignerUser: "customer"},
		{SignerType: "internal", SignerUser: "902"},
	}
	if !documentHasInternalSigner(signers, "902") {
		t.Fatal("expected internal signer to have access")
	}
	if documentHasInternalSigner(signers, "customer") {
		t.Fatal("external signer must not receive internal attachment access")
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

func TestNormalizeRuntimeSignNoteBoxesValidation(t *testing.T) {
	valid := models.SignNoteBox{
		ClientKey:   "box-1",
		PageNo:      1,
		XRatio:      0.1,
		YRatio:      0.2,
		WidthRatio:  0.2,
		HeightRatio: 0.05,
		Text:        " ตรวจแล้ว ",
	}
	boxes, note, err := normalizeRuntimeSignNoteBoxes([]models.SignNoteBox{valid}, 1)
	if err != nil {
		t.Fatalf("expected valid runtime note box, got %v", err)
	}
	if len(boxes) != 1 || boxes[0].Text != "ตรวจแล้ว" || boxes[0].Label != "หมายเหตุผู้เซ็น" {
		t.Fatalf("unexpected normalized boxes: %#v", boxes)
	}
	if boxes[0].FontSizePt != defaultRuntimeSignNoteFontSizePt || boxes[0].TextAlign != "left" || boxes[0].VerticalAlign != "middle" || boxes[0].PaddingPt != defaultRuntimeSignNotePaddingPt {
		t.Fatalf("unexpected default note style: %#v", boxes[0])
	}
	if note != "ตรวจแล้ว" {
		t.Fatalf("unexpected combined note: %q", note)
	}

	cases := []struct {
		name string
		box  models.SignNoteBox
		code string
	}{
		{name: "empty text", box: func() models.SignNoteBox { box := valid; box.Text = " "; return box }(), code: "sign_note_text_required"},
		{name: "invalid page", box: func() models.SignNoteBox { box := valid; box.PageNo = 2; return box }(), code: "sign_note_page_invalid"},
		{name: "out of bounds", box: func() models.SignNoteBox { box := valid; box.XRatio = 0.9; box.WidthRatio = 0.2; return box }(), code: "sign_note_bounds_invalid"},
		{name: "too small", box: func() models.SignNoteBox { box := valid; box.WidthRatio = 0.01; return box }(), code: "sign_note_box_too_small"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := normalizeRuntimeSignNoteBoxes([]models.SignNoteBox{tc.box}, 1)
			var validationErr runtimeSignNoteValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected validation error, got %v", err)
			}
			if validationErr.code != tc.code {
				t.Fatalf("expected code %q, got %q", tc.code, validationErr.code)
			}
		})
	}
}

func TestNormalizeRuntimeSignNoteBoxesNormalizesStyle(t *testing.T) {
	boxes, _, err := normalizeRuntimeSignNoteBoxes([]models.SignNoteBox{
		{
			ClientKey:     "box-1",
			PageNo:        1,
			XRatio:        0.1,
			YRatio:        0.2,
			WidthRatio:    0.2,
			HeightRatio:   0.05,
			Text:          "จัดขวา",
			FontSizePt:    99,
			TextAlign:     " RIGHT ",
			VerticalAlign: "center",
			PaddingPt:     -5,
		},
		{
			ClientKey:     "box-2",
			PageNo:        1,
			XRatio:        0.4,
			YRatio:        0.2,
			WidthRatio:    0.2,
			HeightRatio:   0.05,
			Text:          "ล่าง",
			FontSizePt:    6,
			TextAlign:     "invalid",
			VerticalAlign: "bottom",
			PaddingPt:     99,
		},
	}, 1)
	if err != nil {
		t.Fatalf("expected valid styled runtime note boxes, got %v", err)
	}
	if boxes[0].FontSizePt != maxRuntimeSignNoteFontSizePt || boxes[0].TextAlign != "right" || boxes[0].VerticalAlign != "middle" || boxes[0].PaddingPt != defaultRuntimeSignNotePaddingPt {
		t.Fatalf("unexpected normalized style for box 1: %#v", boxes[0])
	}
	if boxes[1].FontSizePt != minRuntimeSignNoteFontSizePt || boxes[1].TextAlign != "left" || boxes[1].VerticalAlign != "bottom" || boxes[1].PaddingPt != maxRuntimeSignNotePaddingPt {
		t.Fatalf("unexpected normalized style for box 2: %#v", boxes[1])
	}
}

func TestNormalizeRuntimeSignNoteBoxesRejectsTooManyBoxes(t *testing.T) {
	boxes := make([]models.SignNoteBox, maxRuntimeSignNoteBoxes+1)
	for i := range boxes {
		boxes[i] = models.SignNoteBox{
			ClientKey:   "box-" + string(rune('a'+i)),
			PageNo:      1,
			XRatio:      0.1,
			YRatio:      0.2,
			WidthRatio:  0.2,
			HeightRatio: 0.05,
			Text:        "note",
		}
	}
	_, _, err := normalizeRuntimeSignNoteBoxes(boxes, 1)
	var validationErr runtimeSignNoteValidationError
	if !errors.As(err, &validationErr) || validationErr.code != "sign_note_box_count_invalid" {
		t.Fatalf("expected too many boxes validation error, got %v", err)
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
