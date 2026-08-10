package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

const (
	smlSnapshotMaxImageSize    = 4 << 20
	smlAttachmentMaxTotalPages = 200
	smlAttachmentWorkerCount   = 3
	smlAttachmentJPEGQuality   = 82
)

type pdfSnapshotRenderProfile struct {
	DPI     int
	Quality int
}

type smlDocumentImageSnapshot struct {
	PageNo      int
	ContentType string
	SHA256      string
	Data        []byte
}

type pdfSnapshotRenderResult struct {
	Images     []smlDocumentImageSnapshot
	PageCount  int
	TotalPages int
	Truncated  bool
	TotalBytes int
	Profile    pdfSnapshotRenderProfile
	Elapsed    time.Duration
}

var pdfSnapshotProfiles = []pdfSnapshotRenderProfile{
	{DPI: 144, Quality: 82},
	{DPI: 110, Quality: 75},
}

var errSnapshotTooLarge = errors.New("rendered PDF snapshots are too large")

func renderSMLDocumentSnapshots(ctx context.Context, pdfPath string, originalPageCount int) (pdfSnapshotRenderResult, error) {
	if originalPageCount <= 0 {
		return pdfSnapshotRenderResult{}, fmt.Errorf("original PDF page count is missing")
	}
	if strings.TrimSpace(pdfPath) == "" {
		return pdfSnapshotRenderResult{}, fmt.Errorf("final PDF path is missing")
	}

	start := time.Now()
	images, totalBytes, profile, err := runSnapshotProfiles(ctx, pdfPath, originalPageCount, 1)
	if err != nil {
		return pdfSnapshotRenderResult{}, err
	}
	return pdfSnapshotRenderResult{
		Images:     images,
		PageCount:  originalPageCount,
		TotalPages: originalPageCount,
		Truncated:  false,
		TotalBytes: totalBytes,
		Profile:    profile,
		Elapsed:    time.Since(start),
	}, nil
}

// runSnapshotProfiles renders pageCount pages of pdfPath starting at startPageNo,
// retrying across pdfSnapshotProfiles (progressively lower DPI/quality) only when
// the rendered output is too large per errSnapshotTooLarge.
func runSnapshotProfiles(ctx context.Context, pdfPath string, pageCount int, startPageNo int) ([]smlDocumentImageSnapshot, int, pdfSnapshotRenderProfile, error) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return nil, 0, pdfSnapshotRenderProfile{}, fmt.Errorf("pdftoppm not found; install Poppler")
	}

	var lastErr error
	for _, profile := range pdfSnapshotProfiles {
		images, totalBytes, err := renderSMLDocumentSnapshotsWithProfile(ctx, pdfPath, pageCount, profile, startPageNo)
		if err == nil {
			return images, totalBytes, profile, nil
		}
		lastErr = err
		if !errors.Is(err, errSnapshotTooLarge) {
			break
		}
	}
	return nil, 0, pdfSnapshotRenderProfile{}, lastErr
}

func renderSMLDocumentSnapshotsWithProfile(ctx context.Context, pdfPath string, pageCount int, profile pdfSnapshotRenderProfile, startPageNo int) ([]smlDocumentImageSnapshot, int, error) {
	tempDir, err := os.MkdirTemp("", "paperless-sml-images-*")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(tempDir)

	outputPrefix := filepath.Join(tempDir, "page")
	args := []string{
		"-f", "1",
		"-l", fmt.Sprintf("%d", pageCount),
		"-r", fmt.Sprintf("%d", profile.DPI),
		"-jpeg",
		"-jpegopt", fmt.Sprintf("quality=%d", profile.Quality),
		pdfPath,
		outputPrefix,
	}
	cmd := exec.CommandContext(ctx, "pdftoppm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, 0, fmt.Errorf("pdftoppm failed: %s", truncateForMetadata(string(output), 300))
	}

	files, err := filepath.Glob(outputPrefix + "-*.jpg")
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(files)
	if len(files) != pageCount {
		return nil, 0, fmt.Errorf("pdftoppm rendered %d pages, expected %d", len(files), pageCount)
	}

	images := make([]smlDocumentImageSnapshot, 0, len(files))
	totalBytes := 0
	for i, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, 0, err
		}
		totalBytes += len(data)
		if len(data) == 0 || len(data) > smlSnapshotMaxImageSize {
			return nil, 0, errSnapshotTooLarge
		}
		if !isJPEGSnapshot(data) {
			return nil, 0, fmt.Errorf("rendered page %d is not JPEG", i+1)
		}
		sum := sha256.Sum256(data)
		images = append(images, smlDocumentImageSnapshot{
			PageNo:      startPageNo + i,
			ContentType: "image/jpeg",
			SHA256:      hex.EncodeToString(sum[:]),
			Data:        data,
		})
	}
	return images, totalBytes, nil
}

// renderSMLAttachmentPDFSnapshots renders every page of a PDF attachment,
// continuing page numbering from startPageNo.
func renderSMLAttachmentPDFSnapshots(ctx context.Context, pdfPath string, pageCount int, startPageNo int) ([]smlDocumentImageSnapshot, error) {
	if pageCount <= 0 {
		return nil, fmt.Errorf("attachment PDF page count is missing")
	}
	images, _, _, err := runSnapshotProfiles(ctx, pdfPath, pageCount, startPageNo)
	if err != nil {
		return nil, err
	}
	return images, nil
}

// renderSMLAttachmentImageSnapshot turns a single JPEG or PNG attachment into
// one SML page image. PNG is decoded and re-encoded as JPEG since SML expects
// JPEG content.
func renderSMLAttachmentImageSnapshot(data []byte, contentType string, pageNo int) (smlDocumentImageSnapshot, error) {
	var jpegData []byte
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg":
		if !isJPEGSnapshot(data) {
			return smlDocumentImageSnapshot{}, fmt.Errorf("attachment declared as JPEG but is not a valid JPEG")
		}
		jpegData = data
	case "image/png":
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return smlDocumentImageSnapshot{}, fmt.Errorf("cannot decode PNG attachment: %w", err)
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: smlAttachmentJPEGQuality}); err != nil {
			return smlDocumentImageSnapshot{}, fmt.Errorf("cannot convert PNG attachment to JPEG: %w", err)
		}
		jpegData = buf.Bytes()
	default:
		return smlDocumentImageSnapshot{}, fmt.Errorf("unsupported attachment content type: %s", contentType)
	}

	if len(jpegData) == 0 || len(jpegData) > smlSnapshotMaxImageSize {
		return smlDocumentImageSnapshot{}, errSnapshotTooLarge
	}
	sum := sha256.Sum256(jpegData)
	return smlDocumentImageSnapshot{
		PageNo:      pageNo,
		ContentType: "image/jpeg",
		SHA256:      hex.EncodeToString(sum[:]),
		Data:        jpegData,
	}, nil
}

type attachmentSkipReason struct {
	AttachmentID string `json:"attachmentId"`
	Filename     string `json:"filename"`
	Reason       string `json:"reason"`
}

type attachmentSnapshotResult struct {
	Images        []smlDocumentImageSnapshot
	PagesIncluded int
	Skipped       []attachmentSkipReason
	Elapsed       time.Duration
}

type attachmentRenderPlanItem struct {
	Attachment models.SigningDocumentAttachment
	StartPage  int
	PageCount  int
}

type attachmentRenderOutcome struct {
	Index  int
	Images []smlDocumentImageSnapshot
	Err    error
}

// renderSMLAttachmentSnapshots renders every page of every attachment (PDF
// pages or single images), continuing page numbering from startPageNo. It
// pre-plans page ranges from DB-known page counts (no file I/O), enforces
// smlAttachmentMaxTotalPages as a safety ceiling against runaway uploads, then
// renders the remaining attachments concurrently (bounded by
// smlAttachmentWorkerCount) while keeping page numbers deterministic. Any
// single attachment that fails to render is skipped and logged; it never
// fails the whole batch.
func renderSMLAttachmentSnapshots(ctx context.Context, documentID string, attachments []models.SigningDocumentAttachment, startPageNo int) attachmentSnapshotResult {
	start := time.Now()
	result := attachmentSnapshotResult{}

	plan := make([]attachmentRenderPlanItem, 0, len(attachments))
	nextPage := startPageNo
	total := startPageNo - 1
	for _, attachment := range attachments {
		pageCount := attachment.File.PageCount
		if !strings.EqualFold(strings.TrimSpace(attachment.File.ContentType), "application/pdf") {
			pageCount = 1
		}
		if pageCount <= 0 {
			pageCount = 1
		}
		if total+pageCount > smlAttachmentMaxTotalPages {
			reason := attachmentSkipReason{
				AttachmentID: attachment.ID,
				Filename:     attachment.File.OriginalName,
				Reason:       fmt.Sprintf("เกินขีดจำกัดความปลอดภัยรวม %d หน้า", smlAttachmentMaxTotalPages),
			}
			result.Skipped = append(result.Skipped, reason)
			slog.Warn("SML attachment render skipped", "documentId", documentID, "attachmentId", reason.AttachmentID, "filename", reason.Filename, "reason", reason.Reason)
			continue
		}
		plan = append(plan, attachmentRenderPlanItem{Attachment: attachment, StartPage: nextPage, PageCount: pageCount})
		nextPage += pageCount
		total += pageCount
	}

	if len(plan) == 0 {
		result.Elapsed = time.Since(start)
		return result
	}

	outcomes := make([]attachmentRenderOutcome, len(plan))
	sem := make(chan struct{}, smlAttachmentWorkerCount)
	var wg sync.WaitGroup
	for i, item := range plan {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, item attachmentRenderPlanItem) {
			defer wg.Done()
			defer func() { <-sem }()
			images, err := renderSingleAttachment(ctx, item)
			outcomes[idx] = attachmentRenderOutcome{Index: idx, Images: images, Err: err}
		}(i, item)
	}
	wg.Wait()

	for i, outcome := range outcomes {
		item := plan[i]
		if outcome.Err != nil {
			reason := attachmentSkipReason{
				AttachmentID: item.Attachment.ID,
				Filename:     item.Attachment.File.OriginalName,
				Reason:       truncateForMetadata(outcome.Err.Error(), 300),
			}
			result.Skipped = append(result.Skipped, reason)
			slog.Warn("SML attachment render skipped", "documentId", documentID, "attachmentId", reason.AttachmentID, "filename", reason.Filename, "reason", reason.Reason)
			continue
		}
		result.Images = append(result.Images, outcome.Images...)
		result.PagesIncluded += len(outcome.Images)
	}

	result.Elapsed = time.Since(start)
	return result
}

func renderSingleAttachment(ctx context.Context, item attachmentRenderPlanItem) ([]smlDocumentImageSnapshot, error) {
	attachment := item.Attachment
	contentType := strings.ToLower(strings.TrimSpace(attachment.File.ContentType))

	if contentType == "application/pdf" {
		return renderSMLAttachmentPDFSnapshots(ctx, attachment.File.StoragePath, item.PageCount, item.StartPage)
	}

	data, err := os.ReadFile(attachment.File.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read attachment file: %w", err)
	}
	image, err := renderSMLAttachmentImageSnapshot(data, attachment.File.ContentType, item.StartPage)
	if err != nil {
		return nil, err
	}
	return []smlDocumentImageSnapshot{image}, nil
}

func isJPEGSnapshot(data []byte) bool {
	return len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff
}
