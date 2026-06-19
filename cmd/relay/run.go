/*
Copyright 2026 Philip Niedertscheider

Licensed under the Functional Source License, Version 1.1, MIT Future License
(FSL-1.1-MIT). See the LICENSE.md file in the repository root for the full
terms. SPDX-License-Identifier: FSL-1.1-MIT
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"sigs.k8s.io/yaml"

	"github.com/philprime/iris/internal/config"
	"github.com/philprime/iris/internal/observability"
	"github.com/philprime/iris/internal/relay"
)

func run(parent context.Context) error {
	var cfg config.Relay
	if err := config.Load(&cfg); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Sentry.Release == "" {
		cfg.Sentry.Release = sentryReleaseID()
	}

	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	terminal := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger, flush := observability.Setup(ctx, cfg.Sentry, terminal)
	defer flush()

	raw, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("read relay config: %w", err)
	}
	var relayConfig relay.Config
	if err := yaml.Unmarshal(raw, &relayConfig); err != nil {
		return fmt.Errorf("parse relay config: %w", err)
	}

	targets, err := relay.BuildTargets(relayConfig, cfg.MountDir, &http.Client{Timeout: 30 * time.Second})
	if err != nil {
		return fmt.Errorf("build targets: %w", err)
	}

	backend := relay.NewBackend(relay.BackendConfig{
		Routes:      relayConfig.Routes,
		Filters:     relayConfig.Filters,
		Idempotency: relayConfig.Idempotency,
		Targets:     targets,
	}, logger)

	smtpServer := smtp.NewServer(backend)
	smtpServer.Addr = cfg.SMTPAddr
	smtpServer.Domain = "iris-relay"
	smtpServer.AllowInsecureAuth = true

	adminServer := &http.Server{Addr: cfg.AdminAddr, Handler: adminMux(), ReadHeaderTimeout: 5 * time.Second}

	errc := make(chan error, 2)
	go func() {
		logger.InfoContext(ctx, "starting relay admin server", slog.String("addr", cfg.AdminAddr))
		if err := adminServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("admin server: %w", err)
		}
	}()
	go func() {
		logger.InfoContext(ctx, "starting relay smtp server",
			slog.String("addr", cfg.SMTPAddr),
			slog.String("version", version), slog.String("commit", commit), slog.String("buildDate", date))
		if err := smtpServer.ListenAndServe(); err != nil && !errors.Is(err, smtp.ErrServerClosed) {
			errc <- fmt.Errorf("smtp server: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down relay")
	case err := <-errc:
		stop()
		_ = smtpServer.Close()
		shutdownAdmin(adminServer)
		return err
	}

	_ = smtpServer.Close()
	shutdownAdmin(adminServer)
	return nil
}

func adminMux() *http.ServeMux {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	mux.HandleFunc("/livez", ok)
	mux.HandleFunc("/readyz", ok)
	mux.HandleFunc("/healthz", ok)
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

func shutdownAdmin(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

// sentryReleaseID resolves the Sentry release from the ldflags value or the
// build version and commit.
func sentryReleaseID() string {
	if sentryRelease != "" {
		return sentryRelease
	}
	return observability.ReleaseID(version, commit)
}
