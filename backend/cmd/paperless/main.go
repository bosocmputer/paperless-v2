package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bosocmputer/paperless-v2/backend/internal/api"
	"github.com/bosocmputer/paperless-v2/backend/internal/config"
	"github.com/bosocmputer/paperless-v2/backend/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.EnsureSchema(ctx); err != nil {
		logger.Error("ensure schema", "error", err)
		os.Exit(1)
	}

	if err := db.EnsureSuperAdmin(ctx, cfg.SeedSuperAdmin); err != nil {
		logger.Error("seed superadmin", "error", err)
		os.Exit(1)
	}

	apiServer := api.NewServer(cfg, db, logger)
	apiServer.StartAutoFinalizeSweeper(ctx)
	apiServer.StartTenantReadinessRegistry(ctx)

	readTimeout := 30 * time.Second
	writeTimeout := cfg.SMLPaperlessTimeout + 15*time.Second
	if writeTimeout < 60*time.Second {
		writeTimeout = 60 * time.Second
	}

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           apiServer.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("paperless api started", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped", "error", err)
			stop()
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown", "error", err)
		os.Exit(1)
	}
}
