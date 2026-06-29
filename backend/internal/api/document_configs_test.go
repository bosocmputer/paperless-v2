package api

import (
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
