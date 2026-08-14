# Customer Deployment

## Target

- Customer URL: `http://45.122.49.250:8095`
- Stack path: `/data/paperless`
- Compose file: `/data/paperless/compose.yml`
- Production env file: `/data/paperless/config/.env.prod`
- Upload/PDF storage: `/data/paperless/uploads`
- Release evidence: `/data/paperless/releases/<timestamp>/`
- Preflight backup/snapshot: `/data/paperless/preflight-<timestamp>/`

The server has other projects. Keep PaperLess isolated and expose only the approved port.

The same release is also deployed for Wirat Home Mart at `http://43.240.113.44:8691`, using stack path `/data/paperless` and Compose project/container prefix `paperless-wirat`.

The same release is also deployed for Insee Construction at `http://45.122.49.253:8095`, using stack path `/data/paperless` and Compose project/container prefix `paperless-insee`.

The same release is also deployed for Damrong Homeplus at `http://45.122.49.252:8095`, using stack path `/data/paperless` and Compose project/container prefix `paperless-damrong`. This server hosts multiple unrelated customer projects (traefik, smlmcp, pickandpack, tms, accountupdater, pgadmin4) alongside a shared `sml_postgresql` instance serving many unrelated SML tenants; PaperLess containers and network are fully isolated under the `paperless-damrong-*` prefix and only port `8095` is published.

## Current Customer Status - 2026-08-14 (all four shops, paperless-api only): proactive full-codebase audit — 8 fixes

Customer asked for a full audit for other bugs after the two same-day production incidents below (4a018fd, 9d3ac59) on the signature-slot reconciliation logic. Spawned parallel review agents across the backend `internal/store` and `internal/api` layers (correctness, concurrency, simplification, efficiency, altitude, and removed-behavior angles), cross-verified and deduplicated the results down to 8 confirmed findings, fixed all 8, and verified each via `build`/`vet`/`test` plus rolled-back transactions against production data before deploying. **No user-reported symptom triggered any of these** — purely proactive, at the customer's request.

Fixed in `paperless-api:69816ac` (was `9d3ac59`):

1. **`ReplaceSignatureTemplateBoxes`** — added `FOR UPDATE` to the revision check (was missing the same lock pattern already applied to `document_config_steps`). Closes a silent-clobber race: two concurrent template-box saves at the same revision could both pass the optimistic check, and the second commit would discard the first admin's layout with no conflict error.
2. **`DeleteDocumentConfigWorkflow` / `DeleteInternalDocumentMaster`** — added the `pg_advisory_xact_lock` that `ReplaceDocumentConfigWorkflow` already takes for the same `(tenant+screenCode, docFormatCode)` key. Closes a race where a concurrent save and delete on the same workflow could interleave.
3. **`reconcileSignatureTemplateBoxSlotsTx`** — now also reconciles `signer_note_template_boxes` (identical `position_code`/`signer_slot`/`signer_user` shape to `signature_template_boxes`, previously untouched by today's earlier fixes). Without this, the exact same bug class would have silently reproduced on note boxes — rendering under the wrong or departed signer instead of throwing an error.
4. **Same reconciler** — a step whose `ConditionType` changed away from `2` in a save now has its now-meaningless slotted boxes deleted, instead of left stale to collide with a future placement.
5. **`copyDocumentConfigWorkflow`** — now copies all of `User01`–`User10` instead of only `User01`–`User03`. Matches the frontend fix already shipped in `847fd23` after max signers was raised 3→10 in `f15b534`; this backend copy handler was missed at the time.
6. **`ReserveInternalDocument`** — now catches a unique-constraint violation on the idempotency key (reachable via a client double-submit racing the unlocked idempotency check) and falls back to the winning request's document instead of surfacing a raw `500` while also burning a running-number slot on the losing attempt.
7. **`signerRowsForStep`** (condition_type=2 branch) — now matches each configured signer to their box by identity (`SignerUser`, via the existing-but-previously-unused `findBoxForUser` helper) instead of by array position after boxes are sorted by `signer_slot`. Removes a latent dependency on slot order matching `userNN` order that nothing enforced on every write path. Confirmed not reachable via the current UI flow during the audit; fixed as defense-in-depth.
8. **`ReplaceDocumentConfigWorkflow` and its two callers** (save, copy) — now write a `document_config.signature_box_removed` audit event for every box the reconciler deletes as an orphan. The delete itself is unchanged (still outright, not soft-delete — reversing that would be a larger schema change), but the removal is no longer silent.

This is an `api`-only change — `sml-api-bybos`/`paperless-web`/`db` untouched, so only `api` was redeployed on each shop.

- **Damrong Homeplus**: `api` deployed, healthy. Confirmed post-deploy that `1CO` Position 2's 3 signature boxes are unchanged (slots 1-3, matching current Workflow order). Pre-deploy, dry-ran the condition-type-change scenario (fix #4) against this shop's real `1CO` Position 2 data in a rolled-back transaction — confirmed the new delete-on-condition-change logic works cleanly. No `signer_note_template_boxes` rows exist yet on any shop (feature unused so far), so fix #3 could not be exercised against real data; covered by code review and the shared code path with fix #1's already-verified logic. Release evidence `/data/paperless/releases/20260814144526-audit-fixes-69816ac/postdeploy-checks.txt`.
- **Pui, Wirat Home Mart, Insee Construction**: `api` deployed same-session — healthy on each, public URL smoke HTTP 200. Release evidence:
  - Pui: `/data/paperless/releases/20260814144640-audit-fixes-69816ac/postdeploy-checks.txt`
  - Wirat Home Mart: `/data/paperless/releases/20260814144719-audit-fixes-69816ac/postdeploy-checks.txt`
  - Insee Construction: `/data/paperless/releases/20260814144813-audit-fixes-69816ac/postdeploy-checks.txt`

## Current Customer Status - 2026-08-14 (all four shops, paperless-api only): HOTFIX for the slot-reconciliation regression below

Within an hour of the `4a018fd` deploy directly below, Damrong reported a new failure: at `/config/documents/1CO/workflow`, removing a signer from Position 2 (5 signers, one of them the just-fixed `999:ผู้จัดการแผนก` case) and clicking save returned `500 document_config_workflow_save_failed`. Confirmed in `paperless-damrong-api` logs: `ERROR: duplicate key value violates unique constraint "signature_template_boxes_slot_unique_idx"`, `docFormatCode=1CO`.

Root cause: a regression in `4a018fd` itself. `reconcileSignatureTemplateBoxSlotsTx` correctly re-derived slots for signers who stayed in the Workflow but changed index, but for a signer **removed entirely** from the step it left that signer's box on its old slot number (incorrectly treated as "unchanged"). When a remaining signer's new index landed on that stale slot, the two-phase placeholder update never moved the orphaned box out of the way first, so the final `UPDATE` collided with it — **every Workflow save that removed a signer with an existing signature box now failed outright**, a worse regression than the original bug (previously the Workflow itself still saved fine; only *adding* a new box could hit the slot-duplicate error).

Verified root cause and fix against production DB (rolled-back transaction, not live data) reproducing the exact scenario: removing `0021:แจ่มจันทร์` from `1CO` Position 2's 5 signers. Old code: unique-constraint conflict. New code: orphaned box deleted first, remaining boxes re-slotted cleanly with no error.

**Fixed in `paperless-api:9d3ac59`** (was `4a018fd`): `reconcileSignatureTemplateBoxSlotsTx` now deletes a box outright once its signer is no longer in the step's user list at all, before reconciling the remaining boxes' slots via the existing two-phase placeholder update — closing the gap instead of leaving a stale slot to collide with.

This is an `api`-only hotfix, deployed to all four shops within the same session as the regression it fixes (not staged Damrong-first, given the severity — Workflow save was broken for a real edit-in-progress).

- **Damrong Homeplus**: `api` deployed, healthy. Confirmed post-deploy that `1CO` Position 2 still has all 5 original boxes untouched (the failed save attempts rolled back cleanly, no partial writes). Release evidence `/data/paperless/releases/20260814141210-signature-slot-reconcile-hotfix-9d3ac59/postdeploy-checks.txt`. Customer to retest: remove a signer from `1CO` Position 2 and save again.
- **Pui, Wirat Home Mart, Insee Construction**: `api` deployed same-session — healthy on each, public URL smoke HTTP 200. Release evidence:
  - Pui: `/data/paperless/releases/20260814141315-signature-slot-reconcile-hotfix-9d3ac59/postdeploy-checks.txt`
  - Wirat Home Mart: `/data/paperless/releases/20260814141355-signature-slot-reconcile-hotfix-9d3ac59/postdeploy-checks.txt`
  - Insee Construction: `/data/paperless/releases/20260814141441-signature-slot-reconcile-hotfix-9d3ac59/postdeploy-checks.txt`

**Rollback note**: do not roll back to `4a018fd` as a general-purpose rollback target — it carries the regression described above. Only use it to isolate a problem that turns out to be specific to `9d3ac59` itself.

## Current Customer Status - 2026-08-14 (all four shops, paperless-api only): signature-template box slot reconciliation

Damrong reported: at `/config/documents/2CO/signature-template`, adding the 6th signature box for Position 2 ("อนุมัติตรวจสินค้า", signer `999:ผู้จัดการแผนก`) always failed with `กรอบของ Position 2 มีลำดับ signer ซ้ำ`.

Root cause confirmed against production DB and `audit_logs` (not guessed): a box's `signer_slot` is derived from that signer's index in the Workflow step's `userNN` list at the moment the box is created. The customer had edited Position 2's Workflow earlier the same session (multiple `document_config.workflow_save` events, `stepCount` fluctuating 2→3→2) to add the 6th signer and change the user order — but the 5 pre-existing boxes kept their old `signer_slot` values (`1,2,4,5,6` instead of `1,2,3,4,5`), only correct at the time each box was originally placed. `audit_logs` showed the customer retrying `box_add` → `save_error` → `box_delete` roughly 10 times over about an hour, all blocked by the same unresolvable slot collision (slot 6 already held by a different signer whose real current index was 5).

**Immediate fix (this session, before the code deploy)**: corrected the 5 existing boxes' `signer_slot` directly on Damrong's production DB to `1,2,3,4,5` matching their signers' actual index in the current Workflow — verified in a rolled-back transaction first, then applied and confirmed. This alone unblocked the customer from adding the 6th box.

**Code fix in `paperless-api:4a018fd`** (was `6f59db8`): `ReplaceDocumentConfigWorkflow` now re-derives every condition-2 step's signature-template box `signer_slot` values from the just-saved signer list, in the same transaction as the Workflow save. Uses a two-phase update through a placeholder slot (`100000+index`) to avoid colliding with the unique `(template_id, position_code, signer_slot)` index mid-update — a negative placeholder was tried first but rejected by the `signer_slot > 0` check constraint; caught via a rolled-back transaction test against production before shipping, not in production. This prevents the same class of bug on any Workflow whose signer order changes after boxes already exist, on any shop.

This is a `paperless-api`-only change — `paperless-web`/`sml-api-bybos`/`db` untouched, so only `api` was redeployed on each shop.

- **Damrong Homeplus**: `api` deployed, healthy. Confirmed post-deploy that Position 2's boxes still show the corrected slots 1-5 (the earlier manual data fix was not disturbed by the restart). Release evidence `/data/paperless/releases/20260814114250-signature-slot-reconcile-4a018fd/postdeploy-checks.txt`. Customer to confirm adding the 6th box for `999:ผู้จัดการแผนก` now succeeds.
- **Pui, Wirat Home Mart, Insee Construction**: `api` deployed preventively (no reported symptom, no data fix needed — issue was specific to Damrong's `2CO` Position 2 boxes) — healthy on each, public URL smoke HTTP 200. Release evidence:
  - Pui: `/data/paperless/releases/20260814114712-signature-slot-reconcile-4a018fd/postdeploy-checks.txt`
  - Wirat Home Mart: `/data/paperless/releases/20260814114858-signature-slot-reconcile-4a018fd/postdeploy-checks.txt`
  - Insee Construction: `/data/paperless/releases/20260814115349-signature-slot-reconcile-4a018fd/postdeploy-checks.txt`

## Current Customer Status - 2026-08-14 (all four shops, api+web): Workflow delete feature

Customer request: allow deleting a Workflow at `/config/document-configs` when it has never been used to create a document — previously there was no delete action at all for a whole Workflow, only for individual steps within one (and step-delete itself was blocked while any signature-template box referenced that step).

Added `paperless-api:6f59db8` (was `17cd7ce`) / `paperless-web:6f59db8` (was `847fd23`):

- Backend: `DELETE /api/document-config-workflows/{docFormatCode}`. Blocks with `document_config_workflow_in_use` (`409`) if any `signing_documents` row exists for that `(tenant, screen_code, doc_format_code)` — this single check covers both SML-sourced and internal documents, since both write to `signing_documents`. Otherwise deletes that workflow's `signature_templates` rows first (cascading to `signature_template_boxes`/`signer_note_template_boxes` via the existing `ON DELETE CASCADE` FK), then its `document_config_steps` rows, all in one transaction.
- Frontend: delete button (trash icon) added to the Workflow list page (`config/document-configs`), with a confirm dialog before calling the new endpoint.

This is a `paperless-api` + `paperless-web` change only — `sml-api-bybos`/`db` untouched, so only `api` and `web` were redeployed on each shop.

- **Damrong Homeplus**: deployed, both healthy. Release evidence `/data/paperless/releases/20260814105058-workflow-delete-6f59db8/postdeploy-checks.txt`. Did not simulate the delete directly against the DB (would bypass the API layer being added) — customer to retest: create/reach an unused Workflow and confirm the new delete button works, and separately confirm a Workflow with real documents still blocks with `document_config_workflow_in_use`.
- **Pui, Wirat Home Mart, Insee Construction**: deployed together with Damrong (not a Damrong-specific bug fix, a new feature rolled out everywhere at once) — healthy on each, public URL smoke HTTP 200. Release evidence:
  - Pui: `/data/paperless/releases/20260814105232-workflow-delete-6f59db8/postdeploy-checks.txt`
  - Wirat Home Mart: `/data/paperless/releases/20260814111424-workflow-delete-6f59db8/postdeploy-checks.txt`
  - Insee Construction: `/data/paperless/releases/20260814111704-workflow-delete-6f59db8/postdeploy-checks.txt`

## Current Customer Status - 2026-08-13 (all four shops, paperless-api only): stop auto-reseeding PAYREQ/ADV/PREPAY on every masters-list load

Follow-up to the cascade-delete fix directly below. Customer deleted a `virin`-tenant `PREPAY` master (`DELETE` returned `204`, confirmed removed from the DB), but the very next `GET /api/internal-document-masters` list showed a `PREPAY` row again with a brand-new id. Root cause: both `listDocumentTypes` and `listInternalDocumentMasters` handlers called `store.EnsureDefaultInternalDocumentMasters` on every request, which `INSERT`ed `PAYREQ`/`ADV`/`PREPAY` back (`ON CONFLICT (sml_tenant, code) DO NOTHING`, so only when that code didn't already exist) — any delete of one of these three codes was immediately undone by the next page load/list refresh.

Fixed in `paperless-api:17cd7ce` (was `c82ce6e`): removed both call sites and the now-dead `EnsureDefaultInternalDocumentMasters` store function. This is a **deliberate behavior change**, confirmed with the customer/operator before shipping: nothing is auto-seeded on page load anymore — a tenant that has never opened `/config/internal-document-masters` will no longer see `PAYREQ`/`ADV`/`PREPAY` pre-created; the operator creates masters explicitly via the existing "สร้างใหม่" flow during setup, same as any tenant that had one of the three deleted.

This is purely a `paperless-api` fix — no `sml-api-bybos`/`paperless-web` code changed, so only the `api` container was redeployed on each shop; `web`/`db`/`sml-api` untouched everywhere.

- **Damrong Homeplus**: `api` deployed, healthy. Confirmed post-restart that the `virin` tenant still has exactly 3 masters with the same ids as before this deploy (no unexpected auto-create on container start). Release evidence `/data/paperless/releases/20260813114442-remove-master-autoseed-17cd7ce/postdeploy-checks.txt`. Customer to retest: delete a `PAYREQ`/`ADV`/`PREPAY` master, reload the masters page, confirm it does not reappear.
- **Pui, Wirat Home Mart, Insee Construction**: `api` deployed preventively — healthy on each, public URL smoke HTTP 200. Release evidence:
  - Pui: `/data/paperless/releases/20260813114934-remove-master-autoseed-17cd7ce/postdeploy-checks.txt`
  - Wirat Home Mart: `/data/paperless/releases/20260813115155-remove-master-autoseed-17cd7ce/postdeploy-checks.txt`
  - Insee Construction: `/data/paperless/releases/20260813115714-remove-master-autoseed-17cd7ce/postdeploy-checks.txt`

**Rollback note**: rolling back to `c82ce6e` restores the auto-reseed behavior but does not restore any rows deleted while `17cd7ce` was live — deletes are not reversible by an image rollback.

## Current Customer Status - 2026-08-13 (all four shops, paperless-api only): internal document master delete blocked by orphaned config/signature templates

Damrong reported unable to delete an internal document master at `/config/internal-document-masters` with `internal_master_in_use`, despite the `drh` tenant never having created a real internal document. Confirmed against production DB: `drh` had 0 `internal_documents` rows but 1 `signature_templates` row for each of `PAYREQ`/`ADV`/`PREPAY` (from the customer already using "จัดวางกรอบลายเซ็น" to place signer boxes during normal setup, per the Internal Document Rollout runbook above). `DeleteInternalDocumentMaster` blocked deletion whenever a `document_config_steps` or `signature_templates` row existed for the master's code — not only when a real `internal_documents` row existed — so setup-only masters with zero real usage could never be deleted or recoded, only disabled.

Fixed in `paperless-api:c82ce6e` (was `847fd23`): `DeleteInternalDocumentMaster` now runs in a transaction. It still blocks with `internal_master_in_use` if any `internal_documents` row references the master; otherwise it deletes the orphaned `document_config_steps`/`signature_templates` rows for that (tenant, code) before deleting the master, instead of refusing outright.

This is purely a `paperless-api` fix — no `sml-api-bybos`/`paperless-web` code changed, so only the `api` container was redeployed on each shop; `web`/`db`/`sml-api` untouched everywhere.

- **Damrong Homeplus**: `api` deployed, healthy. Release evidence `/data/paperless/releases/20260813111350-internal-master-cascade-delete-c82ce6e/postdeploy-checks.txt`. Did not simulate the delete directly against the DB (would bypass the API layer being fixed) — customer to retest deleting the `drh` `PAYREQ`/`ADV`/`PREPAY` masters from the UI to confirm end-to-end.
- **Pui, Wirat Home Mart, Insee Construction**: `api` deployed preventively (no reported symptom on these three, but the same bug applies to any tenant with a setup-only master) — healthy on each, public URL smoke HTTP 200. Release evidence:
  - Pui: `/data/paperless/releases/20260813112100-internal-master-cascade-delete-c82ce6e/postdeploy-checks.txt`
  - Wirat Home Mart: `/data/paperless/releases/20260813112329-internal-master-cascade-delete-c82ce6e/postdeploy-checks.txt`
  - Insee Construction: `/data/paperless/releases/20260813112637-internal-master-cascade-delete-c82ce6e/postdeploy-checks.txt`

## Current Customer Status - 2026-08-13 (all four shops, sml-api-bybos only): signature permission case-sensitivity fix

Damrong reported all 42 `drh`-tenant users blocked syncing saved signatures with `signature_user_not_allowed` in the "Sync ผู้ใช้และลายเซ็นจาก SML" dialog. Root cause confirmed against live production (not guessed): `isSyncCandidateUser` in `sml-api-bybos` compared `data_group`/`data_code` with exact-match equality, while `sml_database_list` stores tenant `drh` as `data_code='drh'` (lowercase) and `sml_database_list_user_and_group` stores that same tenant's permission rows as `data_code='DRH'` (uppercase) — the two never matched, so every otherwise-valid `active_status=1` user in that tenant was denied. Reproduced directly: the exact failing request against `paperless-damrong-sml-api` returned `403 signature_user_not_allowed` pre-fix; the same SQL run manually with the correct-case `data_code` returned the expected 120 permission rows.

Fixed in `sml-api-bybos:99de187` (was `fd9bafd`): both CTEs (`direct_allowed`, `group_allowed`) in `isSyncCandidateUser` now compare `data_group`/`data_code` via `lower(trim())`, matching the pattern already used everywhere else in `auth.go` (`lookupDatabase`, `listUserDatabases`) — only this one query had the inconsistency. Generic fix, not `drh`-specific; any tenant with mismatched casing between the two tables would hit the same bug.

This is purely an `sml-api-bybos` fix — no `paperless-api`/`paperless-web` code changed, so only the `sml-api` container was redeployed on each shop; `api`/`web`/`db` untouched everywhere.

- **Damrong Homeplus**: `sml-api` deployed, healthy. Verified by re-running the exact previously-failing request (tenant `drh`, userCodes `77711059`/`7770751`/`1411`/`10392`) directly against the container post-deploy — all now return `200 OK` with the real signature image (was `403`). Release evidence `/data/paperless/releases/20260813103738-signature-permission-case-fix-99de187/postdeploy-checks.txt`.
- **Pui, Wirat Home Mart, Insee Construction**: `sml-api` deployed preventively (no reported symptom on these three, but the same code path applies to any shop) — healthy on each, public URL smoke HTTP 200. No app-login credential available in this session to re-run a live signature fetch on these three; fix is the same code path verified working on Damrong. Release evidence:
  - Pui: `/data/paperless/releases/20260813104137-signature-permission-case-fix-99de187/postdeploy-checks.txt`
  - Wirat Home Mart: `/data/paperless/releases/20260813104724-signature-permission-case-fix-99de187/postdeploy-checks.txt`
  - Insee Construction: `/data/paperless/releases/20260813105436-signature-permission-case-fix-99de187/postdeploy-checks.txt`

**Follow-up not yet done**: 42 rows in Damrong's `user_saved_signatures` table still carry the stale `last_error='signature_user_not_allowed'` from before this fix — these are status records only, not currently blocking, and will clear on the next superadmin-triggered "Sync จาก SML" from `/admin/users` for tenant `drh`. Not repaired directly via SQL per this doc's guidance against direct signature-data repair; customer/admin should re-run the sync action to refresh the displayed status.

## Current Customer Status - 2026-08-11 (Wirat, Insee, Damrong): synced to Pui's `847fd23` — PDF clearance notice removed + Workflow signer-slot validation fix

Pui confirmed both fixes below working; rolled the same image tag out to the remaining three shops (`api` and `web` recreated on each, `sml-api`/`db` untouched).

- Image: `ghcr.io/bosocmputer/paperless-api:847fd23`, `ghcr.io/bosocmputer/paperless-web:847fd23`
- Deployed to: Wirat, Insee, Damrong (all four shops now on `847fd23` for `api`/`web`)
- Verification per shop: `docker compose ps` shows `api`/`web` healthy on `847fd23`; public URL smoke `curl` returned HTTP 200 (Wirat `43.240.113.44:8691`, Insee `45.122.49.253:8095`, Damrong `45.122.49.252:8095`)
- Release evidence: `/data/paperless/releases/<timestamp>-pdf-notice-and-signer-slot-fix-847fd23/postdeploy-checks.txt` on each shop
- Rollback per shop: restore that shop's `compose.yml.bak-<timestamp>` (pins `api:2a5cdca`, `web:f42ca71`) and re-run `docker compose up -d --no-deps api web`.

## Current Customer Status - 2026-08-11 (Pui, web-only): Workflow config — attachment requirements now validate against all 10 signer slots

Follow-up to the api-only release below (same image tag `847fd23`, other half of that bundled commit pair). `validateStepForm()` in `frontend/src/views/config/DocumentConfigWorkflow.vue` only copied `user01`-`user03` into its validation draft, a leftover from before `MAX_SIGNERS` was raised to 10 (commit `f15b534`, 2026-08-10). With 4+ signers configured on a step, `signerValues()` on the truncated draft undercounted available slots, so clicking "ใช้กับทุกผู้เซ็น" (apply attachment requirement to all signers) on a step with more than 3 signers produced a false validation error: "ช่องเอกสารแนบบังคับไม่ตรงกับผู้เซ็น" on positions 4 and up. Fixed by spreading all 10 `userNN` fields into the draft via the existing `userFieldsFrom()` helper instead of hardcoding 3.

- Commit: `847fd23`
- Image: `ghcr.io/bosocmputer/paperless-web:847fd23`
- Deployed to: Pui only (`paperless-prod-web` container). `api` was already on this tag from the previous entry; `sml-api`, `db` untouched.
- Verification: `npm run build` clean before deploy; post-deploy `docker compose ps` shows `paperless-prod-web` on tag `847fd23`; public URL smoke `curl` returned HTTP 200. Customer to manually retest the reported repro (Workflow config → 10 signers → mandatory attachment → apply to all signers) before wider rollout.
- Release evidence: `/data/paperless/releases/<timestamp>-workflow-signer-slot-validation-fix-847fd23/postdeploy-checks.txt`
- Rollback: restore `compose.yml.bak-<timestamp>` (pins `paperless-web:f42ca71`) and re-run `docker compose up -d --no-deps web`.
- Not yet deployed to Wirat, Insee, Damrong — awaiting Pui customer confirmation first, per usual convention.

## Current Customer Status - 2026-08-11 (Pui, api-only): removed 15-day reimbursement clearance notice from internal PDF summary

Customer requested removal of the fixed policy line "*ให้ผู้เบิกเงินเคลียร์สำรองจ่ายภายใน 15 วัน นับจากวันรับเงิน" printed under the approval box on every internal document PDF (`drawInternalSummary` in `backend/internal/api/internal_document_pdf.go`). Line and its now-unused text-color/position setup removed; approval box now ends the summary section directly. No other PDF content changed.

- Commit: `4679555` (bundled into image tag `847fd23` alongside the Workflow-config frontend fix above, commit `847fd23` — both are now deployed to Pui as of this entry and the one above)
- Image: `ghcr.io/bosocmputer/paperless-api:847fd23`
- Deployed to: Pui only (`paperless-prod-api` container). `web`, `sml-api`, `db` untouched at time of this specific deploy.
- Verification: `go build ./...` clean, `go test ./internal/api/...` all PDF/internal tests passing before deploy; post-deploy `docker compose ps` shows `paperless-prod-api` healthy on tag `847fd23`; public URL smoke `curl` returned HTTP 200.
- Release evidence: `/data/paperless/releases/<timestamp>-remove-15day-clearance-notice-847fd23/postdeploy-checks.txt`
- Rollback: restore `compose.yml.bak-<timestamp>` (pins `paperless-api:2a5cdca`) and re-run `docker compose up -d --no-deps api`.
- Not yet deployed to Wirat, Insee, Damrong — awaiting go-ahead.

## Current Customer Status - 2026-08-11 (all four shops, web-only): stay-stuck-on-page UX fix after sml_source_changed sign attempt

Follow-up from the `guid_code` fix confirmation: customer tested a different in-flight document (`/admin/signing/tasks/27fd4b01-b7f8-4ec0-97e3-0423983eb19c`) that hit a genuine `sml_source_changed` block on confirm-sign. The API correctly rejected it, but the signer was left stuck on the signing page after the error toast with no way forward — the whole document is blocked at that point (see `blockSigningOnSMLSourceDrift`), so staying on the page serves no purpose.

Fixed in `paperless-web:f42ca71` (was `31fd483`):
- `SigningTask.vue` (internal/admin signing, `/admin/signing/tasks/:taskId`): on `sml_source_changed`/`sml_source_missing`, navigate back to the task list automatically (same `goBackToTasks()` already used on successful sign/reject), instead of leaving the signer stranded on a task that can't become signable again without an admin cancelling/re-importing it. The error toast still shows first.
- `PublicSigning.vue` (external/token-based signing, no task list to return to): added `sml_source_changed`/`sml_source_missing` to the existing terminal-state handler (`handlePublicSigningError`) already used for other "this link can no longer be used" cases (`already_rejected`, session expiry, etc.) — shows the same in-place terminal screen instead of leaving the form active.

- Deployed to **all four shops** (web-only, `--no-deps web`, `api`/`db`/`sml-api` untouched everywhere), HTTP 200 confirmed on each:
  - Pui: `/data/paperless/releases/20260811143439-navigate-away-source-changed-f42ca71/postdeploy-checks.txt`
  - Wirat Home Mart: `/data/paperless/releases/20260811143518-navigate-away-source-changed-f42ca71/postdeploy-checks.txt`
  - Insee Construction: `/data/paperless/releases/20260811143601-navigate-away-source-changed-f42ca71/postdeploy-checks.txt`
  - Damrong Homeplus: `/data/paperless/releases/20260811143628-navigate-away-source-changed-f42ca71/postdeploy-checks.txt`
- Customer notified with a general explainer covering the full "document edited in SML while awaiting signature" flow across all four shops (not shop-specific) — how the per-signer-step check works, what the signer/admin sees, the cancel-and-reimport recovery path (including the create-new-page quick-cancel button), and the clarification that merely opening a document for editing in SML (without saving) does not trigger a block, now that `guid_code` is excluded from the hash. Awaiting customer feedback before treating this batch of fixes (numeric-formatting hash fix, `guid_code` hash fix, history status banner, dialog/panel cleanup, quick-cancel action, navigate-away UX) as fully confirmed across all four shops.

## Current Customer Status - 2026-08-11 (all four shops, sml-api-bybos only): second SML source-hash false-positive fixed — `guid_code` transient edit marker

Customer testing on Pui uncovered a second, independent false-positive source in the SML source-revision hash (the first was numeric trailing-zero drift, fixed in `sml-api-bybos:9875c51`, 2026-08-11 earlier). Document `PU-01-2608001` was blocked in `sml_source_changed` while the customer had it open for editing in the SML ERP UI but had **not yet clicked Save or Cancel**.

Root cause confirmed against live production (not guessed): `ic_trans.guid_code` and `ap_ar_trans.guid_code` are transient markers SML's web service writes (`SMLWebService.RandomGUID@...`) the instant a document is opened for editing, and clears back to empty on both Save and Cancel. A full-table scan of `ic_trans` (3,285 rows, all doc formats) found exactly one row with a non-empty `guid_code` — the one document the tester had open at that moment. These columns were not previously excluded from the hash, so merely viewing/starting to edit an in-flight document in SML (no actual save) was enough to trip the per-signer-step drift check.

Fixed in `sml-api-bybos:fd9bafd` (was `9875c51`): extended the existing `- 'is_lock_record' - 'last_status'` exclusion chain in `candidateSourceRevisionBatchQuery()` to also exclude `- 'guid_code'`, on both the `ic_trans` and `ap_ar_trans` header hashes (confirmed via `information_schema.columns` that neither detail table has this column, so the detail-row hash is untouched). Verified in a rolled-back transaction against a cloned production row: the old formula flips when only `guid_code` changes (proves the bug), the new formula doesn't (proves the fix), and a real business-field edit (`total_amount+1`) still flips both (proves the fix doesn't mask genuine changes). Also checked the real `PU-01-2608001` document directly post-deploy. A runbook for finding future transient/session markers like this one (without a full manual audit of the shared ERP schema) is now documented directly above `candidateSourceRevisionBatchQuery()` in the source.

This is purely an `sml-api-bybos` fix — no `paperless-api`/`paperless-web` code changed, so only the `sml-api` container was redeployed on each shop; `api`/`web`/`db` untouched everywhere.

- **Pui**: `sml-api` deployed, healthy. Rebaseline run via the existing `POST /api/admin/sml/source-revisions/rebaseline` endpoint: 2 in-flight documents found and re-baselined (`PU26060003`, `PU17100009`), 0 skipped, 0 failed. Release evidence `/data/paperless/releases/20260811141842-guid-code-hash-fix-fd9bafd/postdeploy-checks.txt`.
- **Wirat Home Mart**: `sml-api` deployed, healthy. Rebaseline candidate query run directly against the DB (no app-login credential for this shop) — 0 candidates. Release evidence `/data/paperless/releases/20260811141955-guid-code-hash-fix-fd9bafd/postdeploy-checks.txt`.
- **Insee Construction**: `sml-api` deployed, healthy. 0 rebaseline candidates (confirmed `POIN66-5958`, still in terminal `sml_source_changed`, correctly excluded by the candidate filter and untouched). Release evidence `/data/paperless/releases/20260811142038-guid-code-hash-fix-fd9bafd/postdeploy-checks.txt`.
- **Damrong Homeplus**: `sml-api` deployed, healthy. 0 rebaseline candidates. Release evidence `/data/paperless/releases/20260811142123-guid-code-hash-fix-fd9bafd/postdeploy-checks.txt`.

## Current Customer Status - 2026-08-11 (Pui, web-only): quick-cancel a source-drift-blocked document from create-new

Customer question after testing the earlier per-step drift check: when re-uploading a document blocked in `sml_source_changed` (e.g. `PU-01-2608001`), the only recovery path was to click through to the document's detail page and manually type a cancellation reason there. Customer asked whether the system could auto-cancel with a reason supplied automatically.

Fixed in `paperless-web:eaedc20` (was `6ac9502`), keeping a deliberate confirm step rather than fully automatic cancellation (preserves an intentional audit trail — the user still reviews and clicks confirm, only the reason-typing friction is removed):
- `SigningDocumentCreate.vue` (`/signing/documents/new`): the duplicate-block message now shows a "ยกเลิกเอกสารเดิม" button alongside the existing "เปิดเอกสารเดิม" button, but only when the blocking document's status is `sml_source_changed`/`sml_source_missing` (not for ordinary duplicates). Clicking it opens a cancel dialog with the reason pre-filled with the same attention message already shown on screen — still editable, still requires the user to click confirm. After a successful cancel, the duplicate check re-runs automatically so the user can upload the replacement PDF without leaving the page.
- `SigningDocumentDetail.vue`: the existing cancel dialog also now pre-fills the same default reason for these two attention states, for consistency with the new create-new page action.

- Deployed to **Pui only**. Release evidence `/data/paperless/releases/20260811130843-quick-cancel-blocking-doc-eaedc20/postdeploy-checks.txt`. HTTP 200 confirmed post-deploy, `api`/`db`/`sml-api` untouched.
- Follow-up: the two action buttons only right-aligned on desktop widths (parent's `justify-between` on `md:flex-row`); on the stacked mobile layout they fell back to left-aligned under the message text. Added `justify-end` to the button group so it stays right-aligned at any width. Deployed to **Pui only** as `paperless-web:1694c64`, release evidence `/data/paperless/releases/20260811131510-right-align-buttons-1694c64/postdeploy-checks.txt`, HTTP 200 confirmed.
- Second follow-up (screenshot review): on wide screens the message text/tag and the two buttons still shared one crowded row (`md:flex-row justify-between` put them side by side with minimal separation). Removed the `md:flex-row`/`justify-between` so the block always stacks vertically — text on top, button row below, right-aligned — at every screen width, not just mobile. Deployed to **Pui only** as `paperless-web:8ed67d9`, release evidence `/data/paperless/releases/20260811132227-stack-duplicate-block-buttons-8ed67d9/postdeploy-checks.txt`, HTTP 200 confirmed.
- Third follow-up (screenshot review again): the buttons still rendered flush-left under the text — `justify-end` had no effect because the flex containers were only as wide as their content (PrimeVue's `Message` content slot doesn't stretch full-width by default). Added `w-full` to both the outer stack and the button row so `justify-end` has actual room to push the buttons to the right edge. Deployed to **Pui only** as `paperless-web:438b1d1`, release evidence `/data/paperless/releases/20260811133037-fullwidth-duplicate-block-438b1d1/postdeploy-checks.txt`, HTTP 200 confirmed.
- Fourth follow-up: still no change on screen — traced the actual root cause by reading PrimeVue's `Message.vue` source directly this time instead of guessing again. The real flex item that needed to grow was PrimeVue's own `.p-message-text` wrapper (a sibling of the icon inside `.p-message-content`'s flex row), not any div under our control — our slotted content sits one level further in, so `width:100%`/`flex` on our own wrapper never had an effect since `.p-message-text` itself stayed sized to its content, one level up. Added a scoped `:deep(.p-message-text) { flex: 1 1 auto; min-width: 0 }` rule to grow the actual flex item; verified by inspecting the compiled CSS output before deploying rather than shipping another guess. Deployed to **Pui only** as `paperless-web:31fd483`, release evidence `/data/paperless/releases/20260811133928-fix-message-text-flex-31fd483/postdeploy-checks.txt`, HTTP 200 confirmed.
- Not yet rolled out to Wirat, Insee, Damrong — pending customer confirmation on Pui.

## Current Customer Status - 2026-08-11 (Pui, web-only): dialog header cleanup, sign-task panel reorganized

Follow-up UX feedback from the same Pui test session: (1) the "Flow เอกสาร SML" and "ตรวจสอบเอกสารอ้างอิง" dialogs used a bespoke branded header (icon box, gradient background, colored left border) that didn't match the plain-header pattern used by every other dialog in the app; (2) the sign-task page (`/admin/signing/tasks/:taskId`) right-side panel stacked signature, attachments, PDF notes, legal checkbox, and confirm/reject buttons with uniform visual weight and no ordering — testing feedback was "scattered, signer doesn't know what to do first."

Fixed in `paperless-web:6ac9502` (was `762a4b4`):
- Removed the custom `#header` slot and matching `:global()` CSS from both dialogs. The Reference Check dialog is rendered from two places (`SigningWorkspace.vue` and `SigningDocuments.vue`) but was styled from a third file (`DocumentReferenceCheck.vue`, via global selectors targeting DOM it doesn't render) — confirmed via pre-flight grep that these were the only two consumers, then updated all three files together in one commit to avoid leaving either consumer with broken unstyled header markup.
- Reordered the sign-task panel: signature → legal-text checkbox → confirm/reject buttons now form one visually grouped "required path" at the top, framed with the same primary-color-tinted border/background already used elsewhere in the app (`.position-summary`'s color-mix formula, not a new style). Optional tools (related-document lookup, attachments, PDF annotation notes) moved below with lighter visual weight. Pure DOM reorder + CSS — no `v-if`/logic/event-handler changes.

- Deployed to all four shops after customer confirmation on Pui: `paperless-web:6ac9502`, web-only (`--no-deps web`, `api`/`db`/`sml-api` untouched everywhere), HTTP 200 confirmed on every shop post-deploy.
  - Pui: release evidence `/data/paperless/releases/20260811122602-dialog-panel-cleanup-6ac9502/postdeploy-checks.txt`
  - Wirat Home Mart: `/data/paperless/releases/20260811122939-dialog-panel-cleanup-6ac9502/postdeploy-checks.txt`
  - Insee Construction: `/data/paperless/releases/20260811123047-dialog-panel-cleanup-6ac9502/postdeploy-checks.txt`
  - Damrong Homeplus: `/data/paperless/releases/20260811123114-dialog-panel-cleanup-6ac9502/postdeploy-checks.txt`

## Current Customer Status - 2026-08-11 (Pui, web-only): history UI now surfaces SML source-drift status

Customer testing on Pui (`PU-01-2608001`) confirmed the per-signer-step drift check (deployed above) correctly blocks a document mid-flow when SML source data is edited after signing starts — the document disappeared from `/admin/signing/tasks` as expected. Follow-up feedback: the block reason wasn't visible on the history pages, only in the raw sign-attempt error response — a signer/admin looking at `/admin/signing/history` afterward had no way to tell the document had stopped short of reaching SML.

Fixed in `paperless-web:762a4b4` (was `2a5cdca`): `signing_document_status` (`sml_source_changed`/`sml_source_missing`) is now surfaced as a prominent warning Tag/Message wherever signing history is shown — the shared `SigningWorkspace.vue` component (header tag + banner, used by both the detail page and the public/task-signing view), the admin history table (`AdminMySigningHistory.vue`), and the personal history cards (`MySigningHistory.vue`). Previously this only appeared as small muted caption text in the admin table and not at all on the personal history page.

- Deployed to **Pui only** (web-only change, `--no-deps web`, `api`/`db`/`sml-api` untouched) for the customer to re-verify directly against `PU-01-2608001` before rolling out to the other three shops. Release evidence `/data/paperless/releases/20260811115437-history-drift-banner-762a4b4/postdeploy-checks.txt`. HTTP 200 confirmed post-deploy.
- Not yet rolled out to Wirat, Insee, Damrong — pending confirmation from this Pui test.

## Current Customer Status - 2026-08-11 (all four shops: SML source-hash false-positive fix, per-step drift check, Thai time display)

Root cause of a real production incident (PO `POIN66-5958` on Insee, blocked from SML image sync with status `sml_source_changed` despite zero real edits, confirmed via direct SML audit-trail inspection) traced to `sml-api-bybos`'s source-revision hash including unscaled `numeric` columns whose trailing-zero formatting can drift without any real value change. Fixed and rolled out to all four shops:

- `sml-api-bybos:9875c51` (was `b9088b4`) — normalizes every unscaled `numeric` column (header + detail rows, both `ic_trans`/`ic_trans_detail` and `ap_ar_trans`/`ap_ar_trans_detail`) to 2 decimal places before hashing. Verified against live production data (read-only/rolled-back transactions only) before rollout: a cosmetic numeric-format rewrite no longer flips the hash, while a genuine value edit still does, for both header and detail tables.
- `paperless-api`/`paperless-web:2a5cdca` (was `a56a52c`/`162e907`) — three additions:
  - Per-signer-step SML source re-verification (previously only checked at send and finalize, leaving long multi-signer gaps — the Insee incident had a 20-hour gap between two signers — completely unchecked). Fails **open** on SML transport errors/timeouts (does not block signing), fails **closed** only on a confirmed source-changed/source-missing result from SML.
  - Field-level diff now recorded in the audit/event metadata whenever a source mismatch is detected (which SML fields disagree, e.g. `doc_date`, `total_amount`), so a future incident like this one is diagnosable from the audit log alone.
  - New superadmin-only endpoint `POST /api/admin/sml/source-revisions/rebaseline` — re-baselines any still-in-flight (non-terminal-status) SML document's stored source revision against a live SML lookup. Necessary one-time step per shop after the `sml-api-bybos` hash-formula fix ships, since in-flight documents sent under the old formula would otherwise false-positive at their next finalize purely because the formula changed, not because SML data changed. Optimistically locked against concurrent send/finalize; documents already in a terminal attention state (e.g. `POIN66-5958`, already `sml_source_changed`) are excluded by the candidate filter, not specially special-cased.
  - Also pins frontend date/time display to `Asia/Bangkok` (`frontend/src/utils/signingFormatters.js`, consolidating 8 files that previously hand-rolled their own `Intl.DateTimeFormat` with no explicit timezone, silently following the viewer's browser timezone instead of matching SML's Bangkok-local timestamps).

Rollout order per shop: `sml-api` → `api`+`web` (same image tag `2a5cdca` for both) → rebaseline. All four shops completed same-session:

- **Pui**: `sml-api` deployed first standalone, then `api`+`web`. Rebaseline run via the new endpoint: 2 in-flight documents found and re-baselined (`PU17100009`, `PU26060003`), 0 skipped, 0 failed.
- **Wirat Home Mart**: all three images deployed together. Rebaseline candidate query run directly against the DB (no app-login credential available for this shop in this session) — 0 candidates (this shop currently has 0 `signing_documents` rows total).
- **Insee Construction**: all three images deployed together. Rebaseline candidate query run directly against the DB — 0 candidates; confirmed `POIN66-5958` (status `sml_source_changed`, a terminal state) is correctly excluded by the filter and was not touched.
- **Damrong Homeplus**: all three images deployed together. Rebaseline candidate query run directly against the DB — 0 candidates.
- Post-deploy smoke: HTTP 200 on each shop's public URL, all 4 containers (`sml-api`/`api`/`db`/`web`) healthy on every shop after recreation. `db` never recreated anywhere. Release evidence per shop under `/data/paperless/releases/<timestamp>-sml-source-fix-2a5cdca/postdeploy-checks.txt` (and a separate `<timestamp>-sml-hash-fix-9875c51` release folder on Pui for its standalone `sml-api` step).
- **Document `POIN66-5958` itself was intentionally left untouched** throughout, per explicit instruction — it remains in `sml_source_changed`, unresolved, pending a manual cancel/reimport by the customer or an operator.
- Not yet verified: full manual test pass (per-step drift detection catching a mismatch before finalize, Thai-time display across affected pages, fail-open behavior under a real SML outage) — customer/tester to verify directly.
- Customer notified (general update summary covering all four shops, not `POIN66-5958`-specific) and asked to test directly; awaiting feedback before this rollout is considered fully confirmed.

## Current Customer Status - 2026-08-10 (all four shops: signer dropdown filter, web-only)

Frontend-only fix rolled out to all four shops: each signer-slot dropdown in Workflow configuration now excludes usernames already selected in the step's other slots, preventing duplicate-signer selection at the UI level (previously only caught by post-submit validation, which is still in place as a safety net).

- `paperless-web:162e907` deployed to all four shops. `api`, `db`, `sml-api` untouched everywhere (`api` remains `a56a52c`).
- Pui: was already on `web:f15b534` (max-signers-10 UI, deployed same day earlier) → `162e907`. Release evidence `/data/paperless/releases/20260810133730-signer-dropdown-filter-162e907/postdeploy-checks.txt`.
- Wirat Home Mart, Insee Construction, Damrong Homeplus: were still on `web:8066c5d` (predates the max-signers-10 frontend work) → `162e907`. This means these three shops received both the max-signers-10 UI and the dropdown-filter fix in one deployment. Release evidence under each shop's `/data/paperless/releases/<timestamp>-signer-dropdown-filter-162e907/postdeploy-checks.txt`.
- Post-deploy smoke: HTTP 200 on each shop's public URL after `web` container recreation; all `api`/`db`/`sml-api` containers remained healthy and untouched (verified via `docker compose ... ps` before/after on each server).
- **Not yet verified**: full manual UI checklist (open existing ≤3-signer Workflow, add signers 3→10, reopen a 10-signer step and confirm all 10 slots render, remove a middle signer and confirm compacting, create/sign a real SML and Internal document with >3 signers, verify PDF stamp positions) has only been run informally; awaiting customer/tester feedback from Pui before treating the other three shops' rollout as fully confirmed.

## Current Customer Status - 2026-08-08 (all four shops synced to latest)

All four production installations run the same image set as of this date:

- `paperless-api:a56a52c`, `paperless-web:8066c5d`, `sml-api-bybos:b9088b4`
- Pui: `db` untouched, release evidence `/data/paperless/releases/20260808212101-sync-latest/postdeploy-checks.txt`
- Wirat Home Mart: `db` untouched, release evidence `/data/paperless/releases/20260808142410-sync-latest/postdeploy-checks.txt`
- Insee Construction: `db` untouched, release evidence `/data/paperless/releases/20260808142531-sync-latest/postdeploy-checks.txt`
- Damrong Homeplus: already on this version from its own rollout (see the schema-repair and trial-expiry sections below)

Note when picking tags for a Web-only or API-only frontend/backend commit: GitHub Actions only builds the image whose path filter matched (`backend/**` vs `frontend/**`). A commit that only touches `frontend/` will NOT have a `paperless-api` image at that SHA — use the most recent commit SHA that actually touched `backend/` for the API tag. This caused one failed `pull` (`manifest unknown`) on the Pui shop during this rollout before the API tag was corrected.

All three re-synced shops (Pui, Wirat, Insee) keep their existing per-shop `.env.prod` / inline `environment:` config unchanged — this rollout only bumped image tags for `sml-api`, `api`, and `web`; no flags, `ALLOWED_TENANTS`, tenant defaults, or secrets were modified. `db` was never recreated on any shop.

### Known pitfall: `uploads` directory ownership (found and fixed on Damrong Homeplus, 2026-08-10)

The `paperless-api` image runs as a non-root user (`uid=100 gid=101`, named `app` inside the container). `/data/paperless/uploads` must be `chown`-ed to `100:100` on the host **before** the API container ever starts, or every PDF upload/write fails with `upload_write_failed` / `permission denied` even though the API container itself reports healthy (the healthcheck doesn't touch the upload path).

This was missed during Damrong Homeplus's initial `install -d` setup (created as `root:root`), which went unnoticed until a customer tried to upload a signature-template sample PDF two days after go-live. Fixed with:

```bash
chown -R 100:101 /data/paperless/uploads
```

Pui, Wirat Home Mart, and Insee Construction were checked after this was found — all three already had correct `100:101` ownership from their original setup. Verify this on any new shop's `uploads` (and any other bind-mounted, container-writable directory) as part of initial deploy, not just container health.

### Feature flag status across all four shops (verified 2026-08-08)

| Shop | `INTERNAL_DOCUMENTS_ENABLED` | `SML_SIGNATURE_SYNC_ENABLED` | `TRIAL_EXPIRES_AT` |
| --- | --- | --- | --- |
| Pui | `true` | `true` | not set |
| Wirat Home Mart | `true` | `true` | not set |
| Insee Construction | `true` | `true` | not set |
| Damrong Homeplus | `true` | `true` | `2026-10-08` |

## Trial Expiry Feature

`TRIAL_EXPIRES_AT` (optional, `YYYY-MM-DD`) blocks login after the given date (23:59:59 local) with `403 trial_expired` on both the SML and local-fallback login paths. Unset by default — installations without this var are completely unaffected. The value is echoed back in the login/`/me` response as `trialExpiresAt`; the frontend shows a non-dismissible warning banner (`AppTrialBanner.vue`, mounted in `AppLayout.vue`) once 3 days or fewer remain. Already-issued JWTs (max 12h TTL) keep working past expiry until they naturally expire — this only blocks new logins.

To extend or remove a trial: edit `TRIAL_EXPIRES_AT` in that customer's `config/.env.prod`, then `docker compose ... up -d --no-deps api` (no new image needed). Introduced in commit `b5b8be9`.

## Current Customer Status - 2026-08-08 (Damrong Homeplus) — trial expiry rollout

- Second deployment same day. PaperLess Web/API updated from `77b1eb1` to `b5b8be9` (adds the trial expiry feature above); SML API unchanged at `62c8acc`.
- Added `TRIAL_EXPIRES_AT=2026-10-08` to `config/.env.prod` — this customer is a 2-month trial starting 2026-08-08, per explicit customer request. Warning banner will appear in-app starting 2026-10-05.
- Only `api` and `web` were recreated (`--no-deps`); `db` and `sml-api` were left untouched to preserve the SML Postgres connection fixed during initial deploy (see pitfall note below).
- Release evidence: `/data/paperless/releases/20260808192659-trial-expiry-b5b8be9/postdeploy-checks.txt`
- Post-deploy smoke: `/health/live`, `/health/ready` HTTP 200; login endpoint reachable and returning normal auth errors (not `trial_expired`, confirming the trial gate correctly did not trigger before the expiry date).

## Current Customer Status - 2026-08-08 (Damrong Homeplus) — initial deployment

- Initial deployment. PaperLess Web/API release `77b1eb1`, SML API release `62c8acc`.
- Release evidence: `/data/paperless/releases/20260808184235-initial-77b1eb1/postdeploy-checks.txt`
- Default/admin tenant: `homeplus1` (Damrong Homeplus main branch, `branch_code 00000`, `branch_status 0`). `SML_IMAGE_TEMPLATE_DATABASE=homeplus1_images` — chosen because its `sml_doc_images` schema is more complete (includes `image_url` column and full column defaults) than the sibling branch `homeplus5_images`.
- `ALLOWED_TENANTS` scoped to the 19 real SML ERP tenants on this shared Postgres 11 server (`coffee,dcon,ddnt,drh,graphic,homeplus1,homeplus5,jamjan,jirapong,jka,mithtae,naratip,niphon,niphon_tax,taxdcon,taxjka,taxmithtae,virin,zxp`), excluding `smlerpmaindata` (auth DB itself), `control_center` and `wms_app` (unrelated non-SML applications on the same Postgres instance), and all `*_old`/`template0`/`template1`/`postgres` housekeeping databases.
- `INTERNAL_DOCUMENTS_ENABLED=true` and `SML_SIGNATURE_SYNC_ENABLED=true` were enabled from initial deploy (not a phased rollout like Insee's original baseline-off approach), per explicit customer instruction. SML API endpoints for company-profile and saved-signature sync were confirmed available before enabling.

Tenant readiness at initial deploy (`verify-sml-tenant --all-allowed --template homeplus1_images`):

- PASS (13): `drh`, `homeplus1`, `jamjan`, `jirapong`, `jka`, `mithtae`, `niphon`, `niphon_tax`, `taxdcon`, `taxjka`, `taxmithtae`, `virin`
- FAIL (6): `coffee`, `dcon`, `graphic` (image/main `sml_doc_images` columns do not match template), `ddnt` (missing `ddnt_images` database entirely), `homeplus5` (sibling branch, `sml_doc_images` schema missing `image_url` column and defaults), `naratip`, `zxp` (missing `_images` database)
- Failing tenants remain blocked at login until self-service "ตั้งค่า image DB" or admin `provision-sml-image-db` repairs their schema. `homeplus1` (the tenant this customer will actually use) passed every check.

Known deployment pitfall for this install: the pre-existing `sml_postgresql` container's `POSTGRES_PASSWORD` env var (`sml`) only works over the Docker `trust`-authenticated `local`/`127.0.0.1` paths in `pg_hba.conf`; real cross-container TCP connections require `md5` auth with the actual current password, which had been rotated and was not `sml`. Always confirm the SML Postgres password with the customer/SML team before assuming the container's env var reflects the live password.

## Current Customer Status - 2026-07-20

- PaperLess Web release: `20260720112943` (`c3acecb`) on both customer installations
- PaperLess API release retained: `d13867f`; it was not restarted by this Web-only deployment
- SML API release retained: `a51c7e9`; it was not restarted by this deployment
- Shared SML tenant readiness registry is enabled independently in each PaperLess database
- Post-deploy evidence: `/data/paperless/releases/20260720112943/postdeploy-checks.txt` on each server

Latest smoke results:

- Both public URLs, `/health/live`, and `/health/ready` returned HTTP 200 after deployment.
- Both installations run the same Web image digest `sha256:1ea21b709330d22a0c80a87283e611c5a3f81f9ee602e05815ad2f7db95289f8` built by GitHub Actions for `linux/amd64`.
- PaperLess API and DB were healthy; existing DB and SML API container IDs/restart counts were unchanged on both servers.
- Browser smoke confirmed the database batch-check button on desktop and 390px mobile with no horizontal overflow or console errors.
- The additive `sml_tenant_readiness_registry` migration and feature flag were verified on both installations.
- On the Pui installation, STPT moved from `unverified` to `ready`; a second login returned `source=registry`, and selected-database login issued a valid session without another full schema check.
- All 27 databases visible to the test account received their first registry result with no fatal/panic log entries. Non-ready databases remain blocked with their stored SML issue until a user retries after SML repair.
- The previous local Web image `d13867f` remains available for rollback. No source archive or Docker build cache was created on either customer server.

Known tenant readiness note: `PTTP-TAX` was still missing its main tenant DB during the latest login smoke and must remain blocked until SML/admin database setup is complete.

## Port Policy

PaperLess customer deployment uses host port `8095` for the web container.

Do not expose the backend, PaperLess Postgres, or SML API containers directly to the host unless an explicit maintenance window requires it.

## Service Layout

| Service | Purpose | Host Exposure |
|---|---|---|
| `paperless-prod-web` | Nginx + built Vue app + `/api` proxy | `8095` |
| `paperless-prod-api` | Go PaperLess API | Docker network only |
| `paperless-prod-db` | PaperLess application Postgres | Docker network only |
| `paperless-prod-sml-api` | SML bridge for auth, lookup, lock, image upload | Docker network only |
| `sml_postgresql` | Customer SML Postgres, existing container | Existing SML network |

## Data Flow

```mermaid
flowchart LR
  U["Admin/User Browser"] -->|HTTP :8095| W["paperless-prod-web\nNginx + Vue"]
  W -->|/api proxy| A["paperless-prod-api\nGo backend"]
  A -->|PaperLess app data| PDB["paperless-prod-db\nPostgres 16"]
  A -->|PDF/uploads| FS["/data/paperless/uploads"]
  A -->|SML auth/lookup/lock/images| SAPI["paperless-prod-sml-api"]
  SAPI -->|sml_service_network| SMLPG["sml_postgresql\nCustomer SML Postgres"]
  SMLPG --> AUTH["smlerpmaindata\nusers + permissions"]
  SMLPG --> ERP["selected tenant DB\nERP docs + sml_doc_images"]
  SMLPG --> IMG["selected tenant _images DB\nimage_file bytea"]
```

## Environment

Production values belong in `/data/paperless/config/.env.prod` and must not be committed.

Required groups:

- PaperLess Postgres connection and storage paths
- `JWT_SECRET`
- SML API key shared between PaperLess backend and SML API service
- `SML_PAPERLESS_BASE_URL`
- `SML_AUTH_PROVIDER`
- `SML_AUTH_DATAGROUP`
- `SML_IMAGE_TEMPLATE_DATABASE` ต้องชี้ไปยัง `_images` database มาตรฐานของลูกค้ารายนั้น เช่น `vrh_images` ห้ามใช้ค่าจากลูกค้ารายอื่น
- `SML_PAPERLESS_TENANT` default tenant
- `PAPERLESS_LOCAL_AUTH_FALLBACK_ENABLED`
- `PUBLIC_BASE_URL`
- Upload and template limits
- `INTERNAL_DOCUMENTS_ENABLED=true` after the additive migration and backend/frontend smoke checks pass

Provider and data group are system configuration values. The login UI must not ask the user to enter them.

## SML Tenant Image DB Preflight

Every selectable SML tenant must have a matching image database. For example, tenant `stpt` requires:

- `stpt` for ERP document data
- `stpt_images` for image bytes

Both databases must contain `public.sml_doc_images` with the same schema. Tenants created directly in PostgreSQL can miss the `_images` database, which causes PaperLess auto-finalization to stop at `completed_image_failed`.

กำหนด `SML_IMAGE_TEMPLATE_DATABASE` ใน production env ให้ตรงกับลูกค้าก่อนเริ่ม SML API แล้ว run จาก container ก่อนให้ลูกค้าทดสอบ:

```bash
docker exec <sml-api-container> ./verify-sml-tenant --all-allowed --template <template_images_db>
```

If a tenant image DB is missing, create only that image DB with dry-run first, then apply after customer approval:

```bash
docker exec <sml-api-container> ./provision-sml-image-db --tenant <tenant> --template <template_images_db>
docker exec <sml-api-container> ./provision-sml-image-db --tenant <tenant> --template <template_images_db> --apply
docker exec <sml-api-container> ./verify-sml-tenant --tenant <tenant> --template <template_images_db>
```

หน้าเลือก database มีปุ่ม `ตรวจสอบอีกครั้ง` สำหรับอ่าน readiness ล่าสุดหลังผู้ดูแลแก้ config/schema แล้ว ปุ่มนี้ไม่แก้ schema และไม่สร้าง database; การ provision ยังต้องผ่านสถานะที่ระบบรองรับและ approval ตาม runbook นี้

ผลตรวจจะแสดงทุกปัญหาที่พบพร้อมฐานข้อมูลและผู้รับผิดชอบ: ฐานหลัก/`_images` หาย, เปิดฐานไม่ได้หรือสงสัยฐานเสีย, ตารางหาย, columns/sequence/constraints/indexes ไม่ตรง, timeout, หรือระบบ template/readiness ไม่พร้อม ข้อความสำหรับผู้ใช้ต้องไม่แสดง PostgreSQL error ภายใน และต้องไม่แสดง `พร้อมใช้งาน` จน full verification ผ่านจริง

For day-to-day use, PaperLess also supports self-service image DB setup from the login page. If SML reports that a selected database is missing `<tenant>_images` or the `public.sml_doc_images` table is absent, the user can click `ตั้งค่า image DB`; PaperLess verifies the same SML username/password/database permission again, then creates only the missing image database/table through `paperless-prod-sml-api`. Main DB missing or existing schema mismatch cases remain blocked and require admin review.

Do not insert or repair `sml_doc_images` rows by direct SQL during normal operation. Use the PaperLess “ส่งรูป SML อีกครั้ง” retry action so events and lock flow remain auditable.

## Login Verification

Customer login must be verified with real SML credentials from `smlerpmaindata`.

Expected login behavior:

1. User enters SML username/password.
2. PaperLess asks the SML API for allowed databases and quick tenant readiness.
3. User selects a database every login.
4. If only the image DB is missing, user can click `ตั้งค่า image DB`; after success the same database becomes selectable.
5. PaperLess runs a full tenant readiness check before issuing the JWT.
6. PaperLess creates a local user if it does not exist yet.
7. SML `superadmin` maps to PaperLess `superadmin`; other SML users map to PaperLess `admin`. PaperLess-local users remain `user`.

## SML Saved Signature Rollout

The saved-signature feature reads `erp_user.signature_1` from the tenant selected in the JWT session. Enable `SML_SIGNATURE_SYNC_ENABLED=true` only after deploying the SML API endpoints for signature metadata and binary retrieval. Deployment order is SML API, PaperLess backend migration, frontend, then feature flag.

After deployment, sign in as superadmin, select the target database, open `/admin/users`, and run `Sync จาก SML`. Verify the preview summary before confirming. Test with one internal signer first: select `ลายเซ็นที่บันทึกไว้`, review the lazily loaded image, sign a new document, and verify the current/final PDF. Existing completed documents must retain their original signature file/version.

If sync reports a missing or invalid signature, PaperLess preserves the previous saved signature and records a warning. Set `SML_SIGNATURE_SYNC_ENABLED=false` for immediate rollback to draw-only signing; no schema rollback is required.

## Internal Document Rollout

Internal documents require SML API `GET /api/v1/company-profile`, the PaperLess additive migration, and the matching frontend. Deploy in this order: SML API, PaperLess API, PaperLess Web. Keep `INTERNAL_DOCUMENTS_ENABLED=false` until all three services are healthy, then enable it and recreate only the PaperLess API/Web services.

After enabling the flag, verify that the selected tenant has exactly one usable row in `public.erp_company_profile`. Open `Master เอกสารภายใน` as superadmin and confirm the three seeded Masters are inactive. Configure the Workflow, then use `จัดวางกรอบลายเซ็น` to place the required boxes in the blank approval area of the generated A4 template before activating a Master. Do not guess customer signers or activate a production Master with a test Workflow.

Safe smoke checks before customer configuration:

1. The Master list contains `PAYREQ`, `ADV`, and `PREPAY` as inactive.
2. The internal document create route opens and contains no PDF upload step.
3. An inactive or incomplete Master cannot create a document and returns a readable configuration error.
4. Existing SML document create, image upload, and lock flows remain unchanged.

After customer Workflow configuration, create one approved test document with no more than 15 rows, open the generated one-page PDF, edit it once to verify immutable revision behavior and automatic approval-cell placement, send it, and complete signing. Printing the latest revision is optional. Confirm logs contain no SML image or SML lock request for that internal document.

For immediate application rollback set `INTERNAL_DOCUMENTS_ENABLED=false` and restore the previous immutable API/Web image tags. Do not drop the additive tables or columns; existing internal records remain audit data and become visible again when the flag is re-enabled.

### Shared SML database readiness

Set `SML_TENANT_READINESS_REGISTRY_ENABLED=true` so each SML database receives one full schema verification per PaperLess installation. The result is tenant-wide and shared by all users who currently have SML permission for that database. PaperLess still authenticates the user and reloads database permissions from SML on every login.

`ready` results do not expire and are not checked by a periodic worker. A database is checked again only after a failed result is manually retried, a structural SML operation invalidates the stored result, or the application verification version changes. The migration is additive; set the flag to `false` to return to the previous live-check behavior without dropping the registry table.

For an operational recheck of a database that is already ready, a signed-in superadmin can call `POST /api/admin/sml/tenant-readiness/recheck`. The endpoint always uses the tenant in the current JWT session, applies the same advisory lock/cooldown as login checks, and records an audit event; it cannot be used to inspect an arbitrary tenant.

Development default credentials are not assumed to work on the customer server.

## Deploy Checklist

1. Confirm port `8095` is free or assigned to PaperLess.
2. Create/update `/data/paperless/config/.env.prod` with production secrets.
3. Pull or copy the release source/images.
4. Run `docker compose --env-file /data/paperless/config/.env.prod up -d`.
5. Confirm all PaperLess containers are healthy/running.
6. Open `http://45.122.49.250:8095`.
7. Test login with a real SML account.
8. Select the customer tenant database.
9. Run SML tenant image DB preflight for every allowed tenant.
10. Smoke test dashboard, workflow config, document search, PDF preview, signer queue, SML image upload, and SML lock.
11. If saved signatures are enabled, sync one known SML signature and verify explicit saved/drawn selection on a new internal task.

## Container Release Pipeline

PaperLess Web and API images are built by GitHub Actions, not on a developer computer or customer server.

- Web image: `ghcr.io/bosocmputer/paperless-web:<commit-sha>`
- API image: `ghcr.io/bosocmputer/paperless-api:<commit-sha>`
- Target platform: `linux/amd64`
- Release tags are immutable short Git commit SHAs. The `main` tag is informational and must not be used in production Compose.
- GitHub Actions must pass the frontend build or backend test suite before publishing its image.

Production deployment remains a controlled manual step. For each customer:

1. Save the current Compose file and container evidence under `/data/paperless/releases/<timestamp>/`.
2. Replace only the target service image with the immutable GHCR SHA tag.
3. Run `docker compose pull <service>`.
4. Run `docker compose up -d --no-deps <service>`.
5. Verify public URL, `/health/live`, `/health/ready`, container IDs, restart counts, and logs.
6. Keep the previous image and Compose snapshot for rollback.

Example for a Web-only release:

```bash
docker compose --env-file /data/paperless/config/.env.prod pull web
docker compose --env-file /data/paperless/config/.env.prod up -d --no-deps web
```

Do not run `docker build`, `docker system prune`, or a full-stack restart on customer servers. If a release fails, restore the saved Compose file and recreate only the affected service.

## Smoke Commands

From the customer server:

```bash
docker ps --filter "name=paperless-prod"
curl -fsS http://127.0.0.1:8095/
curl -fsS http://127.0.0.1:8095/health/live
curl -fsS http://127.0.0.1:8095/health/ready
```

Do not print secrets in terminal logs that will be copied into tickets or chat.

## Rollback

Keep each deploy timestamped under `/data/paperless/releases/<timestamp>/`.

Rollback should restore:

- Previous compose file
- Previous image tags
- Previous env file backup if changed
- PaperLess DB backup if a schema/data rollback is required
- Upload volume snapshot only if file storage changed incompatibly

Prefer image/compose rollback first. Only roll back database state when the release has written incompatible data and the business owner approves data loss/replay implications.

## Post-Deploy Evidence

Save a short deployment evidence file under `/data/paperless/releases/<timestamp>/postdeploy-checks.txt` with:

- Commit SHA or image tags
- Container names and status
- URL smoke result
- Login/database selection result
- One PDF preview result
- One SML auth/lookup result
- Any known limitation or customer credential blocker
