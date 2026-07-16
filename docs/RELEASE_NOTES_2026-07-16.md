# PaperLess Release Notes - 2026-07-16

## User Account Source

- `/admin/users` now shows whether each account is linked from `SML` or was created inside `PaperLess`.
- Account source is stored explicitly in `users.account_source`; the UI does not infer it from role or saved-signature availability.
- New users created by SML login/auto-provision/batch Sync are stored as `sml`; users created from the PaperLess user form are stored as `paperless`.
- Existing accounts are backfilled once from per-user SML login/provision audits, saved-signature links, and verified historical batch-Sync transactions.
- A successful SML login promotes an existing account source to `sml`; local fallback does not change account source.

## SML Saved Signatures

- Superadmin can sync tenant-scoped saved signatures from `erp_user.signature_1` together with SML users.
- Candidate lists expose only availability, dimensions, byte count, and a source fingerprint; image bytes use a protected internal endpoint.
- Saved images are normalized to immutable transparent PNG files in PaperLess.
- Superadmin can open the current tenant-scoped saved signature from `/admin/users` through a protected, audited, no-store preview. The image is loaded only after clicking the view icon and is never exposed through a public URL.
- Saved-signature status text now distinguishes a normal missing SML image, an invalid image, and an actual sync failure instead of presenting every condition as a failed sync.
- Missing or invalid SML data preserves the previous usable PaperLess signature.
- Internal signers explicitly choose a saved signature or draw a new signature for each task. External signers remain draw-only.
- Signing snapshots the selected file and version so later syncs cannot rewrite completed documents.
- The feature can return to draw-only behavior immediately with `SML_SIGNATURE_SYNC_ENABLED=false`.

## Customer Deployment

- PaperLess API/Web release: `20260716170545` from commit `902df1b`.
- SML API hotfix release: `20260716155654` from commit `4232d27`.
- The SML hotfix includes the login-capable built-in `superadmin` in sync even when an installation stores it with `active_status=0`; ordinary inactive users remain excluded.
- Production login and database selection passed for `STPT`.
- The customer sync created the current `superadmin` saved signature for `STPT`.
- Production QA opened that signature from `/admin/users`; the protected PNG loaded at `1600x912`, the dialog closed cleanly, and the browser console had no errors.
- Production QA confirmed users without an SML signature now show `ไม่มีลายเซ็นใน SML`, while `superadmin` remains `พร้อมใช้`; the former blanket `Sync ไม่สำเร็จ` label is no longer shown in the users table.
- Production migration classified all 18 existing accounts as `SML` from verified audit/sync evidence. Browser QA confirmed the new `แหล่งบัญชี` column and source search, while a rolled-back transaction confirmed a newly created local account defaults to `PaperLess`.
- An unauthenticated saved-signature request returned `401` as expected.
- Build cache and obsolete PaperLess images were cleaned while retaining each active image and its immediate rollback image.

Evidence is stored at `/data/paperless/releases/20260716170545/postdeploy-checks.txt` on the customer server.

## Customer Test

1. Login and select `STPT`.
2. Open `/admin/users`, click `Sync จาก SML`, and review the preview before confirming.
3. Confirm that `superadmin` shows a saved SML signature after sync.
4. Open a new internal signing task and explicitly choose `ลายเซ็นที่บันทึกไว้`.
5. Verify the preview, sign, and check the current/final PDF.
6. Confirm an existing completed document still uses its original signature snapshot.
