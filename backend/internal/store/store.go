package store

import (
	"context"
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
	ErrUsernameTaken           = errors.New("username already exists")
	ErrUserNotFound            = errors.New("user not found")
	ErrDocumentConfigDuplicate = errors.New("document config step already exists")
	ErrDocumentConfigNotFound  = errors.New("document config step not found")
)

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
