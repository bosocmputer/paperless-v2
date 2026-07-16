package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

type SavedSignatureUpsertInput struct {
	UserID        string
	SMLTenant     string
	FileID        string
	SMLUserCode   string
	SourceVersion string
}

func (s *Store) FindUsersByUsernames(ctx context.Context, usernames []string) (map[string]models.User, error) {
	keys := normalizedUsernameKeys(usernames)
	if len(keys) == 0 {
		return map[string]models.User{}, nil
	}
	rows, err := s.pool.Query(ctx, `
SELECT id::text, display_name, username, password_hash, role, status, created_at
FROM users
WHERE lower(trim(username)) = ANY($1::text[])
`, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]models.User, len(keys))
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out[strings.ToLower(strings.TrimSpace(user.Username))] = user
	}
	return out, rows.Err()
}

func (s *Store) ListSavedSignaturesByUsernames(ctx context.Context, tenant string, usernames []string) (map[string]models.UserSavedSignature, error) {
	tenant = NormalizeSMLTenant(tenant)
	keys := normalizedUsernameKeys(usernames)
	if len(keys) == 0 {
		return map[string]models.UserSavedSignature{}, nil
	}
	rows, err := s.pool.Query(ctx, `
SELECT lower(trim(u.username)), us.user_id::text, us.sml_tenant, COALESCE(us.file_id::text,''),
       us.sml_user_code, us.source_version, us.synced_at, us.last_error,
       COALESCE(f.id::text,''), COALESCE(f.original_name,''), COALESCE(f.stored_name,''),
       COALESCE(f.storage_path,''), COALESCE(f.content_type,''), COALESCE(f.size_bytes,0),
       COALESCE(f.page_count,0), COALESCE(f.sha256,''), COALESCE(f.created_by::text,''), f.created_at
FROM user_saved_signatures us
JOIN users u ON u.id = us.user_id
LEFT JOIN uploaded_files f ON f.id = us.file_id
WHERE us.sml_tenant = $1
  AND lower(trim(u.username)) = ANY($2::text[])
`, tenant, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]models.UserSavedSignature, len(keys))
	for rows.Next() {
		var key string
		var item models.UserSavedSignature
		var syncedAt *time.Time
		var fileCreatedAt *time.Time
		if err := rows.Scan(&key, &item.UserID, &item.SMLTenant, &item.FileID, &item.SMLUserCode,
			&item.SourceVersion, &syncedAt, &item.LastError, &item.File.ID, &item.File.OriginalName,
			&item.File.StoredName, &item.File.StoragePath, &item.File.ContentType, &item.File.SizeBytes,
			&item.File.PageCount, &item.File.SHA256, &item.File.CreatedBy, &fileCreatedAt); err != nil {
			return nil, err
		}
		item.SyncedAt = syncedAt
		if fileCreatedAt != nil {
			item.File.CreatedAt = *fileCreatedAt
		}
		out[strings.ToLower(strings.TrimSpace(key))] = item
	}
	return out, rows.Err()
}

func (s *Store) FindUserSavedSignature(ctx context.Context, userID, tenant string) (models.UserSavedSignature, error) {
	tenant = NormalizeSMLTenant(tenant)
	var item models.UserSavedSignature
	var syncedAt *time.Time
	var fileCreatedAt *time.Time
	err := s.pool.QueryRow(ctx, `
SELECT us.user_id::text, us.sml_tenant, COALESCE(us.file_id::text,''), us.sml_user_code,
       us.source_version, us.synced_at, us.last_error,
       COALESCE(f.id::text,''), COALESCE(f.original_name,''), COALESCE(f.stored_name,''),
       COALESCE(f.storage_path,''), COALESCE(f.content_type,''), COALESCE(f.size_bytes,0),
       COALESCE(f.page_count,0), COALESCE(f.sha256,''), COALESCE(f.created_by::text,''), f.created_at
FROM user_saved_signatures us
LEFT JOIN uploaded_files f ON f.id = us.file_id
WHERE us.user_id = NULLIF($1,'')::uuid AND us.sml_tenant = $2
`, userID, tenant).Scan(&item.UserID, &item.SMLTenant, &item.FileID, &item.SMLUserCode,
		&item.SourceVersion, &syncedAt, &item.LastError, &item.File.ID, &item.File.OriginalName,
		&item.File.StoredName, &item.File.StoragePath, &item.File.ContentType, &item.File.SizeBytes,
		&item.File.PageCount, &item.File.SHA256, &item.File.CreatedBy, &fileCreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.UserSavedSignature{}, ErrSavedSignatureUnavailable
	}
	if err != nil {
		return models.UserSavedSignature{}, err
	}
	item.SyncedAt = syncedAt
	if fileCreatedAt != nil {
		item.File.CreatedAt = *fileCreatedAt
	}
	return item, nil
}

func (s *Store) UpsertUserSavedSignature(ctx context.Context, input SavedSignatureUpsertInput) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tenant := NormalizeSMLTenant(input.SMLTenant)
	var oldFileID string
	err = tx.QueryRow(ctx, `
SELECT COALESCE(file_id::text,'')
FROM user_saved_signatures
WHERE user_id = NULLIF($1,'')::uuid AND sml_tenant = $2
FOR UPDATE
`, input.UserID, tenant).Scan(&oldFileID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO user_saved_signatures (user_id, sml_tenant, file_id, sml_user_code, source_version, synced_at, last_error)
VALUES (NULLIF($1,'')::uuid, $2, NULLIF($3,'')::uuid, $4, $5, now(), '')
ON CONFLICT (user_id, sml_tenant) DO UPDATE
SET file_id = EXCLUDED.file_id,
    sml_user_code = EXCLUDED.sml_user_code,
    source_version = EXCLUDED.source_version,
    synced_at = now(),
    last_error = '',
    updated_at = now()
`, input.UserID, tenant, input.FileID, strings.TrimSpace(input.SMLUserCode), strings.TrimSpace(input.SourceVersion)); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return oldFileID, nil
}

func (s *Store) RecordUserSavedSignatureError(ctx context.Context, userID, tenant, smlUserCode, issue string) error {
	tenant = NormalizeSMLTenant(tenant)
	issue = strings.TrimSpace(issue)
	if len(issue) > 160 {
		issue = issue[:160]
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO user_saved_signatures (user_id, sml_tenant, file_id, sml_user_code, source_version, synced_at, last_error)
VALUES (NULLIF($1,'')::uuid, $2, NULL, $3, '', NULL, $4)
ON CONFLICT (user_id, sml_tenant) DO UPDATE
SET sml_user_code = EXCLUDED.sml_user_code,
    last_error = EXCLUDED.last_error,
    updated_at = now()
`, userID, tenant, strings.TrimSpace(smlUserCode), issue)
	return err
}

func (s *Store) TryAdvisoryLock(ctx context.Context, key string) (func(), bool, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, key).Scan(&locked); err != nil {
		conn.Release()
		return nil, false, err
	}
	if !locked {
		conn.Release()
		return nil, false, nil
	}
	release := func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = conn.Exec(releaseCtx, `SELECT pg_advisory_unlock(hashtext($1))`, key)
		conn.Release()
	}
	return release, true, nil
}

func normalizedUsernameKeys(usernames []string) []string {
	out := make([]string, 0, len(usernames))
	seen := map[string]bool{}
	for _, username := range usernames {
		key := strings.ToLower(strings.TrimSpace(username))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}
