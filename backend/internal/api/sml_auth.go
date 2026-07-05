package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

var (
	errSMLAuthInvalidCredentials = errors.New("sml auth invalid credentials")
	errSMLAuthDatabaseDenied     = errors.New("sml auth database denied")
)

type smlAuthLoginRequest struct {
	Provider     string `json:"provider"`
	DataGroup    string `json:"dataGroup"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	DatabaseName string `json:"databaseName,omitempty"`
}

type smlAuthLoginResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Provider  string `json:"provider"`
		DataGroup string `json:"dataGroup"`
		User      struct {
			UserCode  string `json:"userCode"`
			UserName  string `json:"userName"`
			UserLevel int    `json:"userLevel"`
		} `json:"user"`
		Databases        []models.SMLAuthDatabase `json:"databases"`
		SelectedDatabase *models.SMLAuthDatabase  `json:"selectedDatabase"`
	} `json:"data"`
	Error   *smlAPIError `json:"error"`
	Message string       `json:"message"`
}

type smlTenantReadinessResponse struct {
	Success bool                      `json:"success"`
	Data    models.SMLTenantReadiness `json:"data"`
	Error   *smlAPIError              `json:"error"`
	Message string                    `json:"message"`
}

type smlTenantProvisionResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Provisioned bool                      `json:"provisioned"`
		Readiness   models.SMLTenantReadiness `json:"readiness"`
	} `json:"data"`
	Error   *smlAPIError `json:"error"`
	Message string       `json:"message"`
}

type smlAuthResult struct {
	Provider         string
	DataGroup        string
	UserCode         string
	UserName         string
	UserLevel        int
	Databases        []models.SMLAuthDatabase
	SelectedDatabase *models.SMLAuthDatabase
}

func (s *Server) verifySMLLogin(ctx context.Context, username, password, databaseName string) (smlAuthResult, error) {
	if strings.TrimSpace(s.cfg.SMLPaperlessBaseURL) == "" || strings.TrimSpace(s.cfg.SMLPaperlessAPIKey) == "" {
		return smlAuthResult{}, errSMLConfigMissing
	}
	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/auth/sml/login")
	if err != nil {
		return smlAuthResult{}, fmt.Errorf("invalid SML base URL")
	}
	body, err := json.Marshal(smlAuthLoginRequest{
		Provider:     s.cfg.SMLAuthProvider,
		DataGroup:    s.cfg.SMLAuthDataGroup,
		Username:     username,
		Password:     password,
		DatabaseName: databaseName,
	})
	if err != nil {
		return smlAuthResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return smlAuthResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return smlAuthResult{}, err
	}
	defer resp.Body.Close()

	var payload smlAuthLoginResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return smlAuthResult{}, fmt.Errorf("cannot parse SML auth response")
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return smlAuthResult{}, errSMLAuthInvalidCredentials
	}
	if resp.StatusCode == http.StatusForbidden && payload.Error != nil {
		switch strings.TrimSpace(payload.Error.Code) {
		case "database_not_allowed", "auth_database_empty", "auth_scope_invalid":
			return smlAuthResult{}, errSMLAuthDatabaseDenied
		}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return smlAuthResult{}, errors.New(smlErrorMessage(payload.Error, payload.Message, resp.Status))
	}
	if !payload.Success {
		return smlAuthResult{}, errors.New(smlErrorMessage(payload.Error, payload.Message, "SML auth failed"))
	}

	result := smlAuthResult{
		Provider:         payload.Data.Provider,
		DataGroup:        payload.Data.DataGroup,
		UserCode:         payload.Data.User.UserCode,
		UserName:         payload.Data.User.UserName,
		UserLevel:        payload.Data.User.UserLevel,
		Databases:        normalizeSMLAuthDatabases(payload.Data.Databases),
		SelectedDatabase: payload.Data.SelectedDatabase,
	}
	if result.Provider == "" {
		result.Provider = s.cfg.SMLAuthProvider
	}
	if result.DataGroup == "" {
		result.DataGroup = s.cfg.SMLAuthDataGroup
	}
	if result.UserCode == "" {
		result.UserCode = username
	}
	if result.UserName == "" {
		result.UserName = username
	}
	if result.SelectedDatabase != nil {
		selected := normalizeSMLAuthDatabase(*result.SelectedDatabase)
		result.SelectedDatabase = &selected
	}
	return result, nil
}

func (s *Server) fetchSMLTenantReadiness(ctx context.Context, tenant string) (models.SMLTenantReadiness, error) {
	if strings.TrimSpace(s.cfg.SMLPaperlessBaseURL) == "" || strings.TrimSpace(s.cfg.SMLPaperlessAPIKey) == "" {
		return models.SMLTenantReadiness{}, errSMLConfigMissing
	}
	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/tenants/readiness")
	if err != nil {
		return models.SMLTenantReadiness{}, fmt.Errorf("invalid SML base URL")
	}
	query := endpoint.Query()
	query.Set("tenant", store.NormalizeSMLTenant(tenant))
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return models.SMLTenantReadiness{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return models.SMLTenantReadiness{}, err
	}
	defer resp.Body.Close()

	var payload smlTenantReadinessResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return models.SMLTenantReadiness{}, fmt.Errorf("cannot parse SML tenant readiness response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return models.SMLTenantReadiness{}, newSMLRequestError(payload.Error, payload.Message, resp.Status)
	}
	if !payload.Success {
		return models.SMLTenantReadiness{}, newSMLRequestError(payload.Error, payload.Message, "SML tenant readiness failed")
	}
	return payload.Data, nil
}

func (s *Server) provisionSMLTenantImageDatabase(ctx context.Context, tenant string) (models.SMLTenantProvisionResponse, error) {
	if strings.TrimSpace(s.cfg.SMLPaperlessBaseURL) == "" || strings.TrimSpace(s.cfg.SMLPaperlessAPIKey) == "" {
		return models.SMLTenantProvisionResponse{}, errSMLConfigMissing
	}
	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/tenants/image-database")
	if err != nil {
		return models.SMLTenantProvisionResponse{}, fmt.Errorf("invalid SML base URL")
	}
	body, err := json.Marshal(map[string]string{
		"tenant": store.NormalizeSMLTenant(tenant),
	})
	if err != nil {
		return models.SMLTenantProvisionResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return models.SMLTenantProvisionResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return models.SMLTenantProvisionResponse{}, err
	}
	defer resp.Body.Close()

	var payload smlTenantProvisionResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return models.SMLTenantProvisionResponse{}, fmt.Errorf("cannot parse SML tenant provision response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return models.SMLTenantProvisionResponse{}, newSMLRequestError(payload.Error, payload.Message, resp.Status)
	}
	if !payload.Success {
		return models.SMLTenantProvisionResponse{}, newSMLRequestError(payload.Error, payload.Message, "SML tenant provision failed")
	}
	return models.SMLTenantProvisionResponse{
		Provisioned: payload.Data.Provisioned,
		Readiness:   payload.Data.Readiness,
	}, nil
}

func normalizeSMLAuthDatabases(items []models.SMLAuthDatabase) []models.SMLAuthDatabase {
	out := make([]models.SMLAuthDatabase, 0, len(items))
	for _, item := range items {
		item = normalizeSMLAuthDatabase(item)
		if item.Tenant != "" {
			out = append(out, item)
		}
	}
	return out
}

func normalizeSMLAuthDatabase(item models.SMLAuthDatabase) models.SMLAuthDatabase {
	item.DataGroup = strings.TrimSpace(item.DataGroup)
	item.DataCode = strings.TrimSpace(item.DataCode)
	item.DataName = strings.TrimSpace(item.DataName)
	item.DatabaseName = strings.TrimSpace(item.DatabaseName)
	item.Tenant = store.NormalizeSMLTenant(firstNonEmpty(item.Tenant, item.DatabaseName, item.DataCode))
	if item.DatabaseName == "" {
		item.DatabaseName = item.Tenant
	}
	if item.DataCode == "" {
		item.DataCode = strings.ToUpper(item.Tenant)
	}
	if item.DataName == "" {
		item.DataName = item.DataCode
	}
	return item
}

func localFallbackDatabases(defaultTenant, dataGroup string) []models.SMLAuthDatabase {
	tenant := store.NormalizeSMLTenant(defaultTenant)
	return []models.SMLAuthDatabase{{
		DataGroup:    strings.TrimSpace(dataGroup),
		DataCode:     strings.ToUpper(tenant),
		DataName:     strings.ToUpper(tenant),
		DatabaseName: tenant,
		Tenant:       tenant,
	}}
}

func randomLocalPassword() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "paperless-sml-autoprovisioned-password"
	}
	return hex.EncodeToString(buf)
}
