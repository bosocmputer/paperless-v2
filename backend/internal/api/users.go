package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.logger.Error("list users failed", "error", err)
		writeError(w, http.StatusInternalServerError, "users_failed", "Cannot load users right now.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
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
	if current.Role == "admin" && (req.Role != "admin" || req.Status != "active") {
		adminCount, err := s.store.CountActiveAdmins(r.Context())
		if err != nil {
			s.logger.Error("count admins failed", "error", err)
			writeError(w, http.StatusInternalServerError, "user_update_failed", "Cannot update user right now.")
			return
		}
		if adminCount <= 1 {
			writeError(w, http.StatusBadRequest, "last_admin", "At least one active admin is required.")
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
	if user.Role == "admin" && user.Status == "active" {
		adminCount, err := s.store.CountActiveAdmins(r.Context())
		if err != nil {
			s.logger.Error("count admins failed", "error", err)
			writeError(w, http.StatusInternalServerError, "user_deactivate_failed", "Cannot deactivate user right now.")
			return
		}
		if adminCount <= 1 {
			writeError(w, http.StatusBadRequest, "last_admin", "At least one active admin is required.")
			return
		}
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
	if role != "admin" && role != "user" {
		return "Role must be admin or user."
	}
	if status != "active" && status != "inactive" {
		return "Status must be active or inactive."
	}
	return ""
}
