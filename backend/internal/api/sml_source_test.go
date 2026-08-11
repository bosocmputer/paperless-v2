package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bosocmputer/paperless-v2/backend/internal/config"
	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func TestLegacySMLSourceMatchesDocument(t *testing.T) {
	document := models.SigningDocument{
		DocNo:         "PO26070001",
		DocFormatCode: "PO",
		DocDate:       "2026-07-31",
		PartyCode:     "AP001",
		SMLTable:      "ap_ar_trans",
		TransFlag:     19,
		TotalAmount:   1250.50,
	}
	candidate := models.SMLDocumentCandidate{
		DocNo:         "po26070001",
		DocFormatCode: "PO",
		DocDate:       "2026-07-31",
		PartyCode:     "ap001",
		Table:         "AP_AR_TRANS",
		TransFlag:     19,
		TotalAmount:   1250.50,
	}
	if !legacySMLSourceMatchesDocument(candidate, document) {
		t.Fatal("matching legacy snapshot should establish an initial source revision")
	}
	candidate.TotalAmount = 1250.51
	if legacySMLSourceMatchesDocument(candidate, document) {
		t.Fatal("material SML header change must not establish a legacy baseline")
	}
}

func TestDiffSMLSourceCandidateReportsOnlyDisagreeingFields(t *testing.T) {
	document := models.SigningDocument{
		DocNo:         "PO26070001",
		DocFormatCode: "PO",
		DocDate:       "2026-07-31",
		PartyCode:     "AP001",
		SMLTable:      "ap_ar_trans",
		TransFlag:     19,
		TotalAmount:   1250.50,
	}
	candidate := models.SMLDocumentCandidate{
		DocNo:         "po26070001",
		DocFormatCode: "PO",
		DocDate:       "2026-08-01",
		PartyCode:     "ap001",
		Table:         "AP_AR_TRANS",
		TransFlag:     19,
		TotalAmount:   1300.00,
	}
	diffs := diffSMLSourceCandidate(candidate, document)

	got := make(map[string]smlSourceFieldDiff, len(diffs))
	for _, d := range diffs {
		got[d.Field] = d
	}
	if len(diffs) != 2 {
		t.Fatalf("expected exactly 2 field diffs (doc_date, total_amount), got %d: %#v", len(diffs), diffs)
	}
	if d, ok := got["doc_date"]; !ok || d.Stored != "2026-07-31" || d.Current != "2026-08-01" {
		t.Fatalf("doc_date diff = %#v, want stored=2026-07-31 current=2026-08-01", d)
	}
	if d, ok := got["total_amount"]; !ok || d.Stored != "1250.50" || d.Current != "1300.00" {
		t.Fatalf("total_amount diff = %#v, want stored=1250.50 current=1300.00", d)
	}
	if _, ok := got["doc_no"]; ok {
		t.Fatalf("doc_no matches case-insensitively and must not appear in the diff: %#v", diffs)
	}
	if _, ok := got["party_code"]; ok {
		t.Fatalf("party_code matches case-insensitively and must not appear in the diff: %#v", diffs)
	}
}

func TestDiffSMLSourceCandidateEmptyWhenNothingDiffers(t *testing.T) {
	document := models.SigningDocument{
		DocNo:         "PO26070001",
		DocFormatCode: "PO",
		TotalAmount:   1250.50,
	}
	candidate := models.SMLDocumentCandidate{
		DocNo:         "PO26070001",
		DocFormatCode: "PO",
		TotalAmount:   1250.50,
	}
	if diffs := diffSMLSourceCandidate(candidate, document); len(diffs) != 0 {
		t.Fatalf("expected no diffs for matching candidates, got %#v", diffs)
	}
}

func TestSMLSourceStateErrorCarriesSafeState(t *testing.T) {
	err := &smlSourceStateError{State: "sml_source_missing", Message: "missing"}
	if err.Error() != "missing" || err.State != "sml_source_missing" {
		t.Fatalf("unexpected source state error: %#v", err)
	}
}

// unconfiguredServer builds a *Server with no SML API config, no store, and
// a no-op logger. fetchSMLDocumentCandidate short-circuits on missing
// config (errSMLConfigMissing) before it ever needs a store or makes an
// HTTP call, which makes this a safe, DB-free way to exercise
// verifySMLDocumentSourceFailOpen's "not a confirmed source-state error"
// branch without a live database or network.
func unconfiguredServer() *Server {
	return &Server{
		cfg:    config.Config{},
		logger: slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError + 100})),
	}
}

func TestVerifySMLDocumentSourceFailOpenSwallowsNonConfirmedErrors(t *testing.T) {
	s := unconfiguredServer()
	document := models.SigningDocument{
		ID:             "doc-1",
		DocumentSource: "sml",
		DocFormatCode:  "PO",
		DocNo:          "PO26070001",
	}
	// errSMLConfigMissing is not a *smlSourceStateError — it must be
	// swallowed (fail open), not returned, or every document at a shop
	// with a temporarily misconfigured/unreachable SML API would become
	// unsignable the moment this per-step check runs.
	if err := s.verifySMLDocumentSourceFailOpen(context.Background(), document); err != nil {
		t.Fatalf("verifySMLDocumentSourceFailOpen must fail open on non-confirmed errors, got: %v", err)
	}
}

func TestVerifySMLDocumentSourceFailOpenSkipsInternalDocuments(t *testing.T) {
	s := unconfiguredServer()
	document := models.SigningDocument{ID: "doc-2", DocumentSource: "internal"}
	if err := s.verifySMLDocumentSourceFailOpen(context.Background(), document); err != nil {
		t.Fatalf("internal documents must never call out to SML, got: %v", err)
	}
}

func TestBlockSigningOnSMLSourceDriftAllowsSignWhenSMLUnreachable(t *testing.T) {
	s := unconfiguredServer()
	document := models.SigningDocument{
		ID:             "doc-3",
		DocumentSource: "sml",
		DocFormatCode:  "PO",
		DocNo:          "PO26070001",
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/my-signing-tasks/task-1/sign", nil)

	blocked := s.blockSigningOnSMLSourceDrift(w, r, document)

	if blocked {
		t.Fatalf("blockSigningOnSMLSourceDrift must not block signing when SML is merely unreachable/unconfigured, response: %s", w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("blockSigningOnSMLSourceDrift wrote a response (%d) though it should have left the request untouched for the caller to continue", w.Code)
	}
}

func TestBlockSigningOnSMLSourceDriftIsNoopForInternalDocuments(t *testing.T) {
	s := unconfiguredServer()
	document := models.SigningDocument{ID: "doc-4", DocumentSource: "internal"}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/my-signing-tasks/task-1/sign", nil)

	if blocked := s.blockSigningOnSMLSourceDrift(w, r, document); blocked {
		t.Fatalf("internal documents must never be blocked by the SML source check, response: %s", w.Body.String())
	}
}
