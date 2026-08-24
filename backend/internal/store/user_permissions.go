package store

import (
	"context"
	"errors"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

// grantableMenuKeys is the canonical, hardcoded list of menu keys a
// superadmin can grant/revoke per user - mirrors how validateUserFields
// hardcodes the role/status enum in Go rather than a DB lookup table.
var grantableMenuKeys = []string{
	"signing-document-drafts",
	"internal-document-create",
	"signing-documents",
	"signing-document-history",
	"admin-my-signing-tasks",
	"admin-my-signing-history",
	"admin-guide",
	"admin-user-guide",
}

func isGrantableMenuKey(key string) bool {
	for _, k := range grantableMenuKeys {
		if k == key {
			return true
		}
	}
	return false
}

// DefaultGrantedMenuKeys returns a fresh copy of the full grantable-key
// list - the set an unconfigured (no-row) user is treated as having.
func DefaultGrantedMenuKeys() []string {
	out := make([]string, len(grantableMenuKeys))
	copy(out, grantableMenuKeys)
	return out
}

// GetUserMenuPermissions returns the stored row for a user, or a
// default-permissive zero-value (Configured=false, every grantable key,
// DocumentScope="all") when the user has no row yet - absence of a row
// must mean "unrestricted, exactly like today," never "sees nothing."
func (s *Store) GetUserMenuPermissions(ctx context.Context, userID string) (models.UserMenuPermissions, error) {
	row := s.pool.QueryRow(ctx, `
SELECT menu_keys, document_scope, updated_at
FROM user_menu_permissions
WHERE user_id = $1
`, userID)
	var perm models.UserMenuPermissions
	perm.UserID = userID
	err := row.Scan(&perm.MenuKeys, &perm.DocumentScope, &perm.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.UserMenuPermissions{
			UserID:        userID,
			MenuKeys:      DefaultGrantedMenuKeys(),
			DocumentScope: "all",
			Configured:    false,
		}, nil
	}
	if err != nil {
		return models.UserMenuPermissions{}, err
	}
	perm.Configured = true
	return perm, nil
}

// ListUserMenuPermissions returns every user's EXPLICIT permission row
// (only users a superadmin has actually configured). Callers merge in
// per-user defaults for any user not present in the returned map, same
// as GetUserMenuPermissions's single-row default fallback.
func (s *Store) ListUserMenuPermissions(ctx context.Context) (map[string]models.UserMenuPermissions, error) {
	rows, err := s.pool.Query(ctx, `
SELECT user_id::text, menu_keys, document_scope, updated_at
FROM user_menu_permissions
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]models.UserMenuPermissions{}
	for rows.Next() {
		var perm models.UserMenuPermissions
		if err := rows.Scan(&perm.UserID, &perm.MenuKeys, &perm.DocumentScope, &perm.UpdatedAt); err != nil {
			return nil, err
		}
		perm.Configured = true
		out[perm.UserID] = perm
	}
	return out, rows.Err()
}

// UpsertUserMenuPermissions replaces the full grant set for one user.
// expectedUpdatedAt implements optimistic concurrency: nil means the
// caller believes this is the first-ever save (no row existed yet); a
// non-nil value must match the row's current updated_at or the save is
// rejected with ErrPermissionRevisionConflict, so two superadmins saving
// the same user around the same time cannot silently clobber each other.
func (s *Store) UpsertUserMenuPermissions(ctx context.Context, userID, updatedByUserID string, req models.UpdateUserMenuPermissionsRequest) (models.UserMenuPermissions, error) {
	if req.MenuKeys == nil {
		// A nil slice (e.g. the caller omitted "menuKeys" entirely) would
		// bind to SQL NULL, violating menu_keys' NOT NULL constraint with a
		// raw DB error instead of a clean validation error - normalize to
		// an explicit empty grant instead.
		req.MenuKeys = []string{}
	}
	for _, key := range req.MenuKeys {
		if !isGrantableMenuKey(key) {
			return models.UserMenuPermissions{}, ErrInvalidMenuPermissionKey
		}
	}
	if req.DocumentScope != "all" && req.DocumentScope != "own" {
		return models.UserMenuPermissions{}, ErrInvalidDocumentScope
	}

	var perm models.UserMenuPermissions
	perm.UserID = userID

	if req.ExpectedUpdatedAt == nil {
		row := s.pool.QueryRow(ctx, `
INSERT INTO user_menu_permissions (user_id, menu_keys, document_scope, updated_by, updated_at)
VALUES ($1, $2, $3, NULLIF($4,'')::uuid, now())
ON CONFLICT (user_id) DO NOTHING
RETURNING menu_keys, document_scope, updated_at
`, userID, req.MenuKeys, req.DocumentScope, updatedByUserID)
		if err := row.Scan(&perm.MenuKeys, &perm.DocumentScope, &perm.UpdatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return models.UserMenuPermissions{}, ErrPermissionRevisionConflict
			}
			return models.UserMenuPermissions{}, err
		}
		perm.Configured = true
		return perm, nil
	}

	row := s.pool.QueryRow(ctx, `
UPDATE user_menu_permissions
SET menu_keys = $2, document_scope = $3, updated_by = NULLIF($4,'')::uuid, updated_at = now()
WHERE user_id = $1 AND updated_at = $5
RETURNING menu_keys, document_scope, updated_at
`, userID, req.MenuKeys, req.DocumentScope, updatedByUserID, *req.ExpectedUpdatedAt)
	if err := row.Scan(&perm.MenuKeys, &perm.DocumentScope, &perm.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.UserMenuPermissions{}, ErrPermissionRevisionConflict
		}
		return models.UserMenuPermissions{}, err
	}
	perm.Configured = true
	return perm, nil
}

var (
	ErrInvalidMenuPermissionKey = errors.New("invalid menu permission key")
	ErrInvalidDocumentScope     = errors.New("invalid document scope")
)
