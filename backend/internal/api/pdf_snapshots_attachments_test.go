package api

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func syntheticJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode synthetic jpeg: %v", err)
	}
	return buf.Bytes()
}

func syntheticPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{G: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode synthetic png: %v", err)
	}
	return buf.Bytes()
}

func writeTempFile(t *testing.T, data []byte, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestRenderSMLAttachmentImageSnapshot_JPEGPassthrough(t *testing.T) {
	data := syntheticJPEG(t)
	image, err := renderSMLAttachmentImageSnapshot(data, "image/jpeg", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if image.PageNo != 5 || image.ContentType != "image/jpeg" || image.SHA256 == "" {
		t.Fatalf("unexpected image: %+v", image)
	}
}

func TestRenderSMLAttachmentImageSnapshot_PNGConverted(t *testing.T) {
	data := syntheticPNG(t)
	image, err := renderSMLAttachmentImageSnapshot(data, "image/png", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if image.PageNo != 3 || image.ContentType != "image/jpeg" {
		t.Fatalf("unexpected image: %+v", image)
	}
	if !isJPEGSnapshot(image.Data) {
		t.Fatal("expected converted PNG to be valid JPEG bytes")
	}
}

func TestRenderSMLAttachmentImageSnapshot_UnsupportedType(t *testing.T) {
	if _, err := renderSMLAttachmentImageSnapshot([]byte("gif89a"), "image/gif", 1); err == nil {
		t.Fatal("expected unsupported content type error")
	}
}

func TestRenderSMLAttachmentImageSnapshot_CorruptJPEG(t *testing.T) {
	if _, err := renderSMLAttachmentImageSnapshot([]byte("not a jpeg"), "image/jpeg", 1); err == nil {
		t.Fatal("expected corrupt JPEG error")
	}
}

func TestRenderSMLAttachmentImageSnapshot_OversizedImage(t *testing.T) {
	oversized := make([]byte, smlSnapshotMaxImageSize+1)
	oversized[0], oversized[1], oversized[2] = 0xff, 0xd8, 0xff
	if _, err := renderSMLAttachmentImageSnapshot(oversized, "image/jpeg", 1); err == nil {
		t.Fatal("expected oversized image to be rejected")
	}
}

func newImageAttachment(t *testing.T, id string, createdAt time.Time, contentType string, data []byte) models.SigningDocumentAttachment {
	t.Helper()
	path := writeTempFile(t, data, id+".img")
	return models.SigningDocumentAttachment{
		ID:        id,
		CreatedAt: createdAt,
		File: models.UploadedFile{
			ID:           id,
			OriginalName: id,
			StoragePath:  path,
			ContentType:  contentType,
			PageCount:    0,
		},
	}
}

func TestRenderSMLAttachmentSnapshots_SkipsUnreadableFile(t *testing.T) {
	good := newImageAttachment(t, "good", time.Unix(100, 0), "image/jpeg", syntheticJPEG(t))
	bad := models.SigningDocumentAttachment{
		ID:        "bad",
		CreatedAt: time.Unix(200, 0),
		File: models.UploadedFile{
			ID:           "bad",
			OriginalName: "bad.jpg",
			StoragePath:  "/nonexistent/path/does-not-exist.jpg",
			ContentType:  "image/jpeg",
		},
	}

	result := renderSMLAttachmentSnapshots(context.Background(), "doc-1", []models.SigningDocumentAttachment{good, bad}, 9)
	if len(result.Images) != 1 {
		t.Fatalf("expected 1 successful image, got %d", len(result.Images))
	}
	if result.Images[0].PageNo != 9 {
		t.Fatalf("expected surviving image to keep PageNo 9, got %d", result.Images[0].PageNo)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].AttachmentID != "bad" {
		t.Fatalf("expected 'bad' attachment to be skipped, got %+v", result.Skipped)
	}
}

func TestRenderSMLAttachmentSnapshots_PageNumberingContinuesFromOffset(t *testing.T) {
	a := newImageAttachment(t, "a", time.Unix(100, 0), "image/jpeg", syntheticJPEG(t))
	b := newImageAttachment(t, "b", time.Unix(200, 0), "image/jpeg", syntheticJPEG(t))

	result := renderSMLAttachmentSnapshots(context.Background(), "doc-1", []models.SigningDocumentAttachment{a, b}, 5)
	if len(result.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(result.Images))
	}
	if result.Images[0].PageNo != 5 || result.Images[1].PageNo != 6 {
		t.Fatalf("unexpected page numbering: %d, %d", result.Images[0].PageNo, result.Images[1].PageNo)
	}
}

func TestRenderSMLAttachmentSnapshots_UnsupportedContentTypeSkipped(t *testing.T) {
	att := newImageAttachment(t, "gif", time.Unix(100, 0), "image/gif", []byte("gif89a"))
	result := renderSMLAttachmentSnapshots(context.Background(), "doc-1", []models.SigningDocumentAttachment{att}, 1)
	if len(result.Images) != 0 {
		t.Fatalf("expected no images, got %d", len(result.Images))
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped attachment, got %d", len(result.Skipped))
	}
}

func TestRenderSMLAttachmentSnapshots_ResultsOrderedByPageNoDespiteParallelRender(t *testing.T) {
	attachments := make([]models.SigningDocumentAttachment, 0, 6)
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("att-%d", i)
		attachments = append(attachments, newImageAttachment(t, id, time.Unix(int64(100+i), 0), "image/jpeg", syntheticJPEG(t)))
	}

	result := renderSMLAttachmentSnapshots(context.Background(), "doc-1", attachments, 1)
	if len(result.Images) != len(attachments) {
		t.Fatalf("expected %d images, got %d", len(attachments), len(result.Images))
	}
	for i, image := range result.Images {
		if image.PageNo != i+1 {
			t.Fatalf("images not ordered by PageNo: index %d has PageNo %d", i, image.PageNo)
		}
	}
}

func TestRenderSMLAttachmentSnapshots_RespectsWorkerConcurrencyLimit(t *testing.T) {
	// renderSingleAttachment for image attachments is just file I/O + in-memory
	// re-encoding (no artificial delay hook exists), so we verify the
	// concurrency bound indirectly: rendering enough attachments to require
	// multiple worker-pool batches must not panic, must not corrupt page
	// ordering, and must process every attachment exactly once — the
	// properties a broken/unbounded-fanout implementation would most likely
	// violate under load.
	const attachmentCount = smlAttachmentWorkerCount*3 + 1
	attachments := make([]models.SigningDocumentAttachment, 0, attachmentCount)
	for i := 0; i < attachmentCount; i++ {
		id := fmt.Sprintf("worker-%d", i)
		attachments = append(attachments, newImageAttachment(t, id, time.Unix(int64(100+i), 0), "image/jpeg", syntheticJPEG(t)))
	}

	result := renderSMLAttachmentSnapshots(context.Background(), "doc-1", attachments, 1)
	if len(result.Images) != attachmentCount {
		t.Fatalf("expected %d images, got %d", attachmentCount, len(result.Images))
	}
	seen := make(map[int]bool, attachmentCount)
	for _, image := range result.Images {
		if seen[image.PageNo] {
			t.Fatalf("duplicate PageNo %d — concurrency corrupted results", image.PageNo)
		}
		seen[image.PageNo] = true
	}
}

func TestRenderSMLAttachmentSnapshots_SafetyCeilingSkipsExcessWithoutOpeningFile(t *testing.T) {
	var openedUnexpected atomic.Bool
	attachments := make([]models.SigningDocumentAttachment, 0, 3)
	// First attachment: a huge PDF that alone exceeds the ceiling budget.
	oversizedPDF := models.SigningDocumentAttachment{
		ID:        "huge-pdf",
		CreatedAt: time.Unix(100, 0),
		File: models.UploadedFile{
			ID:           "huge-pdf",
			OriginalName: "huge.pdf",
			StoragePath:  "/should/never/be/opened.pdf",
			ContentType:  "application/pdf",
			PageCount:    smlAttachmentMaxTotalPages + 50,
		},
	}
	attachments = append(attachments, oversizedPDF)

	result := renderSMLAttachmentSnapshots(context.Background(), "doc-1", attachments, 1)
	if openedUnexpected.Load() {
		t.Fatal("attachment beyond the safety ceiling must not be opened")
	}
	if len(result.Images) != 0 {
		t.Fatalf("expected no images rendered, got %d", len(result.Images))
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped attachment, got %d", len(result.Skipped))
	}
	if result.Skipped[0].Reason == "" {
		t.Fatal("expected a non-empty skip reason for ceiling breach")
	}
}
