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
		{
			name: "blocks changes to signer slots beyond the original 3-signer cap",
			next: models.DocumentConfigStepRequest{
				ScreenCode:    "PO",
				DocFormatCode: "PO",
				PositionCode:  "3",
				PositionName:  "ผู้อนุมัติ",
				User01:        "901:นาย A",
				User02:        "902:นาย B",
				User04:        "904:นาย D",
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

func TestDuplicateSignerUsername(t *testing.T) {
	tests := []struct {
		name  string
		users []string
		want  bool
	}{
		{name: "no duplicates", users: []string{"901:นาย A", "902:นาย B", "903:นาย C"}, want: false},
		{name: "exact duplicate", users: []string{"901:นาย A", "901:นาย A"}, want: true},
		{name: "case-insensitive duplicate", users: []string{"USER1:นาย A", "user1:นาย A (dup)"}, want: true},
		{name: "empty list", users: []string{}, want: false},
		{name: "duplicate across 10 slots", users: []string{"u1", "u2", "u3", "u4", "u5", "u6", "u7", "u8", "u1"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := duplicateSignerUsername(tt.users); got != tt.want {
				t.Fatalf("duplicateSignerUsername(%v) = %v, want %v", tt.users, got, tt.want)
			}
		})
	}
}

func TestValidateDocumentConfigStepRejectsDuplicateSigner(t *testing.T) {
	req := models.DocumentConfigStepRequest{
		DocFormatCode: "PO",
		PositionCode:  "1",
		PositionName:  "ผู้อนุมัติ",
		SequenceNo:    1,
		ConditionType: 1,
		User01:        "901:นาย A",
		User05:        "901:นาย A",
	}
	got := validateDocumentConfigStep(req)
	if got == "" {
		t.Fatal("expected validation error for duplicate signer, got none")
	}
}

func TestNormalizeDocumentConfigStepCompactsSignerOrder(t *testing.T) {
	req := models.DocumentConfigStepRequest{
		DocFormatCode: "PO",
		PositionCode:  "1",
		PositionName:  "ผู้อนุมัติ",
		SequenceNo:    1,
		ConditionType: 1,
		User01:        "901:นาย A",
		User02:        "",
		User03:        "903:นาย C",
		User04:        "",
		User05:        "905:นาย E",
	}
	got := normalizeDocumentConfigStep(req)
	want := []string{"901:นาย A", "903:นาย C", "905:นาย E"}
	gotUsers := []string{got.User01, got.User02, got.User03, got.User04, got.User05, got.User06, got.User07, got.User08, got.User09, got.User10}
	for i, w := range want {
		if gotUsers[i] != w {
			t.Fatalf("compacted user[%d] = %q, want %q (full result: %#v)", i, gotUsers[i], w, gotUsers)
		}
	}
	for i := len(want); i < len(gotUsers); i++ {
		if gotUsers[i] != "" {
			t.Fatalf("expected trailing slot[%d] to be empty, got %q", i, gotUsers[i])
		}
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

	t.Run("allows internal workflow beyond legacy fixed approval cells", func(t *testing.T) {
		internalFormat := models.SMLDocFormat{Code: "ADV", ScreenCode: internalDocumentScreenCode}
		steps := make([]models.DocumentConfigStepRequest, 7)
		for index := range steps {
			steps[index] = models.DocumentConfigStepRequest{
				PositionCode:  string(rune('A' + index)),
				PositionName:  "ผู้อนุมัติ",
				SequenceNo:    float64(index + 1),
				ConditionType: 3,
			}
		}
		_, messages := normalizeDocumentConfigWorkflowSteps(internalFormat, steps)
		if len(messages) != 0 {
			t.Fatalf("messages = %v, want no legacy A4 signature slot limit", messages)
		}
	})
}
