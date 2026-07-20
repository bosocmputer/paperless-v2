package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/config"
	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

type fakeTenantReadinessRegistry struct {
	mu          sync.Mutex
	entries     map[string]models.SMLTenantReadinessRegistryEntry
	locks       map[string]bool
	listCalls   int
	markCalls   int
	saveCalls   int
	invalidated int
}

func newFakeTenantReadinessRegistry(entries ...models.SMLTenantReadinessRegistryEntry) *fakeTenantReadinessRegistry {
	registry := &fakeTenantReadinessRegistry{
		entries: make(map[string]models.SMLTenantReadinessRegistryEntry),
		locks:   make(map[string]bool),
	}
	for _, entry := range entries {
		registry.entries[fakeReadinessKey(entry.Provider, entry.DataGroup, entry.Tenant)] = entry
	}
	return registry
}

func (f *fakeTenantReadinessRegistry) ListSMLTenantReadiness(_ context.Context, provider, dataGroup string, tenants []string) (map[string]models.SMLTenantReadinessRegistryEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	result := make(map[string]models.SMLTenantReadinessRegistryEntry)
	for _, tenant := range tenants {
		if entry, ok := f.entries[fakeReadinessKey(provider, dataGroup, tenant)]; ok {
			result[store.NormalizeSMLTenant(tenant)] = entry
		}
	}
	return result, nil
}

func (f *fakeTenantReadinessRegistry) GetSMLTenantReadiness(_ context.Context, provider, dataGroup, tenant string) (models.SMLTenantReadinessRegistryEntry, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry, ok := f.entries[fakeReadinessKey(provider, dataGroup, tenant)]
	return entry, ok, nil
}

func (f *fakeTenantReadinessRegistry) MarkSMLTenantReadinessChecking(_ context.Context, provider, dataGroup, tenant string, version int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markCalls++
	key := fakeReadinessKey(provider, dataGroup, tenant)
	f.entries[key] = models.SMLTenantReadinessRegistryEntry{
		SMLTenantReadinessRegistryKey: models.SMLTenantReadinessRegistryKey{Provider: provider, DataGroup: dataGroup, Tenant: store.NormalizeSMLTenant(tenant)},
		RegistryStatus:                "checking",
		Readiness:                     checkingTenantReadiness(tenant),
		VerificationVersion:           version,
		UpdatedAt:                     time.Now(),
	}
	return nil
}

func (f *fakeTenantReadinessRegistry) SaveSMLTenantReadiness(_ context.Context, provider, dataGroup, tenant string, readiness models.SMLTenantReadiness, version int) (models.SMLTenantReadinessRegistryEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saveCalls++
	now := time.Now().UTC()
	status := "not_ready"
	if readiness.OK {
		status = "ready"
	}
	readiness.RegistryStatus = status
	readiness.Source = "registry"
	readiness.VerifiedAt = &now
	readiness.VerificationVersion = version
	entry := models.SMLTenantReadinessRegistryEntry{
		SMLTenantReadinessRegistryKey: models.SMLTenantReadinessRegistryKey{Provider: provider, DataGroup: dataGroup, Tenant: store.NormalizeSMLTenant(tenant)},
		RegistryStatus:                status,
		Readiness:                     readiness,
		VerificationVersion:           version,
		VerifiedAt:                    &now,
		UpdatedAt:                     now,
	}
	f.entries[fakeReadinessKey(provider, dataGroup, tenant)] = entry
	return entry, nil
}

func (f *fakeTenantReadinessRegistry) ListSMLTenantReadinessChecksForResume(context.Context, int) ([]models.SMLTenantReadinessRegistryKey, error) {
	return nil, nil
}

func (f *fakeTenantReadinessRegistry) InvalidateSMLTenantReadiness(ctx context.Context, provider, dataGroup, tenant string, readiness models.SMLTenantReadiness, version int) error {
	f.mu.Lock()
	f.invalidated++
	f.mu.Unlock()
	_, err := f.SaveSMLTenantReadiness(ctx, provider, dataGroup, tenant, readiness, version)
	return err
}

func (f *fakeTenantReadinessRegistry) TryAdvisoryLock(_ context.Context, key string) (func(), bool, error) {
	f.mu.Lock()
	if f.locks[key] {
		f.mu.Unlock()
		return nil, false, nil
	}
	f.locks[key] = true
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		delete(f.locks, key)
		f.mu.Unlock()
	}, true, nil
}

func fakeReadinessKey(provider, dataGroup, tenant string) string {
	scope := store.NormalizeSMLRegistryScope(provider, dataGroup, tenant)
	return scope.Provider + ":" + scope.DataGroup + ":" + scope.Tenant
}

func readyRegistryEntry(tenant string) models.SMLTenantReadinessRegistryEntry {
	now := time.Now().UTC()
	return models.SMLTenantReadinessRegistryEntry{
		SMLTenantReadinessRegistryKey: models.SMLTenantReadinessRegistryKey{Provider: "data", DataGroup: "sml", Tenant: tenant},
		RegistryStatus:                "ready",
		Readiness: models.SMLTenantReadiness{
			OK: true, Status: "ready", Tenant: tenant, ImageDatabase: tenant + "_images",
		},
		VerificationVersion: tenantReadinessVerificationVersion,
		VerifiedAt:          &now,
		UpdatedAt:           now,
	}
}

func TestMergeSMLDatabaseReadinessUsesOneSharedBatch(t *testing.T) {
	registry := newFakeTenantReadinessRegistry(readyRegistryEntry("vrh"))
	server := NewServer(config.Config{
		SMLReadinessRegistry: true,
		SMLAuthProvider:      "data",
		SMLAuthDataGroup:     "sml",
	}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.readinessStore = registry
	result := smlAuthResult{
		Provider:  "data",
		DataGroup: "sml",
		Databases: []models.SMLAuthDatabase{
			{DatabaseName: "VRH", Tenant: "vrh"},
			{DatabaseName: "TEST", Tenant: "test"},
		},
	}

	if err := server.mergeSMLDatabaseReadiness(context.Background(), &result); err != nil {
		t.Fatal(err)
	}
	if registry.listCalls != 1 {
		t.Fatalf("registry list calls = %d, want one batch query", registry.listCalls)
	}
	if readiness := result.Databases[0].Readiness; readiness == nil || !readiness.OK || readiness.Source != "registry" {
		t.Fatalf("ready database readiness = %#v", readiness)
	}
	if readiness := result.Databases[1].Readiness; readiness == nil || readiness.RegistryStatus != "unverified" {
		t.Fatalf("new database readiness = %#v", readiness)
	}
	if got := len(server.readinessQueue); got != 1 {
		t.Fatalf("queued first checks = %d, want 1", got)
	}
}

func TestVerifySMLTenantReadinessReusesReadyRegistryWithoutFullCheck(t *testing.T) {
	const apiKey = "internal-test-key"
	var readinessCalls atomic.Int32
	smlAPI := newReadinessRegistrySMLServer(t, apiKey, &readinessCalls)
	defer smlAPI.Close()

	registry := newFakeTenantReadinessRegistry(readyRegistryEntry("vrh"))
	server := NewServer(config.Config{
		SMLPaperlessBaseURL:  smlAPI.URL,
		SMLPaperlessAPIKey:   apiKey,
		SMLPaperlessTimeout:  2 * time.Second,
		SMLAuthProvider:      "data",
		SMLAuthDataGroup:     "sml",
		SMLReadinessRegistry: true,
	}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.readinessStore = registry

	response := callVerifyDatabase(t, server)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	if readinessCalls.Load() != 0 {
		t.Fatalf("full readiness calls = %d, want 0 for stored ready result", readinessCalls.Load())
	}
	if registry.markCalls != 0 || registry.saveCalls != 0 {
		t.Fatalf("ready registry was rewritten: mark=%d save=%d", registry.markCalls, registry.saveCalls)
	}
}

func TestVerifySMLTenantReadinessRetriesStoredFailureAndPersistsSuccess(t *testing.T) {
	const apiKey = "internal-test-key"
	var readinessCalls atomic.Int32
	smlAPI := newReadinessRegistrySMLServer(t, apiKey, &readinessCalls)
	defer smlAPI.Close()

	failed := readyRegistryEntry("vrh")
	failed.RegistryStatus = "not_ready"
	failed.Readiness = models.SMLTenantReadiness{Status: "schema_mismatch", Tenant: "vrh", ImageDatabase: "vrh_images"}
	failed.VerifiedAt = nil
	registry := newFakeTenantReadinessRegistry(failed)
	server := NewServer(config.Config{
		SMLPaperlessBaseURL:  smlAPI.URL,
		SMLPaperlessAPIKey:   apiKey,
		SMLPaperlessTimeout:  2 * time.Second,
		SMLAuthProvider:      "data",
		SMLAuthDataGroup:     "sml",
		SMLReadinessRegistry: true,
	}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.readinessStore = registry

	response := callVerifyDatabase(t, server)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	if readinessCalls.Load() != 1 {
		t.Fatalf("full readiness calls = %d, want 1 manual retry", readinessCalls.Load())
	}
	if registry.markCalls != 1 || registry.saveCalls != 1 {
		t.Fatalf("retry writes: mark=%d save=%d, want 1/1", registry.markCalls, registry.saveCalls)
	}
	entry, ok, _ := registry.GetSMLTenantReadiness(context.Background(), "data", "sml", "vrh")
	if !ok || entry.RegistryStatus != "ready" || !entry.Readiness.OK {
		t.Fatalf("persisted readiness = %#v", entry)
	}
}

func TestForceTenantReadinessRechecksStoredReadyResult(t *testing.T) {
	const apiKey = "internal-test-key"
	var readinessCalls atomic.Int32
	smlAPI := newReadinessRegistrySMLServer(t, apiKey, &readinessCalls)
	defer smlAPI.Close()

	ready := readyRegistryEntry("VRH")
	ready.VerifiedAt = nil
	registry := newFakeTenantReadinessRegistry(ready)
	server := NewServer(config.Config{
		SMLPaperlessBaseURL:  smlAPI.URL,
		SMLPaperlessAPIKey:   apiKey,
		SMLPaperlessTimeout:  2 * time.Second,
		SMLAuthProvider:      "data",
		SMLAuthDataGroup:     "sml",
		SMLReadinessRegistry: true,
	}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.readinessStore = registry

	readiness, err := server.verifyAndPersistTenantReadiness(context.Background(), tenantReadinessJob{
		SMLTenantReadinessRegistryKey: models.SMLTenantReadinessRegistryKey{
			Provider: "data", DataGroup: "sml", Tenant: "VRH",
		},
		Force: true,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !readiness.OK || readinessCalls.Load() != 1 {
		t.Fatalf("forced readiness = %#v; full calls=%d, want ready/1", readiness, readinessCalls.Load())
	}
}

func TestMergeSMLDatabaseReadinessNormalizesTenantKey(t *testing.T) {
	registry := newFakeTenantReadinessRegistry(readyRegistryEntry("vrh"))
	server := NewServer(config.Config{
		SMLReadinessRegistry: true,
		SMLAuthProvider:      "data",
		SMLAuthDataGroup:     "sml",
	}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.readinessStore = registry
	result := smlAuthResult{
		Provider: "data", DataGroup: "sml",
		Databases: []models.SMLAuthDatabase{{DatabaseName: "VRH", Tenant: "VRH"}},
	}

	if err := server.mergeSMLDatabaseReadiness(context.Background(), &result); err != nil {
		t.Fatal(err)
	}
	if readiness := result.Databases[0].Readiness; readiness == nil || !readiness.OK {
		t.Fatalf("normalized readiness = %#v", readiness)
	}
	if got := len(server.readinessQueue); got != 0 {
		t.Fatalf("unexpected duplicate check queued = %d", got)
	}
}

func TestConcurrentTenantReadinessVerificationRunsOneFullCheck(t *testing.T) {
	const apiKey = "internal-test-key"
	var readinessCalls atomic.Int32
	smlAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tenants/readiness" {
			http.NotFound(w, r)
			return
		}
		readinessCalls.Add(1)
		time.Sleep(100 * time.Millisecond)
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"data": models.SMLTenantReadiness{
				OK: true, Status: "ready", Tenant: "vrh", ImageDatabase: "vrh_images",
			},
		})
	}))
	defer smlAPI.Close()

	registry := newFakeTenantReadinessRegistry()
	server := NewServer(config.Config{
		SMLPaperlessBaseURL:  smlAPI.URL,
		SMLPaperlessAPIKey:   apiKey,
		SMLPaperlessTimeout:  2 * time.Second,
		SMLAuthProvider:      "data",
		SMLAuthDataGroup:     "sml",
		SMLReadinessRegistry: true,
	}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server.readinessStore = registry
	job := tenantReadinessJob{SMLTenantReadinessRegistryKey: models.SMLTenantReadinessRegistryKey{
		Provider: "data", DataGroup: "sml", Tenant: "vrh",
	}}

	results := make(chan models.SMLTenantReadiness, 2)
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			readiness, err := server.verifyAndPersistTenantReadiness(context.Background(), job, true)
			results <- readiness
			errors <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
		if readiness := <-results; !readiness.OK {
			t.Fatalf("readiness = %#v, want ready", readiness)
		}
	}
	if got := readinessCalls.Load(); got != 1 {
		t.Fatalf("full readiness calls = %d, want 1", got)
	}
}

func TestStructuralSMLReadinessErrorClassification(t *testing.T) {
	for _, code := range []string{
		"tenant_image_database_missing",
		"image_db_missing",
		"doc_images_table_missing",
		"main_db_missing",
		"schema_mismatch",
	} {
		if !isStructuralSMLReadinessError(code) {
			t.Fatalf("%s should invalidate stored readiness", code)
		}
	}
	if isStructuralSMLReadinessError("document_not_found") {
		t.Fatal("business errors must not invalidate stored readiness")
	}
}

func newReadinessRegistrySMLServer(t *testing.T, apiKey string, readinessCalls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != apiKey {
			t.Errorf("X-Api-Key = %q", r.Header.Get("X-Api-Key"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/sml/login":
			writeJSON(w, http.StatusOK, map[string]any{
				"success": true,
				"data": map[string]any{
					"provider": "data", "dataGroup": "sml",
					"user": map[string]any{"userCode": "superadmin", "userName": "System Administrator"},
					"databases": []map[string]any{{
						"dataGroup": "sml", "dataCode": "VRH", "dataName": "VRH", "databaseName": "VRH", "tenant": "vrh",
					}},
				},
			})
		case "/api/v1/tenants/readiness":
			readinessCalls.Add(1)
			writeJSON(w, http.StatusOK, map[string]any{
				"success": true,
				"data": models.SMLTenantReadiness{
					OK: true, Status: "ready", Tenant: "vrh", ImageDatabase: "vrh_images",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func callVerifyDatabase(t *testing.T, server *Server) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(`{"username":"superadmin","password":"secret","databaseName":"VRH"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/sml/verify-database", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)
	return response
}

func decodeVerifyResponse(t *testing.T, response *httptest.ResponseRecorder) models.SMLTenantVerifyResponse {
	t.Helper()
	var payload models.SMLTenantVerifyResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
