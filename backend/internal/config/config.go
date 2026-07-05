package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

const defaultJWTSecret = "change-this-before-production"

type Config struct {
	AppName             string
	Env                 string
	Port                string
	DatabaseURL         string
	JWTSecret           string
	JWTTTL              time.Duration
	CORSOrigins         []string
	SMLPaperlessBaseURL string
	SMLPaperlessAPIKey  string
	SMLPaperlessTenant  string
	SMLPaperlessTimeout time.Duration
	SMLAuthProvider     string
	SMLAuthDataGroup    string
	LocalAuthFallback   bool
	FileStorageDir      string
	MaxUploadMB         int64
	MaxAttachmentMB     int64
	MaxTemplatePages    int
	PublicBaseURL       string
	TelegramBotToken    string
	SeedSuperAdmin      models.SeedUser
}

func Load() (Config, error) {
	cfg := Config{
		AppName:     getenv("APP_NAME", "PaperLess"),
		Env:         getenv("APP_ENV", "development"),
		Port:        getenv("APP_PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   getenv("JWT_SECRET", defaultJWTSecret),
		CORSOrigins: splitCSV(getenv("APP_CORS_ORIGINS", "http://localhost:5173,http://localhost:3070")),
		SMLPaperlessBaseURL: strings.TrimRight(
			getenv("SML_PAPERLESS_BASE_URL", "http://192.168.2.109:8201"),
			"/",
		),
		SMLPaperlessAPIKey: getenv("SML_PAPERLESS_API_KEY", ""),
		SMLPaperlessTenant: strings.ToLower(getenv("SML_PAPERLESS_TENANT", "sml1_2026")),
		SMLAuthProvider:    strings.ToLower(getenv("SML_AUTH_PROVIDER", "smlgoh")),
		SMLAuthDataGroup:   strings.ToLower(getenv("SML_AUTH_DATAGROUP", "sml")),
		LocalAuthFallback:  parseBool(getenv("PAPERLESS_LOCAL_AUTH_FALLBACK_ENABLED", "false")),
		FileStorageDir:     getenv("FILE_STORAGE_DIR", "/app/uploads"),
		PublicBaseURL:      strings.TrimRight(getenv("PUBLIC_BASE_URL", ""), "/"),
		TelegramBotToken:   getenv("TELEGRAM_BOT_TOKEN", ""),
		SeedSuperAdmin: models.SeedUser{
			DisplayName: getenv("SEED_SUPERADMIN_NAME", "System Administrator"),
			Username:    getenv("SEED_SUPERADMIN_USERNAME", "superadmin"),
			Password:    getenv("SEED_SUPERADMIN_PASSWORD", "superadmin"),
			Role:        "admin",
		},
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	ttlHours, err := strconv.Atoi(getenv("JWT_TTL_HOURS", "12"))
	if err != nil || ttlHours <= 0 {
		return Config{}, errors.New("JWT_TTL_HOURS must be a positive integer")
	}
	cfg.JWTTTL = time.Duration(ttlHours) * time.Hour

	timeoutSeconds, err := strconv.Atoi(getenv("SML_PAPERLESS_TIMEOUT_SECONDS", "10"))
	if err != nil || timeoutSeconds <= 0 {
		return Config{}, errors.New("SML_PAPERLESS_TIMEOUT_SECONDS must be a positive integer")
	}
	cfg.SMLPaperlessTimeout = time.Duration(timeoutSeconds) * time.Second

	maxUploadMB, err := strconv.Atoi(getenv("MAX_UPLOAD_MB", "15"))
	if err != nil || maxUploadMB <= 0 {
		return Config{}, errors.New("MAX_UPLOAD_MB must be a positive integer")
	}
	cfg.MaxUploadMB = int64(maxUploadMB)

	maxAttachmentMB, err := strconv.Atoi(getenv("MAX_ATTACHMENT_MB", "10"))
	if err != nil || maxAttachmentMB <= 0 {
		return Config{}, errors.New("MAX_ATTACHMENT_MB must be a positive integer")
	}
	cfg.MaxAttachmentMB = int64(maxAttachmentMB)

	maxTemplatePages, err := strconv.Atoi(getenv("MAX_TEMPLATE_PAGES", "20"))
	if err != nil || maxTemplatePages <= 0 {
		return Config{}, errors.New("MAX_TEMPLATE_PAGES must be a positive integer")
	}
	cfg.MaxTemplatePages = maxTemplatePages

	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}
	if err := cfg.validateProduction(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (cfg Config) validateProduction() error {
	if !strings.EqualFold(strings.TrimSpace(cfg.Env), "production") {
		return nil
	}

	issues := []string{}
	if strings.TrimSpace(cfg.JWTSecret) == "" || cfg.JWTSecret == defaultJWTSecret || len(cfg.JWTSecret) < 32 {
		issues = append(issues, "JWT_SECRET must be set to a non-default value with at least 32 characters")
	}
	if strings.TrimSpace(cfg.SMLPaperlessAPIKey) == "" {
		issues = append(issues, "SML_PAPERLESS_API_KEY is required in production")
	}
	if strings.TrimSpace(cfg.PublicBaseURL) == "" {
		issues = append(issues, "PUBLIC_BASE_URL is required in production")
	}
	if cfg.LocalAuthFallback {
		issues = append(issues, "PAPERLESS_LOCAL_AUTH_FALLBACK_ENABLED must be false in production")
	}
	for _, origin := range cfg.CORSOrigins {
		if strings.TrimSpace(origin) == "*" {
			issues = append(issues, "APP_CORS_ORIGINS must not contain wildcard '*' in production")
			break
		}
	}
	if len(issues) > 0 {
		return fmt.Errorf("invalid production config: %s", strings.Join(issues, "; "))
	}
	return nil
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
