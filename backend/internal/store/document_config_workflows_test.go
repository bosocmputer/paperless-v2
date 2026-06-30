package store

import (
	"testing"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func TestComputeDocumentConfigWorkflowRevision(t *testing.T) {
	baseTime := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	steps := []models.DocumentConfigStep{
		{
			ID:            "step-1",
			ScreenCode:    "PO",
			DocFormatCode: "PO",
			PositionCode:  "1",
			PositionName:  "ผู้จัดทำ",
			User01:        "001:น.ส X",
			SequenceNo:    1,
			ConditionType: 1,
			UpdatedAt:     baseTime,
		},
	}

	first := ComputeDocumentConfigWorkflowRevision(steps)
	second := ComputeDocumentConfigWorkflowRevision(steps)
	if first == "" {
		t.Fatalf("revision is empty")
	}
	if first != second {
		t.Fatalf("revision must be stable, got %q and %q", first, second)
	}

	steps[0].PositionName = "ผู้จัดทำเอกสาร"
	changed := ComputeDocumentConfigWorkflowRevision(steps)
	if changed == first {
		t.Fatalf("revision did not change after workflow field changed")
	}
}
