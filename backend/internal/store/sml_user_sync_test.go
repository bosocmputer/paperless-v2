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
	existing := map[string]existingUserSyncState{"pui": {Status: "active"}}

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

func TestSummarizeSMLUserSyncReactivatesExistingInactiveUsers(t *testing.T) {
	candidates := normalizeSMLUserSyncCandidates([]models.SMLUserSyncCandidate{
		{Username: "8001", DisplayName: "bird", PasswordHash: "hash", PasswordSynced: true},
		{Username: "6001", DisplayName: "sand", PasswordHash: "hash", PasswordSynced: true},
	})
	existing := map[string]existingUserSyncState{
		"8001": {Status: "inactive"},
		"6001": {Status: "active"},
	}

	result, err := summarizeSMLUserSync(candidates, existing, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Existing != 2 || result.ToCreate != 0 || result.ToActivate != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.ActivateUsernames) != 1 || result.ActivateUsernames[0] != "8001" {
		t.Fatalf("activate usernames = %+v", result.ActivateUsernames)
	}
}

func TestSummarizeSMLUserSyncRequiresPasswordHashWhenCreating(t *testing.T) {
	_, err := summarizeSMLUserSync([]models.SMLUserSyncCandidate{
		{Username: "AMP", DisplayName: "amp", PasswordSynced: false},
	}, map[string]existingUserSyncState{}, true)
	if err != ErrSMLUserPasswordHashMissing {
		t.Fatalf("err = %v, want ErrSMLUserPasswordHashMissing", err)
	}
}
