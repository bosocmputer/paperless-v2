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
