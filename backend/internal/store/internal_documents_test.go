package store

import (
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
	}
	for input, want := range tests {
		got, err := ParseInternalAmount(input)
		if err != nil || got != want {
			t.Fatalf("ParseInternalAmount(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []string{"", "0.001", "1.2.3", "abc", "-1"} {
		if _, err := ParseInternalAmount(input); err == nil {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
}
