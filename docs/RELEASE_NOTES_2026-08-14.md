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

## Signature-Template Box Slot Reconciliation — Hotfix

Within an hour of the fix above shipping, Damrong hit a regression it introduced: removing a signer (not just reordering one) from a condition-2 Workflow step and saving returned `500 document_config_workflow_save_failed`, confirmed in API logs as a `signature_template_boxes_slot_unique_idx` violation on `1CO` Position 2. The reconciliation logic correctly re-slotted signers who moved index but left a *removed* signer's box on its old slot, so a remaining signer's new index collided with it — every Workflow save that removed a signer with an existing box failed outright, worse than the bug being fixed (previously the Workflow itself still saved fine).

Fixed in `paperless-api:9d3ac59` (was `4a018fd`): a box whose signer is no longer in the step's user list at all is now deleted before the remaining boxes' slots are reconciled, closing the gap instead of leaving a stale slot to collide with. Verified against a rolled-back transaction reproducing the exact failing scenario before deploying. Rolled out to all four shops within the same session as the regression (not staged Damrong-first, given real edit-in-progress was broken). Release evidence under each shop's `/data/paperless/releases/20260814*-signature-slot-reconcile-hotfix-9d3ac59/postdeploy-checks.txt`.

**Do not use `4a018fd` as a rollback target** — it carries this regression. Only roll back to it to isolate a problem specific to `9d3ac59` itself.

## Proactive Full-Codebase Audit — 8 Fixes

Customer asked for a full audit for other bugs after the two same-day incidents above. Spawned parallel review agents across `internal/store`/`internal/api` from multiple angles (correctness, concurrency, simplification, efficiency, altitude, removed-behavior), cross-verified and deduplicated to 8 confirmed findings — **none triggered by a user report**, purely proactive.

Fixed in `paperless-api:69816ac` (was `9d3ac59`):

1. `ReplaceSignatureTemplateBoxes` — added `FOR UPDATE` on the revision check, closing a silent-clobber race identical in shape to the slot-reconciliation bug above, just on template-box saves instead of workflow saves.
2. `DeleteDocumentConfigWorkflow` / `DeleteInternalDocumentMaster` — added the advisory lock `ReplaceDocumentConfigWorkflow` already uses for the same key, closing a concurrent save-vs-delete race.
3. Slot reconciliation now also covers `signer_note_template_boxes` (identical shape to `signature_template_boxes`, previously untouched — would have silently reproduced today's bug on note boxes instead of erroring).
4. Slot reconciliation now deletes boxes for a step whose condition type changed away from "every signer must sign," instead of leaving them stale.
5. `copyDocumentConfigWorkflow` now copies all 10 signer slots instead of just the first 3 — the backend counterpart to a frontend truncation fix shipped in `847fd23` was missed at the time.
6. `ReserveInternalDocument` now resolves a client double-submit racing the idempotency check to the winning document instead of a raw 500.
7. `signerRowsForStep` now matches signature boxes to signers by identity instead of array position, closing a latent (not currently UI-reachable) landmine.
8. Deleting an orphaned signature/sign-note box during a workflow save is now logged to the audit trail (`document_config.signature_box_removed`) — the delete itself is unchanged, just no longer silent.

Deployed same-session, `api`-only, to all four shops (`web`/`db`/`sml-api` untouched). Damrong's `1CO` Position 2 data (3 boxes) confirmed unchanged post-deploy; the condition-type-change scenario (fix #4) was dry-run against that same real data in a rolled-back transaction before deploying and confirmed correct. Release evidence under each shop's `/data/paperless/releases/20260814144*-audit-fixes-69816ac/postdeploy-checks.txt`.

## Status As Of 2026-08-14 (end of session)

- **Workflow delete**: awaiting real-usage feedback from the customer. Did not simulate a delete directly against the production DB (would bypass the very API layer being added), so the delete button has not yet been exercised end-to-end on a real shop. Two things to confirm once tested: (1) deleting a genuinely unused Workflow succeeds and removes it from the list; (2) attempting to delete a Workflow that already has real documents still returns the blocking `document_config_workflow_in_use` error instead of deleting anything.
- **Signature-template slot reconciliation**: Damrong's immediate data fix (2CO Position 2) is confirmed correct. The follow-up hotfix (9d3ac59) is deployed and its own test scenario (removing a signer from 1CO Position 2) passed in a rolled-back transaction, but has not yet been exercised live by the customer through the UI. Awaiting customer confirmation on both: adding the 6th box on 2CO Position 2, and removing a signer + saving on 1CO Position 2.
- **Proactive audit fixes (8 items)**: none were reported by the customer, so there is nothing specific to ask them to retest — normal usage over the coming days is the real-world verification. Two items worth flagging if anything unexpected surfaces: fix #7 (signer-box identity matching) changed matching logic on the document-creation path, and fix #6 (idempotency fallback) changes behavior only under a double-submit race, both low-traffic paths that hadn't been exercised with adversarial timing before this audit.
- No other work in progress or blocked as of this entry.
