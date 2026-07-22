package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func TestInternalDocumentPDFPageCounts(t *testing.T) {
	for _, itemCount := range []int{1, 5, 6, 100} {
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
		if pages > 25 {
			t.Fatalf("render %d items exceeded page limit: %d", itemCount, pages)
		}
		if itemCount == 6 {
			if output := os.Getenv("INTERNAL_PDF_QA_OUTPUT"); output != "" {
				if err := os.WriteFile(output, data, 0o600); err != nil {
					t.Fatalf("write PDF QA artifact: %v", err)
				}
			}
		}
		if itemCount == 100 {
			if output := os.Getenv("INTERNAL_PDF_QA_OUTPUT_100"); output != "" {
				if err := os.WriteFile(output, data, 0o600); err != nil {
					t.Fatalf("write multipage PDF QA artifact: %v", err)
				}
			}
		}
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
