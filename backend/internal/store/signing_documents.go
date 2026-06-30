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
	ScreenCode          string
	Format              models.SMLDocFormat
	Candidate           models.SMLDocumentCandidate
	SignatureTemplateID string
	TemplateSnapshot    any
	LegalNoticeSnapshot models.LegalNoticeSnapshot
	LayoutBoxes         []models.SignatureTemplateBoxRequest
	Configs             []models.DocumentConfigStep
	File                models.UploadedFile
	CurrentFile         *models.UploadedFile
	CurrentLegalVersion string
	ActorID             string
	IPAddress           string
	UserAgent           string
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
	templateSnapshot, err := json.Marshal(input.TemplateSnapshot)
	if err != nil {
		return models.SigningDocument{}, err
	}
	legalNoticeSnapshot, err := json.Marshal(input.LegalNoticeSnapshot)
	if err != nil {
		return models.SigningDocument{}, err
	}

	if err := consumeSigningDocumentUpload(ctx, tx, input.File.ID, input.ActorID); err != nil {
		return models.SigningDocument{}, err
	}
	currentFileID := input.File.ID
	currentHasLegalNotice := false
	if input.CurrentFile != nil && strings.TrimSpace(input.CurrentFile.ID) != "" {
		currentFileID = input.CurrentFile.ID
		currentHasLegalNotice = currentFileID != input.File.ID
	}

	var documentID string
	err = tx.QueryRow(ctx, `
INSERT INTO signing_documents (
    screen_code, doc_format_code, doc_no, sml_table, trans_flag, party_code, party_name, party_type,
    doc_date, total_amount, sml_is_lock_record, status, current_version,
    original_file_id, current_file_id, signature_template_id, config_snapshot, template_snapshot, legal_notice_snapshot, created_by
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::date,$10,$11,'in_progress',1,$12,$13,$14,$15::jsonb,$16::jsonb,$17::jsonb,NULLIF($18,'')::uuid)
RETURNING id::text
`, input.ScreenCode, input.Format.Code, input.Candidate.DocNo, input.Candidate.Table, input.Candidate.TransFlag,
		input.Candidate.PartyCode, input.Candidate.PartyName, input.Candidate.PartyType, input.Candidate.DocDate,
		input.Candidate.TotalAmount, input.Candidate.IsLockRecord, input.File.ID, currentFileID, input.SignatureTemplateID,
		string(configSnapshot), string(templateSnapshot), string(legalNoticeSnapshot), input.ActorID).Scan(&documentID)
	if err != nil {
		if strings.Contains(err.Error(), "signing_documents_active_doc_unique_idx") {
			return models.SigningDocument{}, ErrSigningDocumentDuplicate
		}
		return models.SigningDocument{}, err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO signing_document_versions (document_id, version_no, file_id, kind, created_by)
VALUES ($1, 1, $2, 'original', NULLIF($4,'')::uuid),
       ($1, 1, $3, 'current', NULLIF($4,'')::uuid)
`, documentID, input.File.ID, currentFileID, input.ActorID); err != nil {
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

	boxesByPosition := map[string][]models.SignatureTemplateBoxRequest{}
	for _, box := range input.LayoutBoxes {
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
	if currentHasLegalNotice {
		if err := insertSigningEvent(ctx, tx, documentID, input.ActorID, "", "pdf_stamped", "สร้าง PDF พร้อมข้อความกฎหมายแล้ว", input.IPAddress, input.UserAgent, map[string]any{
			"fileId":                    currentFileID,
			"signatureCount":            0,
			"final":                     false,
			"legalNoticeStamped":        true,
			"legalNoticeDisplayVersion": input.CurrentLegalVersion,
		}); err != nil {
			return models.SigningDocument{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return models.SigningDocument{}, err
	}
	return s.FindSigningDocumentByID(ctx, documentID)
}

func consumeSigningDocumentUpload(ctx context.Context, tx pgx.Tx, fileID, actorID string) error {
	tag, err := tx.Exec(ctx, `
UPDATE signing_document_uploads
SET consumed_at = now()
WHERE file_id = $1
  AND created_by = NULLIF($2, '')::uuid
  AND consumed_at IS NULL
  AND created_at >= now() - interval '24 hours'
`, fileID, actorID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSigningDocumentUploadNotFound
	}
	return nil
}

func (s *Store) ListSigningDocuments(ctx context.Context) ([]models.SigningDocument, error) {
	rows, err := s.pool.Query(ctx, signingDocumentSelect()+`
ORDER BY d.updated_at DESC, d.created_at DESC
`)
	if err != nil {
		return nil, err
	}
	return scanSigningDocumentRows(rows)
}

func (s *Store) GetAdminDashboard(ctx context.Context) (models.AdminDashboard, error) {
	var dashboard models.AdminDashboard
	rows, err := s.pool.Query(ctx, `
SELECT status, COUNT(*)::int
FROM signing_documents
GROUP BY status
`)
	if err != nil {
		return dashboard, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return dashboard, err
		}
		dashboard.Totals.Total += count
		switch status {
		case "draft":
			dashboard.Totals.Draft = count
		case "in_progress":
			dashboard.Totals.InProgress = count
		case "rejected":
			dashboard.Totals.Rejected = count
		case "completed":
			dashboard.Totals.Completed = count
		case "completed_evidence_failed":
			dashboard.Totals.CompletedEvidenceFailed = count
		case "completed_lock_failed":
			dashboard.Totals.CompletedLockFailed = count
		case "cancelled":
			dashboard.Totals.Cancelled = count
		}
	}
	if err := rows.Err(); err != nil {
		return dashboard, err
	}
	dashboard.WorkflowSummary.CompletedDocuments = dashboard.Totals.Completed
	dashboard.WorkflowSummary.EvidenceFailed = dashboard.Totals.CompletedEvidenceFailed
	dashboard.WorkflowSummary.LockFailed = dashboard.Totals.CompletedLockFailed
	dashboard.WorkflowSummary.AttentionDocuments = dashboard.Totals.CompletedEvidenceFailed + dashboard.Totals.CompletedLockFailed

	needsAttention, err := s.listSigningDocumentsByQuery(ctx, `
WHERE d.status IN ('completed_evidence_failed', 'completed_lock_failed')
ORDER BY d.updated_at DESC, d.created_at DESC
LIMIT 5
`)
	if err != nil {
		return dashboard, err
	}
	recent, err := s.listSigningDocumentsByQuery(ctx, `
ORDER BY d.updated_at DESC, d.created_at DESC
LIMIT 6
`)
	if err != nil {
		return dashboard, err
	}
	pendingSummary, err := s.getDashboardPendingSummary(ctx)
	if err != nil {
		return dashboard, err
	}
	pendingByPosition, err := s.listDashboardPendingByPosition(ctx)
	if err != nil {
		return dashboard, err
	}
	pendingDocuments, err := s.listDashboardPendingDocuments(ctx)
	if err != nil {
		return dashboard, err
	}
	dashboard.WorkflowSummary.PendingDocuments = pendingSummary.PendingDocuments
	dashboard.WorkflowSummary.PendingSigners = pendingSummary.PendingSigners
	dashboard.NeedsAttention = needsAttention
	dashboard.RecentDocuments = recent
	dashboard.PendingByPosition = pendingByPosition
	dashboard.PendingDocuments = pendingDocuments
	return dashboard, nil
}

func (s *Store) getDashboardPendingSummary(ctx context.Context) (models.AdminDashboardWorkflowSummary, error) {
	var summary models.AdminDashboardWorkflowSummary
	err := s.pool.QueryRow(ctx, `
SELECT COUNT(DISTINCT d.id)::int, COUNT(sg.id)::int
FROM signing_documents d
JOIN signing_document_signers sg ON sg.document_id = d.id
WHERE d.status = 'in_progress'
  AND sg.status = 'pending'
`).Scan(&summary.PendingDocuments, &summary.PendingSigners)
	return summary, err
}

func (s *Store) listDashboardPendingByPosition(ctx context.Context) ([]models.AdminDashboardPendingByPosition, error) {
	rows, err := s.pool.Query(ctx, `
SELECT sg.position_code,
       sg.position_name,
       sg.condition_type,
       COUNT(DISTINCT d.id)::int AS document_count,
       COUNT(sg.id)::int AS signer_count
FROM signing_documents d
JOIN signing_document_signers sg ON sg.document_id = d.id
WHERE d.status = 'in_progress'
  AND sg.status = 'pending'
GROUP BY sg.position_code, sg.position_name, sg.condition_type
ORDER BY MIN(sg.sequence_no), signer_count DESC, sg.position_code
LIMIT 8
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.AdminDashboardPendingByPosition{}
	for rows.Next() {
		var item models.AdminDashboardPendingByPosition
		if err := rows.Scan(&item.PositionCode, &item.PositionName, &item.ConditionType, &item.DocumentCount, &item.SignerCount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) listDashboardPendingDocuments(ctx context.Context) ([]models.AdminDashboardPendingDocument, error) {
	rows, err := s.pool.Query(ctx, `
SELECT d.id::text,
       d.doc_no,
       d.doc_format_code,
       d.party_name,
       d.party_code,
       COALESCE((array_agg(sg.position_name ORDER BY sg.sequence_no, sg.position_code))[1], '') AS current_position_name,
       COUNT(sg.id)::int AS pending_signer_count,
       d.updated_at
FROM signing_documents d
JOIN signing_document_signers sg ON sg.document_id = d.id
WHERE d.status = 'in_progress'
  AND sg.status = 'pending'
GROUP BY d.id, d.doc_no, d.doc_format_code, d.party_name, d.party_code, d.updated_at, d.created_at
ORDER BY d.updated_at DESC, d.created_at DESC
LIMIT 8
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.AdminDashboardPendingDocument{}
	for rows.Next() {
		var item models.AdminDashboardPendingDocument
		if err := rows.Scan(&item.ID, &item.DocNo, &item.DocFormatCode, &item.PartyName, &item.PartyCode, &item.CurrentPositionName, &item.PendingSignerCount, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) listSigningDocumentsByQuery(ctx context.Context, suffix string, args ...any) ([]models.SigningDocument, error) {
	rows, err := s.pool.Query(ctx, signingDocumentSelect()+suffix, args...)
	if err != nil {
		return nil, err
	}
	return scanSigningDocumentRows(rows)
}

func scanSigningDocumentRows(rows pgx.Rows) ([]models.SigningDocument, error) {
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

func (s *Store) ListSigningDocumentReferencesByDocNos(ctx context.Context, docNos []string) ([]models.SigningDocumentReference, error) {
	seen := map[string]bool{}
	clean := []string{}
	for _, docNo := range docNos {
		docNo = strings.TrimSpace(docNo)
		key := strings.ToUpper(docNo)
		if docNo == "" || seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, docNo)
	}
	if len(clean) == 0 {
		return []models.SigningDocumentReference{}, nil
	}
	rows, err := s.pool.Query(ctx, `
SELECT id::text, doc_no, doc_format_code, status
FROM signing_documents
WHERE doc_no = ANY($1)
ORDER BY updated_at DESC
`, clean)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.SigningDocumentReference{}
	for rows.Next() {
		var item models.SigningDocumentReference
		if err := rows.Scan(&item.ID, &item.DocNo, &item.DocFormatCode, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
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

func (s *Store) ListMySigningTaskQueue(ctx context.Context, username string, readyPage, waitingPage, size int) (models.MySigningTaskQueue, error) {
	username = strings.TrimSpace(username)
	if readyPage < 1 {
		readyPage = 1
	}
	if waitingPage < 1 {
		waitingPage = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 50 {
		size = 50
	}

	var queue models.MySigningTaskQueue
	queue.Pagination.Ready = models.PageMeta{Page: readyPage, Size: size}
	queue.Pagination.Waiting = models.PageMeta{Page: waitingPage, Size: size}

	readyCount, err := s.countMySigningTasksByStatus(ctx, username, "pending")
	if err != nil {
		return queue, err
	}
	waitingCount, err := s.countMySigningTasksByStatus(ctx, username, "waiting")
	if err != nil {
		return queue, err
	}
	queue.Counts = models.MySigningTaskCounts{Ready: readyCount, Waiting: waitingCount}

	ready, readyHasMore, err := s.listMySigningTaskDocumentsByStatus(ctx, username, "pending", readyPage, size)
	if err != nil {
		return queue, err
	}
	waiting, waitingHasMore, err := s.listMySigningTaskDocumentsByStatus(ctx, username, "waiting", waitingPage, size)
	if err != nil {
		return queue, err
	}
	if err := s.attachMySigningTaskBlockers(ctx, waiting); err != nil {
		return queue, err
	}

	queue.Documents = ready
	queue.WaitingDocuments = waiting
	queue.Pagination.Ready.HasMore = readyHasMore
	queue.Pagination.Waiting.HasMore = waitingHasMore
	return queue, nil
}

func (s *Store) countMySigningTasksByStatus(ctx context.Context, username, status string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
SELECT COUNT(sg.id)::int
FROM signing_documents d
JOIN signing_document_signers sg ON sg.document_id = d.id
WHERE d.status = 'in_progress'
  AND sg.status = $2
  AND lower(sg.signer_user) = lower($1)
`, strings.TrimSpace(username), status).Scan(&count)
	return count, err
}

func (s *Store) listMySigningTaskDocumentsByStatus(ctx context.Context, username, status string, page, size int) ([]models.MySigningTaskDocument, bool, error) {
	offset := (page - 1) * size
	rows, err := s.pool.Query(ctx, `
SELECT d.id::text,
       d.doc_no,
       d.doc_format_code,
       d.party_code,
       d.party_name,
       COALESCE(d.doc_date::text, ''),
       d.total_amount,
       d.status,
       d.updated_at,
       sg.id::text,
       sg.step_id::text,
       sg.position_code,
       sg.position_name,
       sg.sequence_no,
       sg.condition_type,
       sg.signer_slot,
       sg.signer_type,
       sg.signer_user,
       sg.signer_name,
       sg.status,
       sg.signed_at,
       sg.rejected_at
FROM signing_documents d
JOIN signing_document_signers sg ON sg.document_id = d.id
WHERE d.status = 'in_progress'
  AND sg.status = $2
  AND lower(sg.signer_user) = lower($1)
ORDER BY d.updated_at DESC, d.created_at DESC, sg.sequence_no, sg.position_code, sg.signer_slot
LIMIT $3 OFFSET $4
`, strings.TrimSpace(username), status, size+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	items := []models.MySigningTaskDocument{}
	for rows.Next() {
		item, err := scanMySigningTaskDocument(rows)
		if err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(items) > size
	if hasMore {
		items = items[:size]
	}
	return items, hasMore, nil
}

func scanMySigningTaskDocument(row rowScanner) (models.MySigningTaskDocument, error) {
	var item models.MySigningTaskDocument
	var task models.MySigningTaskSigner
	err := row.Scan(
		&item.ID, &item.DocNo, &item.DocFormatCode, &item.PartyCode, &item.PartyName, &item.DocDate,
		&item.TotalAmount, &item.Status, &item.UpdatedAt,
		&task.ID, &task.StepID, &task.PositionCode, &task.PositionName, &task.SequenceNo, &task.ConditionType,
		&task.SignerSlot, &task.SignerType, &task.SignerUser, &task.SignerName, &task.Status,
		&task.SignedAt, &task.RejectedAt,
	)
	if err != nil {
		return item, err
	}
	task.DocumentID = item.ID
	item.Task = task
	item.Signers = []models.MySigningTaskSigner{task}
	return item, nil
}

func (s *Store) attachMySigningTaskBlockers(ctx context.Context, documents []models.MySigningTaskDocument) error {
	if len(documents) == 0 {
		return nil
	}
	documentIDs := make([]string, 0, len(documents))
	taskSequenceByDocument := map[string]float64{}
	for _, doc := range documents {
		documentIDs = append(documentIDs, doc.ID)
		taskSequenceByDocument[doc.ID] = doc.Task.SequenceNo
	}

	rows, err := s.pool.Query(ctx, `
SELECT sg.document_id::text,
       sg.id::text,
       sg.step_id::text,
       sg.position_code,
       sg.position_name,
       sg.sequence_no,
       sg.condition_type,
       sg.signer_slot,
       sg.signer_type,
       sg.signer_user,
       sg.signer_name,
       sg.status,
       sg.signed_at,
       sg.rejected_at
FROM signing_document_signers sg
WHERE sg.document_id::text = ANY($1)
  AND sg.status = 'pending'
ORDER BY sg.document_id, sg.sequence_no, sg.position_code, sg.signer_slot, sg.signer_user
`, documentIDs)
	if err != nil {
		return err
	}
	defer rows.Close()

	blockersByDocument := map[string][]models.MySigningTaskBlocker{}
	blockerIndex := map[string]int{}
	for rows.Next() {
		var documentID string
		var signer models.MySigningTaskSigner
		if err := rows.Scan(
			&documentID, &signer.ID, &signer.StepID, &signer.PositionCode, &signer.PositionName, &signer.SequenceNo,
			&signer.ConditionType, &signer.SignerSlot, &signer.SignerType, &signer.SignerUser, &signer.SignerName,
			&signer.Status, &signer.SignedAt, &signer.RejectedAt,
		); err != nil {
			return err
		}
		signer.DocumentID = documentID
		if taskSequence, ok := taskSequenceByDocument[documentID]; ok && signer.SequenceNo >= taskSequence {
			continue
		}
		key := documentID + ":" + signer.StepID
		idx, ok := blockerIndex[key]
		if !ok {
			blockerIndex[key] = len(blockersByDocument[documentID])
			blockersByDocument[documentID] = append(blockersByDocument[documentID], models.MySigningTaskBlocker{
				PositionCode:  signer.PositionCode,
				PositionName:  signer.PositionName,
				SequenceNo:    signer.SequenceNo,
				ConditionType: signer.ConditionType,
				Status:        signer.Status,
			})
			idx = blockerIndex[key]
		}
		blockersByDocument[documentID][idx].Signers = append(blockersByDocument[documentID][idx].Signers, signer)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range documents {
		blockers := blockersByDocument[documents[i].ID]
		for j := range blockers {
			blockers[j].Summary = mySigningBlockerSummary(blockers[j])
		}
		documents[i].BlockedBy = blockers
		documents[i].BlockSummary = mySigningDocumentBlockSummary(blockers)
	}
	return nil
}

func mySigningDocumentBlockSummary(blockers []models.MySigningTaskBlocker) string {
	if len(blockers) == 0 {
		return "รอขั้นตอนก่อนหน้าเสร็จก่อน"
	}
	return blockers[0].Summary
}

func mySigningBlockerSummary(blocker models.MySigningTaskBlocker) string {
	positionName := strings.TrimSpace(blocker.PositionName)
	if positionName == "" {
		positionName = "ขั้นตอนก่อนหน้า"
	}
	names := []string{}
	for _, signer := range blocker.Signers {
		label := strings.TrimSpace(signer.SignerName)
		if label == "" {
			label = strings.TrimSpace(signer.SignerUser)
		}
		if label == "" && signer.SignerType == "external" {
			label = "บุคคลภายนอก"
		}
		if label != "" {
			names = append(names, label)
		}
	}
	nameText := strings.Join(names, ", ")
	switch blocker.ConditionType {
	case 1:
		if nameText == "" {
			return "รอคนใดคนหนึ่งในขั้น " + positionName + " เซ็นก่อน"
		}
		return "รอคนใดคนหนึ่งในขั้น " + positionName + ": " + nameText
	case 2:
		if nameText == "" {
			return "รอทุกคนในขั้น " + positionName + " เซ็นให้ครบ"
		}
		return "รอทุกคนในขั้น " + positionName + ": " + nameText
	case 3:
		if nameText == "" {
			return "รอผู้เซ็นภายนอกในขั้น " + positionName
		}
		return "รอผู้เซ็นภายนอกในขั้น " + positionName + ": " + nameText
	default:
		if nameText == "" {
			return "รอขั้น " + positionName + " เซ็นก่อน"
		}
		return "รอขั้น " + positionName + ": " + nameText
	}
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

func signerRowsForStep(step models.DocumentConfigStep, boxes []models.SignatureTemplateBoxRequest, status string) ([]models.SigningDocumentSigner, error) {
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
		if len(boxes) == 0 {
			return nil, fmt.Errorf("missing signature box for position %s", step.PositionCode)
		}
		for i, box := range boxes {
			if strings.TrimSpace(box.SignerUser) == "" {
				return nil, fmt.Errorf("missing signer user for position %s", step.PositionCode)
			}
			out = append(out, signerFromBox(step, box, i+1, "internal", box.SignerUser, status))
		}
	case 3:
		box := boxes[0]
		out = append(out, signerFromBox(step, box, 1, "external", "", status))
	default:
		return nil, fmt.Errorf("unsupported condition type %d", step.ConditionType)
	}
	return out, nil
}

func signerFromBox(step models.DocumentConfigStep, box models.SignatureTemplateBoxRequest, slot int, signerType, user, status string) models.SigningDocumentSigner {
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

func findBoxForUser(boxes []models.SignatureTemplateBoxRequest, user string) models.SignatureTemplateBoxRequest {
	username, _ := splitSignerUser(user)
	for _, box := range boxes {
		boxUser, _ := splitSignerUser(box.SignerUser)
		if strings.EqualFold(boxUser, username) {
			return box
		}
	}
	return models.SignatureTemplateBoxRequest{}
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
       d.created_at, d.updated_at, d.completed_at, d.locked_at, COALESCE(d.legal_notice_snapshot, '{}'::jsonb)::text,
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
	var legalNoticeRaw string
	var original, current, final models.UploadedFile
	var originalCreated, currentCreated, finalCreated sql.NullTime
	err := row.Scan(
		&doc.ID, &doc.ScreenCode, &doc.DocFormatCode, &doc.DocNo, &doc.SMLTable, &doc.TransFlag,
		&doc.PartyCode, &doc.PartyName, &doc.PartyType, &doc.DocDate, &doc.TotalAmount,
		&doc.SMLIsLockRecord, &doc.Status, &doc.CurrentVersion,
		&doc.OriginalFileID, &doc.CurrentFileID, &doc.FinalFileID, &doc.SignatureTemplateID, &doc.CreatedBy,
		&doc.CreatedAt, &doc.UpdatedAt, &completedAt, &lockedAt, &legalNoticeRaw,
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
	doc.LegalNoticeSnapshot = parseLegalNoticeSnapshot(legalNoticeRaw)
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
