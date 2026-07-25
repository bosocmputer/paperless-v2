package api

import (
	"fmt"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

const (
	internalDocumentPageWidthMM  = 210.0
	internalDocumentPageHeightMM = 297.0
	internalApprovalLeftMM       = 10.0
	internalApprovalTopMM        = 237.0
	internalApprovalWidthMM      = 190.0
	internalApprovalHeightMM     = 42.0
)

// internalApprovalArea is the only part of the generated A4 form where a
// superadmin can place signing boxes. The form stays stable even when a
// workflow has more signers than a pre-drawn grid could accommodate.
type internalApprovalArea struct {
	left   float64
	top    float64
	width  float64
	height float64
}

func internalDocumentApprovalArea() internalApprovalArea {
	return internalApprovalArea{
		left:   internalApprovalLeftMM / internalDocumentPageWidthMM,
		top:    internalApprovalTopMM / internalDocumentPageHeightMM,
		width:  internalApprovalWidthMM / internalDocumentPageWidthMM,
		height: internalApprovalHeightMM / internalDocumentPageHeightMM,
	}
}

func validateInternalApprovalLayoutBoxes(boxes []models.SignatureTemplateBoxRequest) []models.SignatureValidationIssue {
	area := internalDocumentApprovalArea()
	issues := []models.SignatureValidationIssue{}
	for _, box := range boxes {
		if box.PageNo != 1 {
			issues = append(issues, signatureIssue("internal_approval_page_invalid", box.PositionCode, "กรอบลายเซ็นเอกสารภายในต้องอยู่หน้า 1 ในพื้นที่การอนุมัติ"))
			continue
		}
		if box.XRatio < area.left || box.YRatio < area.top || box.XRatio+box.WidthRatio > area.left+area.width || box.YRatio+box.HeightRatio > area.top+area.height {
			issues = append(issues, signatureIssue("internal_approval_area_required", box.PositionCode, fmt.Sprintf("กรอบลายเซ็น %s ต้องอยู่ภายในพื้นที่การอนุมัติของแบบฟอร์ม A4", box.PositionCode)))
		}
	}
	return issues
}

// validateInternalApprovalLayout keeps every configurable mark in the blank
// approval area. The red advance-clearance note is drawn by the form itself
// and is intentionally outside this configurable layout.
func validateInternalApprovalLayout(boxes []models.SignatureTemplateBoxRequest, legalNoticeBox *models.LegalNoticeBoxRequest) []models.SignatureValidationIssue {
	issues := validateInternalApprovalLayoutBoxes(boxes)
	if legalNoticeBox == nil {
		return append(issues, signatureIssue("internal_legal_notice_required", "", "กรุณาวางกรอบข้อความกฎหมายในพื้นที่การอนุมัติของแบบฟอร์ม A4"))
	}

	area := internalDocumentApprovalArea()
	if legalNoticeBox.PageNo != 1 {
		issues = append(issues, signatureIssue("internal_legal_notice_page_invalid", "", "กรอบข้อความกฎหมายเอกสารภายในต้องอยู่หน้า 1 ในพื้นที่การอนุมัติ"))
	} else if legalNoticeBox.XRatio < area.left || legalNoticeBox.YRatio < area.top || legalNoticeBox.XRatio+legalNoticeBox.WidthRatio > area.left+area.width || legalNoticeBox.YRatio+legalNoticeBox.HeightRatio > area.top+area.height {
		issues = append(issues, signatureIssue("internal_legal_notice_area_required", "", "กรอบข้อความกฎหมายต้องอยู่ภายในพื้นที่การอนุมัติของแบบฟอร์ม A4"))
	}

	for _, box := range boxes {
		if box.PageNo != legalNoticeBox.PageNo || !layoutBoxesOverlap(box, *legalNoticeBox) {
			continue
		}
		issues = append(issues, signatureIssue("internal_approval_box_overlap", box.PositionCode, fmt.Sprintf("กรอบลายเซ็น %s ทับกับกรอบข้อความกฎหมาย", box.PositionCode)))
	}
	return issues
}

func layoutBoxesOverlap(signatureBox models.SignatureTemplateBoxRequest, legalBox models.LegalNoticeBoxRequest) bool {
	return signatureBox.XRatio < legalBox.XRatio+legalBox.WidthRatio &&
		signatureBox.XRatio+signatureBox.WidthRatio > legalBox.XRatio &&
		signatureBox.YRatio < legalBox.YRatio+legalBox.HeightRatio &&
		signatureBox.YRatio+signatureBox.HeightRatio > legalBox.YRatio
}
