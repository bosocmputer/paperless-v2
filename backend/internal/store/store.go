package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/auth"
	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

var (
	ErrUsernameTaken             = errors.New("username already exists")
	ErrUserNotFound              = errors.New("user not found")
	ErrDocumentConfigDuplicate   = errors.New("document config step already exists")
	ErrDocumentConfigNotFound    = errors.New("document config step not found")
	ErrSignatureTemplateNotFound = errors.New("signature template not found")
	ErrSignatureRevisionConflict = errors.New("signature template revision conflict")
	ErrSignatureTemplateNotDraft = errors.New("signature template is not draft")
	ErrSignatureTemplateArchived = errors.New("signature template is archived")
	ErrSigningDocumentNotFound   = errors.New("signing document not found")
	ErrSigningDocumentDuplicate  = errors.New("signing document already exists")
	ErrSigningTaskNotFound       = errors.New("signing task not found")
	ErrSigningTaskUnavailable    = errors.New("signing task is not available")
	ErrExternalTokenNotFound     = errors.New("external signing token not found")
	ErrExternalTokenInvalid      = errors.New("external signing token invalid")
	ErrIdempotencyInProgress     = errors.New("idempotency key is already in progress")
)

type IdempotencyClaim struct {
	Claimed      bool
	Response     json.RawMessage
	ResponseCode int
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return s.pool.Ping(ctx)
}

func (s *Store) EnsureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name TEXT NOT NULL,
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'user')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_idx ON users (lower(username));

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id UUID REFERENCES users(id),
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT,
    ip_address TEXT,
    user_agent TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS document_config_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    screen_code TEXT NOT NULL,
    doc_format_code TEXT NOT NULL,
    position_code TEXT NOT NULL,
    position_name TEXT NOT NULL,
    user01 TEXT NOT NULL DEFAULT '',
    user02 TEXT NOT NULL DEFAULT '',
    user03 TEXT NOT NULL DEFAULT '',
    sequence_no DOUBLE PRECISION NOT NULL CHECK (sequence_no > 0),
    condition_type INTEGER NOT NULL CHECK (condition_type IN (1, 2, 3)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE document_config_steps
DROP CONSTRAINT IF EXISTS document_config_steps_screen_code_check;

ALTER TABLE document_config_steps
ADD CONSTRAINT document_config_steps_screen_code_check
CHECK (screen_code <> '' AND length(screen_code) <= 40);

CREATE UNIQUE INDEX IF NOT EXISTS document_config_steps_unique_position_idx
ON document_config_steps (screen_code, lower(doc_format_code), lower(position_code));

CREATE INDEX IF NOT EXISTS document_config_steps_lookup_idx
ON document_config_steps (screen_code, lower(doc_format_code), sequence_no);

CREATE TABLE IF NOT EXISTS uploaded_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_name TEXT NOT NULL,
    stored_name TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0),
    page_count INTEGER NOT NULL DEFAULT 0 CHECK (page_count >= 0),
    sha256 TEXT NOT NULL,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE uploaded_files
ADD COLUMN IF NOT EXISTS page_count INTEGER NOT NULL DEFAULT 0 CHECK (page_count >= 0);

CREATE INDEX IF NOT EXISTS uploaded_files_sha256_idx ON uploaded_files (sha256);

CREATE TABLE IF NOT EXISTS signature_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    screen_code TEXT NOT NULL,
    doc_format_code TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'archived')),
    sample_file_id UUID REFERENCES uploaded_files(id),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_by UUID REFERENCES users(id),
    published_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS signature_templates_active_unique_idx
ON signature_templates (screen_code, lower(doc_format_code))
WHERE status = 'active';

CREATE UNIQUE INDEX IF NOT EXISTS signature_templates_draft_unique_idx
ON signature_templates (screen_code, lower(doc_format_code))
WHERE status = 'draft';

CREATE INDEX IF NOT EXISTS signature_templates_lookup_idx
ON signature_templates (screen_code, lower(doc_format_code), status, version DESC);

CREATE TABLE IF NOT EXISTS signature_template_boxes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID NOT NULL REFERENCES signature_templates(id) ON DELETE CASCADE,
    position_code TEXT NOT NULL,
    signer_slot INTEGER NOT NULL CHECK (signer_slot > 0),
    signer_type TEXT NOT NULL CHECK (signer_type IN ('any', 'internal', 'external')),
    signer_user TEXT NOT NULL DEFAULT '',
    page_no INTEGER NOT NULL CHECK (page_no > 0),
    x_ratio DOUBLE PRECISION NOT NULL,
    y_ratio DOUBLE PRECISION NOT NULL,
    width_ratio DOUBLE PRECISION NOT NULL,
    height_ratio DOUBLE PRECISION NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (x_ratio >= 0 AND y_ratio >= 0),
    CHECK (width_ratio > 0 AND height_ratio > 0),
    CHECK (x_ratio + width_ratio <= 1),
    CHECK (y_ratio + height_ratio <= 1)
);

CREATE UNIQUE INDEX IF NOT EXISTS signature_template_boxes_slot_unique_idx
ON signature_template_boxes (template_id, lower(position_code), signer_slot);

CREATE INDEX IF NOT EXISTS signature_template_boxes_lookup_idx
ON signature_template_boxes (template_id, page_no, lower(position_code), signer_slot);

CREATE TABLE IF NOT EXISTS signing_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    screen_code TEXT NOT NULL,
    doc_format_code TEXT NOT NULL,
    doc_no TEXT NOT NULL,
    sml_table TEXT NOT NULL DEFAULT '',
    trans_flag INTEGER NOT NULL DEFAULT 0,
    party_code TEXT NOT NULL DEFAULT '',
    party_name TEXT NOT NULL DEFAULT '',
    party_type TEXT NOT NULL DEFAULT '',
    doc_date DATE,
    total_amount DOUBLE PRECISION NOT NULL DEFAULT 0,
    sml_is_lock_record INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('draft', 'in_progress', 'rejected', 'completed', 'completed_lock_failed', 'cancelled')),
    current_version INTEGER NOT NULL DEFAULT 1 CHECK (current_version > 0),
    original_file_id UUID REFERENCES uploaded_files(id),
    current_file_id UUID REFERENCES uploaded_files(id),
    final_file_id UUID REFERENCES uploaded_files(id),
    signature_template_id UUID REFERENCES signature_templates(id),
    config_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb,
    template_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    locked_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS signing_documents_active_doc_unique_idx
ON signing_documents (lower(doc_format_code), doc_no)
WHERE status IN ('draft', 'in_progress', 'completed_lock_failed');

CREATE INDEX IF NOT EXISTS signing_documents_status_idx
ON signing_documents (status, updated_at DESC);

CREATE TABLE IF NOT EXISTS signing_document_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES signing_documents(id) ON DELETE CASCADE,
    version_no INTEGER NOT NULL CHECK (version_no > 0),
    file_id UUID NOT NULL REFERENCES uploaded_files(id),
    kind TEXT NOT NULL CHECK (kind IN ('original', 'current', 'final')),
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS signing_document_versions_unique_idx
ON signing_document_versions (document_id, version_no, kind);

CREATE TABLE IF NOT EXISTS signing_document_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES signing_documents(id) ON DELETE CASCADE,
    position_code TEXT NOT NULL,
    position_name TEXT NOT NULL,
    sequence_no DOUBLE PRECISION NOT NULL CHECK (sequence_no > 0),
    condition_type INTEGER NOT NULL CHECK (condition_type IN (1, 2, 3)),
    user01 TEXT NOT NULL DEFAULT '',
    user02 TEXT NOT NULL DEFAULT '',
    user03 TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('waiting', 'pending', 'completed', 'rejected', 'skipped')),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS signing_document_steps_lookup_idx
ON signing_document_steps (document_id, sequence_no);

CREATE TABLE IF NOT EXISTS signing_document_signers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES signing_documents(id) ON DELETE CASCADE,
    step_id UUID NOT NULL REFERENCES signing_document_steps(id) ON DELETE CASCADE,
    position_code TEXT NOT NULL,
    position_name TEXT NOT NULL,
    sequence_no DOUBLE PRECISION NOT NULL CHECK (sequence_no > 0),
    condition_type INTEGER NOT NULL CHECK (condition_type IN (1, 2, 3)),
    signer_slot INTEGER NOT NULL CHECK (signer_slot > 0),
    signer_type TEXT NOT NULL CHECK (signer_type IN ('any', 'internal', 'external')),
    signer_user TEXT NOT NULL DEFAULT '',
    signer_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('waiting', 'pending', 'signed', 'rejected', 'skipped')),
    page_no INTEGER NOT NULL CHECK (page_no > 0),
    x_ratio DOUBLE PRECISION NOT NULL,
    y_ratio DOUBLE PRECISION NOT NULL,
    width_ratio DOUBLE PRECISION NOT NULL,
    height_ratio DOUBLE PRECISION NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    signature_file_id UUID REFERENCES uploaded_files(id),
    signed_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    reject_reason TEXT NOT NULL DEFAULT '',
    device_id TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    external_token_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS signing_document_signers_user_idx
ON signing_document_signers (lower(signer_user), status, sequence_no);

CREATE INDEX IF NOT EXISTS signing_document_signers_doc_idx
ON signing_document_signers (document_id, sequence_no, position_code, signer_slot);

CREATE TABLE IF NOT EXISTS signing_document_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES signing_documents(id) ON DELETE CASCADE,
    actor_user_id UUID REFERENCES users(id),
    actor_label TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS signing_document_events_doc_idx
ON signing_document_events (document_id, created_at DESC);

CREATE TABLE IF NOT EXISTS signing_document_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES signing_documents(id) ON DELETE CASCADE,
    signer_id UUID REFERENCES signing_document_signers(id) ON DELETE SET NULL,
    file_id UUID NOT NULL REFERENCES uploaded_files(id),
    note TEXT NOT NULL DEFAULT '',
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS signing_document_attachments_doc_idx
ON signing_document_attachments (document_id, created_at DESC);

CREATE TABLE IF NOT EXISTS external_signing_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES signing_documents(id) ON DELETE CASCADE,
    signer_id UUID NOT NULL REFERENCES signing_document_signers(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    otp_hash TEXT NOT NULL,
    session_hash TEXT NOT NULL DEFAULT '',
    session_expires_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
    status TEXT NOT NULL CHECK (status IN ('active', 'verified', 'locked', 'used', 'revoked', 'expired')),
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    verified_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS external_signing_tokens_token_hash_idx
ON external_signing_tokens (token_hash);

CREATE INDEX IF NOT EXISTS external_signing_tokens_signer_idx
ON external_signing_tokens (signer_id, status);

CREATE TABLE IF NOT EXISTS idempotency_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key TEXT NOT NULL,
    scope TEXT NOT NULL,
    actor_user_id UUID REFERENCES users(id),
    response_status INTEGER NOT NULL DEFAULT 0,
    response_body JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idempotency_keys_unique_idx
ON idempotency_keys (scope, key, COALESCE(actor_user_id::text, ''));
`)
	return err
}

func (s *Store) EnsureSuperAdmin(ctx context.Context, seed models.SeedUser) error {
	seed.Username = strings.TrimSpace(seed.Username)
	seed.DisplayName = strings.TrimSpace(seed.DisplayName)
	if seed.Username == "" || seed.Password == "" {
		return errors.New("seed superadmin username and password are required")
	}
	if seed.DisplayName == "" {
		seed.DisplayName = seed.Username
	}
	if seed.Role == "" {
		seed.Role = "admin"
	}

	var existingID string
	err := s.pool.QueryRow(ctx, `SELECT id::text FROM users WHERE lower(username) = lower($1)`, seed.Username).Scan(&existingID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	hash, err := auth.HashPassword(seed.Password)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
INSERT INTO users (display_name, username, password_hash, role, status)
VALUES ($1, $2, $3, $4, 'active')
`, seed.DisplayName, seed.Username, hash, seed.Role)
	return err
}

func (s *Store) FindUserByUsername(ctx context.Context, username string) (models.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
SELECT id::text, display_name, username, password_hash, role, status, created_at
FROM users
WHERE lower(username) = lower($1)
`, strings.TrimSpace(username)))
}

func (s *Store) FindUserByID(ctx context.Context, id string) (models.User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
SELECT id::text, display_name, username, password_hash, role, status, created_at
FROM users
WHERE id = $1
`, id))
}

func (s *Store) ListUsers(ctx context.Context) ([]models.User, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id::text, display_name, username, password_hash, role, status, created_at
FROM users
ORDER BY created_at DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []models.User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, req models.CreateUserRequest) (models.User, error) {
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return models.User{}, err
	}

	user, err := scanUser(s.pool.QueryRow(ctx, `
INSERT INTO users (display_name, username, password_hash, role, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING id::text, display_name, username, password_hash, role, status, created_at
`, req.DisplayName, req.Username, hash, req.Role, req.Status))
	if err != nil {
		if strings.Contains(err.Error(), "users_username_lower_idx") {
			return models.User{}, ErrUsernameTaken
		}
		return models.User{}, err
	}
	return user, nil
}

func (s *Store) UpdateUser(ctx context.Context, id string, req models.UpdateUserRequest) (models.User, error) {
	var user models.User
	var err error
	if strings.TrimSpace(req.Password) == "" {
		user, err = scanUser(s.pool.QueryRow(ctx, `
UPDATE users
SET display_name = $1, username = $2, role = $3, status = $4, updated_at = now()
WHERE id = $5
RETURNING id::text, display_name, username, password_hash, role, status, created_at
`, req.DisplayName, req.Username, req.Role, req.Status, id))
	} else {
		hash, hashErr := auth.HashPassword(req.Password)
		if hashErr != nil {
			return models.User{}, hashErr
		}
		user, err = scanUser(s.pool.QueryRow(ctx, `
UPDATE users
SET display_name = $1, username = $2, password_hash = $3, role = $4, status = $5, updated_at = now()
WHERE id = $6
RETURNING id::text, display_name, username, password_hash, role, status, created_at
`, req.DisplayName, req.Username, hash, req.Role, req.Status, id))
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return models.User{}, ErrUserNotFound
	}
	if err != nil {
		if strings.Contains(err.Error(), "users_username_lower_idx") {
			return models.User{}, ErrUsernameTaken
		}
		return models.User{}, err
	}
	return user, nil
}

func (s *Store) CountActiveAdmins(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'admin' AND status = 'active'`).Scan(&count)
	return count, err
}

func (s *Store) WriteAudit(ctx context.Context, actorUserID, action, targetType, targetID, ipAddress, userAgent string) error {
	var actor any
	if actorUserID != "" {
		actor = actorUserID
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5, $6)
`, actor, action, targetType, targetID, ipAddress, userAgent)
	return err
}

func (s *Store) WriteAuditWithMetadata(ctx context.Context, actorUserID, action, targetType, targetID, ipAddress, userAgent string, metadata map[string]any) error {
	var actor any
	if actorUserID != "" {
		actor = actorUserID
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, ip_address, user_agent, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
`, actor, action, targetType, targetID, ipAddress, userAgent, string(metadataJSON))
	return err
}

func (s *Store) ClaimIdempotencyKey(ctx context.Context, scope, key, actorUserID string) (IdempotencyClaim, error) {
	scope = strings.TrimSpace(scope)
	key = strings.TrimSpace(key)
	if scope == "" || key == "" {
		return IdempotencyClaim{Claimed: true}, nil
	}
	if len(key) > 160 {
		key = key[:160]
	}

	var actor any
	if actorUserID != "" {
		actor = actorUserID
	}
	tag, err := s.pool.Exec(ctx, `
INSERT INTO idempotency_keys (scope, key, actor_user_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING
`, scope, key, actor)
	if err != nil {
		return IdempotencyClaim{}, err
	}
	if tag.RowsAffected() == 1 {
		return IdempotencyClaim{Claimed: true}, nil
	}

	var status int
	var body []byte
	err = s.pool.QueryRow(ctx, `
SELECT response_status, response_body
FROM idempotency_keys
WHERE scope = $1 AND key = $2 AND COALESCE(actor_user_id::text, '') = COALESCE(NULLIF($3, '')::uuid::text, '')
`, scope, key, actorUserID).Scan(&status, &body)
	if err != nil {
		return IdempotencyClaim{}, err
	}
	if status == 0 {
		return IdempotencyClaim{}, ErrIdempotencyInProgress
	}
	return IdempotencyClaim{ResponseCode: status, Response: json.RawMessage(body)}, nil
}

func (s *Store) CompleteIdempotencyKey(ctx context.Context, scope, key, actorUserID string, status int, body any) error {
	scope = strings.TrimSpace(scope)
	key = strings.TrimSpace(key)
	if scope == "" || key == "" {
		return nil
	}
	if len(key) > 160 {
		key = key[:160]
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
UPDATE idempotency_keys
SET response_status = $4, response_body = $5::jsonb
WHERE scope = $1 AND key = $2 AND COALESCE(actor_user_id::text, '') = COALESCE(NULLIF($3, '')::uuid::text, '')
`, scope, key, actorUserID, status, string(data))
	return err
}

func (s *Store) ReleaseIdempotencyKey(ctx context.Context, scope, key, actorUserID string) error {
	scope = strings.TrimSpace(scope)
	key = strings.TrimSpace(key)
	if scope == "" || key == "" {
		return nil
	}
	if len(key) > 160 {
		key = key[:160]
	}
	_, err := s.pool.Exec(ctx, `
DELETE FROM idempotency_keys
WHERE scope = $1 AND key = $2 AND response_status = 0 AND COALESCE(actor_user_id::text, '') = COALESCE(NULLIF($3, '')::uuid::text, '')
`, scope, key, actorUserID)
	return err
}

func (s *Store) ListDocumentConfigSteps(ctx context.Context, screenCode, docFormatCode string) ([]models.DocumentConfigStep, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id::text, screen_code, doc_format_code, position_code, position_name, user01, user02, user03,
       sequence_no, condition_type, created_at, updated_at
FROM document_config_steps
WHERE ($1 = '' OR screen_code = $1)
  AND ($2 = '' OR lower(doc_format_code) = lower($2))
ORDER BY screen_code, lower(doc_format_code), sequence_no, position_code
`, screenCode, docFormatCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	steps := []models.DocumentConfigStep{}
	for rows.Next() {
		step, err := scanDocumentConfigStep(rows)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func (s *Store) FindDocumentConfigStepByID(ctx context.Context, id string) (models.DocumentConfigStep, error) {
	step, err := scanDocumentConfigStep(s.pool.QueryRow(ctx, `
SELECT id::text, screen_code, doc_format_code, position_code, position_name, user01, user02, user03,
       sequence_no, condition_type, created_at, updated_at
FROM document_config_steps
WHERE id = $1
`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.DocumentConfigStep{}, ErrDocumentConfigNotFound
	}
	return step, err
}

func (s *Store) ListDocumentConfigUserReferences(ctx context.Context, username string) ([]models.DocumentConfigStep, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id::text, screen_code, doc_format_code, position_code, position_name, user01, user02, user03,
       sequence_no, condition_type, created_at, updated_at
FROM document_config_steps
WHERE lower(split_part(user01, ':', 1)) = lower($1)
   OR lower(split_part(user02, ':', 1)) = lower($1)
   OR lower(split_part(user03, ':', 1)) = lower($1)
ORDER BY screen_code, lower(doc_format_code), sequence_no, position_code
`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	steps := []models.DocumentConfigStep{}
	for rows.Next() {
		step, err := scanDocumentConfigStep(rows)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func (s *Store) CreateDocumentConfigStep(ctx context.Context, req models.DocumentConfigStepRequest) (models.DocumentConfigStep, error) {
	step, err := scanDocumentConfigStep(s.pool.QueryRow(ctx, `
INSERT INTO document_config_steps (
    screen_code, doc_format_code, position_code, position_name, user01, user02, user03,
    sequence_no, condition_type
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id::text, screen_code, doc_format_code, position_code, position_name, user01, user02, user03,
          sequence_no, condition_type, created_at, updated_at
`, req.ScreenCode, req.DocFormatCode, req.PositionCode, req.PositionName, req.User01, req.User02, req.User03, req.SequenceNo, req.ConditionType))
	if err != nil {
		if strings.Contains(err.Error(), "document_config_steps_unique_position_idx") {
			return models.DocumentConfigStep{}, ErrDocumentConfigDuplicate
		}
		return models.DocumentConfigStep{}, err
	}
	return step, nil
}

func (s *Store) UpdateDocumentConfigStep(ctx context.Context, id string, req models.DocumentConfigStepRequest) (models.DocumentConfigStep, error) {
	step, err := scanDocumentConfigStep(s.pool.QueryRow(ctx, `
UPDATE document_config_steps
SET screen_code = $1,
    doc_format_code = $2,
    position_code = $3,
    position_name = $4,
    user01 = $5,
    user02 = $6,
    user03 = $7,
    sequence_no = $8,
    condition_type = $9,
    updated_at = now()
WHERE id = $10
RETURNING id::text, screen_code, doc_format_code, position_code, position_name, user01, user02, user03,
          sequence_no, condition_type, created_at, updated_at
`, req.ScreenCode, req.DocFormatCode, req.PositionCode, req.PositionName, req.User01, req.User02, req.User03, req.SequenceNo, req.ConditionType, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.DocumentConfigStep{}, ErrDocumentConfigNotFound
	}
	if err != nil {
		if strings.Contains(err.Error(), "document_config_steps_unique_position_idx") {
			return models.DocumentConfigStep{}, ErrDocumentConfigDuplicate
		}
		return models.DocumentConfigStep{}, err
	}
	return step, nil
}

func (s *Store) DeleteDocumentConfigStep(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM document_config_steps WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDocumentConfigNotFound
	}
	return nil
}

func (s *Store) CountSignatureTemplateBoxesForConfig(ctx context.Context, screenCode, docFormatCode, positionCode string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
SELECT count(*)
FROM signature_template_boxes b
JOIN signature_templates t ON t.id = b.template_id
WHERE t.status IN ('draft', 'active')
  AND t.screen_code = $1
  AND lower(t.doc_format_code) = lower($2)
  AND lower(b.position_code) = lower($3)
`, screenCode, docFormatCode, positionCode).Scan(&count)
	return count, err
}

func (s *Store) CreateUploadedFile(ctx context.Context, file models.UploadedFile) (models.UploadedFile, error) {
	return scanUploadedFile(s.pool.QueryRow(ctx, `
INSERT INTO uploaded_files (original_name, stored_name, storage_path, content_type, size_bytes, page_count, sha256, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::uuid)
RETURNING id::text, original_name, stored_name, storage_path, content_type, size_bytes, page_count, sha256, COALESCE(created_by::text, ''), created_at
`, file.OriginalName, file.StoredName, file.StoragePath, file.ContentType, file.SizeBytes, file.PageCount, file.SHA256, file.CreatedBy))
}

func (s *Store) FindUploadedFileByID(ctx context.Context, id string) (models.UploadedFile, error) {
	return scanUploadedFile(s.pool.QueryRow(ctx, `
SELECT id::text, original_name, stored_name, storage_path, content_type, size_bytes, page_count, sha256, COALESCE(created_by::text, ''), created_at
FROM uploaded_files
WHERE id = $1
`, id))
}

func (s *Store) GetSignatureTemplateState(ctx context.Context, screenCode, docFormatCode string) (*models.SignatureTemplate, *models.SignatureTemplate, error) {
	rows, err := s.pool.Query(ctx, `
SELECT t.id::text, t.screen_code, t.doc_format_code, t.version, t.status, COALESCE(t.sample_file_id::text, ''),
       t.revision, COALESCE(t.created_by::text, ''), COALESCE(t.published_by::text, ''),
       t.created_at, t.updated_at, t.published_at,
       COALESCE(f.id::text, ''), COALESCE(f.original_name, ''), COALESCE(f.stored_name, ''), COALESCE(f.storage_path, ''),
       COALESCE(f.content_type, ''), COALESCE(f.size_bytes, 0), COALESCE(f.page_count, 0),
       COALESCE(f.sha256, ''), COALESCE(f.created_by::text, ''), f.created_at
FROM signature_templates t
LEFT JOIN uploaded_files f ON f.id = t.sample_file_id
WHERE t.screen_code = $1
  AND lower(t.doc_format_code) = lower($2)
  AND t.status IN ('draft', 'active')
ORDER BY CASE t.status WHEN 'draft' THEN 0 ELSE 1 END, t.version DESC
`, screenCode, docFormatCode)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var draft *models.SignatureTemplate
	var active *models.SignatureTemplate
	for rows.Next() {
		template, err := scanSignatureTemplateWithFile(rows)
		if err != nil {
			return nil, nil, err
		}
		boxes, err := s.ListSignatureTemplateBoxes(ctx, template.ID)
		if err != nil {
			return nil, nil, err
		}
		template.Boxes = boxes
		if template.Status == "draft" && draft == nil {
			copy := template
			draft = &copy
		}
		if template.Status == "active" && active == nil {
			copy := template
			active = &copy
		}
	}
	return draft, active, rows.Err()
}

func (s *Store) FindSignatureTemplateByID(ctx context.Context, id string) (models.SignatureTemplate, error) {
	template, err := scanSignatureTemplateWithFile(s.pool.QueryRow(ctx, `
SELECT t.id::text, t.screen_code, t.doc_format_code, t.version, t.status, COALESCE(t.sample_file_id::text, ''),
       t.revision, COALESCE(t.created_by::text, ''), COALESCE(t.published_by::text, ''),
       t.created_at, t.updated_at, t.published_at,
       COALESCE(f.id::text, ''), COALESCE(f.original_name, ''), COALESCE(f.stored_name, ''), COALESCE(f.storage_path, ''),
       COALESCE(f.content_type, ''), COALESCE(f.size_bytes, 0), COALESCE(f.page_count, 0),
       COALESCE(f.sha256, ''), COALESCE(f.created_by::text, ''), f.created_at
FROM signature_templates t
LEFT JOIN uploaded_files f ON f.id = t.sample_file_id
WHERE t.id = $1
`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return models.SignatureTemplate{}, ErrSignatureTemplateNotFound
	}
	if err != nil {
		return models.SignatureTemplate{}, err
	}
	boxes, err := s.ListSignatureTemplateBoxes(ctx, template.ID)
	if err != nil {
		return models.SignatureTemplate{}, err
	}
	template.Boxes = boxes
	return template, nil
}

func (s *Store) UpsertActiveSignatureTemplateSample(ctx context.Context, screenCode, docFormatCode, uploadedFileID, actorUserID string) (models.SignatureTemplate, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.SignatureTemplate{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var templateID string
	err = tx.QueryRow(ctx, `
SELECT id::text
FROM signature_templates
WHERE screen_code = $1 AND lower(doc_format_code) = lower($2) AND status = 'active'
`, screenCode, docFormatCode).Scan(&templateID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.QueryRow(ctx, `
WITH existing_draft AS (
    SELECT id
    FROM signature_templates
    WHERE screen_code = $1 AND lower(doc_format_code) = lower($2) AND status = 'draft'
    ORDER BY version DESC
    LIMIT 1
),
updated_draft AS (
    UPDATE signature_templates
    SET status = 'active',
        sample_file_id = $3,
        revision = revision + 1,
        published_by = NULLIF($4, '')::uuid,
        published_at = now(),
        updated_at = now()
    WHERE id = (SELECT id FROM existing_draft)
    RETURNING id::text
),
created AS (
    INSERT INTO signature_templates (screen_code, doc_format_code, version, status, sample_file_id, created_by, published_by, published_at)
    SELECT $1, $2, 1, 'active', $3, NULLIF($4, '')::uuid, NULLIF($4, '')::uuid, now()
    WHERE NOT EXISTS (SELECT 1 FROM updated_draft)
    RETURNING id::text
)
SELECT id FROM updated_draft
UNION ALL
SELECT id FROM created
LIMIT 1
`, screenCode, docFormatCode, uploadedFileID, actorUserID).Scan(&templateID); err != nil {
			return models.SignatureTemplate{}, err
		}
	} else if err != nil {
		return models.SignatureTemplate{}, err
	} else {
		if _, err := tx.Exec(ctx, `
UPDATE signature_templates
SET sample_file_id = $1,
    revision = revision + 1,
    published_by = NULLIF($2, '')::uuid,
    published_at = now(),
    updated_at = now()
WHERE id = $3 AND status = 'active'
`, uploadedFileID, actorUserID, templateID); err != nil {
			return models.SignatureTemplate{}, err
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM signature_template_boxes WHERE template_id = $1`, templateID); err != nil {
		return models.SignatureTemplate{}, err
	}

	if _, err := tx.Exec(ctx, `
DELETE FROM signature_templates
WHERE screen_code = $1
  AND lower(doc_format_code) = lower($2)
  AND status = 'draft'
  AND id <> $3
`, screenCode, docFormatCode, templateID); err != nil {
		return models.SignatureTemplate{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return models.SignatureTemplate{}, err
	}
	return s.FindSignatureTemplateByID(ctx, templateID)
}

func (s *Store) ListSignatureTemplateBoxes(ctx context.Context, templateID string) ([]models.SignatureTemplateBox, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id::text, template_id::text, position_code, signer_slot, signer_type, signer_user, page_no,
       x_ratio, y_ratio, width_ratio, height_ratio, label, created_at
FROM signature_template_boxes
WHERE template_id = $1
ORDER BY page_no, lower(position_code), signer_slot
`, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	boxes := []models.SignatureTemplateBox{}
	for rows.Next() {
		box, err := scanSignatureTemplateBox(rows)
		if err != nil {
			return nil, err
		}
		boxes = append(boxes, box)
	}
	return boxes, rows.Err()
}

func (s *Store) ReplaceSignatureTemplateBoxes(ctx context.Context, templateID string, revision int, boxes []models.SignatureTemplateBoxRequest) (models.SignatureTemplate, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.SignatureTemplate{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var currentRevision int
	if err := tx.QueryRow(ctx, `SELECT status, revision FROM signature_templates WHERE id = $1`, templateID).Scan(&status, &currentRevision); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.SignatureTemplate{}, ErrSignatureTemplateNotFound
		}
		return models.SignatureTemplate{}, err
	}
	if status == "archived" {
		return models.SignatureTemplate{}, ErrSignatureTemplateArchived
	}
	if currentRevision != revision {
		return models.SignatureTemplate{}, ErrSignatureRevisionConflict
	}

	if _, err := tx.Exec(ctx, `DELETE FROM signature_template_boxes WHERE template_id = $1`, templateID); err != nil {
		return models.SignatureTemplate{}, err
	}
	for _, box := range boxes {
		if _, err := tx.Exec(ctx, `
INSERT INTO signature_template_boxes (
    template_id, position_code, signer_slot, signer_type, signer_user, page_no,
    x_ratio, y_ratio, width_ratio, height_ratio, label
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
`, templateID, box.PositionCode, box.SignerSlot, box.SignerType, box.SignerUser, box.PageNo, box.XRatio, box.YRatio, box.WidthRatio, box.HeightRatio, box.Label); err != nil {
			return models.SignatureTemplate{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
UPDATE signature_templates
SET revision = revision + 1, updated_at = now()
WHERE id = $1
`, templateID); err != nil {
		return models.SignatureTemplate{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return models.SignatureTemplate{}, err
	}
	return s.FindSignatureTemplateByID(ctx, templateID)
}

func (s *Store) PublishSignatureTemplate(ctx context.Context, templateID, actorUserID string) (models.SignatureTemplate, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.SignatureTemplate{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var screenCode, docFormatCode, status string
	if err := tx.QueryRow(ctx, `
SELECT screen_code, doc_format_code, status
FROM signature_templates
WHERE id = $1
`, templateID).Scan(&screenCode, &docFormatCode, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.SignatureTemplate{}, ErrSignatureTemplateNotFound
		}
		return models.SignatureTemplate{}, err
	}
	if status != "draft" {
		return models.SignatureTemplate{}, ErrSignatureTemplateNotDraft
	}

	if _, err := tx.Exec(ctx, `
UPDATE signature_templates
SET status = 'archived', updated_at = now()
WHERE screen_code = $1
  AND lower(doc_format_code) = lower($2)
  AND status = 'active'
`, screenCode, docFormatCode); err != nil {
		return models.SignatureTemplate{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE signature_templates
SET status = 'active',
    revision = revision + 1,
    published_by = NULLIF($2, '')::uuid,
    published_at = now(),
    updated_at = now()
WHERE id = $1
`, templateID, actorUserID); err != nil {
		return models.SignatureTemplate{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return models.SignatureTemplate{}, err
	}
	return s.FindSignatureTemplateByID(ctx, templateID)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (models.User, error) {
	var user models.User
	err := row.Scan(
		&user.ID,
		&user.DisplayName,
		&user.Username,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.CreatedAt,
	)
	return user, err
}

func scanDocumentConfigStep(row rowScanner) (models.DocumentConfigStep, error) {
	var step models.DocumentConfigStep
	err := row.Scan(
		&step.ID,
		&step.ScreenCode,
		&step.DocFormatCode,
		&step.PositionCode,
		&step.PositionName,
		&step.User01,
		&step.User02,
		&step.User03,
		&step.SequenceNo,
		&step.ConditionType,
		&step.CreatedAt,
		&step.UpdatedAt,
	)
	return step, err
}

func scanUploadedFile(row rowScanner) (models.UploadedFile, error) {
	var file models.UploadedFile
	err := row.Scan(
		&file.ID,
		&file.OriginalName,
		&file.StoredName,
		&file.StoragePath,
		&file.ContentType,
		&file.SizeBytes,
		&file.PageCount,
		&file.SHA256,
		&file.CreatedBy,
		&file.CreatedAt,
	)
	return file, err
}

func scanSignatureTemplateWithFile(row rowScanner) (models.SignatureTemplate, error) {
	var template models.SignatureTemplate
	var publishedAt sql.NullTime
	var fileID, fileOriginalName, fileStoredName, fileStoragePath, fileContentType, fileSHA256, fileCreatedBy string
	var fileSize int64
	var filePageCount int
	var fileCreatedAt sql.NullTime
	err := row.Scan(
		&template.ID,
		&template.ScreenCode,
		&template.DocFormatCode,
		&template.Version,
		&template.Status,
		&template.SampleFileID,
		&template.Revision,
		&template.CreatedBy,
		&template.PublishedBy,
		&template.CreatedAt,
		&template.UpdatedAt,
		&publishedAt,
		&fileID,
		&fileOriginalName,
		&fileStoredName,
		&fileStoragePath,
		&fileContentType,
		&fileSize,
		&filePageCount,
		&fileSHA256,
		&fileCreatedBy,
		&fileCreatedAt,
	)
	if err != nil {
		return template, err
	}
	if publishedAt.Valid {
		template.PublishedAt = &publishedAt.Time
	}
	if fileID != "" {
		template.SampleFile = &models.UploadedFile{
			ID:           fileID,
			OriginalName: fileOriginalName,
			StoredName:   fileStoredName,
			StoragePath:  fileStoragePath,
			ContentType:  fileContentType,
			SizeBytes:    fileSize,
			PageCount:    filePageCount,
			SHA256:       fileSHA256,
			CreatedBy:    fileCreatedBy,
		}
		if fileCreatedAt.Valid {
			template.SampleFile.CreatedAt = fileCreatedAt.Time
		}
	}
	return template, nil
}

func scanSignatureTemplateBox(row rowScanner) (models.SignatureTemplateBox, error) {
	var box models.SignatureTemplateBox
	err := row.Scan(
		&box.ID,
		&box.TemplateID,
		&box.PositionCode,
		&box.SignerSlot,
		&box.SignerType,
		&box.SignerUser,
		&box.PageNo,
		&box.XRatio,
		&box.YRatio,
		&box.WidthRatio,
		&box.HeightRatio,
		&box.Label,
		&box.CreatedAt,
	)
	return box, err
}
