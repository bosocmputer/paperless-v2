# PaperLess Release Notes - 2026-07-16

## SML Saved Signatures

- Superadmin can sync tenant-scoped saved signatures from `erp_user.signature_1` together with SML users.
- Candidate lists expose only availability, dimensions, byte count, and a source fingerprint; image bytes use a protected internal endpoint.
- Saved images are normalized to immutable transparent PNG files in PaperLess.
- Missing or invalid SML data preserves the previous usable PaperLess signature.
- Internal signers explicitly choose a saved signature or draw a new signature for each task. External signers remain draw-only.
- Signing snapshots the selected file and version so later syncs cannot rewrite completed documents.
- The feature can return to draw-only behavior immediately with `SML_SIGNATURE_SYNC_ENABLED=false`.

## Customer Deployment

- PaperLess API/Web release: `20260716153839` from commit `fac91c7`.
- SML API hotfix release: `20260716155654` from commit `4232d27`.
- The SML hotfix includes the login-capable built-in `superadmin` in sync even when an installation stores it with `active_status=0`; ordinary inactive users remain excluded.
- Production login and database selection passed for `STPT`.
- Sync dry-run found the real `superadmin` signature as new, and the binary endpoint returned its JPEG successfully.
- Deployment QA did not execute the actual sync. The customer initiates it from `/admin/users` after reviewing the preview.
- Build cache and obsolete PaperLess images were cleaned while retaining each active image and its immediate rollback image.

Evidence is stored at `/data/paperless/releases/20260716155654/postdeploy-checks.txt` on the customer server.

## Customer Test

1. Login and select `STPT`.
2. Open `/admin/users`, click `Sync จาก SML`, and review the preview before confirming.
3. Confirm that `superadmin` shows a saved SML signature after sync.
4. Open a new internal signing task and explicitly choose `ลายเซ็นที่บันทึกไว้`.
5. Verify the preview, sign, and check the current/final PDF.
6. Confirm an existing completed document still uses its original signature snapshot.
