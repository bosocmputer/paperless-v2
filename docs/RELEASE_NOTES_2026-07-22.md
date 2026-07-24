# Release Notes - 2026-07-22

## Internal Documents And Auto Draft

- Added tenant-scoped internal document Masters with guarded activation, immutable identity fields after use, and seeded inactive `PAYREQ`, `ADV`, and `PREPAY` records.
- Added concurrency-safe running numbers with Bangkok document dates, unique constraints, advisory locks, and idempotent create requests.
- Added structured internal document entry with requester/audit separation, expense rows, backend totals, optimistic revision checks, and creator-only draft editing.
- Added SML company-profile lookup and immutable company snapshot at document creation.
- Added Thai/Lao-capable A4 PDF generation, repeated multipage tables, Thai amount text, last-page signature cells, and the required red settlement notice.
- Added immutable PDF revisions and official print audit support; sending requires the current signature/legal layout, while printing remains optional.
- Added automatic signing drafts without a PDF upload step.
- Added source-aware finalization: SML documents keep the image/lock pipeline; internal documents generate final/evidence PDFs and complete with zero SML image/lock calls.
- Hidden SML Flow, reference checks, and SML retry actions for internal documents.
- Added `INTERNAL_DOCUMENTS_ENABLED` for reversible rollout without schema rollback.

## Deployment

Deploy SML API, PaperLess API, and PaperLess Web in that order. Run the additive migration before enabling the feature flag. The same immutable image digests must be used for both customer installations.
