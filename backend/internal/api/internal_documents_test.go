package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func TestInternalDocumentPDFPageCounts(t *testing.T) {
	for _, itemCount := range []int{1, 5, internalDocumentMaxItems} {
		document := internalPDFTestDocument(itemCount)
		data, pages, err := renderInternalDocumentPDF(document)
		if err != nil {
			t.Fatalf("render %d items: %v", itemCount, err)
		}
		if !bytes.HasPrefix(data, []byte("%PDF")) {
			t.Fatalf("render %d items did not produce a PDF", itemCount)
		}
		parsedPages, err := readPDFPageCount(data)
		if err != nil {
			t.Fatalf("read %d-item PDF: %v", itemCount, err)
		}
		if parsedPages != pages {
			t.Fatalf("render %d items reported %d pages but PDF has %d", itemCount, pages, parsedPages)
		}
		if pages != 1 {
			t.Fatalf("render %d items reported %d pages, want one A4 page", itemCount, pages)
		}
		if itemCount == internalDocumentMaxItems {
			if output := os.Getenv("INTERNAL_PDF_QA_OUTPUT"); output != "" {
				if err := os.WriteFile(output, data, 0o600); err != nil {
					t.Fatalf("write PDF QA artifact: %v", err)
				}
			}
		}
	}
	if _, _, err := renderInternalDocumentPDF(internalPDFTestDocument(internalDocumentMaxItems + 1)); err == nil {
		t.Fatalf("rendering more than %d items must fail", internalDocumentMaxItems)
	}
}

func TestInternalFixedSignatureLayoutMapsWorkflowToBlankA4Cells(t *testing.T) {
	configs := []models.DocumentConfigStep{
		{PositionCode: "1", PositionName: "ผู้ขอเบิก", SequenceNo: 1, ConditionType: 1, User01: "requester:Requester"},
		{PositionCode: "2", PositionName: "ผู้ตรวจสอบ", SequenceNo: 2, ConditionType: 2, User01: "checker:Checker", User02: "approver:Approver"},
		{PositionCode: "3", PositionName: "ผู้อนุมัติ", SequenceNo: 3, ConditionType: 3},
	}
	boxes, selected, placements, err := internalFixedSignatureLayout(configs)
	if err != nil {
		t.Fatalf("fixed layout: %v", err)
	}
	if len(boxes) != 4 || len(selected) != 3 || len(placements) != 4 {
		t.Fatalf("unexpected fixed layout sizes boxes=%d configs=%d placements=%d", len(boxes), len(selected), len(placements))
	}
	seenCells := map[string]bool{}
	for _, box := range boxes {
		if box.PageNo != 1 || box.Label != "" {
			t.Fatalf("internal fixed box must use a blank first-page approval cell: %#v", box)
		}
		cell := fmt.Sprintf("%.4f:%.4f", box.XRatio, box.YRatio)
		if seenCells[cell] {
			t.Fatalf("workflow slots must not share an approval cell: %s", cell)
		}
		seenCells[cell] = true
	}
}

func TestInternalFixedSignatureLayoutRejectsMoreThanSixSlots(t *testing.T) {
	configs := make([]models.DocumentConfigStep, internalDocumentMaxSignatureSlots+1)
	for i := range configs {
		configs[i] = models.DocumentConfigStep{PositionCode: fmt.Sprintf("%d", i+1), PositionName: "ผู้อนุมัติ", SequenceNo: float64(i + 1), ConditionType: 3}
	}
	if _, _, _, err := internalFixedSignatureLayout(configs); err == nil {
		t.Fatalf("workflow with more than %d slots must fail", internalDocumentMaxSignatureSlots)
	}
}

func TestInternalDocumentsNeverRequireSMLFinalization(t *testing.T) {
	if requiresSMLFinalization(models.SigningDocument{DocumentSource: "internal"}) {
		fatalUnexpectedSMLFinalization(t)
	}
	if !requiresSMLFinalization(models.SigningDocument{DocumentSource: "sml"}) {
		t.Fatal("SML document must use SML finalization")
	}
}

func TestInternalDocumentSMLActionGuard(t *testing.T) {
	recorder := httptest.NewRecorder()
	if !rejectInternalDocumentSMLAction(recorder, models.SigningDocument{DocumentSource: "internal"}) {
		t.Fatal("expected internal document action to be rejected")
	}
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("sml_action_not_applicable")) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestInternalTextRunsPreserveThaiLaoAndLatin(t *testing.T) {
	runs := internalTextRuns("ไทย English ລາວ 123")
	if len(runs) < 3 {
		t.Fatalf("expected mixed-script font runs, got %#v", runs)
	}
	joined := ""
	seenLao := false
	seenThai := false
	for _, run := range runs {
		joined += run.text
		seenLao = seenLao || run.font == internalFontLao
		seenThai = seenThai || run.font == internalFontThai
	}
	if joined != "ไทย English ລາວ 123" || !seenLao || !seenThai {
		t.Fatalf("mixed-script runs lost content or font coverage: %#v", runs)
	}
}

func internalPDFTestDocument(itemCount int) models.InternalDocument {
	items := make([]models.InternalDocumentItem, itemCount)
	for i := range items {
		items[i] = models.InternalDocumentItem{SequenceNo: i + 1, Description: "รายการทดสอบ ภาษาไทย Lao ທົດສອບ", Amount: "1250.50"}
	}
	return models.InternalDocument{
		MasterName:     "ใบขอเบิกเงินทดรอง",
		DocumentNo:     "ADV260722-001",
		DocumentDate:   "2026-07-22",
		RequiredDate:   "2026-07-30",
		RequesterName:  "ผู้ขอเบิก ทดสอบ",
		PositionName:   "เจ้าหน้าที่",
		DepartmentName: "ฝ่ายบัญชี",
		Purpose:        "ทดสอบเอกสารภายใน PaperLess ภาษาไทย English ພາສາລາວ",
		TotalAmount:    "1250.50",
		CompanySnapshot: models.InternalDocumentCompanySnapshot{
			DisplayName:     "บริษัท ทดสอบ จำกัด ບໍລິສັດ",
			Address1:        "1270 ถนนทดสอบ",
			TelephoneNumber: "02-000-0000",
			TaxNumber:       "0100000000000",
		},
		Items: items,
	}
}

func fatalUnexpectedSMLFinalization(t *testing.T) {
	t.Helper()
	t.Fatal("internal document must make zero SML image/lock calls")
}
