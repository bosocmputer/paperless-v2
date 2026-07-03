# QA Summary - 2026-07-03

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
curl -fsS http://127.0.0.1:8095/api/live
curl -fsS http://127.0.0.1:8095/api/ready
```

## Manual Regression Checklist

- Login requires database selection every time.
- `sml1_2026` on dev shows existing migrated data.
- A different selected tenant does not show `sml1_2026` data.
- Create document from SML and upload a multi-page PDF.
- Template boxes clone to every uploaded page and remain independently editable.
- Internal signer can sign from mobile width.
- External signer cannot reject, attach files, print, download, or open admin views.
- After signing completes, admin confirm uploads images before SML lock.
- SML image rows contain JPEG bytes in tenant and `_images` databases.
- Admin history preview excludes evidence appendix.
- Evidence dialog shows Thai text, English text, UUIDs, and hashes.
- Print action creates a print event before PDF opens.

## Known Operational Limits

- Read-only viewer is a UX control, not DRM.
- Browser print event proves official print copy was generated, not that paper physically printed.
- Customer login depends on real SML users and database permissions in `smlerpmaindata`.
