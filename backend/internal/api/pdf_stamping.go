package api

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/phpdave11/gofpdf"
	"github.com/phpdave11/gofpdf/contrib/gofpdi"
)

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
	name := fmt.Sprintf("%s-stamped-v%d.pdf", strings.TrimSuffix(filepath.Base(document.OriginalFile.OriginalName), filepath.Ext(document.OriginalFile.OriginalName)), document.CurrentVersion+1)
	uploaded, err := s.storeUploadedBytes(ctx, stamped, name, "signed-document.pdf", "application/pdf", ".pdf", document.OriginalFile.PageCount, document.CreatedBy)
	if err != nil {
		return err
	}
	if err := s.store.UpdateSigningDocumentPDF(ctx, document.ID, uploaded, final); err != nil {
		return err
	}
	action := "pdf_stamped"
	message := "สร้าง PDF พร้อมลายเซ็นล่าสุดแล้ว"
	if final {
		action = "final_pdf_stamped"
		message = "สร้าง Final PDF พร้อมลายเซ็นครบแล้ว"
	}
	return s.store.AddSigningEvent(ctx, document.ID, "", "", action, message, "", "", map[string]any{
		"fileId":         uploaded.ID,
		"signatureCount": len(signers),
		"final":          final,
	})
}

func stampPDFWithSignatures(sourcePath string, pageCount int, signers []models.SigningDocumentSigner, signatureFiles map[string]models.UploadedFile) ([]byte, error) {
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

	for pageNo := 1; pageNo <= pageCount; pageNo++ {
		tpl := importer.ImportPage(pdf, sourcePath, pageNo, "/MediaBox")
		size := importedPageSize(importer, pageNo)
		if size.Wd <= 0 || size.Ht <= 0 {
			return nil, fmt.Errorf("cannot read pdf page size for page %d", pageNo)
		}
		orientation := "P"
		if size.Wd > size.Ht {
			orientation = "L"
		}
		pdf.AddPageFormat(orientation, size)
		importer.UseImportedTemplate(pdf, tpl, 0, 0, size.Wd, size.Ht)

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
	}
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}
	if pdf.Err() {
		return nil, pdf.Error()
	}
	return out.Bytes(), nil
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
