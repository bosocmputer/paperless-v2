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

func TestStepRequestUsers(t *testing.T) {
	step := models.DocumentConfigStepRequest{
		User01: "04002:กุลธิดา",
		User02: "  ",
		User03: "0000:JIRAPONG",
		User06: "999:ผู้จัดการแผนก",
	}
	got := stepRequestUsers(step)
	want := []string{"04002:กุลธิดา", "0000:JIRAPONG", "999:ผู้จัดการแผนก"}
	if len(got) != len(want) {
		t.Fatalf("expected %d users, got %d (%#v)", len(want), len(got), got)
	}
	for i, value := range want {
		if got[i] != value {
			t.Fatalf("expected users[%d] = %q, got %q", i, value, got[i])
		}
	}
}
