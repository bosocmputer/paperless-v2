package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/config"
	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func TestRejectExpiredTrialAllowsNilExpiry(t *testing.T) {
	recorder := httptest.NewRecorder()
	if rejectExpiredTrial(recorder, nil) {
		t.Fatal("expected no trial restriction when TrialExpiresAt is nil")
	}
}

func TestRejectExpiredTrialAllowsFutureExpiry(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	recorder := httptest.NewRecorder()
	if rejectExpiredTrial(recorder, &future) {
		t.Fatal("expected login to be allowed before trial expiry")
	}
}

func TestRejectExpiredTrialBlocksPastExpiry(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	recorder := httptest.NewRecorder()
	if !rejectExpiredTrial(recorder, &past) {
		t.Fatal("expected login to be rejected after trial expiry")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("trial_expired")) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestTenantReadinessCanRepairSchemaColumns(t *testing.T) {
	tests := []struct {
		name      string
		readiness models.SMLTenantReadiness
		want      bool
	}{
		{
			name:      "schema mismatch marked columns-repairable is repairable",
			readiness: models.SMLTenantReadiness{Status: "schema_mismatch", Tenant: "dcon", ColumnsRepairable: true},
			want:      true,
		},
		{
			name:      "schema mismatch without the repairable flag is rejected",
			readiness: models.SMLTenantReadiness{Status: "schema_mismatch", Tenant: "homeplus5", ColumnsRepairable: false},
			want:      false,
		},
		{
			name:      "image db missing is not a column repair",
			readiness: models.SMLTenantReadiness{Status: "image_db_missing", Tenant: "dcon", ColumnsRepairable: true},
			want:      false,
		},
		{
			name:      "schema mismatch without tenant is rejected",
			readiness: models.SMLTenantReadiness{Status: "schema_mismatch", Tenant: "", ColumnsRepairable: true},
			want:      false,
		},
		{
			name:      "ready tenant has nothing to repair",
			readiness: models.SMLTenantReadiness{Status: "ready", Tenant: "dcon", OK: true},
			want:      false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tenantReadinessCanRepairSchemaColumns(tc.readiness); got != tc.want {
				t.Fatalf("tenantReadinessCanRepairSchemaColumns = %v, want %v", got, tc.want)
			}
		})
	}
}

// The login page calls this before anyone is signed in, so it must work without
// a session and must report both states: a trial date, and no trial at all.
func TestTrialStatusReportsConfiguredExpiry(t *testing.T) {
	expires := time.Date(2026, 10, 8, 23, 59, 59, 0, time.UTC)
	s := &Server{cfg: config.Config{TrialExpiresAt: &expires}}

	recorder := httptest.NewRecorder()
	s.trialStatus(recorder, httptest.NewRequest(http.MethodGet, "/api/public/trial", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body struct {
		TrialExpiresAt *time.Time `json:"trialExpiresAt"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TrialExpiresAt == nil || !body.TrialExpiresAt.Equal(expires) {
		t.Fatalf("trialExpiresAt = %v, want %v", body.TrialExpiresAt, expires)
	}
}

func TestTrialStatusReportsNullWhenNoTrial(t *testing.T) {
	s := &Server{cfg: config.Config{}}

	recorder := httptest.NewRecorder()
	s.trialStatus(recorder, httptest.NewRequest(http.MethodGet, "/api/public/trial", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	// Explicit null, not a missing key: the client distinguishes "no trial"
	// from "response was malformed".
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"trialExpiresAt":null`)) {
		t.Fatalf("body = %s, want trialExpiresAt:null", recorder.Body.String())
	}
}
