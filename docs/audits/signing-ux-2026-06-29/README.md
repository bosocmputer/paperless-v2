# Signing Workflow UX/UI Audit - 2026-06-29

> Historical note: this audit captured the early signing UX before the July production hardening work. Current behavior and release status are documented in `docs/RELEASE_NOTES_2026-07-03.md`, `docs/QA_SUMMARY_2026-07-03.md`, and the in-app admin/user guides.

## Scope
- Admin document list: `/signing/documents`
- Admin document detail: `/signing/documents/:id`
- Admin create dialog
- User pending list: `/signing/tasks`
- Desktop viewport: `1366x768`
- Mobile viewport: `390x844`

## Evidence
- `01-admin-detail-current.png` - current browser state, document list
- `02-admin-detail.png` - admin detail first capture
- `02b-admin-detail-after-wait.png` - admin detail after iframe wait
- `03-create-dialog-empty.png` - create dialog capture from current browser state
- `03b-create-dialog-crop.png` - create dialog crop
- `04-user-inbox-admin-empty.png` - user pending list empty state
- `05-desktop-list-1366.png` - desktop list breakpoint
- `06-desktop-detail-1366.png` - desktop detail breakpoint
- `07-mobile-list-390.png` - mobile document list breakpoint
- `08-mobile-tasks-390.png` - mobile pending list breakpoint

## Audit Health Score
| Dimension | Score | Key Finding |
|---|---:|---|
| Accessibility | 2/4 | Many icon buttons have labels, but table/modal and empty states are weak; status copy is not localized. |
| Performance | 3/4 | Core pages are light, but iframe PDF preview is unreliable and no visible fallback exists. |
| Responsive Design | 1/4 | List, detail, and user inbox break at common desktop/mobile sizes. |
| Theming | 3/4 | Uses Sakai/PrimeVue tokens consistently enough; no major palette issue found. |
| Product UX | 2/4 | Workflow exists, but admin/user recovery paths and audit surfaces are not clear enough for production. |
| Total | 11/20 | Acceptable foundation, but not prod-polished yet. |

## Findings

### P1 - Responsive Layout Breaks On Desktop And Mobile
Evidence: `05-desktop-list-1366.png`, `06-desktop-detail-1366.png`, `07-mobile-list-390.png`, `08-mobile-tasks-390.png`

At `1366x768`, the document table and detail screen are clipped horizontally after the Sakai sidebar takes space. At mobile width, the logo, headings, inputs, table columns, and buttons are cut off. This blocks real mobile signing work and makes admin review difficult on normal laptops.

Recommendation:
- Convert signing list and pending list from wide DataTable-first layouts into responsive card rows below tablet width.
- For admin detail, use a two-pane layout only when content width is enough; otherwise stack PDF, summary, steps, and timeline.
- Add explicit `min-width: 0` to grid/flex children that contain tables or iframes.

### P1 - PDF Preview Can Appear Blank In Admin Detail
Evidence: `02-admin-detail.png`, `02b-admin-detail-after-wait.png`, `06-desktop-detail-1366.png`

The DOM confirms an iframe with a blob PDF URL is present, but screenshot evidence shows a blank PDF panel even after waiting. The API PDF itself works and final PDFs render correctly outside the iframe, so the admin screen needs a more reliable viewer/fallback.

Recommendation:
- Use the same `pdfjs-dist` rendering approach as the signature template designer instead of raw iframe.
- Add buttons: `เปิด PDF`, `ดาวน์โหลด Final PDF`, and `Reload PDF`.
- Show a clear error state if PDF render fails.

### P1 - Important Status And Audit Language Is Still Developer-Centric
Evidence: `02b-admin-detail-after-wait.png`, source `SigningDocumentDetail.vue`

The UI shows raw statuses and event actions such as `completed`, `signed`, `sml_lock_success`, `final_pdf_stamped`. Admins need Thai, action-oriented labels.

Recommendation:
- Map all statuses/events to Thai labels, e.g. `completed` -> `เซ็นครบแล้ว`, `sml_lock_success` -> `ล็อก SML สำเร็จ`.
- Keep raw action code only in metadata/debug details.

### P2 - Create Dialog Needs Stronger Guidance And Validation States
Evidence: DOM snapshot for `03-create-dialog-empty.png`, source `SigningDocuments.vue`

The dialog has the correct fields, but it starts with an arbitrary first doc format and a plain empty candidate box. It does not summarize prerequisites before `ส่งเซ็น`, and the disabled/invalid state is toast-driven after clicking.

Recommendation:
- Disable `ส่งเซ็น` until doc format, selected candidate, and PDF file are present.
- Add inline checklist: `เลือกเอกสารจาก SML`, `อัปโหลด PDF`, `ตรวจ template/config`.
- For locked SML documents, make the confirmation warning visually stronger and explain why duplicates are allowed.

### P2 - Empty States Do Not Teach Next Action
Evidence: `04-user-inbox-admin-empty.png`, `08-mobile-tasks-390.png`

The user pending list says only `ไม่มีเอกสารรอเซ็น`. It does not explain whether the user has no task, the workflow has not reached their step, or they should contact admin.

Recommendation:
- Replace with a small empty state: `ยังไม่มีเอกสารที่ถึงลำดับของคุณ`, plus `เอกสารจะปรากฏเมื่อขั้นก่อนหน้าเซ็นครบ`.
- For admin view, provide a link back to `เอกสารเพื่อเซ็น` or timeline.

### P2 - Navigation Contains Placeholder Routes
Evidence: `AppMenu.vue`

`Audit Trail` points to `/pages/empty`; `System Status` points to `/`. This weakens admin trust because these labels imply production surfaces.

Recommendation:
- Hide unfinished items or mark them as `เร็ว ๆ นี้`.
- Build a real audit log page before leaving `Audit Trail` visible.

## Positive Findings
- The main menu has the right conceptual modules: pending signing, signing documents, document config, signature templates.
- The create flow prevents free-text document creation by requiring SML candidate selection and backend validation.
- The backend workflow now has good evidence surfaces: signed/skipped/pending, movement log, final PDF, and SML lock result.
- Admin detail already groups steps and signers by workflow position, which is the right mental model.

## Limits
- This audit used the in-app browser screenshots and DOM snapshots. The PDF iframe may render differently in a normal Chrome/Safari browser, but the blank-panel risk is still important because the UI currently provides no fallback.
- A full user signing form screenshot was not captured because the tested documents had already completed. The pending list and code were reviewed instead.

## Recommended Fix Order
1. Fix responsive layout for `/signing/documents`, `/signing/documents/:id`, and `/signing/tasks`.
2. Replace iframe PDF preview with pdf.js viewer plus download/open fallback.
3. Localize status/event labels and improve timeline readability.
4. Harden create dialog validation and inline helper states.
5. Improve empty states and hide placeholder nav items.
