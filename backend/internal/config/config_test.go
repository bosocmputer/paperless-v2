package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRejectsUnsafeProductionDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://paperless:paperless@localhost:5432/paperless?sslmode=disable")
	t.Setenv("JWT_SECRET", defaultJWTSecret)
	t.Setenv("SML_PAPERLESS_API_KEY", "")
	t.Setenv("PUBLIC_BASE_URL", "")
	t.Setenv("APP_CORS_ORIGINS", "*")
	t.Setenv("PAPERLESS_LOCAL_AUTH_FALLBACK_ENABLED", "true")

	_, err := Load()
	if err == nil {
		t.Fatal("expected production config validation to fail")
	}
	message := err.Error()
	for _, want := range []string{
		"JWT_SECRET",
		"SML_PAPERLESS_API_KEY",
		"PUBLIC_BASE_URL",
		"PAPERLESS_LOCAL_AUTH_FALLBACK_ENABLED",
		"APP_CORS_ORIGINS",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected error to mention %s, got %q", want, message)
		}
	}
}

func TestLoadAcceptsSafeProductionConfig(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://paperless:paperless@localhost:5432/paperless?sslmode=disable")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 64))
	t.Setenv("SML_PAPERLESS_API_KEY", strings.Repeat("b", 32))
	t.Setenv("PUBLIC_BASE_URL", "https://paperless.example.com")
	t.Setenv("APP_CORS_ORIGINS", "https://paperless.example.com")
	t.Setenv("PAPERLESS_LOCAL_AUTH_FALLBACK_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected safe production config to load, got %v", err)
	}
	if cfg.Env != "production" {
		t.Fatalf("expected production env, got %q", cfg.Env)
	}
}

func TestLoadAllowsDevelopmentDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://paperless:paperless@localhost:5432/paperless?sslmode=disable")
	t.Setenv("JWT_SECRET", defaultJWTSecret)
	t.Setenv("SML_PAPERLESS_API_KEY", "")
	t.Setenv("PUBLIC_BASE_URL", "")
	t.Setenv("APP_CORS_ORIGINS", "*")
	t.Setenv("PAPERLESS_LOCAL_AUTH_FALLBACK_ENABLED", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected development defaults to load, got %v", err)
	}
	if !cfg.SMLReadinessRegistry {
		t.Fatal("SML tenant readiness registry should be enabled by default")
	}
}

func TestLoadCanDisableSMLTenantReadinessRegistry(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://paperless:paperless@localhost:5432/paperless?sslmode=disable")
	t.Setenv("SML_TENANT_READINESS_REGISTRY_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SMLReadinessRegistry {
		t.Fatal("SML tenant readiness registry should respect the disabled feature flag")
	}
}

func TestLoadTrialExpiresAtUnsetByDefault(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://paperless:paperless@localhost:5432/paperless?sslmode=disable")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TrialExpiresAt != nil {
		t.Fatal("TrialExpiresAt should be nil when TRIAL_EXPIRES_AT is not set")
	}
}

func TestLoadTrialExpiresAtParsesDateAtEndOfDay(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://paperless:paperless@localhost:5432/paperless?sslmode=disable")
	t.Setenv("TRIAL_EXPIRES_AT", "2026-10-08")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TrialExpiresAt == nil {
		t.Fatal("expected TrialExpiresAt to be set")
	}
	want := time.Date(2026, 10, 8, 23, 59, 59, 999999999, time.UTC)
	if !cfg.TrialExpiresAt.Equal(want) {
		t.Fatalf("expected TrialExpiresAt %v, got %v", want, *cfg.TrialExpiresAt)
	}
}

func TestLoadRejectsInvalidTrialExpiresAt(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://paperless:paperless@localhost:5432/paperless?sslmode=disable")
	t.Setenv("TRIAL_EXPIRES_AT", "not-a-date")

	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid TRIAL_EXPIRES_AT to fail")
	}
}
