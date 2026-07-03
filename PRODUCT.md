# Product

## Name

PaperLess

## Users

PaperLess serves system admins, workflow admins, document admins, internal signers, external signers, and auditors.

Admin and audit work primarily happens on desktop. Signing work is mobile-first because signers often approve documents from phones or tablets.

## Product Purpose

PaperLess receives SML document metadata and PDFs, routes them through controlled e-signature workflows, records evidence, produces signed PDF outputs, sends document page snapshots back to SML, and locks the ERP document after admin confirmation.

Success means:

- Users sign the right document at the right workflow step.
- Admins can see who the document is waiting for without opening every detail page.
- External customers only see a sign-only flow.
- Evidence, print events, SML image upload, and SML lock are traceable.
- Tenant data follows the selected SML database and does not leak across databases.

## Product Principles

- Keep SML as the source of document identity and database access.
- Require database selection on every login.
- Preserve PaperLess local role/status after first SML provisioning.
- Separate signing tasks from PDF stamp placements so one signer can stamp multiple pages without duplicate tasks.
- Show users the simplest surface needed for their role.
- Make admin recovery actions explicit: retry SML images, retry lock, open evidence, print official copy.
- Avoid raw technical errors in the UI; show Thai, action-oriented status copy.
- Prefer read-only PDF preview and official print flow over browser PDF controls.

## Core Flows

### Admin

1. Log in with SML account and choose database.
2. Configure workflow and signature template by document format.
3. Create a signing document from an SML document and PDF.
4. Review cloned multi-page signature/legal-notice placements and edit per page if needed.
5. Send the document into the signing workflow.
6. Track the active queue, including who the document is waiting for and external signer links.
7. Confirm after signing is complete.
8. Verify current PDF, audit evidence, official print copy, SML image upload, and SML lock state.

### Internal Signer

1. Log in with SML account and choose database.
2. Open only tasks that are ready for the signer.
3. Read the PDF in a continuous mobile viewer.
4. Review document flow when needed.
5. Draw a signature, accept the legal notice, and confirm signing.
6. Review own signing history with the signed document only.

### External Signer

1. Open the external signing link.
2. Enter OTP.
3. See only the sign-only workspace.
4. Sign and close the page after success.

External signers do not receive admin actions, attachments, related documents, print/download controls, or timeline noise.

## Brand Personality

Calm, accountable, work-focused. PaperLess should feel like an operations tool people can trust during daily document approval work.

## Design Principles

- Make identity, tenant, document number, and permission visible before important actions.
- Keep admin surfaces dense enough for real work but clear on status and next action.
- Treat mobile signing as a primary workflow, not a responsive afterthought.
- Preserve auditability in interface language: who, what, when, status, and retry result.
- Use PrimeVue/Sakai patterns consistently.
- Avoid landing-page or decorative layouts inside operational screens.

## Accessibility And Inclusion

Target WCAG AA contrast, keyboard-accessible controls, clear focus states, reduced-motion friendly transitions, and touch targets suitable for phones and tablets.
