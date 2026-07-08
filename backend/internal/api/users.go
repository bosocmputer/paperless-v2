package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/auth"
	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

type smlUserSyncCandidatesRequest struct {
	Provider     string `json:"provider"`
	DataGroup    string `json:"dataGroup"`
	DatabaseName string `json:"databaseName"`
}

type smlUserSyncCandidatesResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Provider  string `json:"provider"`
		DataGroup string `json:"dataGroup"`
		Database  struct {
			DataCode     string `json:"dataCode"`
			DataName     string `json:"dataName"`
			DatabaseName string `json:"databaseName"`
			Tenant       string `json:"tenant"`
		} `json:"database"`
		Users []struct {
			UserCode       string `json:"userCode"`
			UserName       string `json:"userName"`
			PasswordHash   string `json:"passwordHash"`
			PasswordSynced bool   `json:"passwordSynced"`
		} `json:"users"`
		Summary struct {
			TotalAllowed      int `json:"totalAllowed"`
			Active            int `json:"active"`
			SkippedInactive   int `json:"skippedInactive"`
			PasswordNotSynced int `json:"passwordNotSynced"`
		} `json:"summary"`
	} `json:"data"`
	Error   *smlAPIError `json:"error"`
	Message string       `json:"message"`
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.logger.Error("list users failed", "error", err)
		writeError(w, http.StatusInternalServerError, "users_failed", "Cannot load users right now.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) syncSMLUsers(w http.ResponseWriter, r *http.Request) {
	var req models.SyncSMLUsersRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
			return
		}
	}

	actor, _ := currentUser(r)
	session, ok := currentSession(r)
	if !ok || strings.TrimSpace(session.SMLTenant) == "" {
		writeError(w, http.StatusBadRequest, "missing_sml_session", "Please login with an SML database before syncing users.")
		return
	}

	start := time.Now()
	candidates, summary, syncDatabase, err := s.fetchSMLUserSyncCandidates(r.Context(), session)
	if errors.Is(err, errSMLConfigMissing) {
		writeError(w, http.StatusServiceUnavailable, "sml_not_configured", "SML PaperLess API is not configured.")
		return
	}
	if err != nil {
		s.logger.Warn("SML user sync candidates failed", "error", err, "tenant", session.SMLTenant, "actor", actor.Username)
		writeError(w, http.StatusBadGateway, "sml_user_sync_failed", "Cannot load users from SML right now.")
		return
	}

	if !req.DryRun {
		for i := range candidates {
			if strings.TrimSpace(candidates[i].PasswordHash) != "" {
				continue
			}
			hash, err := auth.HashPassword(randomLocalPassword())
			if err != nil {
				s.logger.Error("hash fallback SML sync password failed", "error", err, "tenant", session.SMLTenant)
				writeError(w, http.StatusInternalServerError, "sml_user_sync_failed", "Cannot prepare synced user passwords right now.")
				return
			}
			candidates[i].PasswordHash = hash
		}
	}

	result, err := s.store.SyncSMLUsers(r.Context(), models.SMLUserSyncInput{
		Tenant:     session.SMLTenant,
		DryRun:     req.DryRun,
		Candidates: candidates,
	})
	if errors.Is(err, store.ErrSMLUserSyncBatchTooLarge) {
		writeError(w, http.StatusBadRequest, "sml_user_sync_too_large", "Too many SML users to sync at once.")
		return
	}
	if errors.Is(err, store.ErrSMLUserPasswordHashMissing) {
		writeError(w, http.StatusBadGateway, "sml_user_password_not_ready", "Cannot prepare SML user passwords for sync.")
		return
	}
	if err != nil {
		s.logger.Error("sync SML users failed", "error", err, "tenant", session.SMLTenant)
		writeError(w, http.StatusInternalServerError, "sml_user_sync_failed", "Cannot sync SML users right now.")
		return
	}

	response := models.SMLUserSyncResponse{
		DryRun:            req.DryRun,
		Tenant:            syncDatabase.Tenant,
		DataCode:          syncDatabase.DataCode,
		DataName:          syncDatabase.DataName,
		TotalAllowed:      summary.TotalAllowed,
		Active:            summary.Active,
		Existing:          result.Existing,
		ToCreate:          result.ToCreate,
		ToActivate:        result.ToActivate,
		Created:           result.Created,
		Activated:         result.Activated,
		SkippedInactive:   summary.SkippedInactive,
		PasswordNotSynced: result.PasswordNotSynced,
		Users:             result.Users,
	}

	if !req.DryRun {
		if err := s.store.WriteAuditWithMetadata(r.Context(), actor.ID, "user.sml_sync", "tenant", syncDatabase.Tenant, clientIP(r), r.UserAgent(), map[string]any{
			"tenant":            syncDatabase.Tenant,
			"dataCode":          syncDatabase.DataCode,
			"totalAllowed":      response.TotalAllowed,
			"active":            response.Active,
			"existing":          response.Existing,
			"toActivate":        response.ToActivate,
			"created":           response.Created,
			"activated":         response.Activated,
			"skippedInactive":   response.SkippedInactive,
			"passwordNotSynced": response.PasswordNotSynced,
			"elapsedMs":         time.Since(start).Milliseconds(),
		}); err != nil {
			s.logger.Warn("write SML user sync audit failed", "error", err, "tenant", syncDatabase.Tenant)
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req models.CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	req = normalizeCreateUser(req)
	if message := validateCreateUser(req); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_user", message)
		return
	}

	user, err := s.store.CreateUser(r.Context(), req)
	if errors.Is(err, store.ErrUsernameTaken) {
		writeError(w, http.StatusConflict, "username_taken", "Username already exists.")
		return
	}
	if err != nil {
		s.logger.Error("create user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "user_create_failed", "Cannot create user right now.")
		return
	}

	actor, _ := currentUser(r)
	if err := s.store.WriteAudit(r.Context(), actor.ID, "user.create", "user", user.ID, clientIP(r), r.UserAgent()); err != nil {
		s.logger.Warn("write user create audit failed", "error", err, "userID", user.ID)
	}

	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *Server) fetchSMLUserSyncCandidates(ctx context.Context, session models.AuthSession) ([]models.SMLUserSyncCandidate, struct {
	TotalAllowed      int
	Active            int
	SkippedInactive   int
	PasswordNotSynced int
}, struct {
	DataCode string
	DataName string
	Tenant   string
}, error) {
	var emptySummary struct {
		TotalAllowed      int
		Active            int
		SkippedInactive   int
		PasswordNotSynced int
	}
	var emptyDatabase struct {
		DataCode string
		DataName string
		Tenant   string
	}
	if strings.TrimSpace(s.cfg.SMLPaperlessBaseURL) == "" || strings.TrimSpace(s.cfg.SMLPaperlessAPIKey) == "" {
		return nil, emptySummary, emptyDatabase, errSMLConfigMissing
	}
	endpoint, err := url.Parse(s.cfg.SMLPaperlessBaseURL + "/api/v1/auth/sml/users/sync-candidates")
	if err != nil {
		return nil, emptySummary, emptyDatabase, fmt.Errorf("invalid SML base URL")
	}
	databaseName := strings.TrimSpace(session.SMLDataCode)
	if databaseName == "" {
		databaseName = session.SMLTenant
	}
	body, err := json.Marshal(smlUserSyncCandidatesRequest{
		Provider:     firstNonEmpty(session.SMLProvider, s.cfg.SMLAuthProvider),
		DataGroup:    firstNonEmpty(session.SMLDataGroup, s.cfg.SMLAuthDataGroup),
		DatabaseName: databaseName,
	})
	if err != nil {
		return nil, emptySummary, emptyDatabase, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, emptySummary, emptyDatabase, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.cfg.SMLPaperlessAPIKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, emptySummary, emptyDatabase, err
	}
	defer resp.Body.Close()

	var payload smlUserSyncCandidatesResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, emptySummary, emptyDatabase, fmt.Errorf("cannot parse SML user sync response")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, emptySummary, emptyDatabase, newSMLRequestError(payload.Error, payload.Message, resp.Status)
	}
	if !payload.Success {
		return nil, emptySummary, emptyDatabase, newSMLRequestError(payload.Error, payload.Message, "SML user sync failed")
	}

	candidates := make([]models.SMLUserSyncCandidate, 0, len(payload.Data.Users))
	for _, item := range payload.Data.Users {
		username := strings.TrimSpace(item.UserCode)
		if username == "" {
			continue
		}
		displayName := strings.TrimSpace(item.UserName)
		if displayName == "" {
			displayName = username
		}
		candidates = append(candidates, models.SMLUserSyncCandidate{
			Username:       username,
			DisplayName:    displayName,
			PasswordHash:   strings.TrimSpace(item.PasswordHash),
			PasswordSynced: item.PasswordSynced,
		})
	}
	summary := struct {
		TotalAllowed      int
		Active            int
		SkippedInactive   int
		PasswordNotSynced int
	}{
		TotalAllowed:      payload.Data.Summary.TotalAllowed,
		Active:            payload.Data.Summary.Active,
		SkippedInactive:   payload.Data.Summary.SkippedInactive,
		PasswordNotSynced: payload.Data.Summary.PasswordNotSynced,
	}
	database := struct {
		DataCode string
		DataName string
		Tenant   string
	}{
		DataCode: strings.TrimSpace(payload.Data.Database.DataCode),
		DataName: strings.TrimSpace(payload.Data.Database.DataName),
		Tenant:   store.NormalizeSMLTenant(firstNonEmpty(payload.Data.Database.Tenant, payload.Data.Database.DatabaseName, session.SMLTenant)),
	}
	return candidates, summary, database, nil
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_user_id", "User id is required.")
		return
	}

	var req models.UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	req = normalizeUpdateUser(req)
	if message := validateUpdateUser(req); message != "" {
		writeError(w, http.StatusBadRequest, "invalid_user", message)
		return
	}

	current, err := s.store.FindUserByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User was not found.")
		return
	}

	actor, _ := currentUser(r)
	if actor.ID == id && req.Status != "active" {
		writeError(w, http.StatusBadRequest, "cannot_disable_self", "You cannot disable your own account.")
		return
	}
	if actor.ID == id && current.Role == "superadmin" && req.Role != "superadmin" {
		writeError(w, http.StatusBadRequest, "cannot_demote_self", "You cannot remove your own superadmin role.")
		return
	}
	if current.Role == "superadmin" && (req.Role != "superadmin" || req.Status != "active") {
		adminCount, err := s.store.CountActiveSuperAdmins(r.Context())
		if err != nil {
			s.logger.Error("count superadmins failed", "error", err)
			writeError(w, http.StatusInternalServerError, "user_update_failed", "Cannot update user right now.")
			return
		}
		if adminCount <= 1 {
			writeError(w, http.StatusBadRequest, "last_superadmin", "At least one active superadmin is required.")
			return
		}
	}
	if !strings.EqualFold(req.Username, current.Username) || (current.Status == "active" && req.Status != "active") {
		message, err := s.userDocumentConfigReferenceMessage(r.Context(), current)
		if err != nil {
			s.logger.Error("check user document config references failed", "error", err, "userID", id)
			writeError(w, http.StatusInternalServerError, "user_reference_check_failed", "Cannot update user right now.")
			return
		}
		if message != "" {
			writeError(w, http.StatusConflict, "user_in_document_config", message)
			return
		}
	}

	user, err := s.store.UpdateUser(r.Context(), id, req)
	if errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "user_not_found", "User was not found.")
		return
	}
	if errors.Is(err, store.ErrUsernameTaken) {
		writeError(w, http.StatusConflict, "username_taken", "Username already exists.")
		return
	}
	if err != nil {
		s.logger.Error("update user failed", "error", err, "userID", id)
		writeError(w, http.StatusInternalServerError, "user_update_failed", "Cannot update user right now.")
		return
	}

	if err := s.store.WriteAudit(r.Context(), actor.ID, "user.update", "user", user.ID, clientIP(r), r.UserAgent()); err != nil {
		s.logger.Warn("write user update audit failed", "error", err, "userID", user.ID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) deactivateUser(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_user_id", "User id is required.")
		return
	}

	user, err := s.store.FindUserByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user_not_found", "User was not found.")
		return
	}

	actor, _ := currentUser(r)
	if actor.ID == id {
		writeError(w, http.StatusBadRequest, "cannot_disable_self", "You cannot disable your own account.")
		return
	}
	if user.Role == "superadmin" && user.Status == "active" {
		adminCount, err := s.store.CountActiveSuperAdmins(r.Context())
		if err != nil {
			s.logger.Error("count superadmins failed", "error", err)
			writeError(w, http.StatusInternalServerError, "user_deactivate_failed", "Cannot deactivate user right now.")
			return
		}
		if adminCount <= 1 {
			writeError(w, http.StatusBadRequest, "last_superadmin", "At least one active superadmin is required.")
			return
		}
	}
	message, err := s.userDocumentConfigReferenceMessage(r.Context(), user)
	if err != nil {
		s.logger.Error("check user document config references failed", "error", err, "userID", id)
		writeError(w, http.StatusInternalServerError, "user_reference_check_failed", "Cannot deactivate user right now.")
		return
	}
	if message != "" {
		writeError(w, http.StatusConflict, "user_in_document_config", message)
		return
	}

	update := models.UpdateUserRequest{
		DisplayName: user.DisplayName,
		Username:    user.Username,
		Role:        user.Role,
		Status:      "inactive",
	}
	updated, err := s.store.UpdateUser(r.Context(), id, update)
	if err != nil {
		s.logger.Error("deactivate user failed", "error", err, "userID", id)
		writeError(w, http.StatusInternalServerError, "user_deactivate_failed", "Cannot deactivate user right now.")
		return
	}

	if err := s.store.WriteAudit(r.Context(), actor.ID, "user.deactivate", "user", updated.ID, clientIP(r), r.UserAgent()); err != nil {
		s.logger.Warn("write user deactivate audit failed", "error", err, "userID", updated.ID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
}

func normalizeCreateUser(req models.CreateUserRequest) models.CreateUserRequest {
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Username = strings.TrimSpace(req.Username)
	req.Role = normalizeRole(req.Role)
	req.Status = normalizeStatus(req.Status)
	return req
}

func normalizeUpdateUser(req models.UpdateUserRequest) models.UpdateUserRequest {
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Username = strings.TrimSpace(req.Username)
	req.Password = strings.TrimSpace(req.Password)
	req.Role = normalizeRole(req.Role)
	req.Status = normalizeStatus(req.Status)
	return req
}

func normalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return "user"
	}
	return role
}

func normalizeStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return "active"
	}
	return status
}

func validateCreateUser(req models.CreateUserRequest) string {
	if req.DisplayName == "" {
		return "Name is required."
	}
	if req.Username == "" {
		return "Username is required."
	}
	if len(req.Password) < 6 {
		return "Password must be at least 6 characters."
	}
	return validateUserFields(req.Role, req.Status)
}

func validateUpdateUser(req models.UpdateUserRequest) string {
	if req.DisplayName == "" {
		return "Name is required."
	}
	if req.Username == "" {
		return "Username is required."
	}
	if req.Password != "" && len(req.Password) < 6 {
		return "Password must be at least 6 characters."
	}
	return validateUserFields(req.Role, req.Status)
}

func validateUserFields(role, status string) string {
	if role != "superadmin" && role != "admin" && role != "user" {
		return "Role must be superadmin, admin, or user."
	}
	if status != "active" && status != "inactive" {
		return "Status must be active or inactive."
	}
	return ""
}

func (s *Server) userDocumentConfigReferenceMessage(ctx context.Context, user models.User) (string, error) {
	refs, err := s.store.ListDocumentConfigUserReferences(ctx, user.Username)
	if err != nil {
		return "", err
	}
	if len(refs) == 0 {
		return "", nil
	}
	first := refs[0]
	message := fmt.Sprintf(
		"User %s is used in Config เอกสาร %s Position %s (%s). Replace this user in Config เอกสาร before changing username or deactivating.",
		user.Username,
		first.DocFormatCode,
		first.PositionCode,
		first.PositionName,
	)
	if len(refs) > 1 {
		message = fmt.Sprintf("%s Also used by %d more position(s).", message, len(refs)-1)
	}
	return message, nil
}
