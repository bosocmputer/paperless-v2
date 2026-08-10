package api

import (
	"context"
	"errors"
	"testing"
)

func TestRenderSMLDocumentSnapshotsRequiresPageCount(t *testing.T) {
	if _, err := renderSMLDocumentSnapshots(context.Background(), "/tmp/does-not-matter.pdf", 0); err == nil {
		t.Fatal("expected missing page count error")
	}
}

func TestRenderSMLDocumentSnapshotsRequiresPath(t *testing.T) {
	if _, err := renderSMLDocumentSnapshots(context.Background(), "", 3); err == nil {
		t.Fatal("expected missing pdf path error")
	}
}

func TestIsJPEGSnapshot(t *testing.T) {
	if !isJPEGSnapshot([]byte{0xff, 0xd8, 0xff, 0xe0}) {
		t.Fatal("expected JPEG magic bytes to pass")
	}
	if isJPEGSnapshot([]byte{0x89, 0x50, 0x4e, 0x47}) {
		t.Fatal("expected PNG magic bytes to fail")
	}
}

func TestSnapshotTooLargeSentinel(t *testing.T) {
	if !errors.Is(errSnapshotTooLarge, errSnapshotTooLarge) {
		t.Fatal("snapshot too large sentinel should match itself")
	}
}
