package api

import (
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
