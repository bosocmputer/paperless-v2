# PaperLess Release Notes - 2026-08-17

## Required Attachment Cap Removed

Customer report, with a screenshot: at `/config/documents/2JEPO/workflow`, clicking "ใช้กับทุกผู้เซ็น" (apply required-attachment labels to all signers) on a step with 8 signers and 2 attachment labels failed to save with `เอกสารแนบบังคับได้ไม่เกิน 12 รายการ`. The customer's own annotation on the screenshot did the math: 8 signers × 2 labels = 16, exceeding the cap.

- Root cause: the "apply to all signers" button multiplies label count by signer count by design (one copy of each label per signer slot) — this routinely exceeds a step with more than a handful of signers or more than one label. The 12-item cap was an arbitrary constant duplicated in both frontend and backend validation, not tied to any storage constraint (`attachment_requirements` is JSONB with no length limit).
- Fix (`paperless-api:b9c24dc`, was `69816ac`; `paperless-web:b9c24dc`, was `286560b`): removed the cap entirely on both sides, per explicit customer instruction, rather than raising it to a new number.

## Proactive Sweep: No Other Scroll-Clipping Bugs Found

Following up on the 2026-08-15 PDF layout designer scroll fix, the customer asked whether the same `overflow: hidden` parent / `overflow: auto` child mismatch could be hiding elsewhere in the app. A full sweep of every `.vue` file under `frontend/src` containing `overflow` (26 files) found no other occurrence of the pattern — every other scrollable container either sits inside a parent that's flex/grid by default, or has an explicit `max-height`/`height` that doesn't depend on flex sizing. No code change was needed from this investigation.

## Customer Deployment

Deployed same-session to all four shops (Damrong Homeplus, Pui, Wirat Home Mart, Insee Construction) as an `api`+`web` redeploy (`--no-deps api web`) — `db`/`sml-api` kept their exact prior container everywhere.

- Images: `paperless-api:b9c24dc` (was `69816ac`), `paperless-web:b9c24dc` (was `286560b`).
- Verification per shop: `api` `healthy`, `web` `Up`, public URL smoke `HTTP 200`.
- Release evidence under each shop's `/data/paperless/releases/20260817*-remove-attachment-cap-b9c24dc/postdeploy-checks.txt`; full narrative in `docs/CUSTOMER_DEPLOYMENT.md` (search for `2026-08-17`).

## Status As Of 2026-08-17 (end of session)

- **Awaiting real-usage feedback from the customer.** Customer to retest: reopen `2JEPO` Position 1 (8 signers) on Damrong, click "ใช้กับทุกผู้เซ็น" with the same 2 labels, and confirm save now succeeds with all 16 requirement rows.
- All prior open items from 2026-08-14/15 (Workflow delete, signature-template slot reconciliation, the 8 proactive audit fixes, the PDF layout designer scroll fix) remain awaiting confirmation — nothing new to report on those as of this entry.
