package store

import (
	"testing"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func TestSummarizeSMLUserSyncSkipsExistingAndCountsPasswordIssues(t *testing.T) {
	candidates := normalizeSMLUserSyncCandidates([]models.SMLUserSyncCandidate{
		{Username: " PUI ", DisplayName: " pui ", PasswordHash: "hash", PasswordSynced: true},
		{Username: "AMP", DisplayName: "amp", PasswordHash: "", PasswordSynced: false},
		{Username: "AMP", DisplayName: "duplicate", PasswordHash: "hash", PasswordSynced: true},
		{Username: "", DisplayName: "blank"},
	})
	existing := map[string]struct{}{"pui": {}}

	result, err := summarizeSMLUserSync(candidates, existing, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.Existing != 1 || result.ToCreate != 1 || result.PasswordNotSynced != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Users) != 1 || result.Users[0].Username != "AMP" {
		t.Fatalf("users = %+v", result.Users)
	}
}

func TestSummarizeSMLUserSyncRequiresPasswordHashWhenCreating(t *testing.T) {
	_, err := summarizeSMLUserSync([]models.SMLUserSyncCandidate{
		{Username: "AMP", DisplayName: "amp", PasswordSynced: false},
	}, map[string]struct{}{}, true)
	if err != ErrSMLUserPasswordHashMissing {
		t.Fatalf("err = %v, want ErrSMLUserPasswordHashMissing", err)
	}
}
