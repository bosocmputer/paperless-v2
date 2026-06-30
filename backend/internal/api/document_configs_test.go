package api

import (
	"strings"
	"testing"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func TestDocumentConfigTemplateBreakingChange(t *testing.T) {
	current := models.DocumentConfigStep{
		ScreenCode:    "PO",
		DocFormatCode: "PO",
		PositionCode:  "3",
		PositionName:  "ผู้อนุมัติ",
		User01:        "901:นาย A",
		User02:        "902:นาย B",
		SequenceNo:    3,
		ConditionType: 2,
	}

	tests := []struct {
		name string
		next models.DocumentConfigStepRequest
		want bool
	}{
		{
			name: "allows name and sequence changes",
			next: models.DocumentConfigStepRequest{
				ScreenCode:    "PO",
				DocFormatCode: "PO",
				PositionCode:  "3",
				PositionName:  "ผู้อนุมัติเอกสาร",
				User01:        "901:นาย A",
				User02:        "902:นาย B",
				SequenceNo:    3.5,
				ConditionType: 2,
			},
			want: false,
		},
		{
			name: "blocks position code changes",
			next: models.DocumentConfigStepRequest{
				ScreenCode:    "PO",
				DocFormatCode: "PO",
				PositionCode:  "4",
				PositionName:  "ผู้อนุมัติ",
				User01:        "901:นาย A",
				User02:        "902:นาย B",
				SequenceNo:    3,
				ConditionType: 2,
			},
			want: true,
		},
		{
			name: "blocks condition changes",
			next: models.DocumentConfigStepRequest{
				ScreenCode:    "PO",
				DocFormatCode: "PO",
				PositionCode:  "3",
				PositionName:  "ผู้อนุมัติ",
				User01:        "901:นาย A",
				User02:        "902:นาย B",
				SequenceNo:    3,
				ConditionType: 1,
			},
			want: true,
		},
		{
			name: "blocks signer changes",
			next: models.DocumentConfigStepRequest{
				ScreenCode:    "PO",
				DocFormatCode: "PO",
				PositionCode:  "3",
				PositionName:  "ผู้อนุมัติ",
				User01:        "901:นาย A",
				User02:        "903:นาย C",
				SequenceNo:    3,
				ConditionType: 2,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := documentConfigTemplateBreakingChange(current, tt.next); got != tt.want {
				t.Fatalf("documentConfigTemplateBreakingChange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateDocumentConfigStepAllowsExternalWithoutInternalUser(t *testing.T) {
	req := models.DocumentConfigStepRequest{
		DocFormatCode: "PO",
		PositionCode:  "4",
		PositionName:  "ลูกค้า",
		SequenceNo:    4,
		ConditionType: 3,
	}
	if got := validateDocumentConfigStep(req); got != "" {
		t.Fatalf("validateDocumentConfigStep() = %q, want empty", got)
	}
}

func TestNormalizeDocumentConfigWorkflowSteps(t *testing.T) {
	format := models.SMLDocFormat{Code: "PO", ScreenCode: "PO"}

	t.Run("allows condition three without users", func(t *testing.T) {
		steps, messages := normalizeDocumentConfigWorkflowSteps(format, []models.DocumentConfigStepRequest{
			{
				PositionCode:  "4",
				PositionName:  "ลูกค้า",
				SequenceNo:    1,
				ConditionType: 3,
			},
		})
		if len(messages) != 0 {
			t.Fatalf("messages = %v, want none", messages)
		}
		if len(steps) != 1 || steps[0].DocFormatCode != "PO" || steps[0].ScreenCode != "PO" {
			t.Fatalf("normalized steps = %#v", steps)
		}
	})

	t.Run("rejects duplicate position code", func(t *testing.T) {
		_, messages := normalizeDocumentConfigWorkflowSteps(format, []models.DocumentConfigStepRequest{
			{
				PositionCode:  "1",
				PositionName:  "ผู้จัดทำ",
				User01:        "001:น.ส X",
				SequenceNo:    1,
				ConditionType: 1,
			},
			{
				PositionCode:  "1",
				PositionName:  "ผู้ตรวจ",
				User01:        "201:นาย ก",
				SequenceNo:    2,
				ConditionType: 1,
			},
		})
		if len(messages) == 0 {
			t.Fatalf("expected duplicate position validation message")
		}
		joined := strings.Join(messages, " ")
		if !strings.Contains(joined, "duplicated") {
			t.Fatalf("messages = %v, want duplicate message", messages)
		}
	})

	t.Run("requires user for condition one and two", func(t *testing.T) {
		_, messages := normalizeDocumentConfigWorkflowSteps(format, []models.DocumentConfigStepRequest{
			{
				PositionCode:  "1",
				PositionName:  "ผู้จัดทำ",
				SequenceNo:    1,
				ConditionType: 1,
			},
			{
				PositionCode:  "2",
				PositionName:  "ผู้อนุมัติ",
				SequenceNo:    2,
				ConditionType: 2,
			},
		})
		if len(messages) != 2 {
			t.Fatalf("messages = %v, want two missing user messages", messages)
		}
	})
}
