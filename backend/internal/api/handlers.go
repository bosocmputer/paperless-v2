package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/bosocmputer/paperless-v2/backend/internal/auth"
	"github.com/bosocmputer/paperless-v2/backend/internal/models"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
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
	req.DatabaseName = strings.TrimSpace(req.DatabaseName)

	smlResult, smlErr := s.verifySMLLogin(r.Context(), req.Username, req.Password, req.DatabaseName)
	if smlErr == nil {
		s.handleSMLLoginSuccess(w, r, req, smlResult)
		return
	}

	if errors.Is(smlErr, errSMLAuthDatabaseDenied) {
		writeError(w, http.StatusForbidden, "database_not_allowed", "Database is not allowed for this user.")
		return
	}

	if s.cfg.LocalAuthFallback {
		if ok := s.handleLocalFallbackLogin(w, r, req); ok {
			return
		}
	}

	if errors.Is(smlErr, errSMLAuthInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Username or password is incorrect.")
		return
	}
	if errors.Is(smlErr, errSMLConfigMissing) {
		writeError(w, http.StatusServiceUnavailable, "sml_not_configured", "SML PaperLess API is not configured.")
		return
	}
	s.logger.Warn("SML login failed", "error", smlErr, "username", req.Username)
	writeError(w, http.StatusBadGateway, "sml_login_failed", "Cannot verify SML login right now.")
}

func (s *Server) verifySMLTenantReadinessForLogin(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)

	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.DatabaseName = strings.TrimSpace(req.DatabaseName)
	if req.Username == "" || req.Password == "" || req.DatabaseName == "" {
		writeError(w, http.StatusBadRequest, "missing_credentials", "Username, password, and database are required.")
		return
	}

	smlResult, err := s.verifySMLLogin(r.Context(), req.Username, req.Password, req.DatabaseName)
	if errors.Is(err, errSMLAuthInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Username or password is incorrect.")
		return
	}
	if errors.Is(err, errSMLAuthDatabaseDenied) {
		writeError(w, http.StatusForbidden, "database_not_allowed", "Database is not allowed for this user.")
		return
	}
	if errors.Is(err, errSMLConfigMissing) {
		writeError(w, http.StatusServiceUnavailable, "sml_not_configured", "SML PaperLess API is not configured.")
		return
	}
	if err != nil {
		s.logger.Warn("SML login check before tenant readiness verification failed", "error", err, "username", req.Username)
		writeError(w, http.StatusBadGateway, "sml_login_failed", "Cannot verify SML login right now.")
		return
	}
	if smlResult.SelectedDatabase == nil {
		writeError(w, http.StatusForbidden, "database_not_allowed", "Database is not allowed for this user.")
		return
	}

	tenant := smlResult.SelectedDatabase.Tenant
	readiness, err := s.fetchSMLTenantReadiness(r.Context(), tenant)
	if err != nil {
		s.logger.Warn("SML tenant readiness verification failed", "error", err, "tenant", tenant, "username", req.Username)
		writeError(w, http.StatusBadGateway, "tenant_readiness_failed", "Cannot verify selected database readiness right now.")
		return
	}

	s.logger.Info("SML tenant readiness verified during login", "tenant", tenant, "status", readiness.Status, "ready", readiness.OK, "username", req.Username)
	writeJSON(w, http.StatusOK, models.SMLTenantVerifyResponse{Readiness: readiness})
}

func (s *Server) provisionSMLTenantImageDatabaseForLogin(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON.")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.DatabaseName = strings.TrimSpace(req.DatabaseName)
	if req.Username == "" || req.Password == "" || req.DatabaseName == "" {
		writeError(w, http.StatusBadRequest, "missing_credentials", "Username, password, and database are required.")
		return
	}

	smlResult, err := s.verifySMLLogin(r.Context(), req.Username, req.Password, req.DatabaseName)
	if errors.Is(err, errSMLAuthInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Username or password is incorrect.")
		return
	}
	if errors.Is(err, errSMLAuthDatabaseDenied) {
		writeError(w, http.StatusForbidden, "database_not_allowed", "Database is not allowed for this user.")
		return
	}
	if errors.Is(err, errSMLConfigMissing) {
		writeError(w, http.StatusServiceUnavailable, "sml_not_configured", "SML PaperLess API is not configured.")
		return
	}
	if err != nil {
		s.logger.Warn("SML login check before image DB provision failed", "error", err, "username", req.Username)
		writeError(w, http.StatusBadGateway, "sml_login_failed", "Cannot verify SML login right now.")
		return
	}
	if smlResult.SelectedDatabase == nil {
		writeError(w, http.StatusForbidden, "database_not_allowed", "Database is not allowed for this user.")
		return
	}

	tenant := smlResult.SelectedDatabase.Tenant
	readiness, err := s.fetchSMLTenantReadiness(r.Context(), tenant)
	if err != nil {
		s.logger.Warn("SML tenant readiness check before image DB provision failed", "error", err, "tenant", tenant)
		writeError(w, http.StatusBadGateway, "tenant_readiness_failed", "Cannot verify selected database readiness right now.")
		return
	}
	if readiness.OK {
		writeJSON(w, http.StatusOK, models.SMLTenantProvisionResponse{
			Provisioned: false,
			Readiness:   readiness,
		})
		return
	}
	if !tenantReadinessCanSelfProvision(readiness) {
		writeError(w, http.StatusFailedDependency, "tenant_not_provisionable", tenantReadinessLoginMessage(readiness))
		return
	}

	provision, err := s.provisionSMLTenantImageDatabase(r.Context(), tenant)
	if err != nil {
		s.logger.Warn("SML tenant image DB provision failed", "error", err, "tenant", tenant, "username", req.Username)
		writeError(w, http.StatusBadGateway, "tenant_image_db_provision_failed", "Cannot prepare SML image database right now.")
		return
	}
	if !provision.Readiness.OK {
		writeError(w, http.StatusFailedDependency, "tenant_still_not_ready", tenantReadinessLoginMessage(provision.Readiness))
		return
	}
	s.logger.Info("SML tenant image DB provisioned", "tenant", tenant, "imageDatabase", provision.Readiness.ImageDatabase, "username", req.Username, "provisioned", provision.Provisioned)
	writeJSON(w, http.StatusOK, provision)
}

func (s *Server) handleSMLLoginSuccess(w http.ResponseWriter, r *http.Request, req models.LoginRequest, result smlAuthResult) {
	if req.DatabaseName == "" {
		writeJSON(w, http.StatusOK, models.LoginResponse{
			DatabaseRequired: true,
			Databases:        result.Databases,
			AuthSource:       "sml",
		})
		return
	}
	if result.SelectedDatabase == nil {
		writeError(w, http.StatusForbidden, "database_not_allowed", "Database is not allowed for this user.")
		return
	}
	readiness, err := s.fetchSMLTenantReadiness(r.Context(), result.SelectedDatabase.Tenant)
	if err != nil {
		s.logger.Warn("SML tenant readiness check failed", "error", err, "tenant", result.SelectedDatabase.Tenant)
		writeError(w, http.StatusBadGateway, "tenant_readiness_failed", "Cannot verify selected database readiness right now.")
		return
	}
	if !readiness.OK {
		writeTenantReadinessError(w, http.StatusFailedDependency, "tenant_not_ready", tenantReadinessLoginMessage(readiness), readiness)
		return
	}

	user, err := s.findOrProvisionSMLUser(r.Context(), req.Username, result)
	if err != nil {
		s.logger.Error("provision SML user failed", "error", err, "username", req.Username)
		writeError(w, http.StatusInternalServerError, "user_provision_failed", "Cannot prepare PaperLess user right now.")
		return
	}
	if user.Status != "active" {
		writeError(w, http.StatusForbidden, "user_inactive", "User account is inactive.")
		return
	}

	session := models.AuthSession{
		SMLProvider:  result.Provider,
		SMLDataGroup: result.SelectedDatabase.DataGroup,
		SMLDataCode:  result.SelectedDatabase.DataCode,
		SMLTenant:    result.SelectedDatabase.Tenant,
		AuthSource:   "sml",
	}
	if session.SMLProvider == "" {
		session.SMLProvider = s.cfg.SMLAuthProvider
	}
	if session.SMLDataGroup == "" {
		session.SMLDataGroup = result.DataGroup
	}
	token, expiresAt, err := auth.IssueToken(s.cfg.JWTSecret, s.cfg.JWTTTL, user, session)
	if err != nil {
		s.logger.Error("issue token failed", "error", err)
		writeError(w, http.StatusInternalServerError, "login_failed", "Cannot create session right now.")
		return
	}

	if err := s.store.WriteAuditWithMetadata(r.Context(), user.ID, "auth.sml_login", "user", user.ID, clientIP(r), r.UserAgent(), map[string]any{
		"smlProvider":  session.SMLProvider,
		"smlDataGroup": session.SMLDataGroup,
		"smlDataCode":  session.SMLDataCode,
		"smlTenant":    session.SMLTenant,
	}); err != nil {
		s.logger.Warn("write login audit failed", "error", err, "userID", user.ID)
	}

	writeJSON(w, http.StatusOK, models.LoginResponse{
		Token:           token,
		TokenType:       "Bearer",
		ExpiresAt:       &expiresAt,
		User:            &user,
		Session:         &session,
		AuthSource:      "sml",
		TenantReadiness: &readiness,
	})
}

func tenantReadinessLoginMessage(readiness models.SMLTenantReadiness) string {
	imageDatabase := strings.TrimSpace(readiness.ImageDatabase)
	switch readiness.Status {
	case "image_db_missing":
		if imageDatabase != "" {
			return "ฐานข้อมูลนี้ยังไม่พร้อมใช้งานใน PaperLess: ไม่พบฐานข้อมูลรูป " + imageDatabase + " กรุณากดตั้งค่า image DB"
		}
		return "ฐานข้อมูลนี้ยังไม่พร้อมใช้งานใน PaperLess: ไม่พบฐานข้อมูลรูป กรุณากดตั้งค่า image DB"
	case "doc_images_table_missing":
		return "ฐานข้อมูลนี้ยังไม่พร้อมใช้งานใน PaperLess: ยังไม่มีตารางรูปเอกสาร กรุณากดตั้งค่า image DB"
	case "main_db_missing":
		return "ฐานข้อมูลนี้ยังไม่พร้อมใช้งานใน PaperLess: ไม่พบฐานข้อมูล SML หลัก กรุณาแจ้งผู้ดูแลระบบ"
	case "schema_mismatch":
		return "ฐานข้อมูลนี้ยังไม่พร้อมใช้งานใน PaperLess: schema ตารางรูปเอกสารไม่ตรงกับมาตรฐาน กรุณาแจ้งผู้ดูแลระบบ"
	default:
		if strings.TrimSpace(readiness.Message) != "" {
			return "ฐานข้อมูลนี้ยังไม่พร้อมใช้งานใน PaperLess: " + readiness.Message
		}
		return "ฐานข้อมูลนี้ยังไม่พร้อมใช้งานใน PaperLess กรุณาแจ้งผู้ดูแลระบบ"
	}
}

func tenantReadinessCanSelfProvision(readiness models.SMLTenantReadiness) bool {
	switch readiness.Status {
	case "image_db_missing", "doc_images_table_missing":
		return strings.TrimSpace(readiness.Tenant) != ""
	default:
		return false
	}
}

func (s *Server) findOrProvisionSMLUser(ctx context.Context, username string, result smlAuthResult) (models.User, error) {
	user, err := s.store.FindUserByUsername(ctx, username)
	if err == nil {
		if sourceErr := s.store.MarkUserSMLSource(ctx, user.ID); sourceErr != nil {
			s.logger.Warn("mark existing user as SML source failed", "error", sourceErr, "userID", user.ID)
		} else {
			user.AccountSource = "sml"
		}
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return models.User{}, err
	}

	role := "admin"
	if strings.EqualFold(strings.TrimSpace(username), "superadmin") || strings.EqualFold(strings.TrimSpace(result.UserCode), "superadmin") {
		role = "superadmin"
	}
	displayName := strings.TrimSpace(result.UserName)
	if displayName == "" {
		displayName = username
	}
	created, err := s.store.CreateSMLUser(ctx, models.CreateUserRequest{
		DisplayName: displayName,
		Username:    username,
		Password:    randomLocalPassword(),
		Role:        role,
		Status:      "active",
	})
	if errors.Is(err, store.ErrUsernameTaken) {
		user, findErr := s.store.FindUserByUsername(ctx, username)
		if findErr == nil {
			if sourceErr := s.store.MarkUserSMLSource(ctx, user.ID); sourceErr != nil {
				s.logger.Warn("mark raced user as SML source failed", "error", sourceErr, "userID", user.ID)
			} else {
				user.AccountSource = "sml"
			}
		}
		return user, findErr
	}
	if err != nil {
		return models.User{}, err
	}
	_ = s.store.WriteAuditWithMetadata(ctx, created.ID, "auth.user_auto_provisioned", "user", created.ID, "", "", map[string]any{
		"source": "sml",
		"role":   role,
	})
	return created, nil
}

func (s *Server) handleLocalFallbackLogin(w http.ResponseWriter, r *http.Request, req models.LoginRequest) bool {
	user, err := s.store.FindUserByUsername(r.Context(), req.Username)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !auth.CheckPassword(req.Password, user.PasswordHash)) {
		return false
	}
	if err != nil {
		s.logger.Error("local fallback login lookup failed", "error", err)
		return false
	}
	if user.Status != "active" {
		writeError(w, http.StatusForbidden, "user_inactive", "User account is inactive.")
		return true
	}

	databases := localFallbackDatabases(s.cfg.SMLPaperlessTenant, s.cfg.SMLAuthDataGroup)
	if req.DatabaseName == "" {
		writeJSON(w, http.StatusOK, models.LoginResponse{
			DatabaseRequired: true,
			Databases:        databases,
			AuthSource:       "local_fallback",
		})
		return true
	}
	selected := normalizeSMLAuthDatabase(models.SMLAuthDatabase{DatabaseName: req.DatabaseName})
	if selected.Tenant != databases[0].Tenant {
		writeError(w, http.StatusForbidden, "database_not_allowed", "Local fallback can only use the default PaperLess tenant.")
		return true
	}
	session := models.AuthSession{
		SMLProvider:  s.cfg.SMLAuthProvider,
		SMLDataGroup: databases[0].DataGroup,
		SMLDataCode:  databases[0].DataCode,
		SMLTenant:    databases[0].Tenant,
		AuthSource:   "local_fallback",
	}
	token, expiresAt, err := auth.IssueToken(s.cfg.JWTSecret, s.cfg.JWTTTL, user, session)
	if err != nil {
		s.logger.Error("issue fallback token failed", "error", err)
		writeError(w, http.StatusInternalServerError, "login_failed", "Cannot create session right now.")
		return true
	}
	if err := s.store.WriteAuditWithMetadata(r.Context(), user.ID, "auth.local_fallback_login", "user", user.ID, clientIP(r), r.UserAgent(), map[string]any{
		"smlTenant": session.SMLTenant,
	}); err != nil {
		s.logger.Warn("write local fallback login audit failed", "error", err, "userID", user.ID)
	}
	writeJSON(w, http.StatusOK, models.LoginResponse{
		Token:      token,
		TokenType:  "Bearer",
		ExpiresAt:  &expiresAt,
		User:       &user,
		Session:    &session,
		AuthSource: "local_fallback",
	})
	return true
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	session, _ := currentSession(r)
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "session": session})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	if err := s.store.WriteAudit(r.Context(), user.ID, "auth.logout", "user", user.ID, clientIP(r), r.UserAgent()); err != nil {
		s.logger.Warn("write logout audit failed", "error", err, "userID", user.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	setNoStoreHeaders(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeRawJSON(w http.ResponseWriter, status int, payload []byte) {
	setNoStoreHeaders(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	_, _ = w.Write(payload)
}

func setNoStoreHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, models.APIError{Error: code, Message: message})
}

func writeTenantReadinessError(w http.ResponseWriter, status int, code, message string, readiness models.SMLTenantReadiness) {
	writeJSON(w, status, map[string]any{
		"error":     code,
		"message":   message,
		"readiness": readiness,
	})
}
