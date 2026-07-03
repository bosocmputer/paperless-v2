package api

import (
	"errors"
	"testing"
)

func TestSMLSnapshotPageCountCapsAtEight(t *testing.T) {
	count, truncated, err := smlSnapshotPageCount(12)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 8 {
		t.Fatalf("count = %d, want 8", count)
	}
	if !truncated {
		t.Fatal("expected truncated=true")
	}
}

func TestSMLSnapshotPageCountUsesOriginalPagesOnly(t *testing.T) {
	count, truncated, err := smlSnapshotPageCount(3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want original page count 3", count)
	}
	if truncated {
		t.Fatal("did not expect truncation")
	}
}

func TestSMLSnapshotPageCountRequiresPageCount(t *testing.T) {
	if _, _, err := smlSnapshotPageCount(0); err == nil {
		t.Fatal("expected missing page count error")
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
