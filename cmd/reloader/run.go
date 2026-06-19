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

	"github.com/fsnotify/fsnotify"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/philprime/iris/internal/config"
	"github.com/philprime/iris/internal/observability"
)

// debounce is how long to wait for the maps to settle before reloading, since a
// ConfigMap update touches several files at once.
const debounce = 500 * time.Millisecond

func run(parent context.Context) error {
	var cfg config.Reloader
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

	adminServer := &http.Server{Addr: cfg.AdminAddr, Handler: adminMux(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.InfoContext(ctx, "starting reloader admin server", slog.String("addr", cfg.AdminAddr))
		if err := adminServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.ErrorContext(ctx, "admin server failed", slog.Any("error", err))
		}
	}()
	defer shutdownAdmin(adminServer)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()
	if err := watcher.Add(cfg.WatchPath); err != nil {
		return fmt.Errorf("watch %s: %w", cfg.WatchPath, err)
	}

	logger.InfoContext(ctx, "watching postfix maps",
		slog.String("path", cfg.WatchPath),
		slog.String("version", version), slog.String("commit", commit), slog.String("buildDate", date))

	timer := time.NewTimer(debounce)
	timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			timer.Reset(debounce)
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			logger.ErrorContext(ctx, "watch error", slog.Any("error", err))
		case <-timer.C:
			if err := reloadPostfix(ctx, cfg.WatchPath, execRunner); err != nil {
				logger.ErrorContext(ctx, "postfix reload failed", slog.Any("error", err))
				continue
			}
			logger.InfoContext(ctx, "reloaded postfix maps")
		}
	}
}

func adminMux() *http.ServeMux {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	mux.HandleFunc("/livez", ok)
	mux.HandleFunc("/readyz", ok)
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
