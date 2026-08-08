package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
