package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/models"
)

type Config struct {
	AppName        string
	Env            string
	Port           string
	DatabaseURL    string
	JWTSecret      string
	JWTTTL         time.Duration
	CORSOrigins    []string
	SeedSuperAdmin models.SeedUser
}

func Load() (Config, error) {
	cfg := Config{
		AppName:     getenv("APP_NAME", "PaperLess V2"),
		Env:         getenv("APP_ENV", "development"),
		Port:        getenv("APP_PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   getenv("JWT_SECRET", "change-this-before-production"),
		CORSOrigins: splitCSV(getenv("APP_CORS_ORIGINS", "http://localhost:5173,http://localhost:3070")),
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

	if strings.TrimSpace(cfg.JWTSecret) == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}

	return cfg, nil
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
