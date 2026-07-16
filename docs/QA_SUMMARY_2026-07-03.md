# QA Summary - 2026-07-03

## Current Customer Test Status - 2026-07-16

Status: deployed to the customer server and waiting for customer feedback.

- Customer URL: `http://45.122.49.250:8095/`
- Customer release evidence: `/data/paperless/releases/20260716155654/postdeploy-checks.txt`
- PaperLess API/Web image tag tested: `20260716153839`
- SML API image tag tested: `20260716155654`

Latest customer smoke passed after the saved-signature release:

- `paperless-prod-web` was running; API, SML API, and DB were healthy.
- Web `/`, `/health/live`, and `/health/ready` returned HTTP 200.
- SML login returned 27 databases and issued a `superadmin` session for `STPT`.
- Saved-signature dry-run returned one available/new signature and no invalid/failed signature.
- The protected binary endpoint returned the real `superadmin` JPEG at `9000x5284`.
- `/admin/users` showed the SML Sync action and signature-status column with no browser console errors.
- Deployment QA used dry-run only; the customer still controls the first actual signature sync.

Known tenant readiness state from the latest customer smoke: `PTTP-TAX` is missing its main tenant DB and cannot be auto-provisioned by PaperLess.

## Scope

Final QA covered admin, internal user, external signer, SML integration, and customer deployment smoke.

Tested areas:

- SML login and database selection
- Tenant-scoped dashboard/list/history
- Workflow and signature template behavior
- Multi-page PDF placement cloning and per-page editing
- Internal mobile signing
- External sign-only page and already-signed state
- Admin document flow dialog
- Current/final/evidence PDF behavior
- Official print-copy flow
- SML JPEG image upload and lock flow
- Customer Docker stack on port `8095`

## Result

Main production blockers found during QA were fixed:

- SML main tenant `sml_doc_images.image_file` now receives JPEG bytes.
- Evidence PDF uses embedded Thai-capable font.
- User history opens `current` signed PDF by default.
- Login page no longer displays provider/data group text.
- Visible product name is PaperLess.
- SML document search supports both `ic_trans` and `ap_ar_trans`, including partial document number and AR/AP name lookup.
- External signer links are only available when the external signer is pending and it is their turn.
- Completed documents auto-finalize to SML without a manual admin confirm click.
- Role model now separates `superadmin`, `admin`, and `user`, with workflow/template/user configuration limited to `superadmin`.
- Admin document creation can use templates but cannot edit signature/legal-notice boxes.
- Flow เอกสาร can open the signed PaperLess document in a read-only dialog and keep detail navigation separate.

## Validation Commands

Run locally before release:

```bash
npm --prefix frontend run build
cd backend && go test ./...
```

Run on customer server after deploy:

```bash
docker ps --filter "name=paperless-prod"
curl -fsS http://127.0.0.1:8095/
curl -fsS http://127.0.0.1:8095/health/live
curl -fsS http://127.0.0.1:8095/health/ready
```

## Manual Regression Checklist

- Login requires database selection every time.
- `sml1_2026` on dev shows existing migrated data.
- A different selected tenant does not show `sml1_2026` data.
- Create document from SML and upload a multi-page PDF.
- Template boxes clone to every uploaded page and remain independently editable.
- Internal signer can sign from mobile width.
- External signer cannot reject, attach files, print, download, or open admin views.
- After signing completes, PaperLess auto-finalization uploads images before SML lock.
- SML image rows contain JPEG bytes in tenant and `_images` databases.
- Login self-service image DB setup works for a tenant that is missing only the `_images` DB/table.
- Admin history preview excludes evidence appendix.
- Evidence dialog shows Thai text, English text, UUIDs, and hashes.
- Print action creates a print event before PDF opens.

## Known Operational Limits

- Read-only viewer is a UX control, not DRM.
- Browser print event proves official print copy was generated, not that paper physically printed.
- Customer login depends on real SML users and database permissions in `smlerpmaindata`.
- PaperLess can provision missing image DB/table only when the main tenant DB exists and the selected SML user is allowed to access that database.
