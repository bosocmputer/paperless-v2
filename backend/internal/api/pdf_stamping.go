package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/phpdave11/gofpdf"
	"github.com/phpdave11/gofpdf/contrib/gofpdi"
)

//go:embed fonts/Sarabun-Regular.ttf
var sarabunRegular []byte

const evidenceFontFamily = "paperless-evidence"

type finalEvidencePage struct {
	Document            models.SigningDocument
	Signers             []models.SigningDocumentSigner
	SignedContentSHA256 string
	GeneratedAt         time.Time
	LegalText           string
	LegalTextVersion    string
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
	legalNotices := documentLegalNotices(document)
	signaturePlacements := documentSignaturePlacements(document)
	if len(signers) == 0 && len(legalNotices) == 0 {
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

	stamped, err := stampPDFWithSignaturePlacementsAndLegalNotices(document.OriginalFile.StoragePath, document.OriginalFile.PageCount, signers, signatureFiles, signaturePlacements, legalNotices, nil)
	if err != nil {
		return err
	}
	pageCount := document.OriginalFile.PageCount
	action := "pdf_stamped"
	message := "สร้าง PDF พร้อมลายเซ็นล่าสุดแล้ว"

	if final {
		signedContent := stamped
		legalText := signingLegalText
		legalTextVersion := signingLegalTextVersion
		if len(legalNotices) > 0 {
			legalText = firstNonEmpty(legalNotices[0].Text, signingLegalText)
			legalTextVersion = firstNonEmpty(legalNotices[0].TextVersion, signingLegalTextVersion)
		}
		evidence := finalEvidencePage{
			Document:            document,
			Signers:             signers,
			SignedContentSHA256: sha256Hex(signedContent),
			GeneratedAt:         time.Now(),
			LegalText:           legalText,
			LegalTextVersion:    legalTextVersion,
		}
		stamped, err = stampPDFWithSignaturePlacementsAndLegalNotices(document.OriginalFile.StoragePath, document.OriginalFile.PageCount, signers, signatureFiles, signaturePlacements, legalNotices, &evidence)
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
		"fileId":                       uploaded.ID,
		"fileSha256":                   uploaded.SHA256,
		"signatureCount":               len(signers),
		"signatureTransparencyVersion": signatureTransparencyVersion,
		"final":                        final,
		"legalNoticeStamped":           len(legalNotices) > 0,
		"legalNoticeDisplayVersion":    legalNoticesDisplayVersion(legalNotices),
	})
}

func legalNoticeDisplayVersion(notice *models.LegalNoticeSnapshot) string {
	if notice == nil {
		return ""
	}
	return signingLegalNoticePDFDisplayVersion
}

func legalNoticesDisplayVersion(notices []models.LegalNoticeSnapshot) string {
	if len(notices) == 0 {
		return ""
	}
	return signingLegalNoticePDFDisplayVersion
}

func documentLegalNotices(document models.SigningDocument) []models.LegalNoticeSnapshot {
	if len(document.LegalNoticeBoxes) > 0 {
		return document.LegalNoticeBoxes
	}
	if document.LegalNoticeSnapshot != nil {
		return []models.LegalNoticeSnapshot{*document.LegalNoticeSnapshot}
	}
	return nil
}

func documentSignaturePlacements(document models.SigningDocument) []models.SignaturePlacementSnapshot {
	if len(document.SignaturePlacements) > 0 {
		return document.SignaturePlacements
	}
	placements := make([]models.SignaturePlacementSnapshot, 0, len(document.Signers))
	for _, signer := range document.Signers {
		placements = append(placements, models.SignaturePlacementSnapshot{
			PositionCode:  signer.PositionCode,
			PositionName:  signer.PositionName,
			SequenceNo:    signer.SequenceNo,
			ConditionType: signer.ConditionType,
			SignerSlot:    signer.SignerSlot,
			SignerType:    signer.SignerType,
			SignerUser:    signer.SignerUser,
			SignerName:    signer.SignerName,
			PageNo:        signer.PageNo,
			XRatio:        signer.XRatio,
			YRatio:        signer.YRatio,
			WidthRatio:    signer.WidthRatio,
			HeightRatio:   signer.HeightRatio,
			Label:         signer.Label,
		})
	}
	return placements
}

func signingPlacementsForSigners(signers []models.SigningDocumentSigner, placements []models.SignaturePlacementSnapshot) []models.SigningDocumentSigner {
	if len(signers) == 0 {
		return nil
	}
	if len(placements) == 0 {
		return signers
	}
	out := []models.SigningDocumentSigner{}
	seen := map[string]bool{}
	for _, signer := range signers {
		matched := false
		for _, placement := range placements {
			if !signaturePlacementMatchesSigner(placement, signer) {
				continue
			}
			stamp := signer
			stamp.SignerSlot = placement.SignerSlot
			stamp.PageNo = placement.PageNo
			stamp.XRatio = placement.XRatio
			stamp.YRatio = placement.YRatio
			stamp.WidthRatio = placement.WidthRatio
			stamp.HeightRatio = placement.HeightRatio
			stamp.Label = placement.Label
			key := signatureStampKey(stamp)
			if seen[key] {
				matched = true
				continue
			}
			seen[key] = true
			out = append(out, stamp)
			matched = true
		}
		if !matched {
			key := signatureStampKey(signer)
			if !seen[key] {
				seen[key] = true
				out = append(out, signer)
			}
		}
	}
	return out
}

func signatureStampKey(signer models.SigningDocumentSigner) string {
	return fmt.Sprintf("%s:%d:%.6f:%.6f:%.6f:%.6f", signer.SignatureFileID, signer.PageNo, signer.XRatio, signer.YRatio, signer.WidthRatio, signer.HeightRatio)
}

func signaturePlacementMatchesSigner(placement models.SignaturePlacementSnapshot, signer models.SigningDocumentSigner) bool {
	if !strings.EqualFold(strings.TrimSpace(placement.PositionCode), strings.TrimSpace(signer.PositionCode)) {
		return false
	}
	switch signer.ConditionType {
	case 1:
		return strings.EqualFold(strings.TrimSpace(placement.SignerType), "any") || placement.SignerSlot == signer.SignerSlot
	case 2:
		return strings.EqualFold(strings.TrimSpace(placement.SignerType), "internal") &&
			strings.EqualFold(strings.TrimSpace(placement.SignerUser), strings.TrimSpace(signer.SignerUser))
	case 3:
		return strings.EqualFold(strings.TrimSpace(placement.SignerType), "external")
	default:
		return placement.SignerSlot == signer.SignerSlot
	}
}

func stampPDFWithSignatures(sourcePath string, pageCount int, signers []models.SigningDocumentSigner, signatureFiles map[string]models.UploadedFile) ([]byte, error) {
	return renderSignedPDF(sourcePath, pageCount, signers, signatureFiles, nil, nil)
}

func stampPDFWithSignaturesAndLegalNotice(sourcePath string, pageCount int, signers []models.SigningDocumentSigner, signatureFiles map[string]models.UploadedFile, legalNotice *models.LegalNoticeSnapshot) ([]byte, error) {
	return renderSignedPDF(sourcePath, pageCount, signers, signatureFiles, legalNotice, nil)
}

func stampPDFWithSignaturesAndEvidence(sourcePath string, pageCount int, signers []models.SigningDocumentSigner, signatureFiles map[string]models.UploadedFile, legalNotice *models.LegalNoticeSnapshot, evidence finalEvidencePage) ([]byte, error) {
	return renderSignedPDF(sourcePath, pageCount, signers, signatureFiles, legalNotice, &evidence)
}

func renderSignedPDF(sourcePath string, pageCount int, signers []models.SigningDocumentSigner, signatureFiles map[string]models.UploadedFile, legalNotice *models.LegalNoticeSnapshot, evidence *finalEvidencePage) ([]byte, error) {
	legalNotices := []models.LegalNoticeSnapshot{}
	if legalNotice != nil {
		legalNotices = append(legalNotices, *legalNotice)
	}
	return stampPDFWithSignaturePlacementsAndLegalNotices(sourcePath, pageCount, signers, signatureFiles, nil, legalNotices, evidence)
}

func stampPDFWithSignaturePlacementsAndLegalNotices(sourcePath string, pageCount int, signers []models.SigningDocumentSigner, signatureFiles map[string]models.UploadedFile, placements []models.SignaturePlacementSnapshot, legalNotices []models.LegalNoticeSnapshot, evidence *finalEvidencePage) ([]byte, error) {
	if pageCount <= 0 {
		return nil, fmt.Errorf("pdf page count is missing")
	}
	normalizedSignatureFiles, cleanup, err := prepareSignatureFilesForPDF(signatureFiles)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	pdf := gofpdf.New("P", "pt", "A4", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCompression(true)
	if len(legalNotices) > 0 || evidence != nil {
		if err := setupEvidenceFont(pdf); err != nil {
			return nil, err
		}
	}

	importer := gofpdi.NewImporter()
	stampSigners := signingPlacementsForSigners(signers, placements)
	signersByPage := map[int][]models.SigningDocumentSigner{}
	for _, signer := range stampSigners {
		signersByPage[signer.PageNo] = append(signersByPage[signer.PageNo], signer)
	}
	legalNoticesByPage := map[int][]models.LegalNoticeSnapshot{}
	for _, notice := range legalNotices {
		legalNoticesByPage[notice.PageNo] = append(legalNoticesByPage[notice.PageNo], notice)
	}

	if err := importPDFPages(pdf, importer, sourcePath, pageCount, func(pageNo int, size gofpdf.SizeType) {
		for _, notice := range legalNoticesByPage[pageNo] {
			drawLegalNoticeBox(pdf, notice, size)
		}
		for _, signer := range signersByPage[pageNo] {
			file := normalizedSignatureFiles[signer.SignatureFileID]
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

func prepareSignatureFilesForPDF(files map[string]models.UploadedFile) (map[string]models.UploadedFile, func(), error) {
	if len(files) == 0 {
		return map[string]models.UploadedFile{}, func() {}, nil
	}
	normalized := make(map[string]models.UploadedFile, len(files))
	tempPaths := []string{}
	cleanup := func() {
		for _, path := range tempPaths {
			_ = os.Remove(path)
		}
	}
	for id, file := range files {
		if strings.TrimSpace(file.StoragePath) == "" {
			continue
		}
		data, err := os.ReadFile(file.StoragePath)
		if err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("read signature image: %w", err)
		}
		normalizedData, err := normalizeSignatureImage(data)
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
		tempFile, err := os.CreateTemp("", "paperless-signature-*.png")
		if err != nil {
			cleanup()
			return nil, func() {}, err
		}
		tempPath := tempFile.Name()
		if _, err := tempFile.Write(normalizedData); err != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
			cleanup()
			return nil, func() {}, err
		}
		if err := tempFile.Close(); err != nil {
			_ = os.Remove(tempPath)
			cleanup()
			return nil, func() {}, err
		}
		tempPaths = append(tempPaths, tempPath)
		file.StoragePath = tempPath
		file.ContentType = "image/png"
		normalized[id] = file
	}
	return normalized, cleanup, nil
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

func drawLegalNoticeBox(pdf *gofpdf.Fpdf, notice models.LegalNoticeSnapshot, size gofpdf.SizeType) {
	text := legalNoticeDisplayText(firstNonEmpty(notice.Text, signingLegalText))
	x := clampRatio(notice.XRatio) * size.Wd
	y := clampRatio(notice.YRatio) * size.Ht
	w := clampRatio(notice.WidthRatio) * size.Wd
	h := clampRatio(notice.HeightRatio) * size.Ht
	if w <= 0 || h <= 0 {
		return
	}
	padding := 5.0
	textWidth := maxFloat(8, w-(padding*2))
	textHeight := maxFloat(8, h-(padding*2))
	fontSize, lineHeight, lines := fitLegalNoticeText(pdf, text, textWidth, textHeight)
	contentHeight := float64(len(lines)) * lineHeight
	textY := y + padding + maxFloat(0, (textHeight-contentHeight)/2)

	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(156, 163, 175)
	pdf.SetLineWidth(0.8)
	pdf.Rect(x, y, w, h, "FD")
	pdf.SetTextColor(17, 24, 39)
	pdf.SetFont(evidenceFontFamily, "", fontSize)
	for _, line := range lines {
		pdf.SetXY(x+padding, textY)
		pdf.CellFormat(textWidth, lineHeight, line, "", 0, "C", false, 0, "")
		textY += lineHeight
	}
}

func legalNoticeDisplayText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || text == signingLegalText {
		return "เอกสารนี้จัดทำและลงนามในรูปแบบอิเล็กทรอนิกส์ตาม พระราชบัญญัติธุรกรรมทางอิเล็กทรอนิกส์ พุทธศักราช ๒๕๔๔ ผู้ลงนามยืนยันความถูกต้องของเนื้อหาและยอมรับผลผูกพันทางกฎหมายทุกประการ"
	}
	return strings.NewReplacer(
		"พ.ร.บ.", "พระราชบัญญัติ",
		"พ.ศ.", "พุทธศักราช",
		"2544", "๒๕๔๔",
		".", "",
	).Replace(text)
}

func fitLegalNoticeText(pdf *gofpdf.Fpdf, text string, width, height float64) (float64, float64, []string) {
	for fontSize := 11.0; fontSize >= 7.0; fontSize -= 0.5 {
		lineHeight := fontSize * 1.45
		pdf.SetFont(evidenceFontFamily, "", fontSize)
		lines := wrapLegalNoticeText(pdf, text, width)
		if float64(len(lines))*lineHeight <= height {
			return fontSize, lineHeight, lines
		}
	}
	fontSize := 7.0
	lineHeight := fontSize * 1.35
	pdf.SetFont(evidenceFontFamily, "", fontSize)
	return fontSize, lineHeight, wrapLegalNoticeText(pdf, text, width)
}

func wrapLegalNoticeText(pdf *gofpdf.Fpdf, text string, width float64) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{}
	}
	lines := []string{}
	current := ""
	for _, word := range words {
		if pdf.GetStringWidth(word) > width {
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			lines = append(lines, splitLongLegalNoticeWord(pdf, word, width)...)
			continue
		}
		next := word
		if current != "" {
			next = current + " " + word
		}
		if current != "" && pdf.GetStringWidth(next) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current = next
	}
	if current != "" {
		lines = append(lines, current)
	}
	if len(lines) == 0 {
		return []string{text}
	}
	return lines
}

func splitLongLegalNoticeWord(pdf *gofpdf.Fpdf, word string, width float64) []string {
	lines := []string{}
	current := ""
	for _, r := range []rune(word) {
		next := current + string(r)
		if current != "" && pdf.GetStringWidth(next) > width {
			lines = append(lines, current)
			current = string(r)
			continue
		}
		current = next
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
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
	pdf.AddPageFormat("P", gofpdf.SizeType{Wd: 595.28, Ht: 841.89})
	drawEvidenceHeader(pdf, "หลักฐานการลงนามอิเล็กทรอนิกส์")
	y := 92.0
	y = drawParagraph(pdf, 48, y, 499, legalNoticeDisplayText(firstNonEmpty(evidence.LegalText, signingLegalText)))
	y += 12
	rows := []evidenceRow{
		{"เลขที่เอกสาร", evidence.Document.DocNo},
		{"รูปแบบเอกสาร", evidence.Document.DocFormatCode},
		{"รหัสเอกสาร PaperLess", evidence.Document.ID},
		{"วันที่เซ็นครบ", formatEvidenceTime(evidence.Document.CompletedAt)},
		{"สร้างหลักฐานเมื่อ", formatEvidenceTime(&evidence.GeneratedAt)},
		{"รุ่นข้อความกฎหมาย", firstNonEmpty(evidence.LegalTextVersion, signingLegalTextVersion)},
		{"SHA-256 เนื้อหาเอกสารที่เซ็น", evidence.SignedContentSHA256},
	}
	y = drawEvidenceRows(pdf, 48, y, rows)
	y += 14
	pdf.SetFont(evidenceFontFamily, "", 12)
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
			{"ไอพี", signer.IPAddress},
			{"ข้อมูลเบราว์เซอร์", truncateEvidence(signer.UserAgent, 110)},
			{"รหัสอุปกรณ์ (hash)", shortHash(signer.DeviceID)},
			{"ยืนยันข้อความกฎหมาย", "ใช่"},
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
		{"รหัสเอกสาร PaperLess", evidence.Document.ID},
		{"วันที่พิมพ์", formatEvidenceTime(&evidence.PrintedAt)},
		{"ผู้สร้างไฟล์พิมพ์", evidence.PrintedBy},
		{"ช่องทาง", evidence.Channel},
		{"เครื่องพิมพ์", evidence.PrinterName},
		{"เขตเวลาเครื่องผู้ใช้", evidence.ClientTimezone},
		{"ไอพี", evidence.IPAddress},
		{"ข้อมูลเบราว์เซอร์", truncateEvidence(evidence.UserAgent, 110)},
		{"รหัสอุปกรณ์ (hash)", evidence.DeviceIDHash},
		{"SHA-256 ไฟล์ final", evidence.FinalFileSHA256},
	}
	drawEvidenceRows(pdf, 48, 92, rows)
	return nil
}

func setupEvidenceFont(pdf *gofpdf.Fpdf) error {
	if len(sarabunRegular) == 0 {
		return fmt.Errorf("thai/latin pdf font is missing")
	}
	pdf.AddUTF8FontFromBytes(evidenceFontFamily, "", sarabunRegular)
	pdf.SetFont(evidenceFontFamily, "", 11)
	return nil
}

func drawEvidenceHeader(pdf *gofpdf.Fpdf, title string) {
	pdf.SetFillColor(243, 244, 246)
	pdf.Rect(0, 0, 595.28, 62, "F")
	pdf.SetTextColor(17, 24, 39)
	pdf.SetFont(evidenceFontFamily, "", 16)
	pdf.Text(48, 38, title)
	pdf.SetFont(evidenceFontFamily, "", 9)
	pdf.SetTextColor(107, 114, 128)
	pdf.Text(48, 54, "บันทึกหลักฐานโดยระบบ PaperLess")
}

func drawParagraph(pdf *gofpdf.Fpdf, x, y, width float64, text string) float64 {
	pdf.SetXY(x, y)
	pdf.SetFont(evidenceFontFamily, "", 11)
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
		pdf.SetFont(evidenceFontFamily, "", 9)
		pdf.SetTextColor(75, 85, 99)
		pdf.CellFormat(keyWidth, lineHeight, row.key, "1", 0, "L", false, 0, "")
		pdf.SetFont(evidenceFontFamily, "", 9)
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
	loc, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		loc = time.FixedZone("ICT", 7*60*60)
	}
	bangkok := value.In(loc)
	return fmt.Sprintf(
		"%02d/%02d/%04d %02d:%02d:%02d น. (Asia/Bangkok)",
		bangkok.Day(),
		bangkok.Month(),
		bangkok.Year()+543,
		bangkok.Hour(),
		bangkok.Minute(),
		bangkok.Second(),
	)
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

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
