package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func internalRequestWithAmounts(amounts ...string) models.InternalDocumentCreateRequest {
	items := make([]models.InternalDocumentItem, 0, len(amounts))
	for i, amount := range amounts {
		items = append(items, models.InternalDocumentItem{Description: "รายการ " + string(rune('A'+i)), Amount: amount})
	}
	return models.InternalDocumentCreateRequest{
		MasterID:      "22222222-2222-2222-2222-222222222222",
		DocumentDate:  "2026-10-17",
		RequiredDate:  "2026-10-17",
		RequesterName: "ผู้ขอ",
		Purpose:       "ทดสอบ",
		Items:         items,
	}
}

func TestInternalDocumentAcceptsDeductionLinesWhileTotalStaysPositive(t *testing.T) {
	normalized, total, issues := normalizeInternalDocumentCreateRequest(internalRequestWithAmounts("5000", "-500.5"))
	if len(issues) != 0 {
		t.Fatalf("a deduction line must be accepted, got %v", issues)
	}
	if total != 449950 {
		t.Fatalf("total = %d cents, want 449950 (5000.00 - 500.50)", total)
	}
	if normalized.Items[1].Amount != "-500.50" {
		t.Fatalf("stored amount = %q, want -500.50", normalized.Items[1].Amount)
	}
}

func TestInternalDocumentAcceptsNegativeTotal(t *testing.T) {
	// A refund: every line, and so the total, is negative.
	_, total, issues := normalizeInternalDocumentCreateRequest(internalRequestWithAmounts("-1200", "-300.25"))
	if len(issues) != 0 {
		t.Fatalf("a negative total must be accepted, got %v", issues)
	}
	if total != -150025 {
		t.Fatalf("total = %d cents, want -150025", total)
	}
}

func TestInternalDocumentKeepsSignOnSubUnitDeduction(t *testing.T) {
	// "-0.50" used to be stored as +0.50.
	normalized, total, issues := normalizeInternalDocumentCreateRequest(internalRequestWithAmounts("10", "-0.50"))
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %v", issues)
	}
	if normalized.Items[1].Amount != "-0.50" || total != 950 {
		t.Fatalf("got amount %q total %d, want -0.50 and 950", normalized.Items[1].Amount, total)
	}
}

func TestInternalDocumentRejectsZeroLine(t *testing.T) {
	for _, zero := range []string{"0", "0.00", "-0", "-0.00"} {
		_, _, issues := normalizeInternalDocumentCreateRequest(internalRequestWithAmounts("100", zero))
		if len(issues) != 1 || !strings.Contains(issues[0], "รายการที่ 2") || !strings.Contains(issues[0], "ศูนย์") {
			t.Fatalf("zero line %q: issues = %v, want one 'รายการที่ 2 ... ศูนย์'", zero, issues)
		}
	}
}

func TestInternalDocumentRejectsZeroTotal(t *testing.T) {
	// 100 and -100 cancel out: nothing left to approve, and the database refuses it.
	_, total, issues := normalizeInternalDocumentCreateRequest(internalRequestWithAmounts("100", "-100"))
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
	if len(issues) != 1 || !strings.Contains(issues[0], "ยอดรวม") {
		t.Fatalf("issues = %v, want one complaint about the zero total", issues)
	}
}

func TestInternalDocumentBadLineDoesNotAlsoComplainAboutTotal(t *testing.T) {
	_, _, issues := normalizeInternalDocumentCreateRequest(internalRequestWithAmounts("abc", "100"))
	if len(issues) != 1 || !strings.Contains(issues[0], "รายการที่ 1") {
		t.Fatalf("issues = %v, want exactly the bad-line complaint", issues)
	}
}

func TestInternalDocumentRejectsTotalBeyondColumnRange(t *testing.T) {
	big := "9999999999999999.99"
	_, _, issues := normalizeInternalDocumentCreateRequest(internalRequestWithAmounts(big, "1"))
	if len(issues) == 0 || !strings.Contains(strings.Join(issues, " "), "สูงเกินกำหนด") {
		t.Fatalf("issues = %v, want an over-range complaint", issues)
	}
	_, _, issues = normalizeInternalDocumentCreateRequest(internalRequestWithAmounts("-"+big, "-1"))
	if len(issues) == 0 || !strings.Contains(strings.Join(issues, " "), "สูงเกินกำหนด") {
		t.Fatalf("negative side: issues = %v, want an over-range complaint", issues)
	}
}

func TestCentsToAmountKeepsTheSign(t *testing.T) {
	// Formatting cents/100 and cents%100 separately gave "-1.-50" for -150.
	tests := map[int64]string{150: "1.50", 5: "0.05", 0: "0.00", -150: "-1.50", -5: "-0.05", -100000: "-1000.00"}
	for cents, want := range tests {
		if got := centsToAmount(cents); got != want {
			t.Fatalf("centsToAmount(%d) = %q, want %q", cents, got, want)
		}
	}
}

func TestParseAmountCentsKeepsTheSignOfTheWholeValue(t *testing.T) {
	// It summed whole*100 + fraction, which turned "-1.50" into -50 and lost the
	// sign of "-0.50" altogether.
	tests := map[string]int64{"1234.5": 123450, "-1.50": -150, "-0.50": -50, "-1000.00": -100000, "0.05": 5}
	for in, want := range tests {
		got, err := parseAmountCents(in)
		if err != nil || got != want {
			t.Fatalf("parseAmountCents(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "1.2.3", "--1", "1.-5"} {
		if _, err := parseAmountCents(bad); err == nil {
			t.Fatalf("expected %q to be rejected", bad)
		}
	}
}

func TestFormatInternalAmountPrintsNegativeAmounts(t *testing.T) {
	tests := map[string]string{"1234.5": "1,234.50", "-1.50": "-1.50", "-0.50": "-0.50", "-1234567.89": "-1,234,567.89"}
	for in, want := range tests {
		if got := formatInternalAmount(in); got != want {
			t.Fatalf("formatInternalAmount(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestThaiBahtTextHandlesNegativeTotals(t *testing.T) {
	// It returned "" for a negative total, leaving the words box empty next to a
	// real amount.
	tests := map[string]string{
		"1000.00":  "หนึ่งพันบาทถ้วน",
		"-1000.00": "ลบหนึ่งพันบาทถ้วน",
		"-0.50":    "ลบศูนย์บาทห้าสิบสตางค์",
		"-1500.25": "ลบหนึ่งพันห้าร้อยบาทยี่สิบห้าสตางค์",
	}
	for in, want := range tests {
		if got := thaiBahtText(in); got != want {
			t.Fatalf("thaiBahtText(%q) = %q, want %q", in, got, want)
		}
	}
	if got := thaiBahtText("abc"); got != "" {
		t.Fatalf("thaiBahtText(abc) = %q, want empty for an unreadable amount", got)
	}
}

// The document the form can now produce - a charge with a deduction, and a pure
// refund - must still render to a single readable A4 page.
func TestInternalDocumentPDFRendersNegativeAmounts(t *testing.T) {
	cases := map[string]struct {
		amounts []string
		total   string
	}{
		"deduction on a positive total": {[]string{"5000.00", "-500.50", "-1234567.89", "1234567.89"}, "4499.50"},
		"negative total (refund)":       {[]string{"-1200.00", "-300.25"}, "-1500.25"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			document := internalPDFTestDocument(len(tc.amounts))
			for i, amount := range tc.amounts {
				document.Items[i].Amount = amount
			}
			document.TotalAmount = tc.total

			data, pages, err := renderInternalDocumentPDF(document)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if pages != 1 {
				t.Fatalf("pages = %d, want one A4 page", pages)
			}
			if output := os.Getenv("INTERNAL_PDF_NEGATIVE_QA_DIR"); output != "" {
				file := filepath.Join(output, strings.NewReplacer(" ", "_", "(", "", ")", "").Replace(name)+".pdf")
				if err := os.WriteFile(file, data, 0o600); err != nil {
					t.Fatalf("write QA artifact: %v", err)
				}
			}
		})
	}
}
