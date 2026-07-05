# PaperLess

PaperLess is a controlled e-signature workflow for SML-originated PDF documents.
It receives document metadata and PDFs from SML, routes them through configured signing workflows, records audit evidence, writes signed PDF snapshots back to SML, and locks the ERP document after admin confirmation.

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
- Tenant isolation by selected SML database while keeping one PaperLess database
- Admin user management, workflow configuration, and signature template design
- Multi-page document creation with cloned signature/legal-notice placements that remain editable per page
- Internal signer task queue, waiting queue, and signing history
- External sign-only flow with OTP and sanitized public API surface
- Mobile-first signer workspace with continuous PDF viewer
- Transparent PNG signature normalization so PDF stamps do not cover document text
- `current` PDF for signed document preview, `final` PDF for audit evidence appendix
- Read-only PDF dialogs without browser download/print controls
- Official print-copy flow with print events before opening printable PDF
- Final admin confirm flow: signed PDF, SML JPEG snapshots, SML lock, retry states
- SML image upload writes JPEG bytes to both tenant DB and tenant `_images` DB
- In-app admin/user guides with QA screenshots

## Login Model

PaperLess authenticates against the SML auth API. The provider and data group are configured by the system and are not shown as user inputs on the login page.

Login is always two steps:

1. User enters SML username/password.
2. PaperLess verifies SML, shows every allowed database with tenant readiness status, and requires the user to choose one.

The selected database is checked again before JWT issuance. Databases missing `<tenant>_images` or a compatible `public.sml_doc_images` schema are blocked at login so users do not reach a broken confirm/upload flow.

On first successful SML login, PaperLess creates the local user automatically:

- `superadmin` becomes PaperLess role `admin`.
- Other users become role `user`.
- Existing local PaperLess role/status/display name are preserved.
- Inactive PaperLess users cannot log in even if SML credentials are valid.

`PAPERLESS_LOCAL_AUTH_FALLBACK_ENABLED=true` is only for migration/rollout safety. Production should disable it after SML users are ready.

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

When admin confirms a document, the backend:

1. Refreshes/generates the signed `current` PDF.
2. Builds `final` audit PDF.
3. Renders only original document pages, excluding evidence pages.
4. Sends JPEG snapshots for pages `1..min(originalPageCount, 8)` to SML.
5. Writes images to both tenant DB and tenant `_images` DB.
6. Locks the ERP document in SML after image upload succeeds.

If image upload fails, status becomes `completed_image_failed` and SML lock is not attempted. Admin can retry SML images. Retrying a completed document is allowed for repair and remains idempotent.

Before enabling a new SML tenant, verify that both the main database and matching image database exist. Example: tenant `stpt` requires `stpt` and `stpt_images`, both with `public.sml_doc_images`. Use the SML API maintenance command `verify-sml-tenant`; if the image DB is missing, provision it explicitly with `provision-sml-image-db` and then use PaperLess retry instead of direct SQL image inserts.

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

Do not commit real `.env` files, passwords, API keys, tokens, OTPs, PDF bytes, or signature images. Use `.env.example` for placeholders only.
