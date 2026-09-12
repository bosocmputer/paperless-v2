package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func TestDocumentSupportsSMLImages(t *testing.T) {
	tests := []struct {
		name     string
		document models.SigningDocument
		want     bool
	}{
		{
			name:     "sml document with doc no",
			document: models.SigningDocument{DocumentSource: "sml", DocNo: "1EPO2609-00020"},
			want:     true,
		},
		{
			name:     "internal documents never reach SML",
			document: models.SigningDocument{DocumentSource: "internal", DocNo: "INT-0001"},
			want:     false,
		},
		{
			name:     "internal check ignores casing",
			document: models.SigningDocument{DocumentSource: "Internal", DocNo: "INT-0001"},
			want:     false,
		},
		{
			name:     "document without a doc no has nothing to look up",
			document: models.SigningDocument{DocumentSource: "sml", DocNo: "   "},
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := documentSupportsSMLImages(tc.document); got != tc.want {
				t.Fatalf("documentSupportsSMLImages = %v, want %v", got, tc.want)
			}
		})
	}
}

// The UI distinguishes "this image is missing" from "SML is unreachable" so it
// can tell the user whether to retry or to push the images again, so the upstream
// code must survive the hop rather than collapsing into one generic failure.
func TestWriteSMLDocumentImagesErrorMapsUpstreamCodes(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing configuration",
			err:        errSMLConfigMissing,
			wantStatus: 503,
			wantCode:   "sml_not_configured",
		},
		{
			name:       "image not found upstream",
			err:        &smlRequestError{Code: "document_image_not_found"},
			wantStatus: 404,
			wantCode:   "sml_image_not_found",
		},
		{
			name:       "image too large upstream",
			err:        &smlRequestError{Code: "document_image_too_large"},
			wantStatus: 422,
			wantCode:   "sml_image_too_large",
		},
		{
			name:       "any other upstream failure reads as unreachable",
			err:        &smlRequestError{Code: "db_pool_error"},
			wantStatus: 502,
			wantCode:   "sml_images_unavailable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeSMLDocumentImagesError(rec, tc.err)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tc.wantCode) {
				t.Fatalf("body = %s, want code %s", rec.Body.String(), tc.wantCode)
			}
		})
	}
}
