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

## Signature-Template Box Slot Reconciliation

Damrong reported: adding a signature box for the 6th signer of a Workflow position (`/config/documents/2CO/signature-template`, Position 2, signer `999:ผู้จัดการแผนก`) always failed with `กรอบของ Position 2 มีลำดับ signer ซ้ำ`.

Root cause confirmed against production DB and `audit_logs`: a box's page position slot (`signer_slot`) is derived from that signer's index in the Workflow step's signer list at the moment the box is created. Editing the Workflow afterward to reorder or add signers left existing boxes on their old slot numbers — so a signer who moved to a different index collided with whichever box already held that slot number, with no way to resolve it from the UI even though nothing was actually duplicated. `audit_logs` showed the customer retrying add/save/delete about 10 times over roughly an hour, all hitting the same unresolvable collision.

- Immediate fix: manually corrected Damrong's 5 existing Position 2 boxes to the correct slots on production, verified in a rolled-back transaction first.
- Code fix (`paperless-api:4a018fd`, was `6f59db8`): `ReplaceDocumentConfigWorkflow` now re-derives every condition-2 step's box slots from the just-saved signer list, in the same transaction as the Workflow save, using a two-phase update through a placeholder slot to avoid unique-index collisions mid-update. Prevents the same class of bug on any Workflow whose signer order changes after boxes already exist, on any shop.

## Customer Deployment (signature-template fix)

Deployed same-session to all four shops as an `api`-only redeploy (`--no-deps api`) — `web`/`db`/`sml-api` untouched everywhere.

- Image: `paperless-api:4a018fd` (was `6f59db8`).
- Verification per shop: `api` `healthy`, public URL smoke `HTTP 200`, `/health/live` and `/health/ready` both OK. Damrong additionally confirmed post-deploy that Position 2's corrected slots were undisturbed by the restart.
- Release evidence under each shop's `/data/paperless/releases/20260814*-signature-slot-reconcile-4a018fd/postdeploy-checks.txt`.

## Status As Of 2026-08-14 (end of session)

- **Workflow delete**: awaiting real-usage feedback from the customer. Did not simulate a delete directly against the production DB (would bypass the very API layer being added), so the delete button has not yet been exercised end-to-end on a real shop. Two things to confirm once tested: (1) deleting a genuinely unused Workflow succeeds and removes it from the list; (2) attempting to delete a Workflow that already has real documents still returns the blocking `document_config_workflow_in_use` error instead of deleting anything.
- **Signature-template slot reconciliation**: Damrong's immediate data fix is confirmed correct (verified directly on production DB post-deploy). Awaiting customer confirmation that adding the 6th box on `2CO` Position 2 now succeeds in the UI.
- No other work in progress or blocked as of this entry.
