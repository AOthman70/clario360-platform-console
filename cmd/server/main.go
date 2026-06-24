package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/clario360/platform-console/internal/audit"
	"github.com/clario360/platform-console/internal/auth"
	"github.com/clario360/platform-console/internal/config"
	"github.com/clario360/platform-console/internal/gateway"
	"github.com/clario360/platform-console/internal/platform/licensing"
	"github.com/clario360/platform-console/internal/platform/overview"
	"github.com/clario360/platform-console/internal/platform/suites"
	"github.com/clario360/platform-console/internal/platform/tenants"
	"github.com/clario360/platform-console/internal/store/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}

	verifier, err := auth.NewVerifier(cfg.JWTPublicKeyPEM, cfg.JWTIssuer, cfg.JWTAudience)
	if err != nil {
		return err
	}

	// Concrete Postgres-backed stores and a fail-closed audit recorder.
	recorder := audit.NewRecorder(postgres.NewAuditSink(pool))
	handlers := gateway.Handlers{
		Overview:  overview.New(postgres.NewOverviewStore(pool)),
		Tenants:   tenants.New(postgres.NewTenantStore(pool), recorder),
		Suites:    suites.New(postgres.NewSuiteStore(pool), recorder),
		Licensing: licensing.New(postgres.NewLicenceStore(pool)),
	}

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      gateway.Router(verifier, handlers),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	errc := make(chan error, 1)
	go func() {
		slog.Info("platform-console listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
