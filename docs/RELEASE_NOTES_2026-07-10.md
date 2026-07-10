# PaperLess Release Notes - 2026-07-10

## Batch PDF Draft Import

- Added `นำเข้าหลายไฟล์` to the draft queue for document formats with an active Workflow and Active Template.
- PDF filenames are treated as SML document numbers and validated server-side.
- Added batch SML exact-match lookup across `ic_trans` and `ap_ar_trans` with a maximum of 30 document numbers per request.
- Added staged-upload ownership, 24-hour expiry, PDF/page validation, duplicate checks, SML lock confirmation, and context-version protection.
- Batch duplicate validation blocks existing active, completed, or rejected PaperLess documents; a deliberately cancelled document remains eligible for a new draft.
- Batch creation is limited to two concurrent items and uses a stable idempotency key per file.
- Successful drafts remain saved when another item fails; only retryable failures are offered for retry.
- Removing a row or discarding the dialog deletes unconsumed staged uploads.
- Batch import always uses the Active Template and creates drafts only; it does not send documents into the signing workflow automatically.

## Limits And Safety

- Maximum 30 PDFs per batch.
- Maximum 15 MB per PDF (production upload configuration).
- Maximum 100 PDF pages combined per batch.
- Document number derived from filename is limited to 25 characters and rejects path separators/control characters.
- Audit metadata contains only tenant, document format, counts, bytes, page count, elapsed context, and error codes; PDF bytes are never logged.
