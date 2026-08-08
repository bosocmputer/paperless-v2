package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
