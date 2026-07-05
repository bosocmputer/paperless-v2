package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/auth"
	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

type contextKey string

const userContextKey contextKey = "user"
const sessionContextKey contextKey = "session"

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if token == "" || token == header {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Login is required.")
			return
		}

		claims, err := auth.ParseToken(s.cfg.JWTSecret, token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Session is invalid or expired.")
			return
		}

		user, err := s.store.FindUserByID(r.Context(), claims.Subject)
		if err != nil || user.Status != "active" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "User is not active.")
			return
		}

		session := s.sessionFromClaims(claims)
		ctx := context.WithValue(r.Context(), userContextKey, user)
		ctx = context.WithValue(ctx, sessionContextKey, session)
		ctx = store.WithSMLTenant(ctx, session.SMLTenant)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) sessionFromClaims(claims auth.Claims) models.AuthSession {
	session := models.AuthSession{
		SMLProvider:  strings.TrimSpace(claims.SMLProvider),
		SMLDataGroup: strings.TrimSpace(claims.SMLDataGroup),
		SMLDataCode:  strings.TrimSpace(claims.SMLDataCode),
		SMLTenant:    store.NormalizeSMLTenant(claims.SMLTenant),
		AuthSource:   strings.TrimSpace(claims.AuthSource),
	}
	if session.SMLProvider == "" {
		session.SMLProvider = s.cfg.SMLAuthProvider
	}
	if session.SMLDataGroup == "" {
		session.SMLDataGroup = s.cfg.SMLAuthDataGroup
	}
	if session.SMLDataCode == "" {
		session.SMLDataCode = strings.ToUpper(session.SMLTenant)
	}
	if session.AuthSource == "" {
		session.AuthSource = "legacy"
	}
	return session
}

func currentUser(r *http.Request) (models.User, bool) {
	user, ok := r.Context().Value(userContextKey).(models.User)
	return user, ok
}

func currentSession(r *http.Request) (models.AuthSession, bool) {
	session, ok := r.Context().Value(sessionContextKey).(models.AuthSession)
	return session, ok
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := currentUser(r)
		if !isAdminRole(user.Role) {
			writeError(w, http.StatusForbidden, "forbidden", "Admin permission is required.")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (s *Server) requireSuperAdmin(next http.Handler) http.Handler {
	return s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := currentUser(r)
		if user.Role != "superadmin" {
			writeError(w, http.StatusForbidden, "forbidden", "Superadmin permission is required.")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func isAdminRole(role string) bool {
	return role == "admin" || role == "superadmin"
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				s.logger.Error("panic recovered", "value", value, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal_error", "Unexpected server error.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
