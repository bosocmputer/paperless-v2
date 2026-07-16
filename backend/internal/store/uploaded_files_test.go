package store

import (
	"strings"
	"testing"
)

func TestDeleteUploadedFileIfUnreferencedChecksAllKnownReferences(t *testing.T) {
	for _, table := range []string{
		"signing_document_uploads",
		"signature_templates",
		"signing_documents",
		"signing_document_versions",
		"signing_document_signers",
		"user_saved_signatures",
		"signing_document_attachments",
		"signing_document_print_events",
	} {
		if !strings.Contains(deleteUploadedFileIfUnreferencedSQL, table) {
			t.Fatalf("deleteUploadedFileIfUnreferencedSQL must guard %s references", table)
		}
	}
	if !strings.Contains(deleteUploadedFileIfUnreferencedSQL, "RETURNING f.storage_path") {
		t.Fatalf("deleteUploadedFileIfUnreferencedSQL must return the storage path for filesystem cleanup")
	}
}
