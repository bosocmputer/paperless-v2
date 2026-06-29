package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type CreateSigningDocumentInput struct {
	ScreenCode string
	Format     models.SMLDocFormat
	Candidate  models.SMLDocumentCandidate
	Template   models.SignatureTemplate
	Configs    []models.DocumentConfigStep
	File       models.UploadedFile
	ActorID    string
	IPAddress  string
	UserAgent  string
}

type SignTaskResult struct {
	DocumentID string
	Completed  bool
}

type CreatePrintEventInput struct {
	DocumentID      string
	FileID          string
	Channel         string
	PrinterName     string
	DeviceIDHash    string
	ClientTimezone  string
	FinalFileSHA256 string
	PrintedBy       string
	PrintedByLabel  string
	IPAddress       string
	UserAgent       string
}

func (s *Store) CreateSigningDocument(ctx context.Context, input CreateSigningDocumentInput) (models.SigningDocument, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.SigningDocument{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	configSnapshot, err := json.Marshal(input.Configs)
	if err != nil {
		return models.SigningDocument{}, err
	}
	templateSnapshot, err := json.Marshal(input.Template)
	if err != nil {
		return models.SigningDocument{}, err
	}

	var documentID string
	err = tx.QueryRow(ctx, `
INSERT INTO signing_documents (
    screen_code, doc_format_code, doc_no, sml_table, trans_flag, party_code, party_name, party_type,
    doc_date, total_amount, sml_is_lock_record, status, current_version,
    original_file_id, current_file_id, signature_template_id, config_snapshot, template_snapshot, created_by
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::date,$10,$11,'in_progress',1,$12,$12,$13,$14::jsonb,$15::jsonb,NULLIF($16,'')::uuid)
RETURNING id::text
`, input.ScreenCode, input.Format.Code, input.Candidate.DocNo, input.Candidate.Table, input.Candidate.TransFlag,
		input.Candidate.PartyCode, input.Candidate.PartyName, input.Candidate.PartyType, input.Candidate.DocDate,
		input.Candidate.TotalAmount, input.Candidate.IsLockRecord, input.File.ID, input.Template.ID,
		string(configSnapshot), string(templateSnapshot), input.ActorID).Scan(&documentID)
	if err != nil {
		if strings.Contains(err.Error(), "signing_documents_active_doc_unique_idx") {
			return models.SigningDocument{}, ErrSigningDocumentDuplicate
		}
		return models.SigningDocument{}, err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO signing_document_versions (document_id, version_no, file_id, kind, created_by)
VALUES ($1, 1, $2, 'original', NULLIF($3,'')::uuid),
       ($1, 1, $2, 'current', NULLIF($3,'')::uuid)
`, documentID, input.File.ID, input.ActorID); err != nil {
		return models.SigningDocument{}, err
	}

	configs := append([]models.DocumentConfigStep(nil), input.Configs...)
	sort.Slice(configs, func(i, j int) bool {
		if configs[i].SequenceNo == configs[j].SequenceNo {
			return configs[i].PositionCode < configs[j].PositionCode
		}
		return configs[i].SequenceNo < configs[j].SequenceNo
	})
	firstSequence := 0.0
	if len(configs) > 0 {
		firstSequence = configs[0].SequenceNo
	}

	boxesByPosition := map[string][]models.SignatureTemplateBox{}
	for _, box := range input.Template.Boxes {
		key := strings.ToLower(strings.TrimSpace(box.PositionCode))
		boxesByPosition[key] = append(boxesByPosition[key], box)
	}
	for key := range boxesByPosition {
		sort.Slice(boxesByPosition[key], func(i, j int) bool {
			return boxesByPosition[key][i].SignerSlot < boxesByPosition[key][j].SignerSlot
		})
	}

	for _, step := range configs {
		stepStatus := "waiting"
		if step.SequenceNo == firstSequence {
			stepStatus = "pending"
		}
		var stepID string
		if err := tx.QueryRow(ctx, `
INSERT INTO signing_document_steps (
    document_id, position_code, position_name, sequence_no, condition_type, user01, user02, user03, status
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING id::text
`, documentID, step.PositionCode, step.PositionName, step.SequenceNo, step.ConditionType, step.User01, step.User02, step.User03, stepStatus).Scan(&stepID); err != nil {
			return models.SigningDocument{}, err
		}

		boxes := boxesByPosition[strings.ToLower(step.PositionCode)]
		signers, err := signerRowsForStep(step, boxes, stepStatus)
		if err != nil {
			return models.SigningDocument{}, err
		}
		for _, signer := range signers {
			if _, err := tx.Exec(ctx, `
INSERT INTO signing_document_signers (
    document_id, step_id, position_code, position_name, sequence_no, condition_type,
    signer_slot, signer_type, signer_user, signer_name, status,
    page_no, x_ratio, y_ratio, width_ratio, height_ratio, label
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
`, documentID, stepID, step.PositionCode, step.PositionName, step.SequenceNo, step.ConditionType,
				signer.SignerSlot, signer.SignerType, signer.SignerUser, signer.SignerName, signer.Status,
				signer.PageNo, signer.XRatio, signer.YRatio, signer.WidthRatio, signer.HeightRatio, signer.Label); err != nil {
				return models.SigningDocument{}, err
			}
		}
	}

	if err := insertSigningEvent(ctx, tx, documentID, input.ActorID, "", "document_created", "สร้างเอกสารเพื่อเซ็น", input.IPAddress, input.UserAgent, map[string]any{
		"docNo": input.Candidate.DocNo,
	}); err != nil {
		return models.SigningDocument{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return models.SigningDocument{}, err
	}
	return s.FindSigningDocumentByID(ctx, documentID)
}

func (s *Store) ListSigningDocuments(ctx context.Context) ([]models.SigningDocument, error) {
	rows, err := s.pool.Query(ctx, signingDocumentSelect()+`
ORDER BY d.updated_at DESC, d.created_at DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	documents := []models.SigningDocument{}
	for rows.Next() {
		doc, err := scanSigningDocument(rows)
		if err != nil {
			return nil, err
		}
		documents = append(documents, doc)
	}
	return documents, rows.Err()
}

func (s *Store) FindSigningDocumentByID(ctx context.Context, id string) (models.SigningDocument, error) {
	doc, err := scanSigningDocument(s.pool.QueryRow(ctx, signingDocumentSelect()+`WHERE d.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.SigningDocument{}, ErrSigningDocumentNotFound
	}
	if err != nil {
		return models.SigningDocument{}, err
	}
	steps, err := s.ListSigningDocumentSteps(ctx, id)
	if err != nil {
		return models.SigningDocument{}, err
	}
	signers, err := s.ListSigningDocumentSigners(ctx, id)
	if err != nil {
		return models.SigningDocument{}, err
	}
	events, err := s.ListSigningDocumentEvents(ctx, id)
	if err != nil {
		return models.SigningDocument{}, err
	}
	attachments, err := s.ListSigningDocumentAttachments(ctx, id)
	if err != nil {
		return models.SigningDocument{}, err
	}
	printEvents, err := s.ListSigningDocumentPrintEvents(ctx, id)
	if err != nil {
		return models.SigningDocument{}, err
	}
	doc.Steps = steps
	doc.Signers = signers
	doc.Events = events
	doc.Attachments = attachments
	doc.PrintEvents = printEvents
	return doc, nil
}

func (s *Store) ListPendingSigningTasksForUser(ctx context.Context, username string) ([]models.SigningDocument, error) {
	rows, err := s.pool.Query(ctx, `
SELECT DISTINCT d.id::text
FROM signing_documents d
JOIN signing_document_signers sg ON sg.document_id = d.id
WHERE d.status = 'in_progress'
  AND sg.status = 'pending'
  AND lower(sg.signer_user) = lower($1)
ORDER BY d.id::text
`, strings.TrimSpace(username))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []models.SigningDocument{}
	for _, id := range ids {
		doc, err := s.FindSigningDocumentByID(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}

func (s *Store) FindSigningTaskByID(ctx context.Context, taskID string) (models.SigningDocumentSigner, error) {
	signer, err := scanSigningDocumentSigner(s.pool.QueryRow(ctx, signingSignerSelect()+`WHERE sg.id = $1`, taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.SigningDocumentSigner{}, ErrSigningTaskNotFound
	}
	return signer, err
}

func (s *Store) SignInternalTask(ctx context.Context, taskID, username, signatureFileID, deviceID, ipAddress, userAgent, legalTextVersion string) (SignTaskResult, error) {
	return s.signTask(ctx, taskID, username, signatureFileID, deviceID, ipAddress, userAgent, legalTextVersion, false)
}

func (s *Store) SignExternalTask(ctx context.Context, taskID, signatureFileID, deviceID, ipAddress, userAgent, legalTextVersion string) (SignTaskResult, error) {
	return s.signTask(ctx, taskID, "", signatureFileID, deviceID, ipAddress, userAgent, legalTextVersion, true)
}

func (s *Store) RejectInternalTask(ctx context.Context, taskID, username, reason, deviceID, ipAddress, userAgent string) (string, error) {
	return s.rejectTask(ctx, taskID, username, reason, deviceID, ipAddress, userAgent, false)
}

func (s *Store) RejectExternalTask(ctx context.Context, taskID, reason, deviceID, ipAddress, userAgent string) (string, error) {
	return s.rejectTask(ctx, taskID, "", reason, deviceID, ipAddress, userAgent, true)
}

func (s *Store) MarkDocumentLockResult(ctx context.Context, documentID string, ok bool, metadata map[string]any) error {
	status := "completed"
	lockedAt := "now()"
	if !ok {
		status = "completed_lock_failed"
		lockedAt = "NULL"
	}
	_, err := s.pool.Exec(ctx, `
UPDATE signing_documents
SET status = $2, locked_at = `+lockedAt+`, updated_at = now()
WHERE id = $1
`, documentID, status)
	if err != nil {
		return err
	}
	action := "sml_lock_success"
	message := "Lock SML สำเร็จ"
	if !ok {
		action = "sml_lock_failed"
		message = "Lock SML ไม่สำเร็จ"
	}
	return s.AddSigningEvent(ctx, documentID, "", "", action, message, "", "", metadata)
}

func (s *Store) MarkDocumentEvidenceFailed(ctx context.Context, documentID string, metadata map[string]any) error {
	_, err := s.pool.Exec(ctx, `
UPDATE signing_documents
SET status = 'completed_evidence_failed', updated_at = now()
WHERE id = $1
`, documentID)
	if err != nil {
		return err
	}
	return s.AddSigningEvent(ctx, documentID, "", "", "final_pdf_failed", "สร้าง Final PDF/evidence ไม่สำเร็จ", "", "", metadata)
}

func (s *Store) UpdateSigningDocumentPDF(ctx context.Context, documentID string, file models.UploadedFile, final bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentVersion int
	if err := tx.QueryRow(ctx, `SELECT current_version FROM signing_documents WHERE id = $1 FOR UPDATE`, documentID).Scan(&currentVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSigningDocumentNotFound
		}
		return err
	}
	nextVersion := currentVersion + 1
	if _, err := tx.Exec(ctx, `
UPDATE signing_documents
SET current_file_id = $2,
    final_file_id = CASE WHEN $3 THEN $2 ELSE final_file_id END,
    current_version = $4,
    updated_at = now()
WHERE id = $1
`, documentID, file.ID, final, nextVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO signing_document_versions (document_id, version_no, file_id, kind)
VALUES ($1, $2, $3, 'current')
`, documentID, nextVersion, file.ID); err != nil {
		return err
	}
	if final {
		if _, err := tx.Exec(ctx, `
INSERT INTO signing_document_versions (document_id, version_no, file_id, kind)
VALUES ($1, $2, $3, 'final')
`, documentID, nextVersion, file.ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) AddSigningEvent(ctx context.Context, documentID, actorUserID, actorLabel, action, message, ipAddress, userAgent string, metadata map[string]any) error {
	return insertSigningEvent(ctx, s.pool, documentID, actorUserID, actorLabel, action, message, ipAddress, userAgent, metadata)
}

func (s *Store) AddSigningAttachment(ctx context.Context, documentID, signerID, fileID, note, createdBy string) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO signing_document_attachments (document_id, signer_id, file_id, note, created_by)
VALUES ($1, NULLIF($2,'')::uuid, $3, $4, NULLIF($5,'')::uuid)
`, documentID, signerID, fileID, strings.TrimSpace(note), createdBy)
	return err
}

func (s *Store) RegenerateExternalToken(ctx context.Context, signerID, tokenHash, otpHash, createdBy string, expiresAt time.Time) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var documentID string
	if err := tx.QueryRow(ctx, `
SELECT document_id::text
FROM signing_document_signers
WHERE id = $1 AND signer_type = 'external'
`, signerID).Scan(&documentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrSigningTaskNotFound
		}
		return "", err
	}

	if _, err := tx.Exec(ctx, `
UPDATE external_signing_tokens
SET status = 'revoked', revoked_at = now()
WHERE signer_id = $1 AND status IN ('active', 'verified')
`, signerID); err != nil {
		return "", err
	}

	var tokenID string
	if err := tx.QueryRow(ctx, `
INSERT INTO external_signing_tokens (document_id, signer_id, token_hash, otp_hash, expires_at, status, created_by)
VALUES ($1, $2, $3, $4, $5, 'active', NULLIF($6,'')::uuid)
RETURNING id::text
`, documentID, signerID, tokenHash, otpHash, expiresAt, createdBy).Scan(&tokenID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
UPDATE signing_document_signers
SET external_token_id = $1
WHERE id = $2
`, tokenID, signerID); err != nil {
		return "", err
	}
	if err := insertSigningEvent(ctx, tx, documentID, createdBy, "", "external_token_generated", "สร้าง public link/OTP", "", "", map[string]any{
		"signerId": signerID,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return tokenID, nil
}

func (s *Store) VerifyExternalOTP(ctx context.Context, tokenHash, otpHash, sessionHash string, sessionExpiresAt time.Time) (models.SigningDocumentSigner, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.SigningDocumentSigner{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var tokenID, signerID, status string
	var attempts, maxAttempts int
	var expiresAt time.Time
	var currentOTPHash string
	err = tx.QueryRow(ctx, `
SELECT id::text, signer_id::text, otp_hash, status, attempts, max_attempts, expires_at
FROM external_signing_tokens
WHERE token_hash = $1
FOR UPDATE
`, tokenHash).Scan(&tokenID, &signerID, &currentOTPHash, &status, &attempts, &maxAttempts, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.SigningDocumentSigner{}, ErrExternalTokenNotFound
	}
	if err != nil {
		return models.SigningDocumentSigner{}, err
	}
	if status != "active" && status != "verified" {
		return models.SigningDocumentSigner{}, ErrExternalTokenInvalid
	}
	if time.Now().After(expiresAt) {
		_, _ = tx.Exec(ctx, `UPDATE external_signing_tokens SET status = 'expired' WHERE id = $1`, tokenID)
		_ = tx.Commit(ctx)
		return models.SigningDocumentSigner{}, ErrExternalTokenInvalid
	}
	if currentOTPHash != otpHash {
		nextAttempts := attempts + 1
		nextStatus := status
		if nextAttempts >= maxAttempts {
			nextStatus = "locked"
		}
		_, _ = tx.Exec(ctx, `UPDATE external_signing_tokens SET attempts = $2, status = $3 WHERE id = $1`, tokenID, nextAttempts, nextStatus)
		_ = tx.Commit(ctx)
		return models.SigningDocumentSigner{}, ErrExternalTokenInvalid
	}
	if _, err := tx.Exec(ctx, `
UPDATE external_signing_tokens
SET status = 'verified', session_hash = $2, session_expires_at = $3, verified_at = now()
WHERE id = $1
`, tokenID, sessionHash, sessionExpiresAt); err != nil {
		return models.SigningDocumentSigner{}, err
	}
	signer, err := scanSigningDocumentSigner(tx.QueryRow(ctx, signingSignerSelect()+`WHERE sg.id = $1`, signerID))
	if err != nil {
		return models.SigningDocumentSigner{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.SigningDocumentSigner{}, err
	}
	return signer, nil
}

func (s *Store) FindExternalSignerBySession(ctx context.Context, tokenHash, sessionHash string) (models.SigningDocumentSigner, error) {
	signer, err := scanSigningDocumentSigner(s.pool.QueryRow(ctx, signingSignerSelect()+`
JOIN external_signing_tokens et ON et.signer_id = sg.id
WHERE et.token_hash = $1
  AND et.session_hash = $2
  AND et.status = 'verified'
  AND et.session_expires_at > now()
`, tokenHash, sessionHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.SigningDocumentSigner{}, ErrExternalTokenInvalid
	}
	return signer, err
}

func (s *Store) ListSigningDocumentSteps(ctx context.Context, documentID string) ([]models.SigningDocumentStep, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id::text, document_id::text, position_code, position_name, sequence_no, condition_type, user01, user02, user03, status, completed_at
FROM signing_document_steps
WHERE document_id = $1
ORDER BY sequence_no, position_code
`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.SigningDocumentStep{}
	for rows.Next() {
		item, err := scanSigningDocumentStep(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListSigningDocumentSigners(ctx context.Context, documentID string) ([]models.SigningDocumentSigner, error) {
	rows, err := s.pool.Query(ctx, signingSignerSelect()+`
WHERE sg.document_id = $1
ORDER BY sg.sequence_no, sg.position_code, sg.signer_slot, sg.signer_user
`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.SigningDocumentSigner{}
	for rows.Next() {
		item, err := scanSigningDocumentSigner(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListSigningDocumentEvents(ctx context.Context, documentID string) ([]models.SigningDocumentEvent, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id::text, document_id::text, COALESCE(actor_user_id::text,''), actor_label, action, message, ip_address, user_agent, metadata, created_at
FROM signing_document_events
WHERE document_id = $1
ORDER BY created_at DESC
`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.SigningDocumentEvent{}
	for rows.Next() {
		item, err := scanSigningDocumentEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListSigningDocumentAttachments(ctx context.Context, documentID string) ([]models.SigningDocumentAttachment, error) {
	rows, err := s.pool.Query(ctx, `
SELECT a.id::text, a.document_id::text, COALESCE(a.signer_id::text,''), a.file_id::text, a.note, COALESCE(a.created_by::text,''), a.created_at,
       f.id::text, f.original_name, f.stored_name, f.storage_path, f.content_type, f.size_bytes, f.page_count, f.sha256, COALESCE(f.created_by::text,''), f.created_at
FROM signing_document_attachments a
JOIN uploaded_files f ON f.id = a.file_id
WHERE a.document_id = $1
ORDER BY a.created_at DESC
`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.SigningDocumentAttachment{}
	for rows.Next() {
		var item models.SigningDocumentAttachment
		if err := rows.Scan(&item.ID, &item.DocumentID, &item.SignerID, &item.FileID, &item.Note, &item.CreatedBy, &item.CreatedAt,
			&item.File.ID, &item.File.OriginalName, &item.File.StoredName, &item.File.StoragePath, &item.File.ContentType,
			&item.File.SizeBytes, &item.File.PageCount, &item.File.SHA256, &item.File.CreatedBy, &item.File.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListSigningDocumentPrintEvents(ctx context.Context, documentID string) ([]models.SigningDocumentPrintEvent, error) {
	rows, err := s.pool.Query(ctx, `
SELECT p.id::text, p.document_id::text, p.file_id::text, p.channel, p.printer_name, p.device_id_hash,
       p.client_timezone, p.final_file_sha256, COALESCE(p.printed_by::text,''), p.ip_address, p.user_agent, p.printed_at,
       f.id::text, f.original_name, f.stored_name, f.storage_path, f.content_type, f.size_bytes, f.page_count, f.sha256, COALESCE(f.created_by::text,''), f.created_at
FROM signing_document_print_events p
JOIN uploaded_files f ON f.id = p.file_id
WHERE p.document_id = $1
ORDER BY p.printed_at DESC
`, documentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.SigningDocumentPrintEvent{}
	for rows.Next() {
		item, err := scanSigningDocumentPrintEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) FindSigningDocumentPrintEvent(ctx context.Context, documentID, printEventID string) (models.SigningDocumentPrintEvent, error) {
	item, err := scanSigningDocumentPrintEvent(s.pool.QueryRow(ctx, `
SELECT p.id::text, p.document_id::text, p.file_id::text, p.channel, p.printer_name, p.device_id_hash,
       p.client_timezone, p.final_file_sha256, COALESCE(p.printed_by::text,''), p.ip_address, p.user_agent, p.printed_at,
       f.id::text, f.original_name, f.stored_name, f.storage_path, f.content_type, f.size_bytes, f.page_count, f.sha256, COALESCE(f.created_by::text,''), f.created_at
FROM signing_document_print_events p
JOIN uploaded_files f ON f.id = p.file_id
WHERE p.document_id = $1 AND p.id = $2
`, documentID, printEventID))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.SigningDocumentPrintEvent{}, ErrSigningDocumentNotFound
	}
	return item, err
}

func (s *Store) CreateSigningDocumentPrintEvent(ctx context.Context, input CreatePrintEventInput) (models.SigningDocumentPrintEvent, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.SigningDocumentPrintEvent{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	if err := tx.QueryRow(ctx, `
INSERT INTO signing_document_print_events (
    document_id, file_id, channel, printer_name, device_id_hash, client_timezone,
    final_file_sha256, printed_by, ip_address, user_agent
)
VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,$9,$10)
RETURNING id::text
`, input.DocumentID, input.FileID, input.Channel, input.PrinterName, input.DeviceIDHash, input.ClientTimezone,
		input.FinalFileSHA256, input.PrintedBy, input.IPAddress, input.UserAgent).Scan(&id); err != nil {
		return models.SigningDocumentPrintEvent{}, err
	}
	if err := insertSigningEvent(ctx, tx, input.DocumentID, input.PrintedBy, input.PrintedByLabel, "document_printed", "พิมพ์เอกสาร official copy", input.IPAddress, input.UserAgent, map[string]any{
		"printEventId":    id,
		"channel":         input.Channel,
		"printerName":     input.PrinterName,
		"finalFileSha256": input.FinalFileSHA256,
		"clientTimezone":  input.ClientTimezone,
	}); err != nil {
		return models.SigningDocumentPrintEvent{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.SigningDocumentPrintEvent{}, err
	}
	return s.FindSigningDocumentPrintEvent(ctx, input.DocumentID, id)
}

func (s *Store) signTask(ctx context.Context, taskID, username, signatureFileID, deviceID, ipAddress, userAgent, legalTextVersion string, external bool) (SignTaskResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SignTaskResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	signer, err := scanSigningDocumentSigner(tx.QueryRow(ctx, signingSignerSelect()+`WHERE sg.id = $1 FOR UPDATE OF sg`, taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SignTaskResult{}, ErrSigningTaskNotFound
	}
	if err != nil {
		return SignTaskResult{}, err
	}
	if signer.Status != "pending" {
		return SignTaskResult{}, ErrSigningTaskUnavailable
	}
	if !external && !strings.EqualFold(signer.SignerUser, username) {
		return SignTaskResult{}, ErrSigningTaskUnavailable
	}
	if external && signer.SignerType != "external" {
		return SignTaskResult{}, ErrSigningTaskUnavailable
	}

	if _, err := tx.Exec(ctx, `
UPDATE signing_document_signers
SET status = 'signed',
    signature_file_id = $2,
    signed_at = now(),
    device_id = $3,
    ip_address = $4,
    user_agent = $5
WHERE id = $1
`, taskID, signatureFileID, deviceID, ipAddress, userAgent); err != nil {
		return SignTaskResult{}, err
	}
	if err := insertSigningEvent(ctx, tx, signer.DocumentID, "", signer.SignerName, "signed", signer.PositionName+" เซ็นเอกสารแล้ว", ipAddress, userAgent, map[string]any{
		"signerId":         taskID,
		"position":         signer.PositionCode,
		"legalTextVersion": strings.TrimSpace(legalTextVersion),
		"legalAccepted":    true,
	}); err != nil {
		return SignTaskResult{}, err
	}
	completed, err := advanceDocumentAfterSign(ctx, tx, signer)
	if err != nil {
		return SignTaskResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SignTaskResult{}, err
	}
	return SignTaskResult{DocumentID: signer.DocumentID, Completed: completed}, nil
}

func (s *Store) rejectTask(ctx context.Context, taskID, username, reason, deviceID, ipAddress, userAgent string, external bool) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	signer, err := scanSigningDocumentSigner(tx.QueryRow(ctx, signingSignerSelect()+`WHERE sg.id = $1 FOR UPDATE OF sg`, taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSigningTaskNotFound
	}
	if err != nil {
		return "", err
	}
	if signer.Status != "pending" {
		return "", ErrSigningTaskUnavailable
	}
	if !external && !strings.EqualFold(signer.SignerUser, username) {
		return "", ErrSigningTaskUnavailable
	}
	if external && signer.SignerType != "external" {
		return "", ErrSigningTaskUnavailable
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "ไม่ระบุเหตุผล"
	}
	if _, err := tx.Exec(ctx, `
UPDATE signing_document_signers
SET status = 'rejected', rejected_at = now(), reject_reason = $2, device_id = $3, ip_address = $4, user_agent = $5
WHERE id = $1
`, taskID, reason, deviceID, ipAddress, userAgent); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE signing_document_steps SET status = 'rejected' WHERE id = $1`, signer.StepID); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE signing_documents SET status = 'rejected', updated_at = now() WHERE id = $1`, signer.DocumentID); err != nil {
		return "", err
	}
	if err := insertSigningEvent(ctx, tx, signer.DocumentID, "", signer.SignerName, "rejected", signer.PositionName+" ปฏิเสธเอกสาร", ipAddress, userAgent, map[string]any{
		"signerId": taskID,
		"reason":   reason,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return signer.DocumentID, nil
}

func advanceDocumentAfterSign(ctx context.Context, tx pgx.Tx, signer models.SigningDocumentSigner) (bool, error) {
	if signer.ConditionType == 1 {
		if _, err := tx.Exec(ctx, `
UPDATE signing_document_signers
SET status = 'skipped'
WHERE step_id = $1 AND id <> $2 AND status IN ('waiting', 'pending')
`, signer.StepID, signer.ID); err != nil {
			return false, err
		}
		return completeStepAndMaybeDocument(ctx, tx, signer)
	}

	var remaining int
	if err := tx.QueryRow(ctx, `
SELECT count(*)
FROM signing_document_signers
WHERE step_id = $1 AND status NOT IN ('signed', 'skipped')
`, signer.StepID).Scan(&remaining); err != nil {
		return false, err
	}
	if remaining > 0 {
		return false, nil
	}
	return completeStepAndMaybeDocument(ctx, tx, signer)
}

func completeStepAndMaybeDocument(ctx context.Context, tx pgx.Tx, signer models.SigningDocumentSigner) (bool, error) {
	if _, err := tx.Exec(ctx, `
UPDATE signing_document_steps
SET status = 'completed', completed_at = now()
WHERE id = $1
`, signer.StepID); err != nil {
		return false, err
	}

	var nextStepID string
	err := tx.QueryRow(ctx, `
SELECT id::text
FROM signing_document_steps
WHERE document_id = $1 AND status = 'waiting'
ORDER BY sequence_no, position_code
LIMIT 1
`, signer.DocumentID).Scan(&nextStepID)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := tx.Exec(ctx, `
UPDATE signing_documents
SET status = 'completed', completed_at = now(), updated_at = now()
WHERE id = $1
`, signer.DocumentID); err != nil {
			return false, err
		}
		if err := insertSigningEvent(ctx, tx, signer.DocumentID, "", "", "document_completed", "เอกสารเซ็นครบแล้ว", "", "", nil); err != nil {
			return false, err
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE signing_document_steps SET status = 'pending' WHERE id = $1`, nextStepID); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE signing_document_signers SET status = 'pending' WHERE step_id = $1 AND status = 'waiting'`, nextStepID); err != nil {
		return false, err
	}
	if err := insertSigningEvent(ctx, tx, signer.DocumentID, "", "", "step_available", "เปิดขั้นตอนถัดไปให้เซ็น", "", "", map[string]any{"stepId": nextStepID}); err != nil {
		return false, err
	}
	return false, nil
}

func signerRowsForStep(step models.DocumentConfigStep, boxes []models.SignatureTemplateBox, status string) ([]models.SigningDocumentSigner, error) {
	if len(boxes) == 0 {
		return nil, fmt.Errorf("missing signature box for position %s", step.PositionCode)
	}
	users := configStepUsers(step)
	out := []models.SigningDocumentSigner{}
	switch step.ConditionType {
	case 1:
		if len(users) == 0 {
			return nil, fmt.Errorf("missing signer users for position %s", step.PositionCode)
		}
		box := boxes[0]
		for i, user := range users {
			out = append(out, signerFromBox(step, box, i+1, "any", user, status))
		}
	case 2:
		if len(users) == 0 {
			return nil, fmt.Errorf("missing signer users for position %s", step.PositionCode)
		}
		for i, user := range users {
			box := findBoxForUser(boxes, user)
			if box.ID == "" {
				return nil, fmt.Errorf("missing signature box for user %s", user)
			}
			out = append(out, signerFromBox(step, box, i+1, "internal", user, status))
		}
	case 3:
		box := boxes[0]
		out = append(out, signerFromBox(step, box, 1, "external", "", status))
	default:
		return nil, fmt.Errorf("unsupported condition type %d", step.ConditionType)
	}
	return out, nil
}

func signerFromBox(step models.DocumentConfigStep, box models.SignatureTemplateBox, slot int, signerType, user, status string) models.SigningDocumentSigner {
	username, display := splitSignerUser(user)
	if signerType == "external" {
		display = "บุคคลภายนอก"
	}
	return models.SigningDocumentSigner{
		PositionCode:  step.PositionCode,
		PositionName:  step.PositionName,
		SequenceNo:    step.SequenceNo,
		ConditionType: step.ConditionType,
		SignerSlot:    slot,
		SignerType:    signerType,
		SignerUser:    username,
		SignerName:    display,
		Status:        status,
		PageNo:        box.PageNo,
		XRatio:        box.XRatio,
		YRatio:        box.YRatio,
		WidthRatio:    box.WidthRatio,
		HeightRatio:   box.HeightRatio,
		Label:         box.Label,
	}
}

func findBoxForUser(boxes []models.SignatureTemplateBox, user string) models.SignatureTemplateBox {
	username, _ := splitSignerUser(user)
	for _, box := range boxes {
		boxUser, _ := splitSignerUser(box.SignerUser)
		if strings.EqualFold(boxUser, username) {
			return box
		}
	}
	return models.SignatureTemplateBox{}
}

func configStepUsers(step models.DocumentConfigStep) []string {
	values := []string{}
	for _, value := range []string{step.User01, step.User02, step.User03} {
		value = strings.TrimSpace(value)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func splitSignerUser(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	parts := strings.SplitN(value, ":", 2)
	username := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return username, username
	}
	display := strings.TrimSpace(parts[1])
	if display == "" {
		display = username
	}
	return username, display
}

func signingDocumentSelect() string {
	return `
SELECT d.id::text, d.screen_code, d.doc_format_code, d.doc_no, d.sml_table, d.trans_flag,
       d.party_code, d.party_name, d.party_type, COALESCE(d.doc_date::text,''), d.total_amount,
       d.sml_is_lock_record, d.status, d.current_version,
       COALESCE(d.original_file_id::text,''), COALESCE(d.current_file_id::text,''), COALESCE(d.final_file_id::text,''),
       COALESCE(d.signature_template_id::text,''), COALESCE(d.created_by::text,''),
       d.created_at, d.updated_at, d.completed_at, d.locked_at,
       COALESCE(of.id::text,''), COALESCE(of.original_name,''), COALESCE(of.stored_name,''), COALESCE(of.storage_path,''), COALESCE(of.content_type,''), COALESCE(of.size_bytes,0), COALESCE(of.page_count,0), COALESCE(of.sha256,''), COALESCE(of.created_by::text,''), of.created_at,
       COALESCE(cf.id::text,''), COALESCE(cf.original_name,''), COALESCE(cf.stored_name,''), COALESCE(cf.storage_path,''), COALESCE(cf.content_type,''), COALESCE(cf.size_bytes,0), COALESCE(cf.page_count,0), COALESCE(cf.sha256,''), COALESCE(cf.created_by::text,''), cf.created_at,
       COALESCE(ff.id::text,''), COALESCE(ff.original_name,''), COALESCE(ff.stored_name,''), COALESCE(ff.storage_path,''), COALESCE(ff.content_type,''), COALESCE(ff.size_bytes,0), COALESCE(ff.page_count,0), COALESCE(ff.sha256,''), COALESCE(ff.created_by::text,''), ff.created_at
FROM signing_documents d
LEFT JOIN uploaded_files of ON of.id = d.original_file_id
LEFT JOIN uploaded_files cf ON cf.id = d.current_file_id
LEFT JOIN uploaded_files ff ON ff.id = d.final_file_id
`
}

func signingSignerSelect() string {
	return `
SELECT sg.id::text, sg.document_id::text, sg.step_id::text, sg.position_code, sg.position_name, sg.sequence_no,
       sg.condition_type, sg.signer_slot, sg.signer_type, sg.signer_user, sg.signer_name, sg.status,
       sg.page_no, sg.x_ratio, sg.y_ratio, sg.width_ratio, sg.height_ratio, sg.label,
       COALESCE(sg.signature_file_id::text,''), sg.signed_at, sg.rejected_at, sg.reject_reason,
       sg.device_id, sg.ip_address, sg.user_agent, COALESCE(sg.external_token_id::text,'')
FROM signing_document_signers sg
`
}

func scanSigningDocument(row rowScanner) (models.SigningDocument, error) {
	var doc models.SigningDocument
	var completedAt, lockedAt sql.NullTime
	var original, current, final models.UploadedFile
	var originalCreated, currentCreated, finalCreated sql.NullTime
	err := row.Scan(
		&doc.ID, &doc.ScreenCode, &doc.DocFormatCode, &doc.DocNo, &doc.SMLTable, &doc.TransFlag,
		&doc.PartyCode, &doc.PartyName, &doc.PartyType, &doc.DocDate, &doc.TotalAmount,
		&doc.SMLIsLockRecord, &doc.Status, &doc.CurrentVersion,
		&doc.OriginalFileID, &doc.CurrentFileID, &doc.FinalFileID, &doc.SignatureTemplateID, &doc.CreatedBy,
		&doc.CreatedAt, &doc.UpdatedAt, &completedAt, &lockedAt,
		&original.ID, &original.OriginalName, &original.StoredName, &original.StoragePath, &original.ContentType, &original.SizeBytes, &original.PageCount, &original.SHA256, &original.CreatedBy, &originalCreated,
		&current.ID, &current.OriginalName, &current.StoredName, &current.StoragePath, &current.ContentType, &current.SizeBytes, &current.PageCount, &current.SHA256, &current.CreatedBy, &currentCreated,
		&final.ID, &final.OriginalName, &final.StoredName, &final.StoragePath, &final.ContentType, &final.SizeBytes, &final.PageCount, &final.SHA256, &final.CreatedBy, &finalCreated,
	)
	if err != nil {
		return doc, err
	}
	if completedAt.Valid {
		doc.CompletedAt = &completedAt.Time
	}
	if lockedAt.Valid {
		doc.LockedAt = &lockedAt.Time
	}
	if original.ID != "" {
		if originalCreated.Valid {
			original.CreatedAt = originalCreated.Time
		}
		doc.OriginalFile = &original
	}
	if current.ID != "" {
		if currentCreated.Valid {
			current.CreatedAt = currentCreated.Time
		}
		doc.CurrentFile = &current
	}
	if final.ID != "" {
		if finalCreated.Valid {
			final.CreatedAt = finalCreated.Time
		}
		doc.FinalFile = &final
	}
	return doc, nil
}

func scanSigningDocumentStep(row rowScanner) (models.SigningDocumentStep, error) {
	var step models.SigningDocumentStep
	var completedAt sql.NullTime
	err := row.Scan(&step.ID, &step.DocumentID, &step.PositionCode, &step.PositionName, &step.SequenceNo, &step.ConditionType,
		&step.User01, &step.User02, &step.User03, &step.Status, &completedAt)
	if completedAt.Valid {
		step.CompletedAt = &completedAt.Time
	}
	return step, err
}

func scanSigningDocumentSigner(row rowScanner) (models.SigningDocumentSigner, error) {
	var signer models.SigningDocumentSigner
	var signedAt, rejectedAt sql.NullTime
	err := row.Scan(&signer.ID, &signer.DocumentID, &signer.StepID, &signer.PositionCode, &signer.PositionName, &signer.SequenceNo,
		&signer.ConditionType, &signer.SignerSlot, &signer.SignerType, &signer.SignerUser, &signer.SignerName, &signer.Status,
		&signer.PageNo, &signer.XRatio, &signer.YRatio, &signer.WidthRatio, &signer.HeightRatio, &signer.Label,
		&signer.SignatureFileID, &signedAt, &rejectedAt, &signer.RejectReason, &signer.DeviceID, &signer.IPAddress,
		&signer.UserAgent, &signer.ExternalTokenID)
	if signedAt.Valid {
		signer.SignedAt = &signedAt.Time
	}
	if rejectedAt.Valid {
		signer.RejectedAt = &rejectedAt.Time
	}
	return signer, err
}

func scanSigningDocumentEvent(row rowScanner) (models.SigningDocumentEvent, error) {
	var event models.SigningDocumentEvent
	var metadataBytes []byte
	err := row.Scan(&event.ID, &event.DocumentID, &event.ActorUserID, &event.ActorLabel, &event.Action, &event.Message,
		&event.IPAddress, &event.UserAgent, &metadataBytes, &event.CreatedAt)
	if err != nil {
		return event, err
	}
	event.Metadata = map[string]any{}
	if len(metadataBytes) > 0 {
		_ = json.Unmarshal(metadataBytes, &event.Metadata)
	}
	return event, nil
}

func scanSigningDocumentPrintEvent(row rowScanner) (models.SigningDocumentPrintEvent, error) {
	var item models.SigningDocumentPrintEvent
	err := row.Scan(&item.ID, &item.DocumentID, &item.FileID, &item.Channel, &item.PrinterName, &item.DeviceIDHash,
		&item.ClientTimezone, &item.FinalFileSHA256, &item.PrintedBy, &item.IPAddress, &item.UserAgent, &item.PrintedAt,
		&item.File.ID, &item.File.OriginalName, &item.File.StoredName, &item.File.StoragePath, &item.File.ContentType,
		&item.File.SizeBytes, &item.File.PageCount, &item.File.SHA256, &item.File.CreatedBy, &item.File.CreatedAt)
	return item, err
}

type signingEventWriter interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func insertSigningEvent(ctx context.Context, q signingEventWriter, documentID, actorUserID, actorLabel, action, message, ipAddress, userAgent string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
INSERT INTO signing_document_events (document_id, actor_user_id, actor_label, action, message, ip_address, user_agent, metadata)
VALUES ($1, NULLIF($2,'')::uuid, $3, $4, $5, $6, $7, $8::jsonb)
`, documentID, actorUserID, actorLabel, action, message, ipAddress, userAgent, string(data))
	return err
}
