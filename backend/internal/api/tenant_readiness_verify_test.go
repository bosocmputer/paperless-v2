package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/config"
	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

func TestVerifySMLTenantReadinessForLoginReturnsLatestReadiness(t *testing.T) {
	const apiKey = "internal-test-key"
	var readinessCalls atomic.Int32
	smlAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != apiKey {
			t.Errorf("X-Api-Key = %q, want test key", r.Header.Get("X-Api-Key"))
		}
		switch r.URL.Path {
		case "/api/v1/auth/sml/login":
			var request smlAuthLoginRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode SML auth request: %v", err)
			}
			if request.Provider != "data" || request.DataGroup != "sml" || request.DatabaseName != "" {
				t.Errorf("unexpected SML auth scope: %#v", request)
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"success": true,
				"data": map[string]any{
					"provider":  "data",
					"dataGroup": "sml",
					"user": map[string]any{
						"userCode": "superadmin",
						"userName": "System Administrator",
					},
					"databases": []map[string]any{{
						"dataGroup": "sml", "dataCode": "VRH", "dataName": "VRH", "databaseName": "VRH", "tenant": "vrh",
					}},
					"selectedDatabase": map[string]any{
						"dataGroup": "sml", "dataCode": "VRH", "dataName": "VRH", "databaseName": "VRH", "tenant": "vrh",
					},
				},
			})
		case "/api/v1/tenants/readiness":
			readinessCalls.Add(1)
			if r.URL.Query().Get("tenant") != "vrh" {
				t.Errorf("tenant = %q, want vrh", r.URL.Query().Get("tenant"))
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"success": true,
				"data": models.SMLTenantReadiness{
					OK:            true,
					Status:        "ready",
					Tenant:        "vrh",
					ImageDatabase: "vrh_images",
					Template:      "vrh_images",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(smlAPI.Close)

	server := NewServer(config.Config{
		SMLPaperlessBaseURL: smlAPI.URL,
		SMLPaperlessAPIKey:  apiKey,
		SMLPaperlessTimeout: 2 * time.Second,
		SMLAuthProvider:     "data",
		SMLAuthDataGroup:    "sml",
	}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	body := []byte(`{"username":"superadmin","password":"secret","databaseName":"VRH","authSource":"sml"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/sml/verify-database", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
	}
	var payload models.SMLTenantVerifyResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Readiness.OK || payload.Readiness.Tenant != "vrh" || payload.Readiness.Template != "vrh_images" {
		t.Fatalf("unexpected readiness: %#v", payload.Readiness)
	}
	if readinessCalls.Load() != 1 {
		t.Fatalf("readiness calls = %d, want 1", readinessCalls.Load())
	}
}

func TestVerifySMLTenantReadinessForLoginRejectsInvalidCredentials(t *testing.T) {
	var readinessCalls atomic.Int32
	smlAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/tenants/readiness" {
			readinessCalls.Add(1)
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false,
			"message": "invalid credentials",
		})
	}))
	t.Cleanup(smlAPI.Close)

	server := NewServer(config.Config{
		SMLPaperlessBaseURL: smlAPI.URL,
		SMLPaperlessAPIKey:  "internal-test-key",
		SMLPaperlessTimeout: 2 * time.Second,
		SMLAuthProvider:     "data",
		SMLAuthDataGroup:    "sml",
	}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	body := []byte(`{"username":"superadmin","password":"wrong","databaseName":"VRH"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/sml/verify-database", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", response.Code, response.Body.String())
	}
	if readinessCalls.Load() != 0 {
		t.Fatalf("readiness calls = %d, want 0 after failed authentication", readinessCalls.Load())
	}
}

func TestVerifySMLTenantReadinessForLoginRejectsDatabaseOutsideUserScope(t *testing.T) {
	var readinessCalls atomic.Int32
	smlAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/sml/login":
			writeJSON(w, http.StatusOK, map[string]any{
				"success": true,
				"data": map[string]any{
					"provider": "data", "dataGroup": "sml",
					"user": map[string]any{"userCode": "limited-user", "userName": "Limited User"},
					"databases": []map[string]any{{
						"dataGroup": "sml", "dataCode": "VRH", "dataName": "VRH", "databaseName": "VRH", "tenant": "vrh",
					}},
				},
			})
		case "/api/v1/tenants/readiness":
			readinessCalls.Add(1)
			writeJSON(w, http.StatusOK, map[string]any{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(smlAPI.Close)

	server := NewServer(config.Config{
		SMLPaperlessBaseURL: smlAPI.URL,
		SMLPaperlessAPIKey:  "internal-test-key",
		SMLPaperlessTimeout: 2 * time.Second,
		SMLAuthProvider:     "data",
		SMLAuthDataGroup:    "sml",
	}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	body := []byte(`{"username":"limited-user","password":"secret","databaseName":"OTHER"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/sml/verify-database", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", response.Code, response.Body.String())
	}
	if readinessCalls.Load() != 0 {
		t.Fatalf("readiness calls = %d, want 0 for an unauthorized database", readinessCalls.Load())
	}
}
