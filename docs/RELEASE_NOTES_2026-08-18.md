# PaperLess Release Notes - 2026-08-18

## Sign-Note Box: Scroll Jump and Hard-to-Drag Fix

Customer relayed two related complaints from an end-user about the sign-note box feature in `ContinuousPdfViewer.vue` (the main signing-task PDF viewer): entering text-edit on a note felt like it "jumped to a different position," and dragging the box to reposition it was difficult and unreliable.

Both traced to the same code:

1. `focusEditingNoteBox()` called `scrollIntoView({block:'center'})` on every edit, re-centering the whole PDF page around whatever box was clicked. The box's coordinates never actually changed — the viewport did — but for a box near the top/bottom of a tall page it read exactly like the note had moved.
2. The only drag handle was a small (~22px) circle at the box's top-left corner, outside the box itself, and it disappeared entirely while the box was in text-edit mode — requiring the user to exit edit mode first just to find and grab it.

Fix (`paperless-web:72974a7`, was `b5bd587`):

- Removed the `scrollIntoView` call. `editor.focus({preventScroll:true})` already keeps focus without forcing a scroll.
- Made the box body itself draggable. `pointerdown` on it starts a deferred move that only commits once the pointer crosses a 4px threshold — a plain tap still opens the text editor (the same click-vs-drag distinction browsers use natively), while a press-and-drag repositions the box. The small corner handle and resize corner remain available as an explicit alternative.

## Customer Deployment

Deployed same-session to all four shops (Damrong Homeplus, Pui, Wirat Home Mart, Insee Construction) as a `web`-only redeploy (`--no-deps web`) — `api`/`db`/`sml-api` kept their exact prior container everywhere.

- Image: `paperless-web:72974a7` (was `b5bd587`).
- Verification per shop: `web` `Up`, public URL smoke `HTTP 200`.
- Release evidence under each shop's `/data/paperless/releases/20260818*-notebox-scroll-drag-fix-72974a7/postdeploy-checks.txt`; full narrative in `docs/CUSTOMER_DEPLOYMENT.md` (search for `2026-08-18`).

## Status As Of 2026-08-18 (end of session)

- **Awaiting real-usage feedback from the customer.** Customer to retest with the end-user who originally reported this: open a signing task with sign-note boxes, click into a note near the bottom of a tall page and confirm the view no longer re-centers, then confirm dragging the box body itself works reliably (including a brief hold-before-move not immediately opening text-edit).
- All prior open items (Workflow delete, signature-template slot reconciliation, the 8 proactive audit fixes, the PDF layout designer horizontal-scroll and Shift+wheel fixes, the required-attachment cap removal) remain awaiting confirmation — nothing new to report on those as of this entry.
