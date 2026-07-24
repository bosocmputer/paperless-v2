package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const internalScreenCode = "INTERNAL"

var internalRunningHashes = regexp.MustCompile(`#+`)

type ReserveInternalDocumentInput struct {
	MasterID       string
	DocumentDate   time.Time
	RequiredDate   time.Time
	RequesterName  string
	PositionName   string
	DepartmentName string
	Purpose        string
	TotalAmount    string
	Items          []models.InternalDocumentItem
	Company        models.InternalDocumentCompanySnapshot
	IdempotencyKey string
	ActorID        string
}

func (s *Store) EnsureDefaultInternalDocumentMasters(ctx context.Context, actorID string) error {
	tenant := NormalizeSMLTenant(tenantFilterValue(ctx))
	_, err := s.pool.Exec(ctx, `
INSERT INTO internal_document_masters (sml_tenant, code, name, prefix, running_pattern, status, created_by)
VALUES ($1, 'PAYREQ', 'ใบขออนุมัติจ่าย', 'PAY', '@YYMMDD-###', 'inactive', NULLIF($2,'')::uuid),
       ($1, 'ADV', 'ใบขอเบิกเงินทดรอง', 'ADV', '@YYMMDD-###', 'inactive', NULLIF($2,'')::uuid),
       ($1, 'PREPAY', 'ใบขอจ่ายเงินล่วงหน้า', 'PRE', '@YYMMDD-###', 'inactive', NULLIF($2,'')::uuid)
ON CONFLICT (sml_tenant, code) DO NOTHING
`, tenant, actorID)
	return err
}

func (s *Store) ListInternalDocumentMasters(ctx context.Context) ([]models.InternalDocumentMaster, error) {
	tenant := tenantFilterValue(ctx)
	rows, err := s.pool.Query(ctx, internalMasterSelect()+`
WHERE ($1 = '' OR m.sml_tenant = $1)
ORDER BY lower(m.code)`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.InternalDocumentMaster{}
	for rows.Next() {
		item, err := scanInternalMaster(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) FindInternalDocumentMaster(ctx context.Context, id string) (models.InternalDocumentMaster, error) {
	tenant := tenantFilterValue(ctx)
	item, err := scanInternalMaster(s.pool.QueryRow(ctx, internalMasterSelect()+`
WHERE m.id = $1 AND ($2 = '' OR m.sml_tenant = $2)`, id, tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrInternalMasterNotFound
	}
	return item, err
}

func (s *Store) FindInternalDocumentMasterByCode(ctx context.Context, code string) (models.InternalDocumentMaster, error) {
	tenant := tenantFilterValue(ctx)
	item, err := scanInternalMaster(s.pool.QueryRow(ctx, internalMasterSelect()+`
WHERE lower(m.code) = lower($1) AND ($2 = '' OR m.sml_tenant = $2)`, code, tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrInternalMasterNotFound
	}
	return item, err
}

func (s *Store) CreateInternalDocumentMaster(ctx context.Context, req models.InternalDocumentMasterRequest, actorID string) (models.InternalDocumentMaster, error) {
	tenant := NormalizeSMLTenant(tenantFilterValue(ctx))
	var id string
	err := s.pool.QueryRow(ctx, `
INSERT INTO internal_document_masters (sml_tenant, code, name, prefix, running_pattern, status, created_by)
VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid)
RETURNING id::text`, tenant, req.Code, req.Name, req.Prefix, req.RunningPattern, req.Status, actorID).Scan(&id)
	if isUniqueViolation(err) {
		return models.InternalDocumentMaster{}, ErrInternalMasterDuplicate
	}
	if err != nil {
		return models.InternalDocumentMaster{}, err
	}
	return s.FindInternalDocumentMaster(ctx, id)
}

func (s *Store) UpdateInternalDocumentMaster(ctx context.Context, id string, req models.InternalDocumentMasterRequest) (models.InternalDocumentMaster, error) {
	tenant := tenantFilterValue(ctx)
	tag, err := s.pool.Exec(ctx, `
UPDATE internal_document_masters m
SET code = $1, name = $2, prefix = $3, running_pattern = $4, status = $5,
    revision = revision + 1, updated_at = now()
WHERE id = $6 AND ($7 = '' OR sml_tenant = $7) AND revision = $8
  AND (
    NOT EXISTS (SELECT 1 FROM internal_documents d WHERE d.master_id = m.id)
    OR (code = $1 AND prefix = $3 AND running_pattern = $4)
  )`, req.Code, req.Name, req.Prefix, req.RunningPattern, req.Status, id, tenant, req.Revision)
	if isUniqueViolation(err) {
		return models.InternalDocumentMaster{}, ErrInternalMasterDuplicate
	}
	if err != nil {
		return models.InternalDocumentMaster{}, err
	}
	if tag.RowsAffected() == 0 {
		current, findErr := s.FindInternalDocumentMaster(ctx, id)
		if findErr != nil {
			return models.InternalDocumentMaster{}, findErr
		}
		if current.DocumentCount > 0 && (current.Code != req.Code || current.Prefix != req.Prefix || current.RunningPattern != req.RunningPattern) {
			return models.InternalDocumentMaster{}, ErrInternalMasterInUse
		}
		return models.InternalDocumentMaster{}, ErrInternalMasterRevisionConflict
	}
	return s.FindInternalDocumentMaster(ctx, id)
}

func (s *Store) DeleteInternalDocumentMaster(ctx context.Context, id string) error {
	tenant := tenantFilterValue(ctx)
	tag, err := s.pool.Exec(ctx, `
DELETE FROM internal_document_masters m
WHERE id = $1 AND ($2 = '' OR sml_tenant = $2)
  AND NOT EXISTS (SELECT 1 FROM internal_documents d WHERE d.master_id = m.id)
  AND NOT EXISTS (SELECT 1 FROM document_config_steps c WHERE c.sml_tenant = m.sml_tenant AND c.screen_code = 'INTERNAL' AND lower(c.doc_format_code) = lower(m.code))
  AND NOT EXISTS (SELECT 1 FROM signature_templates t WHERE t.sml_tenant = m.sml_tenant AND t.screen_code = 'INTERNAL' AND lower(t.doc_format_code) = lower(m.code))
`, id, tenant)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, findErr := s.FindInternalDocumentMaster(ctx, id); findErr != nil {
			return findErr
		}
		return ErrInternalMasterInUse
	}
	return nil
}

func (s *Store) ReserveInternalDocument(ctx context.Context, input ReserveInternalDocumentInput) (models.InternalDocument, bool, error) {
	tenant := NormalizeSMLTenant(tenantFilterValue(ctx))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.InternalDocument{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existingID string
	err = tx.QueryRow(ctx, `SELECT id::text FROM internal_documents WHERE sml_tenant=$1 AND created_by=$2 AND idempotency_key=$3`, tenant, input.ActorID, input.IdempotencyKey).Scan(&existingID)
	if err == nil {
		_ = tx.Rollback(ctx)
		doc, findErr := s.FindInternalDocumentByID(ctx, existingID)
		return doc, true, findErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return models.InternalDocument{}, false, err
	}

	var master models.InternalDocumentMaster
	err = tx.QueryRow(ctx, `
SELECT id::text, sml_tenant, code, name, prefix, running_pattern, status, revision, created_at, updated_at
FROM internal_document_masters
WHERE id=$1 AND sml_tenant=$2
FOR UPDATE`, input.MasterID, tenant).Scan(&master.ID, &master.SMLTenant, &master.Code, &master.Name, &master.Prefix, &master.RunningPattern, &master.Status, &master.Revision, &master.CreatedAt, &master.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.InternalDocument{}, false, ErrInternalMasterNotFound
	}
	if err != nil {
		return models.InternalDocument{}, false, err
	}
	if master.Status != "active" {
		return models.InternalDocument{}, false, ErrInternalDocumentInvalidStatus
	}
	periodKey, renderedPattern, digits, err := renderInternalRunningPattern(master.RunningPattern, input.DocumentDate)
	if err != nil {
		return models.InternalDocument{}, false, err
	}
	var next int
	err = tx.QueryRow(ctx, `
INSERT INTO internal_document_running_counters (sml_tenant, master_id, period_key, last_value)
VALUES ($1,$2,$3,1)
ON CONFLICT (sml_tenant, master_id, period_key)
DO UPDATE SET last_value=internal_document_running_counters.last_value+1, updated_at=now()
RETURNING last_value`, tenant, master.ID, periodKey).Scan(&next)
	if err != nil {
		return models.InternalDocument{}, false, err
	}
	documentNo := master.Prefix + strings.Replace(renderedPattern, strings.Repeat("#", digits), fmt.Sprintf("%0*d", digits, next), 1)
	if len(documentNo) > 40 {
		return models.InternalDocument{}, false, fmt.Errorf("document number exceeds 40 characters")
	}
	companyRaw, err := json.Marshal(input.Company)
	if err != nil {
		return models.InternalDocument{}, false, err
	}
	var documentID string
	err = tx.QueryRow(ctx, `
INSERT INTO internal_documents (
  sml_tenant, master_id, master_code, master_name, master_revision, prefix_snapshot, pattern_snapshot,
  document_no, document_date, required_date, requester_name, position_name, department_name, purpose,
  total_amount, company_snapshot, status, revision, idempotency_key, created_by
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::numeric,$16::jsonb,'generating',1,$17,$18)
RETURNING id::text`, tenant, master.ID, master.Code, master.Name, master.Revision, master.Prefix, master.RunningPattern,
		documentNo, input.DocumentDate, input.RequiredDate, input.RequesterName, input.PositionName, input.DepartmentName, input.Purpose,
		input.TotalAmount, string(companyRaw), input.IdempotencyKey, input.ActorID).Scan(&documentID)
	if err != nil {
		return models.InternalDocument{}, false, err
	}
	for i, item := range input.Items {
		if _, err := tx.Exec(ctx, `INSERT INTO internal_document_items (document_id,revision,sequence_no,description,amount) VALUES ($1,1,$2,$3,$4::numeric)`, documentID, i+1, item.Description, item.Amount); err != nil {
			return models.InternalDocument{}, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return models.InternalDocument{}, false, err
	}
	doc, err := s.FindInternalDocumentByID(ctx, documentID)
	return doc, false, err
}

func (s *Store) MarkInternalDocumentGenerationFailed(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE internal_documents SET status='generation_failed', updated_at=now() WHERE id=$1 AND status='generating'`, id)
	return err
}

func (s *Store) FindSigningDocumentByInternalDocumentID(ctx context.Context, internalID string) (models.SigningDocument, error) {
	tenant := tenantFilterValue(ctx)
	var signingID string
	err := s.pool.QueryRow(ctx, `
SELECT id::text
FROM signing_documents
WHERE internal_document_id=$1
  AND document_source='internal'
  AND ($2='' OR sml_tenant=$2)
`, internalID, tenant).Scan(&signingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.SigningDocument{}, ErrSigningDocumentNotFound
	}
	if err != nil {
		return models.SigningDocument{}, err
	}
	return s.FindSigningDocumentByID(ctx, signingID)
}

func (s *Store) CompleteInternalDocumentCreate(ctx context.Context, internalID, signingID string, file models.UploadedFile, actorID string) (models.InternalDocument, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.InternalDocument{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var versionID string
	err = tx.QueryRow(ctx, `
INSERT INTO internal_document_versions (document_id,revision,file_id,sha256,page_count,created_by)
VALUES ($1,1,$2,$3,$4,$5)
ON CONFLICT (document_id,revision) DO UPDATE SET file_id=EXCLUDED.file_id, sha256=EXCLUDED.sha256, page_count=EXCLUDED.page_count
RETURNING id::text`, internalID, file.ID, file.SHA256, file.PageCount, actorID).Scan(&versionID)
	if err != nil {
		return models.InternalDocument{}, err
	}
	tag, err := tx.Exec(ctx, `
UPDATE internal_documents
SET current_version_id=$2, signing_document_id=$3, status='draft', updated_at=now()
WHERE id=$1 AND status IN ('generating','generation_failed')`, internalID, versionID, signingID)
	if err != nil {
		return models.InternalDocument{}, err
	}
	if tag.RowsAffected() == 0 {
		return models.InternalDocument{}, ErrInternalDocumentInvalidStatus
	}
	if err := tx.Commit(ctx); err != nil {
		return models.InternalDocument{}, err
	}
	return s.FindInternalDocumentByID(ctx, internalID)
}

type UpdateInternalDocumentRevisionInput struct {
	InternalID       string
	ExpectedRevision int
	RequiredDate     time.Time
	RequesterName    string
	PositionName     string
	DepartmentName   string
	Purpose          string
	TotalAmount      string
	Items            []models.InternalDocumentItem
	OriginalFile     models.UploadedFile
	CurrentFile      models.UploadedFile
	ActorID          string
	IPAddress        string
	UserAgent        string
}

func (s *Store) UpdateInternalDocumentRevision(ctx context.Context, input UpdateInternalDocumentRevisionInput) (models.InternalDocument, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.InternalDocument{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var revision int
	var status, createdBy, signingID string
	err = tx.QueryRow(ctx, `SELECT revision,status,created_by::text,COALESCE(signing_document_id::text,'') FROM internal_documents WHERE id=$1 AND ($2='' OR sml_tenant=$2) FOR UPDATE`, input.InternalID, tenantFilterValue(ctx)).Scan(&revision, &status, &createdBy, &signingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.InternalDocument{}, ErrInternalDocumentNotFound
	}
	if err != nil {
		return models.InternalDocument{}, err
	}
	if revision != input.ExpectedRevision {
		return models.InternalDocument{}, ErrInternalDocumentRevisionConflict
	}
	if status != "draft" || createdBy != input.ActorID || signingID == "" {
		return models.InternalDocument{}, ErrInternalDocumentInvalidStatus
	}
	var signingStatus string
	if err := tx.QueryRow(ctx, `
SELECT status
FROM signing_documents
WHERE id=$1
FOR UPDATE`, signingID).Scan(&signingStatus); err != nil {
		return models.InternalDocument{}, err
	}
	if signingStatus != "draft" {
		return models.InternalDocument{}, ErrInternalDocumentInvalidStatus
	}
	next := revision + 1
	for i, item := range input.Items {
		if _, err := tx.Exec(ctx, `INSERT INTO internal_document_items(document_id,revision,sequence_no,description,amount) VALUES($1,$2,$3,$4,$5::numeric)`, input.InternalID, next, i+1, item.Description, item.Amount); err != nil {
			return models.InternalDocument{}, err
		}
	}
	var versionID string
	if err := tx.QueryRow(ctx, `INSERT INTO internal_document_versions(document_id,revision,file_id,sha256,page_count,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, input.InternalID, next, input.OriginalFile.ID, input.OriginalFile.SHA256, input.OriginalFile.PageCount, input.ActorID).Scan(&versionID); err != nil {
		return models.InternalDocument{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE internal_documents SET required_date=$2,requester_name=$3,position_name=$4,department_name=$5,purpose=$6,total_amount=$7::numeric,revision=$8,current_version_id=$9,updated_at=now() WHERE id=$1`, input.InternalID, input.RequiredDate, input.RequesterName, input.PositionName, input.DepartmentName, input.Purpose, input.TotalAmount, next, versionID); err != nil {
		return models.InternalDocument{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM signing_document_signers WHERE document_id=$1`, signingID); err != nil {
		return models.InternalDocument{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE signing_document_steps SET status='waiting', completed_at=NULL WHERE document_id=$1`, signingID); err != nil {
		return models.InternalDocument{}, err
	}
	var signingVersion int
	if err := tx.QueryRow(ctx, `
UPDATE signing_documents
SET original_file_id=$2,
    current_file_id=$3,
    current_version=current_version+1,
    party_name=$4,
    total_amount=$5::numeric,
    template_snapshot='{ "source": "internal_draft_layout_required" }'::jsonb,
    signature_template_id=NULL,
    signature_placement_snapshot='[]'::jsonb,
    legal_notice_snapshot='{}'::jsonb,
    legal_notice_boxes_snapshot='[]'::jsonb,
    layout_ready=false,
    updated_at=now()
WHERE id=$1
RETURNING current_version`, signingID, input.OriginalFile.ID, input.CurrentFile.ID, input.RequesterName, input.TotalAmount).Scan(&signingVersion); err != nil {
		return models.InternalDocument{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO signing_document_versions(document_id,version_no,file_id,kind,created_by) VALUES($1,$2,$3,'original',$5),($1,$2,$4,'current',$5)`, signingID, signingVersion, input.OriginalFile.ID, input.CurrentFile.ID, input.ActorID); err != nil {
		return models.InternalDocument{}, err
	}
	if err := insertSigningEvent(ctx, tx, signingID, input.ActorID, "", "internal_document_revised", "แก้ไขแบบฟอร์มเอกสารภายในและต้องจัดวางกรอบใหม่", input.IPAddress, input.UserAgent, map[string]any{"internalRevision": next, "layoutInvalidated": true}); err != nil {
		return models.InternalDocument{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.InternalDocument{}, err
	}
	return s.FindInternalDocumentByID(ctx, input.InternalID)
}

func (s *Store) FindInternalDocumentByID(ctx context.Context, id string) (models.InternalDocument, error) {
	tenant := tenantFilterValue(ctx)
	var item models.InternalDocument
	var companyRaw string
	var printedAt, sentAt *time.Time
	err := s.pool.QueryRow(ctx, `
SELECT d.id::text,d.sml_tenant,d.master_id::text,d.master_code,d.master_name,d.master_revision,d.prefix_snapshot,d.pattern_snapshot,
       d.document_no,d.document_date::text,d.required_date::text,d.requester_name,d.position_name,d.department_name,d.purpose,
       d.total_amount::text,d.status,d.revision,COALESCE(d.current_version_id::text,''),COALESCE(d.signing_document_id::text,''),
       d.company_snapshot::text,d.created_by::text,d.created_at,d.updated_at,
       COALESCE(v.id::text,''),COALESCE(v.file_id::text,''),COALESCE(v.sha256,''),COALESCE(v.page_count,0),v.printed_at,v.sent_at,v.created_at,
       EXISTS(
           SELECT 1
           FROM internal_document_print_events p
           JOIN signing_documents s ON s.id=d.signing_document_id
           WHERE p.document_id=d.id AND p.revision=d.revision AND p.file_id=s.current_file_id
       )
FROM internal_documents d
LEFT JOIN internal_document_versions v ON v.id=d.current_version_id
WHERE d.id=$1 AND ($2='' OR d.sml_tenant=$2)`, id, tenant).Scan(
		&item.ID, &item.SMLTenant, &item.MasterID, &item.MasterCode, &item.MasterName, &item.MasterRevision, &item.PrefixSnapshot, &item.PatternSnapshot,
		&item.DocumentNo, &item.DocumentDate, &item.RequiredDate, &item.RequesterName, &item.PositionName, &item.DepartmentName, &item.Purpose,
		&item.TotalAmount, &item.Status, &item.Revision, &item.CurrentVersionID, &item.SigningDocumentID, &companyRaw, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt,
		new(string), new(string), new(string), new(int), &printedAt, &sentAt, new(time.Time), &item.CurrentRevisionPrint,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrInternalDocumentNotFound
	}
	if err != nil {
		return item, err
	}
	_ = json.Unmarshal([]byte(companyRaw), &item.CompanySnapshot)
	items, err := s.listInternalDocumentItems(ctx, item.ID, item.Revision)
	if err != nil {
		return item, err
	}
	item.Items = items
	if item.CurrentVersionID != "" {
		version, err := s.findInternalVersion(ctx, item.CurrentVersionID)
		if err != nil {
			return item, err
		}
		item.CurrentVersion = &version
	}
	return item, nil
}

func (s *Store) listInternalDocumentItems(ctx context.Context, documentID string, revision int) ([]models.InternalDocumentItem, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text,sequence_no,description,amount::text FROM internal_document_items WHERE document_id=$1 AND revision=$2 ORDER BY sequence_no`, documentID, revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []models.InternalDocumentItem{}
	for rows.Next() {
		var item models.InternalDocumentItem
		if err := rows.Scan(&item.ID, &item.SequenceNo, &item.Description, &item.Amount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) findInternalVersion(ctx context.Context, id string) (models.InternalDocumentVersion, error) {
	var item models.InternalDocumentVersion
	err := s.pool.QueryRow(ctx, `SELECT id::text,document_id::text,revision,file_id::text,sha256,page_count,printed_at,sent_at,created_at FROM internal_document_versions WHERE id=$1`, id).Scan(&item.ID, &item.DocumentID, &item.Revision, &item.FileID, &item.SHA256, &item.PageCount, &item.PrintedAt, &item.SentAt, &item.CreatedAt)
	return item, err
}

func (s *Store) CreateInternalDocumentPrintEvent(ctx context.Context, internalID, actorID, ipAddress, userAgent string, allowSuperAdmin bool) (models.InternalDocument, models.UploadedFile, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.InternalDocument{}, models.UploadedFile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var versionID, fileID, sha, createdBy, status string
	var revision int
	var layoutReady bool
	err = tx.QueryRow(ctx, `
SELECT d.current_version_id::text,d.revision,d.created_by::text,d.status,COALESCE(s.current_file_id::text,''),s.layout_ready
FROM internal_documents d
JOIN signing_documents s ON s.id=d.signing_document_id AND s.document_source='internal'
WHERE d.id=$1 AND ($2='' OR d.sml_tenant=$2)
FOR UPDATE OF d,s`, internalID, tenantFilterValue(ctx)).Scan(&versionID, &revision, &createdBy, &status, &fileID, &layoutReady)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.InternalDocument{}, models.UploadedFile{}, ErrInternalDocumentNotFound
	}
	if err != nil {
		return models.InternalDocument{}, models.UploadedFile{}, err
	}
	if (createdBy != actorID && !allowSuperAdmin) || status != "draft" {
		return models.InternalDocument{}, models.UploadedFile{}, ErrInternalDocumentInvalidStatus
	}
	if !layoutReady || strings.TrimSpace(fileID) == "" {
		return models.InternalDocument{}, models.UploadedFile{}, ErrSigningDocumentLayoutRequired
	}
	if err := tx.QueryRow(ctx, `SELECT sha256 FROM uploaded_files WHERE id=NULLIF($1,'')::uuid`, fileID).Scan(&sha); err != nil {
		return models.InternalDocument{}, models.UploadedFile{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO internal_document_print_events(document_id,version_id,revision,file_id,file_sha256,printed_by,ip_address,user_agent) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, internalID, versionID, revision, fileID, sha, actorID, ipAddress, userAgent); err != nil {
		return models.InternalDocument{}, models.UploadedFile{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE internal_document_versions SET printed_at=COALESCE(printed_at,now()) WHERE id=$1`, versionID); err != nil {
		return models.InternalDocument{}, models.UploadedFile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.InternalDocument{}, models.UploadedFile{}, err
	}
	doc, err := s.FindInternalDocumentByID(ctx, internalID)
	if err != nil {
		return models.InternalDocument{}, models.UploadedFile{}, err
	}
	file, err := s.FindUploadedFileByID(ctx, fileID)
	return doc, file, err
}

func (s *Store) MarkInternalDocumentSent(ctx context.Context, signingDocumentID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE internal_documents d SET status='in_progress',updated_at=now() FROM signing_documents s WHERE s.id=$1 AND d.id=s.internal_document_id`, signingDocumentID)
	return err
}

func (s *Store) MarkInternalDocumentCompleted(ctx context.Context, signingDocumentID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `UPDATE signing_documents SET status='completed',completed_at=COALESCE(completed_at,now()),locked_at=NULL,updated_at=now() WHERE id=$1 AND document_source='internal'`, signingDocumentID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE internal_documents d SET status='completed',updated_at=now() FROM signing_documents s WHERE s.id=$1 AND d.id=s.internal_document_id`, signingDocumentID); err != nil {
		return err
	}
	if err = insertSigningEvent(ctx, tx, signingDocumentID, "", "", "internal_document_completed", "เอกสารภายในเสร็จสมบูรณ์ใน PaperLess", "", "", map[string]any{"documentSource": "internal"}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func internalMasterSelect() string {
	return `
SELECT m.id::text,m.sml_tenant,m.code,m.name,m.prefix,m.running_pattern,m.status,m.revision,
       (SELECT COUNT(*)::int FROM internal_documents d WHERE d.master_id=m.id),
       EXISTS(SELECT 1 FROM document_config_steps c WHERE c.sml_tenant=m.sml_tenant AND c.screen_code='INTERNAL' AND lower(c.doc_format_code)=lower(m.code)),
       EXISTS(
         SELECT 1
         FROM signature_templates t
         WHERE t.sml_tenant=m.sml_tenant
           AND t.screen_code='INTERNAL'
           AND lower(t.doc_format_code)=lower(m.code)
           AND t.status='active'
           AND COALESCE(t.legal_notice_box, '{}'::jsonb) <> '{}'::jsonb
           AND EXISTS (SELECT 1 FROM signature_template_boxes b WHERE b.template_id=t.id)
       ),
       m.created_at,m.updated_at
FROM internal_document_masters m `
}

func scanInternalMaster(row rowScanner) (models.InternalDocumentMaster, error) {
	var item models.InternalDocumentMaster
	err := row.Scan(&item.ID, &item.SMLTenant, &item.Code, &item.Name, &item.Prefix, &item.RunningPattern, &item.Status, &item.Revision, &item.DocumentCount, &item.WorkflowReady, &item.TemplateReady, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func renderInternalRunningPattern(pattern string, date time.Time) (string, string, int, error) {
	pattern = strings.ToUpper(strings.TrimSpace(pattern))
	pattern = strings.TrimPrefix(pattern, "@")
	matches := internalRunningHashes.FindAllStringIndex(pattern, -1)
	if len(matches) != 1 {
		return "", "", 0, fmt.Errorf("running pattern must contain one # group")
	}
	digits := matches[0][1] - matches[0][0]
	if digits < 1 || digits > 9 {
		return "", "", 0, fmt.Errorf("running digits must be between 1 and 9")
	}
	year := date.Year()
	rendered := strings.NewReplacer("YYYY", fmt.Sprintf("%04d", year), "YY", fmt.Sprintf("%02d", year%100), "MM", fmt.Sprintf("%02d", int(date.Month())), "DD", fmt.Sprintf("%02d", date.Day())).Replace(pattern)
	period := strings.NewReplacer("#", "", "-", "", "/", "", ".", "").Replace(rendered)
	return period, rendered, digits, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func ParseInternalAmount(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("amount is required")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return 0, errors.New("amount is invalid")
	}
	whole := parts[0]
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if len(frac) > 2 {
		return 0, errors.New("amount supports at most 2 decimals")
	}
	for len(frac) < 2 {
		frac += "0"
	}
	if whole == "" {
		whole = "0"
	}
	wholeValue, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || wholeValue < 0 {
		return 0, errors.New("amount is invalid")
	}
	fracValue, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, errors.New("amount is invalid")
	}
	if wholeValue > 9999999999999999 {
		return 0, errors.New("amount is too large")
	}
	return wholeValue*100 + fracValue, nil
}
