package store

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestRenderInternalRunningPattern(t *testing.T) {
	date := time.Date(2026, time.July, 22, 9, 30, 0, 0, time.FixedZone("Asia/Bangkok", 7*60*60))
	period, rendered, digits, err := renderInternalRunningPattern("@yymmdd-###", date)
	if err != nil {
		t.Fatalf("render running pattern: %v", err)
	}
	if period != "260722" || rendered != "260722-###" || digits != 3 {
		t.Fatalf("unexpected running result: period=%q rendered=%q digits=%d", period, rendered, digits)
	}
}

func TestRenderInternalRunningPatternRejectsMultipleCounters(t *testing.T) {
	_, _, _, err := renderInternalRunningPattern("@YY##-MM##", time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected multiple counter groups to be rejected")
	}
}

func TestRenderInternalRunningPatternSupportsFourDigitYear(t *testing.T) {
	period, rendered, digits, err := renderInternalRunningPattern("@yyyy.mm.dd-####", time.Date(2026, 7, 22, 23, 59, 0, 0, time.FixedZone("Asia/Bangkok", 7*60*60)))
	if err != nil {
		t.Fatalf("render running pattern: %v", err)
	}
	if period != "20260722" || rendered != "2026.07.22-####" || digits != 4 {
		t.Fatalf("unexpected running result: period=%q rendered=%q digits=%d", period, rendered, digits)
	}
}

func TestParseInternalAmount(t *testing.T) {
	tests := map[string]int64{
		"1":         100,
		"1.5":       150,
		"123456.78": 12345678,
		"1.":        100,
		".5":        50,
		"0":         0,
		// Deductions and refunds: the sign applies to the whole value.
		"-1":         -100,
		"-1.5":       -150,
		"-123456.78": -12345678,
		".5 ":        50,
		"-.5":        -50,
		// "-0.50" used to come back as +50: the whole part "-0" parses to 0, so
		// the sign vanished and a deduction was stored as a charge.
		"-0.50": -50,
		"-0.05": -5,
		// Zero is a valid number; whether a zero line is allowed is the
		// caller's rule, not the parser's.
		"-0":    0,
		"-0.00": 0,
	}
	for input, want := range tests {
		got, err := ParseInternalAmount(input)
		if err != nil || got != want {
			t.Fatalf("ParseInternalAmount(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []string{
		"", "0.001", "1.2.3", "abc",
		// A second sign, or a sign anywhere but the front.
		"--1", "-+1", "+1", "1-", "- 1", "-1.-5",
		// Nothing numeric in it at all.
		"-", ".", "-.",
		"10000000000000000", "-10000000000000000",
	} {
		if _, err := ParseInternalAmount(input); err == nil {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
}

func TestFindInternalDocumentScanAllowsMissingCurrentVersion(t *testing.T) {
	expected := errors.New("stop after destination assertion")
	_, _, err := scanInternalDocument(internalDocumentMissingVersionRow{t: t, err: expected})
	if !errors.Is(err, expected) {
		t.Fatalf("scan error = %v, want %v", err, expected)
	}
}

func TestValidateInternalDraftSendRequiresLayoutButNotPrint(t *testing.T) {
	if err := validateInternalDraftSend(false); !errors.Is(err, ErrSigningDocumentLayoutRequired) {
		t.Fatalf("missing layout error = %v, want %v", err, ErrSigningDocumentLayoutRequired)
	}
	if err := validateInternalDraftSend(true); err != nil {
		t.Fatalf("layout-ready draft must be sendable without a print event: %v", err)
	}
}

// internalDocumentMissingVersionRow asserts the destination type for the
// nullable created_at from the LEFT JOIN in FindInternalDocumentByID.
type internalDocumentMissingVersionRow struct {
	t   *testing.T
	err error
}

func (r internalDocumentMissingVersionRow) Scan(dest ...any) error {
	r.t.Helper()
	if len(dest) != 32 {
		r.t.Fatalf("FindInternalDocumentByID scan destinations = %d, want 32", len(dest))
	}
	if _, ok := dest[30].(*sql.NullTime); !ok {
		r.t.Fatalf("version created_at destination = %T, want *sql.NullTime", dest[30])
	}
	return r.err
}
