package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	smlSnapshotMaxPages     = 8
	smlSnapshotMaxImageSize = 4 << 20
	smlSnapshotMaxRawSize   = 16 << 20
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
	pageCount, truncated, err := smlSnapshotPageCount(originalPageCount)
	if err != nil {
		return pdfSnapshotRenderResult{}, err
	}
	if strings.TrimSpace(pdfPath) == "" {
		return pdfSnapshotRenderResult{}, fmt.Errorf("final PDF path is missing")
	}
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return pdfSnapshotRenderResult{}, fmt.Errorf("pdftoppm not found; install Poppler")
	}

	start := time.Now()
	var lastErr error
	for _, profile := range pdfSnapshotProfiles {
		images, totalBytes, err := renderSMLDocumentSnapshotsWithProfile(ctx, pdfPath, pageCount, profile)
		if err == nil {
			return pdfSnapshotRenderResult{
				Images:     images,
				PageCount:  pageCount,
				TotalPages: originalPageCount,
				Truncated:  truncated,
				TotalBytes: totalBytes,
				Profile:    profile,
				Elapsed:    time.Since(start),
			}, nil
		}
		lastErr = err
		if !errors.Is(err, errSnapshotTooLarge) {
			break
		}
	}
	return pdfSnapshotRenderResult{}, lastErr
}

func smlSnapshotPageCount(originalPageCount int) (int, bool, error) {
	if originalPageCount <= 0 {
		return 0, false, fmt.Errorf("original PDF page count is missing")
	}
	if originalPageCount > smlSnapshotMaxPages {
		return smlSnapshotMaxPages, true, nil
	}
	return originalPageCount, false, nil
}

func renderSMLDocumentSnapshotsWithProfile(ctx context.Context, pdfPath string, pageCount int, profile pdfSnapshotRenderProfile) ([]smlDocumentImageSnapshot, int, error) {
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
		if len(data) == 0 || len(data) > smlSnapshotMaxImageSize || totalBytes > smlSnapshotMaxRawSize {
			return nil, 0, errSnapshotTooLarge
		}
		if !isJPEGSnapshot(data) {
			return nil, 0, fmt.Errorf("rendered page %d is not JPEG", i+1)
		}
		sum := sha256.Sum256(data)
		images = append(images, smlDocumentImageSnapshot{
			PageNo:      i + 1,
			ContentType: "image/jpeg",
			SHA256:      hex.EncodeToString(sum[:]),
			Data:        data,
		})
	}
	return images, totalBytes, nil
}

func isJPEGSnapshot(data []byte) bool {
	return len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff
}
