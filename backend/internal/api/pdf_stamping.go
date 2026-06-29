package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/phpdave11/gofpdf"
	"github.com/phpdave11/gofpdf/contrib/gofpdi"
)

//go:embed fonts/NotoSansThai-Regular.ttf
var notoSansThaiRegular []byte

type finalEvidencePage struct {
	Document            models.SigningDocument
	Signers             []models.SigningDocumentSigner
	SignedContentSHA256 string
	GeneratedAt         time.Time
}

type printEvidencePage struct {
	Document        models.SigningDocument
	PrintedAt       time.Time
	PrintedBy       string
	Channel         string
	PrinterName     string
	DeviceIDHash    string
	ClientTimezone  string
	IPAddress       string
	UserAgent       string
	FinalFileSHA256 string
}

func (s *Server) refreshStampedPDF(ctx context.Context, documentID string, final bool) error {
	document, err := s.store.FindSigningDocumentByID(ctx, documentID)
	if err != nil {
		return err
	}
	if document.OriginalFile == nil || document.OriginalFile.StoragePath == "" {
		return fmt.Errorf("original pdf is missing")
	}
	signers := signedDocumentSigners(document.Signers)
	if len(signers) == 0 {
		return nil
	}
	signatureFiles := map[string]models.UploadedFile{}
	for _, signer := range signers {
		if _, ok := signatureFiles[signer.SignatureFileID]; ok {
			continue
		}
		file, err := s.store.FindUploadedFileByID(ctx, signer.SignatureFileID)
		if err != nil {
			return err
		}
		signatureFiles[signer.SignatureFileID] = file
	}

	stamped, err := stampPDFWithSignatures(document.OriginalFile.StoragePath, document.OriginalFile.PageCount, signers, signatureFiles)
	if err != nil {
		return err
	}
	pageCount := document.OriginalFile.PageCount
	action := "pdf_stamped"
	message := "สร้าง PDF พร้อมลายเซ็นล่าสุดแล้ว"

	if final {
		evidence := finalEvidencePage{
			Document:            document,
			Signers:             signers,
			SignedContentSHA256: sha256Hex(stamped),
			GeneratedAt:         time.Now(),
		}
		stamped, err = stampPDFWithSignaturesAndEvidence(document.OriginalFile.StoragePath, document.OriginalFile.PageCount, signers, signatureFiles, evidence)
		if err != nil {
			return err
		}
		if count, err := readPDFPageCount(stamped); err == nil && count > 0 {
			pageCount = count
		} else {
			pageCount = document.OriginalFile.PageCount + 1
		}
		action = "final_pdf_ready"
		message = "สร้าง Final PDF พร้อม evidence แล้ว"
	}

	name := fmt.Sprintf("%s-stamped-v%d.pdf", strings.TrimSuffix(filepath.Base(document.OriginalFile.OriginalName), filepath.Ext(document.OriginalFile.OriginalName)), document.CurrentVersion+1)
	uploaded, err := s.storeUploadedBytes(ctx, stamped, name, "signed-document.pdf", "application/pdf", ".pdf", pageCount, document.CreatedBy)
	if err != nil {
		return err
	}
	if err := s.store.UpdateSigningDocumentPDF(ctx, document.ID, uploaded, final); err != nil {
		return err
	}
	return s.store.AddSigningEvent(ctx, document.ID, "", "", action, message, "", "", map[string]any{
		"fileId":         uploaded.ID,
		"fileSha256":     uploaded.SHA256,
		"signatureCount": len(signers),
		"final":          final,
	})
}

func stampPDFWithSignatures(sourcePath string, pageCount int, signers []models.SigningDocumentSigner, signatureFiles map[string]models.UploadedFile) ([]byte, error) {
	return renderSignedPDF(sourcePath, pageCount, signers, signatureFiles, nil)
}

func stampPDFWithSignaturesAndEvidence(sourcePath string, pageCount int, signers []models.SigningDocumentSigner, signatureFiles map[string]models.UploadedFile, evidence finalEvidencePage) ([]byte, error) {
	return renderSignedPDF(sourcePath, pageCount, signers, signatureFiles, &evidence)
}

func renderSignedPDF(sourcePath string, pageCount int, signers []models.SigningDocumentSigner, signatureFiles map[string]models.UploadedFile, evidence *finalEvidencePage) ([]byte, error) {
	if pageCount <= 0 {
		return nil, fmt.Errorf("pdf page count is missing")
	}
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCompression(true)

	importer := gofpdi.NewImporter()
	signersByPage := map[int][]models.SigningDocumentSigner{}
	for _, signer := range signers {
		signersByPage[signer.PageNo] = append(signersByPage[signer.PageNo], signer)
	}

	if err := importPDFPages(pdf, importer, sourcePath, pageCount, func(pageNo int, size gofpdf.SizeType) {
		for _, signer := range signersByPage[pageNo] {
			file := signatureFiles[signer.SignatureFileID]
			if file.StoragePath == "" {
				continue
			}
			x := clampRatio(signer.XRatio) * size.Wd
			y := clampRatio(signer.YRatio) * size.Ht
			w := clampRatio(signer.WidthRatio) * size.Wd
			h := clampRatio(signer.HeightRatio) * size.Ht
			if w <= 0 || h <= 0 {
				continue
			}
			pdf.ImageOptions(file.StoragePath, x, y, w, h, false, gofpdf.ImageOptions{ImageType: imageTypeForContent(file.ContentType), ReadDpi: false}, 0, "")
		}
	}); err != nil {
		return nil, err
	}
	if evidence != nil {
		if err := addFinalEvidencePage(pdf, *evidence); err != nil {
			return nil, err
		}
	}
	return outputPDF(pdf)
}

func createPrintCopyPDF(sourcePath string, pageCount int, evidence printEvidencePage) ([]byte, error) {
	if pageCount <= 0 {
		return nil, fmt.Errorf("pdf page count is missing")
	}
	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCompression(true)
	importer := gofpdi.NewImporter()
	if err := importPDFPages(pdf, importer, sourcePath, pageCount, nil); err != nil {
		return nil, err
	}
	if err := addPrintEvidencePage(pdf, evidence); err != nil {
		return nil, err
	}
	return outputPDF(pdf)
}

func importPDFPages(pdf *gofpdf.Fpdf, importer *gofpdi.Importer, sourcePath string, pageCount int, onPage func(pageNo int, size gofpdf.SizeType)) error {
	for pageNo := 1; pageNo <= pageCount; pageNo++ {
		tpl := importer.ImportPage(pdf, sourcePath, pageNo, "/MediaBox")
		size := importedPageSize(importer, pageNo)
		if size.Wd <= 0 || size.Ht <= 0 {
			return fmt.Errorf("cannot read pdf page size for page %d", pageNo)
		}
		orientation := "P"
		if size.Wd > size.Ht {
			orientation = "L"
		}
		pdf.AddPageFormat(orientation, size)
		importer.UseImportedTemplate(pdf, tpl, 0, 0, size.Wd, size.Ht)
		if onPage != nil {
			onPage(pageNo, size)
		}
	}
	return nil
}

func outputPDF(pdf *gofpdf.Fpdf) ([]byte, error) {
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}
	if pdf.Err() {
		return nil, pdf.Error()
	}
	return out.Bytes(), nil
}

func addFinalEvidencePage(pdf *gofpdf.Fpdf, evidence finalEvidencePage) error {
	if err := setupEvidenceFont(pdf); err != nil {
		return err
	}
	pdf.AddPageFormat("P", gofpdf.SizeType{Wd: 595.28, Ht: 841.89})
	drawEvidenceHeader(pdf, "หลักฐานการลงนามอิเล็กทรอนิกส์")
	y := 92.0
	y = drawParagraph(pdf, 48, y, 499, signingLegalText)
	y += 12
	rows := []evidenceRow{
		{"เลขที่เอกสาร", evidence.Document.DocNo},
		{"รูปแบบเอกสาร", evidence.Document.DocFormatCode},
		{"PaperLess document id", evidence.Document.ID},
		{"วันที่เอกสารเซ็นครบ", formatEvidenceTime(evidence.Document.CompletedAt)},
		{"สร้างหลักฐานเมื่อ", formatEvidenceTime(&evidence.GeneratedAt)},
		{"Legal text version", signingLegalTextVersion},
		{"Signed content SHA-256", evidence.SignedContentSHA256},
		{"Final PDF SHA-256", "บันทึกในระบบหลังสร้างไฟล์ final สำเร็จ"},
	}
	y = drawEvidenceRows(pdf, 48, y, rows)
	y += 14
	pdf.SetFont("noto-thai", "", 12)
	pdf.SetTextColor(31, 41, 55)
	pdf.Text(48, y, "ผู้ลงนาม")
	y += 12
	for _, signer := range evidence.Signers {
		if y > 760 {
			pdf.AddPageFormat("P", gofpdf.SizeType{Wd: 595.28, Ht: 841.89})
			drawEvidenceHeader(pdf, "หลักฐานการลงนามอิเล็กทรอนิกส์")
			y = 92
		}
		rows := []evidenceRow{
			{"ตำแหน่ง", signer.PositionCode + " - " + signer.PositionName},
			{"ผู้ลงนาม", signer.SignerName},
			{"เวลาลงนาม", formatEvidenceTime(signer.SignedAt)},
			{"IP", signer.IPAddress},
			{"User agent", truncateEvidence(signer.UserAgent, 110)},
			{"Device id hash", shortHash(signer.DeviceID)},
			{"Legal accepted", "true"},
		}
		y = drawEvidenceRows(pdf, 48, y, rows)
		y += 10
	}
	return nil
}

func addPrintEvidencePage(pdf *gofpdf.Fpdf, evidence printEvidencePage) error {
	if err := setupEvidenceFont(pdf); err != nil {
		return err
	}
	pdf.AddPageFormat("P", gofpdf.SizeType{Wd: 595.28, Ht: 841.89})
	drawEvidenceHeader(pdf, "หลักฐานการพิมพ์เอกสาร")
	rows := []evidenceRow{
		{"เลขที่เอกสาร", evidence.Document.DocNo},
		{"รูปแบบเอกสาร", evidence.Document.DocFormatCode},
		{"PaperLess document id", evidence.Document.ID},
		{"เวลาพิมพ์", formatEvidenceTime(&evidence.PrintedAt)},
		{"ผู้พิมพ์", evidence.PrintedBy},
		{"ช่องทาง", evidence.Channel},
		{"เครื่องพิมพ์", evidence.PrinterName},
		{"Client timezone", evidence.ClientTimezone},
		{"IP", evidence.IPAddress},
		{"User agent", truncateEvidence(evidence.UserAgent, 110)},
		{"Device id hash", evidence.DeviceIDHash},
		{"Final PDF SHA-256", evidence.FinalFileSHA256},
	}
	drawEvidenceRows(pdf, 48, 92, rows)
	return nil
}

func setupEvidenceFont(pdf *gofpdf.Fpdf) error {
	if len(notoSansThaiRegular) == 0 {
		return fmt.Errorf("thai pdf font is missing")
	}
	pdf.AddUTF8FontFromBytes("noto-thai", "", notoSansThaiRegular)
	pdf.SetFont("noto-thai", "", 11)
	return nil
}

func drawEvidenceHeader(pdf *gofpdf.Fpdf, title string) {
	pdf.SetFillColor(243, 244, 246)
	pdf.Rect(0, 0, 595.28, 62, "F")
	pdf.SetTextColor(17, 24, 39)
	pdf.SetFont("noto-thai", "", 16)
	pdf.Text(48, 38, title)
	pdf.SetFont("noto-thai", "", 9)
	pdf.SetTextColor(107, 114, 128)
	pdf.Text(48, 54, "PaperLess evidence record")
}

func drawParagraph(pdf *gofpdf.Fpdf, x, y, width float64, text string) float64 {
	pdf.SetXY(x, y)
	pdf.SetFont("noto-thai", "", 11)
	pdf.SetTextColor(31, 41, 55)
	pdf.MultiCell(width, 17, text, "1", "L", false)
	_, nextY := pdf.GetXY()
	return nextY
}

type evidenceRow struct {
	key   string
	value string
}

func drawEvidenceRows(pdf *gofpdf.Fpdf, x, y float64, rows []evidenceRow) float64 {
	keyWidth := 138.0
	valueWidth := 361.0
	lineHeight := 15.0
	for _, row := range rows {
		if y > 792 {
			pdf.AddPageFormat("P", gofpdf.SizeType{Wd: 595.28, Ht: 841.89})
			drawEvidenceHeader(pdf, "หลักฐานเพิ่มเติม")
			y = 92
		}
		value := row.value
		if strings.TrimSpace(value) == "" {
			value = "-"
		}
		pdf.SetXY(x, y)
		pdf.SetFont("noto-thai", "", 9)
		pdf.SetTextColor(75, 85, 99)
		pdf.CellFormat(keyWidth, lineHeight, row.key, "1", 0, "L", false, 0, "")
		pdf.SetFont("noto-thai", "", 9)
		pdf.SetTextColor(17, 24, 39)
		pdf.MultiCell(valueWidth, lineHeight, value, "1", "L", false)
		_, y = pdf.GetXY()
	}
	return y
}

func signedDocumentSigners(signers []models.SigningDocumentSigner) []models.SigningDocumentSigner {
	out := make([]models.SigningDocumentSigner, 0, len(signers))
	for _, signer := range signers {
		if signer.Status == "signed" && signer.SignatureFileID != "" {
			out = append(out, signer)
		}
	}
	return out
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func shortHash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "not_provided"
	}
	hash := hashSecret(value)
	if len(hash) > 16 {
		return hash[:16]
	}
	return hash
}

func truncateEvidence(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}

func formatEvidenceTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "-"
	}
	return value.Format(time.RFC3339)
}

func importedPageSize(importer *gofpdi.Importer, pageNo int) gofpdf.SizeType {
	sizes := importer.GetPageSizes()
	if page, ok := sizes[pageNo]; ok {
		if media, ok := page["/MediaBox"]; ok {
			return gofpdf.SizeType{Wd: media["w"], Ht: media["h"]}
		}
	}
	return gofpdf.SizeType{}
}

func imageTypeForContent(contentType string) string {
	if strings.Contains(strings.ToLower(contentType), "jpeg") || strings.Contains(strings.ToLower(contentType), "jpg") {
		return "jpg"
	}
	return "png"
}

func clampRatio(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
