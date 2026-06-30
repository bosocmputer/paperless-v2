package api

import (
	"image"
	"image/color"
	"image/png"
	"os"
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
