# PaperLess

PaperLess is a controlled e-signature workflow for SML-originated PDF documents.
It receives document metadata and PDFs from SML, routes them through configured signing workflows, records audit evidence, writes signed PDF snapshots back to SML, and locks the ERP document automatically after the final signature.

## Stack

- Frontend: Vue 3, Vite, PrimeVue 4, Sakai layout
- Backend: Go HTTP API
- Database: PostgreSQL 16 for PaperLess application data
- ERP integration: SML API service and customer SML PostgreSQL databases
- PDF tools: Poppler `pdftoppm` for page snapshots, PDF.js for read-only preview, embedded Sarabun font for Thai evidence PDF
- Runtime: Docker Compose

## Current Capabilities

- SML login with two-step database selection on every login
- Automatic PaperLess user provisioning from SML login
- Superadmin-triggered SML user sync with tenant-scoped saved signature import from `erp_user.signature_1`
- Tenant isolation by selected SML database while keeping one PaperLess database
- Superadmin user management, workflow configuration, and signature template design
- Admin document operations with locked signature/template placements
- Batch PDF import into the draft queue with filename-to-SML validation, duplicate checks, partial retry, and staged-file cleanup
- Multi-page document creation with cloned signature/legal-notice placements that remain editable per page
- Internal signer task queue, waiting queue, and signing history
- External sign-only flow with OTP and sanitized public API surface
- Mobile-first signer workspace with continuous PDF viewer
- Transparent PNG signature normalization so PDF stamps do not cover document text
- `current` PDF for signed document preview, `final` PDF for audit evidence appendix
- Read-only PDF dialogs without browser download/print controls
- Official print-copy flow with print events before opening printable PDF
- Automatic finalization after the last signature: signed PDF, SML JPEG snapshots, SML lock, retry states
- SML image upload writes JPEG bytes to both tenant DB and tenant `_images` DB
- In-app admin/user guides with QA screenshots

## Login Model

PaperLess authenticates against the SML auth API. The provider and data group are configured by the system and are not shown as user inputs on the login page.

Login is always two steps:

1. User enters SML username/password.
2. PaperLess verifies SML, shows every allowed database, and requires the user to choose one. A database found in `pg_database` is shown as `ต้องตรวจสอบ` until the user runs the full connection/schema check; name existence alone is never shown as ready.
3. If the selected database is missing `<tenant>_images` or the `public.sml_doc_images` table, the user can click **ตั้งค่า image DB** on the login page. PaperLess re-verifies the SML username/password/database permission before asking the SML API to create only the missing image database/table.

The selected database is checked again before JWT issuance. Databases missing `<tenant>_images` or a compatible `public.sml_doc_images` schema are blocked at login so users do not reach a broken SML finalization/upload flow.

On first successful SML login, PaperLess creates the local user automatically:

- SML `superadmin` becomes PaperLess role `superadmin`.
- Other SML users become role `admin`.
- PaperLess-local users are created as role `user`.
- Existing local PaperLess role/status/display name are preserved.
- Inactive PaperLess users cannot log in even if SML credentials are valid.

`PAPERLESS_LOCAL_AUTH_FALLBACK_ENABLED=true` is only for migration/rollout safety. Production should disable it after SML users are ready.

## Saved Signatures From SML

`Sync จาก SML` on the superadmin user page synchronizes active SML users and saved signatures for the currently selected tenant. Users are matched case-insensitively by SML user code and `erp_user.code`. Signature images are normalized to immutable transparent PNG files; a missing or invalid SML image never deletes the last usable PaperLess signature.

When a saved signature is available, an internal signer must explicitly choose either the saved signature or a newly drawn signature for each task. The saved image is loaded only after selection. Signing snapshots the selected immutable file and source version, so a later SML sync does not rewrite completed documents. External signers can only draw a new signature.

Set `SML_SIGNATURE_SYNC_ENABLED=false` to disable SML signature sync and saved-signature selection without affecting the normal drawing flow. Deploy the SML API before the PaperLess backend/frontend when enabling this feature.

## Tenant Model

The selected SML database becomes the active tenant in the JWT/session:

- `smlProvider`
- `smlDataGroup`
- `smlDataCode`
- `smlTenant`

Documents, workflow configuration, dashboard counts, user tasks, history, signature templates, duplicate checks, SML image upload, SML lock, and related document lookups are scoped by tenant. Actions on an existing document use `document.smlTenant`, not the currently selected session tenant, so retries and confirmations remain consistent.

Existing production data was migrated to tenant `sml1_2026`.

## PDF And Audit Model

PaperLess separates PDF versions intentionally:

- `current`: the actual signed document, used for normal admin/user preview.
- `final`: signed document plus electronic-signature evidence appendix, used for audit.
- `print copy`: actual signed document plus print evidence page, generated only after a print event is recorded.

Admin history preview opens `current` by default. Admin can explicitly open signing evidence. User history also shows `current` by default so users see the real completed document, not the evidence appendix.

## SML Image And Lock Flow

When the last signer completes a document, the backend starts a background finalization worker:

1. Refreshes/generates the signed `current` PDF.
2. Builds `final` audit PDF.
3. Renders only original document pages, excluding evidence pages.
4. Sends JPEG snapshots for pages `1..min(originalPageCount, 8)` to SML.
5. Writes images to both tenant DB and tenant `_images` DB.
6. Locks the ERP document in SML after image upload succeeds.

The intermediate status is `auto_confirming`. If image upload fails, status becomes `completed_image_failed` and SML lock is not attempted. Admin can retry SML images. Retrying a completed document is allowed for repair and remains idempotent.

Before enabling a new SML tenant, verify that both the main database and matching image database exist. Example: tenant `stpt` requires `stpt` and `stpt_images`, both with `public.sml_doc_images`. Use the SML API maintenance command `verify-sml-tenant`; if the image DB or table is missing, the login page can self-provision it after SML credential verification, or an admin can provision it explicitly with `provision-sml-image-db`. Use PaperLess retry instead of direct SQL image inserts.

## Batch Draft Import

Admins can open `เอกสารเตรียมส่ง` and choose `นำเข้าหลายไฟล์` to stage up to 30 PDFs for one document format. Each filename must equal the SML document number, for example `QT26070001.pdf`. PaperLess validates Workflow, Active Template, PDF integrity, a combined 100-page limit, SML exact matches, PaperLess duplicates, and SML lock state before creating anything.

Valid items are created as independent drafts with concurrency capped at two. Successful items remain saved if another item fails; retry reuses the same idempotency key and cannot create a duplicate after a network timeout. Removing a row or discarding the dialog deletes unconsumed staged uploads, with the 24-hour cleanup job retained as a fallback.

## Local Development

Copy the example environment and start the stack:

```bash
cp .env.example .env
docker compose up -d --build
```

Default local ports:

- Frontend: `http://localhost:3070`
- Backend API: `http://localhost:8080`
- PaperLess PostgreSQL: `localhost:54320`

If you run the backend directly on macOS, install Poppler first:

```bash
brew install poppler
```

## Build And Test

Frontend:

```bash
npm --prefix frontend run build
```

Backend:

```bash
cd backend
go test ./...
```

## Deployment Notes

Dev server deployment currently uses port `3070`.

Customer deployment is documented in [docs/CUSTOMER_DEPLOYMENT.md](docs/CUSTOMER_DEPLOYMENT.md). The customer stack is deployed under `/data/paperless`, exposes only the frontend on port `8095`, and talks to SML through the shared Docker network.

Latest customer test status and known tenant readiness items are tracked in [docs/QA_SUMMARY_2026-07-03.md](docs/QA_SUMMARY_2026-07-03.md) and [docs/RELEASE_NOTES_2026-07-16.md](docs/RELEASE_NOTES_2026-07-16.md).

Do not commit real `.env` files, passwords, API keys, tokens, OTPs, PDF bytes, or signature images. Use `.env.example` for placeholders only.
