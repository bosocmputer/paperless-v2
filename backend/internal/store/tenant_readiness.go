package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

func NormalizeSMLRegistryScope(provider, dataGroup, tenant string) models.SMLTenantReadinessRegistryKey {
	return models.SMLTenantReadinessRegistryKey{
		Provider:  strings.ToLower(strings.TrimSpace(provider)),
		DataGroup: strings.ToLower(strings.TrimSpace(dataGroup)),
		Tenant:    NormalizeSMLTenant(tenant),
	}
}

func (s *Store) ListSMLTenantReadiness(
	ctx context.Context,
	provider, dataGroup string,
	tenants []string,
) (map[string]models.SMLTenantReadinessRegistryEntry, error) {
	scope := NormalizeSMLRegistryScope(provider, dataGroup, DefaultSMLTenant)
	normalizedTenants := normalizeTenantList(tenants)
	if len(normalizedTenants) == 0 {
		return map[string]models.SMLTenantReadinessRegistryEntry{}, nil
	}

	rows, err := s.pool.Query(ctx, `
SELECT provider, data_group, sml_tenant, status, result, verification_version, verified_at, updated_at
FROM sml_tenant_readiness_registry
WHERE provider = $1
  AND data_group = $2
  AND sml_tenant = ANY($3)
`, scope.Provider, scope.DataGroup, normalizedTenants)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]models.SMLTenantReadinessRegistryEntry, len(normalizedTenants))
	for rows.Next() {
		entry, scanErr := scanSMLTenantReadinessEntry(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result[entry.Tenant] = entry
	}
	return result, rows.Err()
}

func (s *Store) GetSMLTenantReadiness(
	ctx context.Context,
	provider, dataGroup, tenant string,
) (models.SMLTenantReadinessRegistryEntry, bool, error) {
	scope := NormalizeSMLRegistryScope(provider, dataGroup, tenant)
	entry, err := scanSMLTenantReadinessEntry(s.pool.QueryRow(ctx, `
SELECT provider, data_group, sml_tenant, status, result, verification_version, verified_at, updated_at
FROM sml_tenant_readiness_registry
WHERE provider = $1 AND data_group = $2 AND sml_tenant = $3
`, scope.Provider, scope.DataGroup, scope.Tenant))
	if err == pgx.ErrNoRows {
		return models.SMLTenantReadinessRegistryEntry{}, false, nil
	}
	if err != nil {
		return models.SMLTenantReadinessRegistryEntry{}, false, err
	}
	return entry, true, nil
}

func (s *Store) MarkSMLTenantReadinessChecking(
	ctx context.Context,
	provider, dataGroup, tenant string,
	verificationVersion int,
) error {
	scope := NormalizeSMLRegistryScope(provider, dataGroup, tenant)
	readiness := models.SMLTenantReadiness{
		OK:                  false,
		Status:              "checking",
		Message:             "กำลังตรวจสอบความพร้อมของฐานข้อมูล",
		Tenant:              scope.Tenant,
		ImageDatabase:       scope.Tenant + "_images",
		RegistryStatus:      "checking",
		IsChecking:          true,
		Source:              "registry",
		VerificationVersion: verificationVersion,
	}
	payload, err := json.Marshal(readiness)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO sml_tenant_readiness_registry (
    provider, data_group, sml_tenant, status, result, verification_version, verified_at, updated_at
) VALUES ($1, $2, $3, 'checking', $4, $5, NULL, now())
ON CONFLICT (provider, data_group, sml_tenant) DO UPDATE
SET status = 'checking',
    result = EXCLUDED.result,
    verification_version = EXCLUDED.verification_version,
    verified_at = NULL,
    updated_at = now()
`, scope.Provider, scope.DataGroup, scope.Tenant, payload, verificationVersion)
	return err
}

func (s *Store) SaveSMLTenantReadiness(
	ctx context.Context,
	provider, dataGroup, tenant string,
	readiness models.SMLTenantReadiness,
	verificationVersion int,
) (models.SMLTenantReadinessRegistryEntry, error) {
	scope := NormalizeSMLRegistryScope(provider, dataGroup, tenant)
	verifiedAt := time.Now().UTC()
	registryStatus := "not_ready"
	if readiness.OK {
		registryStatus = "ready"
	}
	readiness.Tenant = scope.Tenant
	if strings.TrimSpace(readiness.ImageDatabase) == "" {
		readiness.ImageDatabase = scope.Tenant + "_images"
	}
	readiness.RegistryStatus = registryStatus
	readiness.VerifiedAt = &verifiedAt
	readiness.IsChecking = false
	readiness.Source = "registry"
	readiness.VerificationVersion = verificationVersion
	payload, err := json.Marshal(readiness)
	if err != nil {
		return models.SMLTenantReadinessRegistryEntry{}, err
	}

	entry, err := scanSMLTenantReadinessEntry(s.pool.QueryRow(ctx, `
INSERT INTO sml_tenant_readiness_registry (
    provider, data_group, sml_tenant, status, result, verification_version, verified_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (provider, data_group, sml_tenant) DO UPDATE
SET status = EXCLUDED.status,
    result = EXCLUDED.result,
    verification_version = EXCLUDED.verification_version,
    verified_at = EXCLUDED.verified_at,
    updated_at = now()
RETURNING provider, data_group, sml_tenant, status, result, verification_version, verified_at, updated_at
`, scope.Provider, scope.DataGroup, scope.Tenant, registryStatus, payload, verificationVersion, verifiedAt))
	if err != nil {
		return models.SMLTenantReadinessRegistryEntry{}, err
	}
	return entry, nil
}

func (s *Store) ListSMLTenantReadinessChecksForResume(
	ctx context.Context,
	verificationVersion int,
) ([]models.SMLTenantReadinessRegistryKey, error) {
	rows, err := s.pool.Query(ctx, `
SELECT provider, data_group, sml_tenant
FROM sml_tenant_readiness_registry
WHERE status = 'checking'
   OR verification_version < $1
ORDER BY updated_at ASC
`, verificationVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]models.SMLTenantReadinessRegistryKey, 0)
	for rows.Next() {
		var item models.SMLTenantReadinessRegistryKey
		if err := rows.Scan(&item.Provider, &item.DataGroup, &item.Tenant); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) InvalidateSMLTenantReadiness(
	ctx context.Context,
	provider, dataGroup, tenant string,
	readiness models.SMLTenantReadiness,
	verificationVersion int,
) error {
	readiness.OK = false
	_, err := s.SaveSMLTenantReadiness(ctx, provider, dataGroup, tenant, readiness, verificationVersion)
	return err
}

type tenantReadinessRow interface {
	Scan(dest ...any) error
}

func scanSMLTenantReadinessEntry(row tenantReadinessRow) (models.SMLTenantReadinessRegistryEntry, error) {
	var entry models.SMLTenantReadinessRegistryEntry
	var payload []byte
	err := row.Scan(
		&entry.Provider,
		&entry.DataGroup,
		&entry.Tenant,
		&entry.RegistryStatus,
		&payload,
		&entry.VerificationVersion,
		&entry.VerifiedAt,
		&entry.UpdatedAt,
	)
	if err != nil {
		return models.SMLTenantReadinessRegistryEntry{}, err
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &entry.Readiness); err != nil {
			return models.SMLTenantReadinessRegistryEntry{}, err
		}
	}
	entry.Readiness.RegistryStatus = entry.RegistryStatus
	entry.Readiness.VerifiedAt = entry.VerifiedAt
	entry.Readiness.IsChecking = entry.RegistryStatus == "checking"
	entry.Readiness.Source = "registry"
	entry.Readiness.VerificationVersion = entry.VerificationVersion
	return entry, nil
}

func normalizeTenantList(tenants []string) []string {
	result := make([]string, 0, len(tenants))
	seen := make(map[string]struct{}, len(tenants))
	for _, tenant := range tenants {
		tenant = NormalizeSMLTenant(tenant)
		if _, ok := seen[tenant]; ok {
			continue
		}
		seen[tenant] = struct{}{}
		result = append(result, tenant)
	}
	return result
}
