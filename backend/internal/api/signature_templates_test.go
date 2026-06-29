package api

import (
	"errors"
	"strings"
	"testing"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func TestValidateSignatureTemplatePOConditions(t *testing.T) {
	configs := []models.DocumentConfigStep{
		{PositionCode: "1", PositionName: "ผู้จัดทำ", ConditionType: 1},
		{PositionCode: "2", PositionName: "ผู้ตรวจสอบ", ConditionType: 1},
		{PositionCode: "3", PositionName: "ผู้อนุมัติ", User01: "901:นาย A", User02: "902:นาย B", ConditionType: 2},
	}
	template := models.SignatureTemplate{
		SampleFileID: "file-1",
		SampleFile:   &models.UploadedFile{PageCount: 1},
		Boxes: []models.SignatureTemplateBox{
			{PositionCode: "1", SignerSlot: 1, SignerType: "any", PageNo: 1, XRatio: 0.1, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
			{PositionCode: "2", SignerSlot: 1, SignerType: "any", PageNo: 1, XRatio: 0.3, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
			{PositionCode: "3", SignerSlot: 1, SignerType: "internal", SignerUser: "901:นาย A", PageNo: 1, XRatio: 0.5, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
			{PositionCode: "3", SignerSlot: 2, SignerType: "internal", SignerUser: "902:นาย B", PageNo: 1, XRatio: 0.7, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		},
	}

	issues := validateSignatureTemplate(template, configs, 20)
	if len(issues) != 0 {
		t.Fatalf("expected no validation issues, got %#v", issues)
	}
}

func TestValidateSignatureTemplateRejectsMissingConditionTwoUser(t *testing.T) {
	configs := []models.DocumentConfigStep{
		{PositionCode: "3", PositionName: "ผู้อนุมัติ", User01: "901:นาย A", User02: "902:นาย B", ConditionType: 2},
	}
	template := models.SignatureTemplate{
		SampleFileID: "file-1",
		SampleFile:   &models.UploadedFile{PageCount: 1},
		Boxes: []models.SignatureTemplateBox{
			{PositionCode: "3", SignerSlot: 1, SignerType: "internal", SignerUser: "901:นาย A", PageNo: 1, XRatio: 0.5, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		},
	}

	issues := validateSignatureTemplate(template, configs, 20)
	if !hasSignatureIssue(issues, "condition_all_box_count_invalid") {
		t.Fatalf("expected condition_all_box_count_invalid, got %#v", issues)
	}
	if !hasSignatureIssue(issues, "condition_all_missing_user_box") {
		t.Fatalf("expected condition_all_missing_user_box, got %#v", issues)
	}
}

func TestValidateSignatureTemplateRejectsWrongSignerTypes(t *testing.T) {
	configs := []models.DocumentConfigStep{
		{PositionCode: "1", PositionName: "ผู้จัดทำ", ConditionType: 1},
		{PositionCode: "4", PositionName: "ลูกค้า", ConditionType: 3},
	}
	template := models.SignatureTemplate{
		SampleFileID: "file-1",
		SampleFile:   &models.UploadedFile{PageCount: 1},
		Boxes: []models.SignatureTemplateBox{
			{PositionCode: "1", SignerSlot: 1, SignerType: "internal", SignerUser: "001:น.ส X", PageNo: 1, XRatio: 0.1, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
			{PositionCode: "4", SignerSlot: 1, SignerType: "internal", SignerUser: "999:Temp", PageNo: 1, XRatio: 0.4, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		},
	}

	issues := validateSignatureTemplate(template, configs, 20)
	if !hasSignatureIssue(issues, "condition_any_type_invalid") {
		t.Fatalf("expected condition_any_type_invalid, got %#v", issues)
	}
	if !hasSignatureIssue(issues, "condition_external_type_invalid") {
		t.Fatalf("expected condition_external_type_invalid, got %#v", issues)
	}
}

func TestNormalizeAndValidateBoxRequestsRejectsStructuralErrors(t *testing.T) {
	_, issues := normalizeAndValidateBoxRequests([]models.SignatureTemplateBoxRequest{
		{PositionCode: "1", SignerSlot: 1, SignerType: "any", PageNo: 1, XRatio: 0.1, YRatio: 0.1, WidthRatio: 0.3, HeightRatio: 0.1},
		{PositionCode: "1", SignerSlot: 1, SignerType: "any", PageNo: 2, XRatio: 0.9, YRatio: 0.9, WidthRatio: 0.2, HeightRatio: 0.2},
	}, 1)

	if !hasSignatureIssue(issues, "box_signer_slot_duplicate") {
		t.Fatalf("expected box_signer_slot_duplicate, got %#v", issues)
	}
	if !hasSignatureIssue(issues, "box_page_invalid") {
		t.Fatalf("expected box_page_invalid, got %#v", issues)
	}
	if !hasSignatureIssue(issues, "box_bounds_invalid") {
		t.Fatalf("expected box_bounds_invalid, got %#v", issues)
	}
}

func hasSignatureIssue(issues []models.SignatureValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestDecodeSignatureDesignerEventPayloadRejectsOversizedBody(t *testing.T) {
	_, err := decodeSignatureDesignerEventPayload(strings.NewReader(strings.Repeat("x", int(maxSignatureDesignerEventBytes)+1)), maxSignatureDesignerEventBytes)
	if !errors.Is(err, errSignatureDesignerEventTooLarge) {
		t.Fatalf("expected errSignatureDesignerEventTooLarge, got %v", err)
	}
}

func TestDecodeSignatureDesignerEventPayloadRejectsUnknownFields(t *testing.T) {
	body := `{"event":"save_success","sessionId":"s1","signerUser":"901:นาย A"}`
	_, err := decodeSignatureDesignerEventPayload(strings.NewReader(body), maxSignatureDesignerEventBytes)
	if err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
}

func TestNormalizeSignatureDesignerEventMetadataKeepsOnlySafeFields(t *testing.T) {
	template := models.SignatureTemplate{
		ID:            "template-1",
		ScreenCode:    "PO",
		DocFormatCode: "PO",
	}
	req := models.SignatureDesignerEventRequest{
		Event:                "save_success",
		SessionID:            strings.Repeat("a", 120),
		DocFormatCode:        "CLIENT_SENT_VALUE",
		PositionCode:         "3",
		ConditionType:        2,
		ElapsedMS:            12_500,
		BoxCount:             4,
		ValidationIssueCount: 0,
		Viewport:             models.SignatureDesignerViewport{Width: 1652, Height: 1324},
	}

	metadata, err := normalizeSignatureDesignerEventMetadata(req, template)
	if err != nil {
		t.Fatalf("expected safe event metadata, got error %v", err)
	}
	if metadata["event"] != "save_success" {
		t.Fatalf("expected save_success event, got %#v", metadata["event"])
	}
	if metadata["docFormatCode"] != "PO" {
		t.Fatalf("expected server-side doc format code, got %#v", metadata["docFormatCode"])
	}
	if got := metadata["sessionId"].(string); len(got) != 80 {
		t.Fatalf("expected session id to be truncated to 80 chars, got %d", len(got))
	}
	if _, ok := metadata["signerUser"]; ok {
		t.Fatal("metadata must not include signer user")
	}
}

func TestNormalizeSignatureDesignerEventMetadataRejectsInvalidEvent(t *testing.T) {
	_, err := normalizeSignatureDesignerEventMetadata(models.SignatureDesignerEventRequest{Event: "mousemove"}, models.SignatureTemplate{})
	if !errors.Is(err, errSignatureDesignerEventInvalid) {
		t.Fatalf("expected errSignatureDesignerEventInvalid, got %v", err)
	}
}

func TestNormalizeSignatureDesignerEventMetadataClampsBounds(t *testing.T) {
	metadata, err := normalizeSignatureDesignerEventMetadata(models.SignatureDesignerEventRequest{
		Event:                "pdf_render_error",
		ConditionType:        99,
		ElapsedMS:            -1,
		BoxCount:             5000,
		ValidationIssueCount: 5000,
		Viewport:             models.SignatureDesignerViewport{Width: 20000, Height: -1},
	}, models.SignatureTemplate{})
	if err != nil {
		t.Fatalf("expected metadata, got %v", err)
	}
	if metadata["conditionType"] != 0 || metadata["elapsedMs"] != int64(0) || metadata["boxCount"] != 1000 || metadata["validationIssueCount"] != 1000 {
		t.Fatalf("metadata bounds were not clamped: %#v", metadata)
	}
	viewport := metadata["viewport"].(map[string]any)
	if viewport["width"] != 10000 || viewport["height"] != 0 {
		t.Fatalf("viewport bounds were not clamped: %#v", viewport)
	}
}
