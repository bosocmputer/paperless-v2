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
