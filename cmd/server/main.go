package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/local/claude-relay/internal/config"
	"github.com/local/claude-relay/internal/cryptox"
	"github.com/local/claude-relay/internal/httpapi"
	"github.com/local/claude-relay/internal/store"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		logger.Error("create data directory", "error", err)
		os.Exit(1)
	}
	vault, err := cryptox.NewVault(cfg.DataDir, cfg.MasterKey)
	if err != nil {
		logger.Error("initialize secret vault", "error", err)
		os.Exit(1)
	}
	data, err := store.Open(cfg.DatabasePath, vault)
	if err != nil {
		logger.Error("initialize database", "error", err)
		os.Exit(1)
	}
	defer data.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	createdAdmin, err := data.BootstrapAdmin(ctx, cfg.AdminUsername, cfg.AdminPassword)
	if err != nil {
		cancel()
		logger.Error("bootstrap administrator", "error", err)
		os.Exit(1)
	}
	createdAccount, err := data.BootstrapAccount(ctx, cfg.BootstrapAPIKey, cfg.BootstrapBaseURL)
	cancel()
	if err != nil {
		logger.Error("bootstrap upstream account", "error", err)
		os.Exit(1)
	}
	if createdAdmin {
		logger.Warn("initial administrator created", "username", cfg.AdminUsername,
			"message", "change the bootstrap password immediately after first login")
	}
	if createdAccount {
		logger.Info("initial Anthropic account imported from environment")
	}

	api, err := httpapi.New(cfg, data, logger, version)
	if err != nil {
		logger.Error("initialize HTTP API", "error", err)
		os.Exit(1)
	}
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	stopMaintenance := make(chan struct{})
	go maintenanceLoop(data, logger, stopMaintenance)

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("Claude Relay started", "listen", cfg.ListenAddr, "version", version)
		serverErrors <- server.ListenAndServe()
	}()

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-shutdownSignal:
		logger.Info("shutdown requested", "signal", sig.String())
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server stopped unexpectedly", "error", err)
		}
	}
	close(stopMaintenance)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		_ = server.Close()
	}
	logger.Info("Claude Relay stopped")
}

func maintenanceLoop(data *store.Store, logger *slog.Logger, stop <-chan struct{}) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			settings, err := data.GetSettings(ctx)
			if err == nil {
				deleted, purgeErr := data.PurgeUsageBefore(ctx, time.Now().AddDate(0, 0, -settings.LogRetentionDays))
				if purgeErr != nil {
					logger.Warn("usage log maintenance failed", "error", purgeErr)
				} else if deleted > 0 {
					logger.Info("expired usage logs removed", "count", deleted)
				}
			}
			_ = data.DeleteExpiredSessions(ctx)
			cancel()
		case <-stop:
			return
		}
	}
}
