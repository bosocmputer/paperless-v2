# PaperLess User Manual

This manual summarizes the production workflow. The in-app guides under admin and signer pages include screenshots from QA and should be used for hands-on training.

## Admin Flow

SML `superadmin` becomes PaperLess `superadmin` and can manage users, workflow, and signature templates. Other SML users become PaperLess `admin` and can create/send documents, but cannot edit workflow or signature box placement templates.

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

For PDFs with more pages than the template, PaperLess clones the first-page pattern to every uploaded PDF page. Superadmin can edit, delete, or add boxes per page before saving the document. Admin uses the published template boxes as read-only placement.

### 3. Create Signing Document

1. Open `เอกสารเตรียมส่ง`.
2. Create a new document.
3. Search/select the SML document.
4. Upload the real PDF.
5. Review signature and legal notice boxes on every page.
6. Save as draft.
7. Send the document when ready.

To prepare many documents of the same type:

1. Open `เอกสารเตรียมส่ง` and click `นำเข้าหลายไฟล์`.
2. Select a document format that already has Workflow and an Active Template.
3. Select up to 30 PDFs. Name each file exactly as its SML document number, such as `QT26070001.pdf`.
4. Click `อัปโหลดและตรวจสอบ` and review SML metadata, duplicate warnings, PDF page count, and status for every row.
5. Remove invalid/duplicate rows. For an SML-locked document, confirm that row explicitly before import.
6. Click `ยืนยันนำเข้า`. Successful files become drafts; failed files can be retried without recreating successful items.

Batch import uses the Active Template automatically and does not open the placement designer. The combined PDFs in one batch may contain at most 100 pages. Closing and discarding a batch removes unconsumed staged files.

### 4. Create An Internal Document

Superadmin prepares an internal document once:

1. Open `Master เอกสารภายใน` and configure the name, code, prefix, and running pattern.
2. Configure its Workflow to determine the signing positions and sequence.
3. Activate the Master after its Workflow is ready. An Active Template is not required for internal documents.

Admin or superadmin then creates the real document:

1. Open `สร้างเอกสารภายใน` and select an active Master.
2. Enter the requester, position, department, purpose, required date, and at least one expense row.
3. Save once. PaperLess reserves the document number, creates the PDF, and creates the draft automatically; no PDF upload is required.
4. Use `แก้ไขแบบฟอร์ม` while the document is still a draft. Each save creates a new immutable revision.
5. Review the fixed one-page A4 PDF. It supports up to 15 expense rows and uses the signature boxes that a Superadmin configured once in the Workflow's approval area.
6. Open `พิมพ์ PDF` for the latest revision when a printable copy is needed. Printing is optional and recorded for audit.
7. Send the draft to the normal signing Workflow. No per-document placement is required.

After sending, the form and layout are locked. To stop an internal document, the creator or superadmin uses `ยกเลิก` and enters a reason. The history remains available, outstanding external links are revoked, and a cancelled document can be copied into a new draft with a new document number.

Internal documents use the company profile from the selected SML database only at creation time. They finish entirely in PaperLess and never upload images or lock transactions in SML.

### 5. Track Active Documents

Open `เอกสารรอเซ็น`.

The list shows document status and who the document is waiting for. For documents with an external signer, admin can create or copy the external signing link from the list/detail surfaces.

Use `Flow เอกสาร` to inspect related SML flow without leaving the current page.

### 6. Completed Signing

After all required signers are complete, PaperLess automatically generates the signed document and final audit PDF.

- SML documents then upload JPEG snapshots and lock the ERP transaction. If upload or lock fails, admin uses the corresponding retry action.
- Internal documents finish in PaperLess. They do not show SML Flow/reference checks or SML retry actions.

### 7. Correct A Wrong SML Document

PaperLess never deletes signing evidence. Use the action that matches the document state:

1. A draft that has not been sent: choose `ลบแบบร่าง`. The draft becomes `ยกเลิก` in history and the same SML document number may be imported again with a new PDF.
2. A document that is currently collecting signatures: the creator, admin, or superadmin chooses `ยกเลิกเอกสาร` and enters a reason. Pending signers are skipped and any external signing link/OTP is revoked.
3. A signer whose turn has arrived may choose `ปฏิเสธ` and must enter a reason. PaperLess stops the remaining flow and retains the rejection in history.
4. From a cancelled or rejected SML document, choose `สร้างฉบับแก้ไข`. Upload the corrected PDF again. PaperLess creates a new attempt with the same SML number; it does not copy the old PDF, signatures, attachments, or signer state.

The document detail shows `ฉบับที่ N` and links to the previous/next attempt. A completed SML document remains permanent and cannot be recreated with the same number.

If SML data is changed or deleted after signing begins, PaperLess stops before uploading images or locking SML and shows an attention status. Cancel that attempt, then create a corrected attempt from the latest SML/PDF data. Do not use retry for this case.

### 8. Review History And Evidence

Open `ประวัติเอกสาร`.

- The list includes completed, rejected, and cancelled documents with a clear status tag.
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
| SML image upload failed | Admin uses retry SML images on the same document attempt |
| SML lock failed | Admin retries lock after image upload is successful on the same document attempt |
| SML source was changed or removed | Cancel the stopped attempt, then create a corrected attempt and upload the newest PDF |
| Internal document cannot be sent | Arrange signature/legal boxes, then send again; printing the latest PDF revision is optional |
| Internal Master cannot be activated | Complete its Running pattern and Workflow |
| Company profile unavailable | Ask SML ERP support to verify one usable row in `public.erp_company_profile` |
| External link already used | Generate a new external link/OTP from admin detail if business allows |

## Safety Notes

- Do not share OTP, external signing links, screenshots with customer data, PDF bytes, or signature images outside approved channels.
- Do not edit SML image rows manually for normal repair; use PaperLess retry actions.
- Read-only PDF preview reduces user error but is not DRM.
- Browser-based systems can record print-copy creation, but cannot guarantee physical printer output.
