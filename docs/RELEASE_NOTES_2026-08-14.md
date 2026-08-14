# PaperLess Release Notes - 2026-08-14

## Workflow Delete

- Added `DELETE /api/document-config-workflows/{docFormatCode}` so a superadmin can delete an entire Workflow at `/config/document-configs`, not just individual steps within one.
- Blocks with `document_config_workflow_in_use` (`409`) if any `signing_documents` row exists for that `(tenant, screen_code, doc_format_code)` — covers both SML-sourced and internal documents, since both write to `signing_documents`.
- When unused, deletes the workflow's `signature_templates` first (cascading to `signature_template_boxes`/`signer_note_template_boxes` via the existing `ON DELETE CASCADE` FK), then its `document_config_steps`, all in one transaction — no orphaned rows left behind.
- Added a delete button (trash icon, with confirm dialog) to the Workflow list page.

## Customer Deployment

Deployed same-session to all four shops (Damrong Homeplus, Pui, Wirat Home Mart, Insee Construction) as a single-service pair redeploy (`--no-deps api web`) — `db` and `sml-api` kept their exact prior container everywhere.

- Image: `paperless-api:6f59db8` (was `17cd7ce`), `paperless-web:6f59db8` (was `847fd23`).
- Verification per shop: `api`/`web` `healthy`/`Up`, public URL smoke `HTTP 200`, `/health/live` and `/health/ready` both OK.
- Release evidence under each shop's `/data/paperless/releases/20260814*-workflow-delete-6f59db8/postdeploy-checks.txt`; full narrative in `docs/CUSTOMER_DEPLOYMENT.md` (search for `2026-08-14`).

This was a new feature request (not a Damrong-specific bug report), so it went to all four shops together rather than Damrong-first.

## Status As Of 2026-08-14 (end of session)

- **Awaiting real-usage feedback from the customer.** Did not simulate a delete directly against the production DB (would bypass the very API layer being added), so the delete button has not yet been exercised end-to-end on a real shop.
- Two things to confirm once tested: (1) deleting a genuinely unused Workflow succeeds and removes it from the list; (2) attempting to delete a Workflow that already has real documents still returns the blocking `document_config_workflow_in_use` error instead of deleting anything.
- No other work in progress or blocked as of this entry.
