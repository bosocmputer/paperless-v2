# PaperLess Release Notes - 2026-08-15

## PDF Layout Designer Horizontal Scroll Fix

Customer report, with a screenshot: in the "จัดวางกรอบบน PDF ฉบับจริง" layout designer (used to place signature boxes on the real uploaded PDF), a landscape page zoomed past 100% could not be scrolled horizontally — content past the pane's right edge was cut off entirely. Reported as affecting all four shops (shared frontend code, not shop-specific data). The customer separately confirmed `ContinuousPdfViewer.vue` (the main signing-task PDF viewer used everywhere else in the app) scrolls correctly, which narrowed the bug down to `DocumentLayoutDesigner.vue` specifically.

- Root cause: `.pdf-pane` was a plain block element in the default (non-`fullHeight`) dialog mode — only the `fullHeight`/maximized mode had `display: flex; flex-direction: column`. Combined with `.pdf-pane`'s `overflow: hidden`, this clipped any content in the inner scrollable `.pdf-viewport` wider than the pane, instead of letting the viewport's own `overflow: auto` handle it.
- Fix (`paperless-web:286560b`, was `6f59db8`): made `.pdf-pane` a flex column unconditionally, so `.pdf-viewport` gets a real flex-constrained box to scroll within in both dialog modes (normal and maximized). Merged the previously mode-duplicated flex CSS rules into the base selectors.
- Per explicit customer instruction, this was not reproduced in a live dev-server/browser session before deploying — verified by code reading and a clean `npm run build` only.

## Customer Deployment

Deployed same-session to all four shops (Damrong Homeplus, Pui, Wirat Home Mart, Insee Construction) as a `web`-only redeploy (`--no-deps web`) — `api`/`db`/`sml-api` kept their exact prior container everywhere.

- Image: `paperless-web:286560b` (was `6f59db8`).
- Verification per shop: `web` `Up`, public URL smoke `HTTP 200`.
- Release evidence under each shop's `/data/paperless/releases/20260815*-pdf-pane-scroll-fix-286560b/postdeploy-checks.txt`; full narrative in `docs/CUSTOMER_DEPLOYMENT.md` (search for `2026-08-15`).

## Status As Of 2026-08-15 (end of session)

- **Awaiting real-usage feedback from the customer.** This fix was deployed without a live browser reproduction per explicit customer instruction, so it has not yet been exercised end-to-end. Customer to retest: open the layout designer (non-maximized dialog) on a landscape document, zoom past the point the page overflows the pane, and confirm horizontal scroll now works on all four shops.
- All prior open items (Workflow delete, signature-template slot reconciliation on 2CO/1CO Position 2, the 8 proactive audit fixes) remain awaiting confirmation as of the previous two days' entries — nothing new to report on those as of this entry.
