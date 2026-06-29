package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/auth"
	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/jackc/pgx/v5"
)

func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"app":    s.cfg.AppName,
	})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "Database is not ready.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "missing_credentials", "Username and password are required.")
		return
	}

	user, err := s.store.FindUserByUsername(r.Context(), req.Username)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !auth.CheckPassword(req.Password, user.PasswordHash)) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Username or password is incorrect.")
		return
	}
	if err != nil {
		s.logger.Error("login lookup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "login_failed", "Cannot login right now.")
		return
	}
	if user.Status != "active" {
		writeError(w, http.StatusForbidden, "user_inactive", "User account is inactive.")
		return
	}

	token, expiresAt, err := auth.IssueToken(s.cfg.JWTSecret, s.cfg.JWTTTL, user)
	if err != nil {
		s.logger.Error("issue token failed", "error", err)
		writeError(w, http.StatusInternalServerError, "login_failed", "Cannot create session right now.")
		return
	}

	if err := s.store.WriteAudit(r.Context(), user.ID, "auth.login", "user", user.ID, clientIP(r), r.UserAgent()); err != nil {
		s.logger.Warn("write login audit failed", "error", err, "userID", user.ID)
	}

	writeJSON(w, http.StatusOK, models.LoginResponse{
		Token:     token,
		TokenType: "Bearer",
		ExpiresAt: expiresAt,
		User:      user,
	})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	if err := s.store.WriteAudit(r.Context(), user.ID, "auth.logout", "user", user.ID, clientIP(r), r.UserAgent()); err != nil {
		s.logger.Warn("write logout audit failed", "error", err, "userID", user.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, models.APIError{Error: code, Message: message})
}
