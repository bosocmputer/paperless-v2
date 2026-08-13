# PaperLess Release Notes - 2026-08-13

## SML Signature Sync Permission Case-Sensitivity

- Fixed `isSyncCandidateUser` in `sml-api-bybos` comparing `data_group`/`data_code` with exact-match equality instead of case-insensitively.
- Root cause: `sml_database_list` (database registry) and `sml_database_list_user_and_group` (permission table) store the same tenant's `data_code` with inconsistent casing (e.g. `drh` vs `DRH`), which the old exact-match SQL never reconciled — every otherwise-valid, `active_status=1` user in an affected tenant was denied with `signature_user_not_allowed`.
- Both CTEs (`direct_allowed`, `group_allowed`) now use `lower(trim())`, matching the pattern already used elsewhere in `auth.go`.
- Generic fix, not tenant-specific; any tenant with mismatched casing between the two tables was at risk.

## Internal Document Master Delete Blocked By Setup-Only Config

- Fixed `DeleteInternalDocumentMaster` in `paperless-api` refusing to delete a master whenever a `document_config_steps` (Workflow config) or `signature_templates` (signer-box placement) row existed for its code — even with zero real `internal_documents` ever created from it.
- Deletion now runs in a transaction: still blocks with `internal_master_in_use` if a real `internal_documents` row references the master; otherwise it deletes the orphaned `document_config_steps`/`signature_templates` rows for that (tenant, code) first, then deletes the master.

## Internal Document Master Auto-Reseed Removed

- Fixed `PAYREQ`/`ADV`/`PREPAY` being silently re-created (new id, `ON CONFLICT DO NOTHING`) on every `/api/internal-document-masters` and `/api/document-types` list load — a delete of one of these three codes was undone by the very next page refresh, making them effectively undeletable.
- Removed the auto-seed call (`EnsureDefaultInternalDocumentMasters`) from both endpoints and the now-dead store function.
- **Deliberate behavior change**: nothing is auto-created on page load anymore. A tenant that has never opened `/config/internal-document-masters` will no longer see the three standard masters pre-created — an operator must create them explicitly via the existing "สร้างใหม่" flow during setup, same as any tenant that had one deleted.

## Customer Deployment

All three fixes deployed same-session to all four shops (Pui, Wirat Home Mart, Insee Construction, Damrong Homeplus), each as a single-service redeploy (`--no-deps`) — untouched services kept their exact prior image/container:

| Fix | Image | Service redeployed |
| --- | --- | --- |
| Signature permission case-fix | `sml-api-bybos:99de187` (was `fd9bafd`) | `sml-api` only |
| Master cascade-delete | `paperless-api:c82ce6e` (was `847fd23`) | `api` only |
| Master auto-reseed removal | `paperless-api:17cd7ce` (was `c82ce6e`) | `api` only |

Verification per shop: container `healthy`, public URL smoke `HTTP 200`, `/health/live` and `/health/ready` both OK. Release evidence under each shop's `/data/paperless/releases/<timestamp>-.../postdeploy-checks.txt`; full narrative and root-cause detail in `docs/CUSTOMER_DEPLOYMENT.md` (search for `2026-08-13`).

Damrong Homeplus was the origin shop for all three reports/fixes; the other three shops received the same fixes preventively (no reported symptom there, but all three bugs are generic — not tenant-specific).

## Status As Of 2026-08-13 (end of session)

- Signature permission fix: **confirmed working** on Damrong — the exact previously-failing request (tenant `drh`, 4 sample usercodes) was re-tested live post-deploy and returned `200 OK` with the real signature image.
- Master cascade-delete + auto-reseed removal: **confirmed working** on Damrong — customer deleted a `virin`-tenant `PREPAY` master (`204`) and confirmed via follow-up `GET` that it did not reappear.
- **Awaiting real-usage feedback from the customer** on all three fixes before treating this batch as fully confirmed across all four shops. Nothing else is blocked or in progress; no further action queued on this side unless new feedback arrives.
- Known outstanding cleanup (not a bug, no code change needed): Damrong's `drh` tenant still has 42 rows in `user_saved_signatures` with a stale `last_error='signature_user_not_allowed'` from before the signature fix. These are display-only status records, not currently blocking anything — they clear on the next superadmin-triggered "Sync จาก SML" from `/admin/users`. Customer has not been asked to do this yet.
