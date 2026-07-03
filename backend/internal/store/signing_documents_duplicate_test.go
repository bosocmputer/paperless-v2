package store

import (
	"strings"
	"testing"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func TestBuildSigningDocumentDuplicateCheckResultBlocksUnfinishedStatuses(t *testing.T) {
	for _, status := range []string{"draft", "in_progress", "pending_confirm", "completed_evidence_failed", "completed_image_failed", "completed_lock_failed"} {
		t.Run(status, func(t *testing.T) {
			result := buildSigningDocumentDuplicateCheckResult([]models.SigningDocumentReference{{
				ID:            "doc-1",
				DocNo:         "PO26060001",
				DocFormatCode: "PO",
				Status:        status,
			}})
			if result.CanCreate {
				t.Fatalf("expected status %s to block create", status)
			}
			if result.BlockingDocument == nil || result.BlockingDocument.Status != status {
				t.Fatalf("unexpected blocking document: %#v", result.BlockingDocument)
			}
			if !strings.Contains(result.Message, "กรุณาเปิดเอกสารเดิม") {
				t.Fatalf("expected actionable Thai message, got %q", result.Message)
			}
		})
	}
}

func TestBuildSigningDocumentDuplicateCheckResultAllowsFinishedStatusesWithWarning(t *testing.T) {
	result := buildSigningDocumentDuplicateCheckResult([]models.SigningDocumentReference{
		{ID: "doc-1", DocNo: "PO26060001", DocFormatCode: "PO", Status: "completed"},
		{ID: "doc-2", DocNo: "PO26060001", DocFormatCode: "PO", Status: "rejected"},
		{ID: "doc-3", DocNo: "PO26060001", DocFormatCode: "PO", Status: "cancelled"},
	})
	if !result.CanCreate {
		t.Fatal("finished/cancelled/rejected documents should not block new draft creation")
	}
	if result.BlockingDocument != nil {
		t.Fatalf("did not expect blocking document, got %#v", result.BlockingDocument)
	}
	if len(result.PreviousDocuments) != 3 {
		t.Fatalf("expected previous documents to be returned, got %d", len(result.PreviousDocuments))
	}
	if !strings.Contains(result.Message, "เคยมีเอกสารนี้") {
		t.Fatalf("expected warning message, got %q", result.Message)
	}
}

func TestBuildSigningDocumentDuplicateCheckResultPrefersLatestBlockingDocument(t *testing.T) {
	result := buildSigningDocumentDuplicateCheckResult([]models.SigningDocumentReference{
		{ID: "doc-block", DocNo: "PO26060001", DocFormatCode: "PO", Status: "in_progress"},
		{ID: "doc-old", DocNo: "PO26060001", DocFormatCode: "PO", Status: "completed"},
	})
	if result.CanCreate {
		t.Fatal("blocking document should prevent create")
	}
	if result.BlockingDocument == nil || result.BlockingDocument.ID != "doc-block" {
		t.Fatalf("expected first blocking document, got %#v", result.BlockingDocument)
	}
	if len(result.PreviousDocuments) != 1 || result.PreviousDocuments[0].ID != "doc-old" {
		t.Fatalf("expected finished document to remain as previous context, got %#v", result.PreviousDocuments)
	}
}
