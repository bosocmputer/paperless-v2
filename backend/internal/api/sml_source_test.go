package api

import (
	"testing"

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

func TestSMLSourceStateErrorCarriesSafeState(t *testing.T) {
	err := &smlSourceStateError{State: "sml_source_missing", Message: "missing"}
	if err.Error() != "missing" || err.State != "sml_source_missing" {
		t.Fatalf("unexpected source state error: %#v", err)
	}
}
