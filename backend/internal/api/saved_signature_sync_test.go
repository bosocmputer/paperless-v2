package api

import (
	"testing"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func TestPlanSMLSavedSignatures(t *testing.T) {
	candidates := []models.SMLUserSyncCandidate{
		{Username: "new", SignatureAvailable: true, SignatureVersion: "v1"},
		{Username: "changed", SignatureAvailable: true, SignatureVersion: "v2"},
		{Username: "same", SignatureAvailable: true, SignatureVersion: "v1"},
		{Username: "missing", SignatureIssue: "signature_missing"},
		{Username: "invalid", SignatureIssue: "signature_invalid"},
	}
	existing := map[string]models.UserSavedSignature{
		"changed": {FileID: "old", SourceVersion: "v1", File: models.UploadedFile{ID: "old"}},
		"same":    {FileID: "same", SourceVersion: "v1", File: models.UploadedFile{ID: "same"}},
	}
	plans, result := planSMLSavedSignatures(candidates, existing)
	want := []string{"new", "changed", "unchanged", "missing", "invalid"}
	for index, status := range want {
		if plans[index].Status != status {
			t.Fatalf("plan %d status = %q, want %q", index, plans[index].Status, status)
		}
	}
	if result.Available != 3 || result.New != 1 || result.Changed != 1 || result.Unchanged != 1 || result.Missing != 1 || result.Invalid != 1 {
		t.Fatalf("unexpected summary: %+v", result)
	}
}

func TestPlanSMLSavedSignaturesTreatsMissingFileAsNew(t *testing.T) {
	candidates := []models.SMLUserSyncCandidate{{Username: "user", SignatureAvailable: true, SignatureVersion: "v1"}}
	existing := map[string]models.UserSavedSignature{"user": {SourceVersion: "v1"}}
	plans, _ := planSMLSavedSignatures(candidates, existing)
	if plans[0].Status != "new" {
		t.Fatalf("status = %q, want new", plans[0].Status)
	}
}
