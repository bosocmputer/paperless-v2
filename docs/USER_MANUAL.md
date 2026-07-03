# PaperLess User Manual

This manual summarizes the production workflow. The in-app guides under admin and signer pages include screenshots from QA and should be used for hands-on training.

## Admin Flow

### 1. Log In

1. Open PaperLess.
2. Enter SML username and password.
3. Select the SML database for this session.
4. Enter PaperLess.

The selected database scopes documents, dashboard counts, workflows, templates, and signer queues.

### 2. Prepare Workflow

1. Open `ตั้งค่า Workflow`.
2. Configure document format and signing steps.
3. Open the signature template designer for that document format.
4. Place signature boxes and legal notice boxes.
5. Save the template.

For PDFs with more pages than the template, PaperLess clones the first-page pattern to every uploaded PDF page. Admin can still edit, delete, or add boxes per page before saving the document.

### 3. Create Signing Document

1. Open `เอกสารเตรียมส่ง`.
2. Create a new document.
3. Search/select the SML document.
4. Upload the real PDF.
5. Review signature and legal notice boxes on every page.
6. Save as draft.
7. Send the document when ready.

### 4. Track Active Documents

Open `เอกสารรอเซ็น`.

The list shows document status and who the document is waiting for. For documents with an external signer, admin can create or copy the external signing link from the list/detail surfaces.

Use `Flow เอกสาร` to inspect related SML flow without leaving the current page.

### 5. Confirm Completed Signing

After all required signers are complete:

1. Open the document detail.
2. Review the current signed PDF.
3. Confirm the document.

PaperLess will generate the final audit PDF, upload JPEG snapshots to SML, and lock the ERP document. If image upload fails, use retry. If lock fails after images succeed, retry lock from the detail page.

### 6. Review History And Evidence

Open `ประวัติเอกสารเซ็น`.

- `ดูเอกสารเซ็นครบ` opens the current signed document.
- `ดูหลักฐานการลงนาม` opens the final audit evidence PDF.
- `พิมพ์เอกสาร` creates a print event before opening the printable PDF.

Users should print from PaperLess so print history is captured.

## Internal Signer Flow

### 1. Log In

1. Enter SML username/password.
2. Select the database for this session.
3. Open the signer workspace.

### 2. Sign A Task

1. Open `งานรอเซ็น`.
2. Select a document that is ready for you.
3. Read the PDF using the continuous viewer.
4. Open `Flow เอกสาร` if context is needed.
5. Draw the signature.
6. Confirm the legal notice checkbox.
7. Press confirm signing.

If your user is assigned to consecutive workflow positions, the next task appears only after the previous step is complete.

### 3. Review Own History

Open `ประวัติการเซ็น`.

User history focuses on the user's own signing result and the current signed document. It does not show admin audit evidence by default.

## External Signer Flow

1. Open the signing link sent by admin.
2. Enter OTP.
3. Read the document.
4. Draw signature and confirm.
5. Close the page after the success message.

External signers only see the signing task. They do not see attachments, admin timeline, print/download controls, related-document actions, or internal admin functions.

## Error And Recovery

| Situation | Action |
|---|---|
| User cannot log in | Verify SML account and database permission first, then PaperLess user status |
| Wrong database selected | Log out and log in again, then select the correct database |
| PDF preview fails | Refresh/reopen the page; if it persists, report document number to admin |
| SML image upload failed | Admin uses retry SML images |
| SML lock failed | Admin retries lock after image upload is successful |
| External link already used | Generate a new external link/OTP from admin detail if business allows |

## Safety Notes

- Do not share OTP, external signing links, screenshots with customer data, PDF bytes, or signature images outside approved channels.
- Do not edit SML image rows manually for normal repair; use PaperLess retry actions.
- Read-only PDF preview reduces user error but is not DRM.
- Browser-based systems can record print-copy creation, but cannot guarantee physical printer output.
