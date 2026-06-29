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
