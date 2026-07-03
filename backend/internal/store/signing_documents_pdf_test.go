package store

import (
	"strings"
	"testing"
)

func TestSigningDocumentPDFUpdateStatementKeepsCurrentFileWhenFinal(t *testing.T) {
	sql := signingDocumentPDFUpdateStatement(true)
	if strings.Contains(sql, "current_file_id") {
		t.Fatalf("final PDF update must not replace current_file_id: %s", sql)
	}
	if !strings.Contains(sql, "final_file_id") {
		t.Fatalf("final PDF update must set final_file_id: %s", sql)
	}
	if signingDocumentPDFVersionKind(true) != "final" {
		t.Fatalf("expected final version kind")
	}
}

func TestSigningDocumentPDFUpdateStatementSetsCurrentFileWhenNotFinal(t *testing.T) {
	sql := signingDocumentPDFUpdateStatement(false)
	if !strings.Contains(sql, "current_file_id") {
		t.Fatalf("current PDF update must set current_file_id: %s", sql)
	}
	if strings.Contains(sql, "final_file_id") {
		t.Fatalf("current PDF update must not replace final_file_id: %s", sql)
	}
	if signingDocumentPDFVersionKind(false) != "current" {
		t.Fatalf("expected current version kind")
	}
}
