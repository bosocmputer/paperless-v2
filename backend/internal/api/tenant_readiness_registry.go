package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

const (
	tenantReadinessVerificationVersion = 1
	tenantReadinessWorkerCount         = 2
	tenantReadinessQueueSize           = 512
	tenantReadinessManualCooldown      = 5 * time.Second
)

type tenantReadinessJob struct {
	models.SMLTenantReadinessRegistryKey
	Force bool
}

func (s *Server) initTenantReadinessQueue() {
	if s.readinessQueue == nil {
		s.readinessQueue = make(chan tenantReadinessJob, tenantReadinessQueueSize)
	}
	if s.readinessPriority == nil {
		s.readinessPriority = make(chan tenantReadinessJob, tenantReadinessQueueSize)
	}
	if s.readinessSlots == nil {
		s.readinessSlots = make(chan struct{}, tenantReadinessWorkerCount)
	}
	if s.readinessPending == nil {
		s.readinessPending = make(map[string]struct{})
	}
}

func (s *Server) StartTenantReadinessRegistry(ctx context.Context) {
	if !s.cfg.SMLReadinessRegistry || s.readinessStore == nil {
		return
	}
	s.readinessWorkersOnce.Do(func() {
		for i := 0; i < tenantReadinessWorkerCount; i++ {
			go s.runTenantReadinessWorker(ctx)
		}
		go s.resumeStaleTenantReadinessChecks(ctx)
	})
}

func (s *Server) resumeStaleTenantReadinessChecks(ctx context.Context) {
	listCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	items, err := s.readinessStore.ListSMLTenantReadinessChecksForResume(
		listCtx,
		tenantReadinessVerificationVersion,
	)
	if err != nil {
		s.logger.Warn("resume tenant readiness checks failed", "error", err)
		return
	}
	for _, item := range items {
		s.enqueueTenantReadiness(tenantReadinessJob{SMLTenantReadinessRegistryKey: item}, false)
	}
}

func (s *Server) runTenantReadinessWorker(ctx context.Context) {
	for {
		job, ok := s.nextTenantReadinessJob(ctx)
		if !ok {
			return
		}
		checkCtx, cancel := context.WithTimeout(ctx, s.tenantReadinessCheckTimeout())
		_, err := s.verifyAndPersistTenantReadiness(checkCtx, job, false)
		cancel()
		if err != nil && !errors.Is(err, context.Canceled) {
			s.logger.Warn("tenant readiness background check failed", "error", err, "tenant", job.Tenant)
		}
		s.finishTenantReadinessJob(job)
	}
}

func (s *Server) nextTenantReadinessJob(ctx context.Context) (tenantReadinessJob, bool) {
	select {
	case job := <-s.readinessPriority:
		return job, true
	default:
	}
	select {
	case <-ctx.Done():
		return tenantReadinessJob{}, false
	case job := <-s.readinessPriority:
		return job, true
	case job := <-s.readinessQueue:
		return job, true
	}
}

func (s *Server) enqueueTenantReadiness(job tenantReadinessJob, priority bool) {
	if !s.cfg.SMLReadinessRegistry || s.readinessStore == nil {
		return
	}
	job.Provider = strings.ToLower(strings.TrimSpace(job.Provider))
	job.DataGroup = strings.ToLower(strings.TrimSpace(job.DataGroup))
	job.Tenant = store.NormalizeSMLTenant(job.Tenant)
	key := tenantReadinessJobKey(job)

	s.readinessPendingMu.Lock()
	if _, exists := s.readinessPending[key]; exists {
		s.readinessPendingMu.Unlock()
		return
	}
	s.readinessPending[key] = struct{}{}
	s.readinessPendingMu.Unlock()

	queue := s.readinessQueue
	if priority {
		queue = s.readinessPriority
	}
	select {
	case queue <- job:
	default:
		s.finishTenantReadinessJob(job)
		s.logger.Warn("tenant readiness queue is full", "tenant", job.Tenant, "priority", priority)
	}
}

func (s *Server) finishTenantReadinessJob(job tenantReadinessJob) {
	s.readinessPendingMu.Lock()
	delete(s.readinessPending, tenantReadinessJobKey(job))
	s.readinessPendingMu.Unlock()
}

func tenantReadinessJobKey(job tenantReadinessJob) string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(job.Provider)),
		strings.ToLower(strings.TrimSpace(job.DataGroup)),
		store.NormalizeSMLTenant(job.Tenant),
	}, ":")
}

func (s *Server) mergeSMLDatabaseReadiness(ctx context.Context, result *smlAuthResult) error {
	if !s.cfg.SMLReadinessRegistry || s.readinessStore == nil || len(result.Databases) == 0 {
		return nil
	}
	provider := firstNonEmpty(result.Provider, s.cfg.SMLAuthProvider)
	dataGroup := firstNonEmpty(result.DataGroup, s.cfg.SMLAuthDataGroup)
	tenants := make([]string, 0, len(result.Databases))
	for _, database := range result.Databases {
		tenants = append(tenants, database.Tenant)
	}
	entries, err := s.readinessStore.ListSMLTenantReadiness(ctx, provider, dataGroup, tenants)
	if err != nil {
		return err
	}

	for i := range result.Databases {
		database := &result.Databases[i]
		normalizedTenant := store.NormalizeSMLTenant(database.Tenant)
		entry, exists := entries[normalizedTenant]
		if exists && entry.VerificationVersion == tenantReadinessVerificationVersion {
			readiness := readinessFromRegistryEntry(entry)
			database.Readiness = &readiness
			continue
		}
		readiness := unverifiedTenantReadiness(normalizedTenant)
		database.Readiness = &readiness
		s.enqueueTenantReadiness(tenantReadinessJob{SMLTenantReadinessRegistryKey: models.SMLTenantReadinessRegistryKey{
			Provider: provider, DataGroup: dataGroup, Tenant: normalizedTenant,
		}}, false)
	}
	return nil
}

func (s *Server) selectedTenantReadiness(
	ctx context.Context,
	provider, dataGroup, tenant string,
) (models.SMLTenantReadiness, error) {
	if !s.cfg.SMLReadinessRegistry || s.readinessStore == nil {
		readiness, err := s.fetchSMLTenantReadiness(ctx, tenant)
		if err == nil {
			readiness.Source = "live"
		}
		return readiness, err
	}
	entry, exists, err := s.readinessStore.GetSMLTenantReadiness(ctx, provider, dataGroup, tenant)
	if err != nil {
		return models.SMLTenantReadiness{}, err
	}
	if exists && entry.VerificationVersion == tenantReadinessVerificationVersion {
		return readinessFromRegistryEntry(entry), nil
	}
	readiness := unverifiedTenantReadiness(tenant)
	s.enqueueTenantReadiness(tenantReadinessJob{SMLTenantReadinessRegistryKey: models.SMLTenantReadinessRegistryKey{
		Provider: provider, DataGroup: dataGroup, Tenant: tenant,
	}}, true)
	return readiness, nil
}

func (s *Server) verifyAndPersistTenantReadiness(
	ctx context.Context,
	job tenantReadinessJob,
	waitForExisting bool,
) (models.SMLTenantReadiness, error) {
	if !s.cfg.SMLReadinessRegistry || s.readinessStore == nil {
		readiness, err := s.fetchSMLTenantReadiness(ctx, job.Tenant)
		if err == nil {
			readiness.Source = "live"
		}
		return readiness, err
	}

	job.Provider = firstNonEmpty(strings.ToLower(strings.TrimSpace(job.Provider)), s.cfg.SMLAuthProvider)
	job.DataGroup = firstNonEmpty(strings.ToLower(strings.TrimSpace(job.DataGroup)), s.cfg.SMLAuthDataGroup)
	job.Tenant = store.NormalizeSMLTenant(job.Tenant)
	release, locked, err := s.readinessStore.TryAdvisoryLock(ctx, "paperless:sml-readiness:"+tenantReadinessJobKey(job))
	if err != nil {
		return models.SMLTenantReadiness{}, err
	}
	if !locked {
		if waitForExisting {
			return s.waitForTenantReadiness(ctx, job)
		}
		entry, exists, getErr := s.readinessStore.GetSMLTenantReadiness(ctx, job.Provider, job.DataGroup, job.Tenant)
		if getErr != nil {
			return models.SMLTenantReadiness{}, getErr
		}
		if exists {
			return readinessFromRegistryEntry(entry), nil
		}
		return checkingTenantReadiness(job.Tenant), nil
	}
	defer release()

	entry, exists, err := s.readinessStore.GetSMLTenantReadiness(ctx, job.Provider, job.DataGroup, job.Tenant)
	if err != nil {
		return models.SMLTenantReadiness{}, err
	}
	if exists && entry.VerificationVersion == tenantReadinessVerificationVersion {
		if entry.RegistryStatus == "ready" && !job.Force {
			return readinessFromRegistryEntry(entry), nil
		}
		if entry.RegistryStatus == "not_ready" && !job.Force {
			return readinessFromRegistryEntry(entry), nil
		}
		if job.Force && entry.VerifiedAt != nil && time.Since(*entry.VerifiedAt) < tenantReadinessManualCooldown {
			return readinessFromRegistryEntry(entry), nil
		}
	}
	select {
	case s.readinessSlots <- struct{}{}:
		defer func() { <-s.readinessSlots }()
	case <-ctx.Done():
		return models.SMLTenantReadiness{}, ctx.Err()
	}
	if err := s.readinessStore.MarkSMLTenantReadinessChecking(
		ctx, job.Provider, job.DataGroup, job.Tenant, tenantReadinessVerificationVersion,
	); err != nil {
		return models.SMLTenantReadiness{}, err
	}

	startedAt := time.Now()
	readiness, fetchErr := s.fetchSMLTenantReadiness(ctx, job.Tenant)
	if fetchErr != nil {
		readiness = tenantReadinessFromVerificationError(job.Tenant, fetchErr)
	}
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer saveCancel()
	saved, saveErr := s.readinessStore.SaveSMLTenantReadiness(
		saveCtx,
		job.Provider,
		job.DataGroup,
		job.Tenant,
		readiness,
		tenantReadinessVerificationVersion,
	)
	if saveErr != nil {
		return models.SMLTenantReadiness{}, saveErr
	}
	s.logger.Info("SML tenant readiness persisted", "tenant", job.Tenant, "status", saved.RegistryStatus, "ready", saved.Readiness.OK, "elapsedMs", time.Since(startedAt).Milliseconds())
	return readinessFromRegistryEntry(saved), nil
}

func (s *Server) recheckCurrentSMLTenantReadiness(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.SMLReadinessRegistry || s.readinessStore == nil {
		writeError(w, http.StatusNotFound, "tenant_readiness_registry_disabled", "Shared database readiness is not enabled.")
		return
	}
	session, ok := currentSession(r)
	if !ok || strings.TrimSpace(session.SMLTenant) == "" {
		writeError(w, http.StatusBadRequest, "tenant_session_missing", "Select an SML database before checking readiness.")
		return
	}

	job := tenantReadinessJob{
		SMLTenantReadinessRegistryKey: models.SMLTenantReadinessRegistryKey{
			Provider:  firstNonEmpty(session.SMLProvider, s.cfg.SMLAuthProvider),
			DataGroup: firstNonEmpty(session.SMLDataGroup, s.cfg.SMLAuthDataGroup),
			Tenant:    session.SMLTenant,
		},
		Force: true,
	}
	checkCtx, cancel := context.WithTimeout(r.Context(), s.tenantReadinessCheckTimeout())
	defer cancel()
	readiness, err := s.verifyAndPersistTenantReadiness(checkCtx, job, true)
	if err != nil && readiness.RegistryStatus != "checking" {
		s.logger.Warn("superadmin tenant readiness recheck failed", "error", err, "tenant", session.SMLTenant)
		writeError(w, http.StatusBadGateway, "tenant_readiness_failed", "Cannot verify selected database readiness right now.")
		return
	}

	actor, _ := currentUser(r)
	if auditErr := s.store.WriteAuditWithMetadata(r.Context(), actor.ID, "sml_tenant.readiness_recheck", "tenant", store.NormalizeSMLTenant(session.SMLTenant), clientIP(r), r.UserAgent(), map[string]any{
		"tenant":              store.NormalizeSMLTenant(session.SMLTenant),
		"status":              readiness.RegistryStatus,
		"verificationVersion": readiness.VerificationVersion,
	}); auditErr != nil {
		s.logger.Warn("write tenant readiness recheck audit failed", "error", auditErr, "tenant", session.SMLTenant)
	}
	writeJSON(w, http.StatusOK, models.SMLTenantVerifyResponse{Readiness: readiness})
}

func (s *Server) waitForTenantReadiness(ctx context.Context, job tenantReadinessJob) (models.SMLTenantReadiness, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		entry, exists, err := s.readinessStore.GetSMLTenantReadiness(ctx, job.Provider, job.DataGroup, job.Tenant)
		if err != nil {
			return models.SMLTenantReadiness{}, err
		}
		if exists && entry.RegistryStatus != "checking" {
			return readinessFromRegistryEntry(entry), nil
		}
		select {
		case <-ctx.Done():
			return checkingTenantReadiness(job.Tenant), ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Server) tenantReadinessCheckTimeout() time.Duration {
	timeout := s.cfg.SMLPaperlessTimeout + 5*time.Second
	if timeout < 15*time.Second {
		return 15 * time.Second
	}
	return timeout
}

func readinessFromRegistryEntry(entry models.SMLTenantReadinessRegistryEntry) models.SMLTenantReadiness {
	readiness := entry.Readiness
	readiness.RegistryStatus = entry.RegistryStatus
	readiness.VerifiedAt = entry.VerifiedAt
	readiness.IsChecking = entry.RegistryStatus == "checking"
	readiness.Source = "registry"
	readiness.VerificationVersion = entry.VerificationVersion
	if strings.TrimSpace(readiness.Tenant) == "" {
		readiness.Tenant = entry.Tenant
	}
	return readiness
}

func unverifiedTenantReadiness(tenant string) models.SMLTenantReadiness {
	tenant = store.NormalizeSMLTenant(tenant)
	return models.SMLTenantReadiness{
		OK:                  false,
		Status:              "unverified",
		Message:             "ฐานข้อมูลนี้ยังไม่เคยตรวจ ระบบจะตรวจให้อัตโนมัติ",
		Tenant:              tenant,
		ImageDatabase:       tenant + "_images",
		RegistryStatus:      "unverified",
		Source:              "registry",
		VerificationVersion: tenantReadinessVerificationVersion,
	}
}

func checkingTenantReadiness(tenant string) models.SMLTenantReadiness {
	readiness := unverifiedTenantReadiness(tenant)
	readiness.Status = "checking"
	readiness.RegistryStatus = "checking"
	readiness.Message = "กำลังตรวจสอบความพร้อมของฐานข้อมูล"
	readiness.IsChecking = true
	return readiness
}

func (s *Server) invalidateTenantReadinessForStructuralError(ctx context.Context, err error) {
	if !s.cfg.SMLReadinessRegistry || s.readinessStore == nil || err == nil {
		return
	}
	var requestErr *smlRequestError
	if !errors.As(err, &requestErr) || !isStructuralSMLReadinessError(requestErr.Code) {
		return
	}
	tenant := s.smlTenantForContext(ctx)
	readiness := models.SMLTenantReadiness{
		OK:            false,
		Status:        requestErr.Code,
		Message:       "โครงสร้างฐานข้อมูล SML เปลี่ยนและต้องตรวจสอบอีกครั้ง",
		Tenant:        tenant,
		ImageDatabase: tenant + "_images",
		Issues: []models.SMLTenantReadyIssue{{
			Code: requestErr.Code, Owner: "sml_erp", Message: "ฐานข้อมูล SML ไม่ตรงกับโครงสร้างที่ PaperLess ต้องใช้ กรุณาแก้ไขแล้วกดตรวจสอบอีกครั้ง",
		}},
	}
	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if saveErr := s.readinessStore.InvalidateSMLTenantReadiness(
		saveCtx,
		s.cfg.SMLAuthProvider,
		s.cfg.SMLAuthDataGroup,
		tenant,
		readiness,
		tenantReadinessVerificationVersion,
	); saveErr != nil {
		s.logger.Warn("invalidate tenant readiness failed", "error", saveErr, "tenant", tenant, "errorCode", requestErr.Code)
	}
}

func isStructuralSMLReadinessError(code string) bool {
	switch strings.TrimSpace(code) {
	case "tenant_image_database_missing", "image_db_missing", "doc_images_table_missing", "main_db_missing", "schema_mismatch":
		return true
	default:
		return false
	}
}
