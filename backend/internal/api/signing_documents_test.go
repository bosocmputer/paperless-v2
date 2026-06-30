package api

import (
	"bytes"
	"strings"
	"testing"

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
	for _, event := range []string{"ready_task_open", "waiting_queue_seen", "waiting_task_open"} {
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
	}

	normalized, selected, issues := validateSigningDocumentLayout(boxes, configs, 1)
	if len(issues) != 0 {
		t.Fatalf("expected partial layout to be valid, got %#v", issues)
	}
	if len(selected) != 2 {
		t.Fatalf("expected only boxed positions to be selected, got %d", len(selected))
	}
	if len(normalized) != 2 {
		t.Fatalf("expected two normalized boxes, got %d", len(normalized))
	}
	if normalized[1].SignerUser != "901:นาย A" {
		t.Fatalf("expected signer user to normalize to configured label, got %q", normalized[1].SignerUser)
	}
}

func TestValidateSigningDocumentLayoutRejectsZeroBoxes(t *testing.T) {
	_, _, issues := validateSigningDocumentLayout(nil, []models.DocumentConfigStep{
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

func TestValidateSigningDocumentLayoutRejectsDuplicateConditionTwoUser(t *testing.T) {
	configs := []models.DocumentConfigStep{
		{PositionCode: "3", PositionName: "ผู้อนุมัติ", User01: "901:นาย A", User02: "902:นาย B", ConditionType: 2},
	}
	boxes := []models.SignatureTemplateBoxRequest{
		{PositionCode: "3", SignerType: "internal", SignerUser: "901", PageNo: 1, XRatio: 0.1, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		{PositionCode: "3", SignerType: "internal", SignerUser: "901:นาย A", PageNo: 1, XRatio: 0.4, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
	}

	_, _, issues := validateSigningDocumentLayout(boxes, configs, 1)
	if !hasSignatureIssue(issues, "condition_all_duplicate_user_box") {
		t.Fatalf("expected duplicate user issue, got %#v", issues)
	}
}
