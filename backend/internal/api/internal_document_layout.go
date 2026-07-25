package api

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

// internalFixedSignatureLayout maps each required workflow signer to one of the
// blank approval cells that the fixed A4 internal-document form reserves.
func internalFixedSignatureLayout(configs []models.DocumentConfigStep) ([]models.SignatureTemplateBoxRequest, []models.DocumentConfigStep, []models.SignaturePlacementSnapshot, error) {
	sorted := append([]models.DocumentConfigStep(nil), configs...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].SequenceNo == sorted[j].SequenceNo {
			return strings.ToLower(sorted[i].PositionCode) < strings.ToLower(sorted[j].PositionCode)
		}
		return sorted[i].SequenceNo < sorted[j].SequenceNo
	})

	boxes := make([]models.SignatureTemplateBoxRequest, 0, internalDocumentMaxSignatureSlots)
	for _, step := range sorted {
		users := stepUsers(step)
		switch step.ConditionType {
		case 1:
			if len(users) == 0 {
				return nil, nil, nil, fmt.Errorf("ตำแหน่ง %s ต้องกำหนดผู้เซ็นอย่างน้อยหนึ่งคน", step.PositionName)
			}
			boxes = append(boxes, internalFixedSignatureBox(len(boxes), step, 1, "any", ""))
		case 2:
			if len(users) == 0 {
				return nil, nil, nil, fmt.Errorf("ตำแหน่ง %s ต้องกำหนดผู้เซ็นอย่างน้อยหนึ่งคน", step.PositionName)
			}
			for index, user := range users {
				boxes = append(boxes, internalFixedSignatureBox(len(boxes), step, index+1, "internal", user))
			}
		case 3:
			boxes = append(boxes, internalFixedSignatureBox(len(boxes), step, 1, "external", ""))
		default:
			return nil, nil, nil, fmt.Errorf("ตำแหน่ง %s มีเงื่อนไขผู้เซ็นไม่ถูกต้อง", step.PositionName)
		}
	}

	if len(boxes) > internalDocumentMaxSignatureSlots {
		return nil, nil, nil, fmt.Errorf("Workflow เอกสารภายในใช้ช่องลายเซ็น %d ช่อง แต่แบบฟอร์ม A4 รองรับสูงสุด %d ช่อง", len(boxes), internalDocumentMaxSignatureSlots)
	}

	layout, selected, placements, issues := validateSigningDocumentLayout(boxes, sorted, 1)
	if len(issues) > 0 {
		return nil, nil, nil, fmt.Errorf("Workflow เอกสารภายในยังไม่พร้อม: %s", issues[0].Message)
	}
	return layout, selected, placements, nil
}

func internalFixedSignatureBox(index int, step models.DocumentConfigStep, signerSlot int, signerType, signerUser string) models.SignatureTemplateBoxRequest {
	const (
		pageWidthMM  = 210.0
		pageHeightMM = 297.0
		leftMM       = 10.0
		approvalTop  = 237.0
		cellWidthMM  = 190.0 / internalDocumentApprovalColumns
		cellHeightMM = 21.0
	)
	row := index / internalDocumentApprovalColumns
	column := index % internalDocumentApprovalColumns
	return models.SignatureTemplateBoxRequest{
		ClientKey:    fmt.Sprintf("internal-fixed-%d", index+1),
		PositionCode: step.PositionCode,
		SignerSlot:   signerSlot,
		SignerType:   signerType,
		SignerUser:   signerUser,
		PageNo:       1,
		XRatio:       (leftMM + float64(column)*cellWidthMM) / pageWidthMM,
		YRatio:       (approvalTop + float64(row)*cellHeightMM) / pageHeightMM,
		WidthRatio:   cellWidthMM / pageWidthMM,
		HeightRatio:  cellHeightMM / pageHeightMM,
	}
}

func internalWorkflowSlotCountFromRequests(steps []models.DocumentConfigStepRequest) int {
	count := 0
	for _, step := range steps {
		if step.ConditionType == 2 {
			count += len(documentConfigStepUsers(step.User01, step.User02, step.User03))
			continue
		}
		count++
	}
	return count
}

func internalWorkflowSlotCount(steps []models.DocumentConfigStep) int {
	count := 0
	for _, step := range steps {
		if step.ConditionType == 2 {
			count += len(documentConfigStepUsers(step.User01, step.User02, step.User03))
			continue
		}
		count++
	}
	return count
}
