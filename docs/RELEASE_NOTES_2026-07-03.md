# PaperLess Release Notes - 2026-07-03

## Summary

This release turns PaperLess from the initial pilot stack into a production-ready SML-integrated signing system. It covers SML login/database selection, tenant isolation, mobile signer UX, external sign-only flow, multi-page template cloning, transparent signatures, read-only PDF preview, evidence/print audit flows, SML image upload, and customer Docker deployment preparation.

## Backend

- Added SML-authenticated login flow through the PaperLess SML API bridge.
- Added two-step login support: verify credentials first, choose database second.
- Added auto-provisioning of PaperLess users from SML users.
- Added local-auth fallback flag for rollout safety.
- Added tenant fields to signing documents and scoped document/workflow/template queries by selected SML tenant.
- Added multi-placement snapshots for signatures and legal notices.
- Separated signer tasks from PDF stamp placements.
- Added transparent signature normalization and visible-ink validation.
- Added current/final/print-copy PDF version handling.
- Added Thai-capable evidence PDF rendering with embedded Sarabun font.
- Added SML image rendering/upload with JPEG validation, page cap, retry, and completed repair support.
- Added official print-copy event creation before opening printable PDFs.
- Hardened public external signing API responses and state transitions.

## Frontend

- Renamed visible product text to PaperLess.
- Rebuilt login as SML credential step plus database selection step.
- Removed provider/data group text from the login page because these are system-managed.
- Added tenant-aware session state in the auth store.
- Added continuous PDF viewer with lazy rendering and cancellation.
- Added read-only PDF dialog without browser print/download toolbar.
- Improved mobile signer layout and reduced nonessential card density.
- Added external sign-only UX and success/already-signed states.
- Reworked admin document flow dialog and active menu mapping.
- Added admin external signer section with generate/copy link fallback.
- Added in-app admin guide and user guide with screenshots.
- Kept the user guide accessible from an icon in signer topbar and also available inside admin.

## SML Integration

- SML document snapshots are sent as JPEGs for up to 8 original document pages.
- Evidence appendix pages are excluded from SML snapshots.
- `sml_doc_images.image_file` is written in both the tenant DB and tenant `_images` DB.
- Main and `_images` rows share `guid_code`, `image_order`, and `system_id=PAPERLESS`.
- SML lock runs only after image upload succeeds.
- Retry remains idempotent and replaces rows by document number.

## QA Status

Validated flows include:

- Admin login and database selection.
- Admin create/send/confirm/history.
- Internal signer mobile queue, signing, flow dialog, and history.
- External signer OTP and sign-only flow.
- Current PDF vs final evidence PDF behavior.
- Official print copy creation.
- SML image upload and lock retry behavior.
- Customer deployment smoke on port `8095`.

Run before release:

```bash
npm --prefix frontend run build
cd backend && go test ./...
```

## Operational Notes

- Customer deploy path: `/data/paperless`
- Customer web URL: `http://45.122.49.250:8095`
- Customer stack should expose only the web container on host port `8095`.
- Backend, PaperLess Postgres, and SML API containers should stay internal to Docker networks.
- Real secrets must live only in `/data/paperless/config/.env.prod` on the server, not in git.
- Real SML credentials are required on the customer server; test credentials from dev are not valid there.
