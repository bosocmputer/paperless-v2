# PaperLess Release Notes - 2026-08-18

## Landscape Source PDF Clipped to Portrait on Save

Customer reported: on `/signing/documents/new`, uploading a landscape-orientation source PDF resulted in the saved/stamped document being cut into portrait.

Root cause: scan/mobile-capture PDFs commonly declare landscape via the page's `/Rotate 90|270` entry instead of a swapped `/MediaBox` — every normal viewer honors `/Rotate`. The PDF import library (`gofpdi`) already rotates the drawn page content correctly for this case, but the separate call used to size the *destination* page (`Importer.GetPageSizes()`) reads only the raw, un-rotated `/MediaBox`. A landscape source page was therefore built onto a portrait-shaped destination page, clipping/squeezing the correctly-rotated content into the wrong shape. Affects both new signing-document creation and print-copy generation (both share the same `importPDFPages` function).

Fix (`paperless-api:9531674`, was `b9c24dc`):

- `importPDFPages` now independently reads each page's inherited `/Rotate` (via `github.com/ledongthuc/pdf`, already a dependency) and swaps width/height before choosing the destination page's orientation, matching what the importer actually draws.
- Verified with two new unit tests against a hand-built PDF carrying a genuine `/Rotate 90` entry (`gofpdf` has no API to produce one for a fixture, so the PDF's object/xref structure was constructed by hand).
- Swept the codebase for the same bug class: confirmed the rotation-blind page-sizing call is only ever reached through `importPDFPages` — no other independent PDF page-sizing logic exists elsewhere in the codebase.

## Landscape Source PDF Still Clipped to Portrait (Second, Separate Bug)

Customer retested after the fix above and reported the same document still came out portrait. Investigation against the customer's actual uploaded file (pulled from the Pui production server, signing document `IA26050134`) found this is a **different bug**: the source file was already genuinely landscape (`/MediaBox` 841.92×595.32, `/Rotate 0` — produced by Windows "Print to PDF" from a landscape layout), not a `/Rotate`-declared page, so the fix above didn't apply to it.

Root cause: `importPDFPages` correctly picked destination orientation `"L"` for this page, but passed `gofpdf.AddPageFormat` the already-landscape-shaped size (`Wd>Ht`). `gofpdf.AddPageFormat` always expects its size argument in portrait-native order (`Wd<=Ht`) and swaps internally when told `"L"` — passing an already-swapped size caused a second swap, landing back on portrait. This bug pre-dates today's session entirely (present since `importPDFPages` was first written) and affects any source PDF whose `/MediaBox` is directly landscape, regardless of `/Rotate`.

Fix (`paperless-api:8c4133e`, was `9531674`):

- `importPDFPages` now always passes `AddPageFormat` a portrait-native-order size, swapping back before the call whenever the destination orientation is `"L"`.
- Reproduced end-to-end against the customer's real file before and after the fix (portrait 595.32×841.92 before, landscape 841.92×595.32 — matching the original — after).
- New regression test builds a genuinely landscape-native source PDF and asserts the *final output* PDF's page dimensions (the previous test suite only checked `importPDFPages`' intermediate callback value, which is why it didn't catch this). Confirmed the new test fails without the fix and passes with it.

**Existing documents created before this fix retain their broken portrait PDF** — this only prevents the bug going forward; there is no automatic repair of already-stored files. For document `IA26050134` specifically (still in `draft` status, never sent/signed), the simplest safe fix is for the customer to delete and re-upload it now that the fix is live, rather than a one-off data repair script touching production.

## Sign-Note Box: Scroll Jump and Hard-to-Drag Fix

Customer relayed two related complaints from an end-user about the sign-note box feature in `ContinuousPdfViewer.vue` (the main signing-task PDF viewer): entering text-edit on a note felt like it "jumped to a different position," and dragging the box to reposition it was difficult and unreliable.

Both traced to the same code:

1. `focusEditingNoteBox()` called `scrollIntoView({block:'center'})` on every edit, re-centering the whole PDF page around whatever box was clicked. The box's coordinates never actually changed — the viewport did — but for a box near the top/bottom of a tall page it read exactly like the note had moved.
2. The only drag handle was a small (~22px) circle at the box's top-left corner, outside the box itself, and it disappeared entirely while the box was in text-edit mode — requiring the user to exit edit mode first just to find and grab it.

Fix (`paperless-web:72974a7`, was `b5bd587`):

- Removed the `scrollIntoView` call. `editor.focus({preventScroll:true})` already keeps focus without forcing a scroll.
- Made the box body itself draggable. `pointerdown` on it starts a deferred move that only commits once the pointer crosses a 4px threshold — a plain tap still opens the text editor (the same click-vs-drag distinction browsers use natively), while a press-and-drag repositions the box. The small corner handle and resize corner remain available as an explicit alternative.

## Customer Deployment

Both fixes above deployed same-session to all four shops (Damrong Homeplus, Pui, Wirat Home Mart, Insee Construction).

**Landscape PDF rotation fix** — `api`-only redeploy (`--no-deps api`) — `web`/`db`/`sml-api` kept their exact prior container everywhere.

- Image: `paperless-api:9531674` (was `b9c24dc`).
- Verification per shop: `api` `Up`/healthy, public URL smoke `HTTP 200`, `/health/live` `HTTP 200`.
- Release evidence: Damrong `/data/paperless/releases/20260818115529-landscape-pdf-rotation-fix-9531674/postdeploy-checks.txt`, Pui `/data/paperless/releases/20260818045629-landscape-pdf-rotation-fix-9531674/postdeploy-checks.txt`, Wirat `/data/paperless/releases/20260818045728-landscape-pdf-rotation-fix-9531674/postdeploy-checks.txt`, Insee `/data/paperless/releases/20260818045821-landscape-pdf-rotation-fix-9531674/postdeploy-checks.txt`; full narrative in `docs/CUSTOMER_DEPLOYMENT.md` (search for `landscape source PDF`).

**Note-box scroll/drag fix** — `web`-only redeploy (`--no-deps web`) — `api`/`db`/`sml-api` kept their exact prior container everywhere.

- Image: `paperless-web:72974a7` (was `b5bd587`).
- Verification per shop: `web` `Up`, public URL smoke `HTTP 200`.
- Release evidence under each shop's `/data/paperless/releases/20260818*-notebox-scroll-drag-fix-72974a7/postdeploy-checks.txt`; full narrative in `docs/CUSTOMER_DEPLOYMENT.md` (search for `2026-08-18`).

## Status As Of 2026-08-18 (end of session)

- **Landscape PDF rotation fix**: deployed and smoke-tested (container healthy, HTTP 200) on all four shops. Awaiting customer confirmation with a real landscape-orientation scan/mobile-capture upload on `/signing/documents/new`, and ideally a print-copy check too since that flow shares the fix.
- **Awaiting real-usage feedback from the customer** on the note-box fix. Customer to retest with the end-user who originally reported this: open a signing task with sign-note boxes, click into a note near the bottom of a tall page and confirm the view no longer re-centers, then confirm dragging the box body itself works reliably (including a brief hold-before-move not immediately opening text-edit).
- All prior open items (Workflow delete, signature-template slot reconciliation, the 8 proactive audit fixes, the PDF layout designer horizontal-scroll and Shift+wheel fixes, the required-attachment cap removal) remain awaiting confirmation — nothing new to report on those as of this entry.
