package api

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/phpdave11/gofpdf"
)

func TestStampPDFWithSignatures(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.pdf")
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Text(72, 72, "source")
	if err := pdf.OutputFileAndClose(source); err != nil {
		t.Fatalf("create source pdf: %v", err)
	}

	signaturePath := filepath.Join(dir, "signature.png")
	signatureFile, err := os.Create(signaturePath)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 24, 8))
	for x := 0; x < 24; x++ {
		img.Set(x, 4, color.RGBA{R: 20, G: 20, B: 20, A: 255})
	}
	if err := png.Encode(signatureFile, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := signatureFile.Close(); err != nil {
		t.Fatalf("close png: %v", err)
	}

	now := time.Now()
	out, err := stampPDFWithSignatures(source, 1, []models.SigningDocumentSigner{{
		PageNo:          1,
		XRatio:          0.1,
		YRatio:          0.7,
		WidthRatio:      0.2,
		HeightRatio:     0.08,
		Status:          "signed",
		SignatureFileID: "sig-1",
		SignedAt:        &now,
	}}, map[string]models.UploadedFile{
		"sig-1": {StoragePath: signaturePath, ContentType: "image/png"},
	})
	if err != nil {
		t.Fatalf("stamp pdf: %v", err)
	}
	if !strings.HasPrefix(string(out), "%PDF-") {
		t.Fatalf("stamped output is not a PDF")
	}
	if len(out) <= len("%PDF-") {
		t.Fatalf("stamped output is unexpectedly small: %d bytes", len(out))
	}
}

func TestStampPDFWithSignaturesKeepsSignatureBackgroundTransparent(t *testing.T) {
	pdftoppm, err := exec.LookPath("pdftoppm")
	if err != nil {
		t.Skip("pdftoppm is required for visual PDF transparency check")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source.pdf")
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFillColor(220, 40, 40)
	pdf.Rect(0, 0, 595, 842, "F")
	if err := pdf.OutputFileAndClose(source); err != nil {
		t.Fatalf("create source pdf: %v", err)
	}

	signaturePath := filepath.Join(dir, "signature-white-bg.png")
	signatureFile, err := os.Create(signaturePath)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 120, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 120; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	for x := 8; x < 112; x++ {
		img.SetRGBA(x, 24, color.RGBA{R: 17, G: 24, B: 39, A: 255})
		img.SetRGBA(x, 25, color.RGBA{R: 17, G: 24, B: 39, A: 255})
	}
	if err := png.Encode(signatureFile, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := signatureFile.Close(); err != nil {
		t.Fatalf("close png: %v", err)
	}

	now := time.Now()
	out, err := stampPDFWithSignatures(source, 1, []models.SigningDocumentSigner{{
		PageNo:          1,
		XRatio:          0.2,
		YRatio:          0.2,
		WidthRatio:      0.3,
		HeightRatio:     0.08,
		Status:          "signed",
		SignatureFileID: "sig-1",
		SignedAt:        &now,
	}}, map[string]models.UploadedFile{
		"sig-1": {StoragePath: signaturePath, ContentType: "image/png"},
	})
	if err != nil {
		t.Fatalf("stamp pdf: %v", err)
	}
	outPDF := filepath.Join(dir, "out.pdf")
	if err := os.WriteFile(outPDF, out, 0o600); err != nil {
		t.Fatalf("write output pdf: %v", err)
	}
	prefix := filepath.Join(dir, "rendered")
	if output, err := exec.Command(pdftoppm, "-png", "-r", "72", outPDF, prefix).CombinedOutput(); err != nil {
		t.Fatalf("render pdf: %v: %s", err, string(output))
	}
	renderedFile := prefix + "-1.png"
	rendered, err := os.Open(renderedFile)
	if err != nil {
		t.Fatalf("open rendered png: %v", err)
	}
	defer rendered.Close()
	renderedImage, err := png.Decode(rendered)
	if err != nil {
		t.Fatalf("decode rendered png: %v", err)
	}
	r, g, b, _ := renderedImage.At(128, 178).RGBA()
	if r>>8 > 245 && g>>8 > 245 && b>>8 > 245 {
		t.Fatalf("signature background rendered white at sample pixel: rgb=(%d,%d,%d)", r>>8, g>>8, b>>8)
	}
	if r>>8 < 150 || g>>8 > 100 || b>>8 > 100 {
		t.Fatalf("sample pixel did not preserve source background: rgb=(%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

func TestStampPDFWithFinalEvidenceAddsPage(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.pdf")
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Text(72, 72, "source")
	if err := pdf.OutputFileAndClose(source); err != nil {
		t.Fatalf("create source pdf: %v", err)
	}

	signaturePath := filepath.Join(dir, "signature.png")
	signatureFile, err := os.Create(signaturePath)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 24, 8))
	for x := 0; x < 24; x++ {
		img.Set(x, 4, color.RGBA{R: 20, G: 20, B: 20, A: 255})
	}
	if err := png.Encode(signatureFile, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := signatureFile.Close(); err != nil {
		t.Fatalf("close png: %v", err)
	}

	now := time.Now()
	signers := []models.SigningDocumentSigner{{
		PageNo:          1,
		XRatio:          0.1,
		YRatio:          0.7,
		WidthRatio:      0.2,
		HeightRatio:     0.08,
		Status:          "signed",
		SignatureFileID: "sig-1",
		PositionCode:    "1",
		PositionName:    "ผู้จัดทำ",
		SignerName:      "001:น.ส X",
		SignedAt:        &now,
		IPAddress:       "127.0.0.1",
		UserAgent:       "test browser",
		DeviceID:        "device-1",
	}}
	files := map[string]models.UploadedFile{"sig-1": {StoragePath: signaturePath, ContentType: "image/png"}}
	signed, err := stampPDFWithSignatures(source, 1, signers, files)
	if err != nil {
		t.Fatalf("stamp pdf: %v", err)
	}
	out, err := stampPDFWithSignaturesAndEvidence(source, 1, signers, files, nil, finalEvidencePage{
		Document:            models.SigningDocument{ID: "doc-1", DocNo: "PO26060001", DocFormatCode: "PO", CompletedAt: &now},
		Signers:             signers,
		SignedContentSHA256: sha256Hex(signed),
		GeneratedAt:         now,
	})
	if err != nil {
		t.Fatalf("stamp evidence pdf: %v", err)
	}
	pageCount, err := readPDFPageCount(out)
	if err != nil {
		t.Fatalf("read page count: %v", err)
	}
	if pageCount != 2 {
		t.Fatalf("expected evidence page, got %d pages", pageCount)
	}
}

func TestFinalEvidencePDFEmbedsMixedThaiLatinAuditText(t *testing.T) {
	pdftotext, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext is required for evidence text extraction check")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source.pdf")
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Text(72, 72, "signed content")
	if err := pdf.OutputFileAndClose(source); err != nil {
		t.Fatalf("create source pdf: %v", err)
	}

	signaturePath := filepath.Join(dir, "signature.png")
	signatureFile, err := os.Create(signaturePath)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 32, 10))
	for x := 0; x < 32; x++ {
		img.Set(x, 5, color.RGBA{R: 20, G: 20, B: 20, A: 255})
	}
	if err := png.Encode(signatureFile, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := signatureFile.Close(); err != nil {
		t.Fatalf("close png: %v", err)
	}

	now := time.Date(2026, 7, 1, 4, 9, 10, 0, time.UTC)
	docID := "268215b9-9979-4076-88a0-238c5eced4f2"
	signedHash := strings.Repeat("a", 64)
	signers := []models.SigningDocumentSigner{{
		PageNo:          1,
		XRatio:          0.1,
		YRatio:          0.7,
		WidthRatio:      0.2,
		HeightRatio:     0.08,
		Status:          "signed",
		SignatureFileID: "sig-1",
		PositionCode:    "3",
		PositionName:    "ผู้อนุมัติ / AUTHORIZED",
		SignerName:      "นาย ทดสอบ PaperLess",
		SignedAt:        &now,
		IPAddress:       "127.0.0.1",
		UserAgent:       "Mozilla/5.0 PaperLess QA Asia/Bangkok",
		DeviceID:        "device-uuid-5a0a2a5f-1e49-4d7a-a16c-d45c994901b6",
	}}
	files := map[string]models.UploadedFile{"sig-1": {StoragePath: signaturePath, ContentType: "image/png"}}
	out, err := stampPDFWithSignaturesAndEvidence(source, 1, signers, files, nil, finalEvidencePage{
		Document:            models.SigningDocument{ID: docID, DocNo: "INV26070001", DocFormatCode: "INV", CompletedAt: &now},
		Signers:             signers,
		SignedContentSHA256: signedHash,
		GeneratedAt:         now,
		LegalText:           signingLegalText,
		LegalTextVersion:    signingLegalTextVersion,
	})
	if err != nil {
		t.Fatalf("stamp evidence pdf: %v", err)
	}
	outPDF := filepath.Join(dir, "evidence.pdf")
	if err := os.WriteFile(outPDF, out, 0o600); err != nil {
		t.Fatalf("write evidence pdf: %v", err)
	}
	textBytes, err := exec.Command(pdftotext, "-layout", outPDF, "-").Output()
	if err != nil {
		t.Fatalf("extract evidence text: %v", err)
	}
	text := string(textBytes)
	for _, want := range []string{
		"INV26070001",
		"PaperLess",
		docID,
		"Asia/Bangkok",
		signedHash,
		"ผู้อนุมัติ",
		"นาย ทดสอบ",
		"Mozilla/5.0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("evidence text missing %q in:\n%s", want, text)
		}
	}
}

func TestStampPDFWithLegalNoticeAndFinalEvidenceAddsPage(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.pdf")
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Text(72, 72, "source")
	if err := pdf.OutputFileAndClose(source); err != nil {
		t.Fatalf("create source pdf: %v", err)
	}

	signaturePath := filepath.Join(dir, "signature.png")
	signatureFile, err := os.Create(signaturePath)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 24, 8))
	for x := 0; x < 24; x++ {
		img.Set(x, 4, color.RGBA{R: 20, G: 20, B: 20, A: 255})
	}
	if err := png.Encode(signatureFile, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := signatureFile.Close(); err != nil {
		t.Fatalf("close png: %v", err)
	}

	now := time.Now()
	signers := []models.SigningDocumentSigner{{
		PageNo:          1,
		XRatio:          0.1,
		YRatio:          0.78,
		WidthRatio:      0.2,
		HeightRatio:     0.08,
		Status:          "signed",
		SignatureFileID: "sig-1",
		SignedAt:        &now,
	}}
	files := map[string]models.UploadedFile{"sig-1": {StoragePath: signaturePath, ContentType: "image/png"}}
	notice := &models.LegalNoticeSnapshot{
		Text:        signingLegalText,
		TextVersion: signingLegalTextVersion,
		Source:      "per_document",
		PageNo:      1,
		XRatio:      0.2,
		YRatio:      0.62,
		WidthRatio:  0.6,
		HeightRatio: 0.08,
		Label:       "ข้อความกฎหมาย",
	}

	signed, err := stampPDFWithSignaturesAndLegalNotice(source, 1, signers, files, notice)
	if err != nil {
		t.Fatalf("stamp legal notice pdf: %v", err)
	}
	out, err := stampPDFWithSignaturesAndEvidence(source, 1, signers, files, notice, finalEvidencePage{
		Document:            models.SigningDocument{ID: "doc-1", DocNo: "PO26060001", DocFormatCode: "PO", CompletedAt: &now},
		Signers:             signers,
		SignedContentSHA256: sha256Hex(signed),
		GeneratedAt:         now,
		LegalText:           signingLegalText,
		LegalTextVersion:    signingLegalTextVersion,
	})
	if err != nil {
		t.Fatalf("stamp evidence pdf: %v", err)
	}
	pageCount, err := readPDFPageCount(out)
	if err != nil {
		t.Fatalf("read page count: %v", err)
	}
	if pageCount != 2 {
		t.Fatalf("expected evidence page, got %d pages", pageCount)
	}
}

func TestStampPDFWithLegalNoticeWorksBeforeAnySignature(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.pdf")
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Text(72, 72, "source")
	if err := pdf.OutputFileAndClose(source); err != nil {
		t.Fatalf("create source pdf: %v", err)
	}

	out, err := stampPDFWithSignaturesAndLegalNotice(source, 1, nil, nil, &models.LegalNoticeSnapshot{
		Text:        signingLegalText,
		TextVersion: signingLegalTextVersion,
		Source:      "per_document",
		PageNo:      1,
		XRatio:      0.2,
		YRatio:      0.62,
		WidthRatio:  0.6,
		HeightRatio: 0.08,
		Label:       "ข้อความกฎหมาย",
	})
	if err != nil {
		t.Fatalf("stamp legal notice pdf: %v", err)
	}
	if !strings.HasPrefix(string(out), "%PDF-") {
		t.Fatalf("legal notice output is not a PDF")
	}
	pageCount, err := readPDFPageCount(out)
	if err != nil {
		t.Fatalf("read page count: %v", err)
	}
	if pageCount != 1 {
		t.Fatalf("expected same page count, got %d pages", pageCount)
	}
}

func TestLegalNoticeDisplayTextUsesThaiSafeLegalCopy(t *testing.T) {
	text := legalNoticeDisplayText(signingLegalText)
	if strings.Contains(text, ".") || strings.Contains(text, "2544") {
		t.Fatalf("display text should use normalized Thai legal copy: %q", text)
	}
	if !strings.Contains(text, "พระราชบัญญัติ") || !strings.Contains(text, "๒๕๔๔") {
		t.Fatalf("display text should preserve legal meaning in Thai-safe form: %q", text)
	}
}

func TestCreatePrintCopyPDFAddsPage(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.pdf")
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Text(72, 72, "source")
	if err := pdf.OutputFileAndClose(source); err != nil {
		t.Fatalf("create source pdf: %v", err)
	}

	now := time.Now()
	out, err := createPrintCopyPDF(source, 1, printEvidencePage{
		Document:        models.SigningDocument{ID: "doc-1", DocNo: "PO26060001", DocFormatCode: "PO"},
		PrintedAt:       now,
		PrintedBy:       "Admin",
		Channel:         "web",
		PrinterName:     "not_available_web_browser",
		DeviceIDHash:    "abc123",
		ClientTimezone:  "Asia/Bangkok",
		IPAddress:       "127.0.0.1",
		UserAgent:       "test browser",
		FinalFileSHA256: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatalf("create print copy: %v", err)
	}
	pageCount, err := readPDFPageCount(out)
	if err != nil {
		t.Fatalf("read page count: %v", err)
	}
	if pageCount != 2 {
		t.Fatalf("expected print evidence page, got %d pages", pageCount)
	}
}

func TestCreatePrintCopyPDFUsesRequestedDocumentPagesOnly(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "final-with-evidence.pdf")
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Text(72, 72, "signed content")
	pdf.AddPage()
	pdf.Text(72, 72, "electronic signature evidence appendix")
	if err := pdf.OutputFileAndClose(source); err != nil {
		t.Fatalf("create source pdf: %v", err)
	}

	now := time.Now()
	out, err := createPrintCopyPDF(source, 1, printEvidencePage{
		Document:        models.SigningDocument{ID: "doc-1", DocNo: "PO26060001", DocFormatCode: "PO"},
		PrintedAt:       now,
		PrintedBy:       "Admin",
		Channel:         "web",
		PrinterName:     "not_available_web_browser",
		DeviceIDHash:    "abc123",
		ClientTimezone:  "Asia/Bangkok",
		IPAddress:       "127.0.0.1",
		UserAgent:       "test browser",
		FinalFileSHA256: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatalf("create print copy: %v", err)
	}
	pageCount, err := readPDFPageCount(out)
	if err != nil {
		t.Fatalf("read page count: %v", err)
	}
	if pageCount != 2 {
		t.Fatalf("expected signed content plus print evidence only, got %d pages", pageCount)
	}
}

func TestFormatEvidenceTimeUsesBangkokBuddhistYear(t *testing.T) {
	value := time.Date(2026, 7, 1, 4, 9, 10, 0, time.UTC)
	got := formatEvidenceTime(&value)
	want := "01/07/2569 11:09:10 น. (Asia/Bangkok)"
	if got != want {
		t.Fatalf("formatEvidenceTime() = %q, want %q", got, want)
	}
}

func TestSigningPlacementsForSignersExpandsSingleSignerToMultiplePages(t *testing.T) {
	signers := []models.SigningDocumentSigner{{
		ID:              "signer-1",
		PositionCode:    "1",
		ConditionType:   1,
		SignerType:      "any",
		SignerSlot:      2,
		Status:          "signed",
		SignatureFileID: "file-1",
		PageNo:          1,
		XRatio:          0.1,
		YRatio:          0.7,
		WidthRatio:      0.2,
		HeightRatio:     0.08,
	}}
	placements := []models.SignaturePlacementSnapshot{
		{PositionCode: "1", ConditionType: 1, SignerType: "any", SignerSlot: 1, PageNo: 1, XRatio: 0.1, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
		{PositionCode: "1", ConditionType: 1, SignerType: "any", SignerSlot: 1, PageNo: 2, XRatio: 0.1, YRatio: 0.7, WidthRatio: 0.2, HeightRatio: 0.08},
	}

	stampSigners := signingPlacementsForSigners(signers, placements)
	if len(stampSigners) != 2 {
		t.Fatalf("expected two stamp signers, got %d", len(stampSigners))
	}
	if stampSigners[0].PageNo != 1 || stampSigners[1].PageNo != 2 {
		t.Fatalf("expected stamp pages 1 and 2, got %#v", stampSigners)
	}
	if stampSigners[0].SignatureFileID != "file-1" || stampSigners[1].SignatureFileID != "file-1" {
		t.Fatalf("expected both placements to use the signed file")
	}
}
