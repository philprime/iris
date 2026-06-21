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

	"github.com/philprime/iris/internal/adminserver"
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
		cfg.Sentry.Release = observability.ResolveRelease(sentryRelease, version, commit)
	}

	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()

	terminal := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger, flush := observability.Setup(ctx, cfg.Sentry, terminal)
	defer flush()

	adminServer := adminserver.New(cfg.AdminAddr, newHealthEngine(logger))
	go func() {
		logger.InfoContext(ctx, "starting reloader admin server", slog.String("addr", cfg.AdminAddr))
		if err := adminServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.ErrorContext(ctx, "admin server failed", slog.Any("error", err))
		}
	}()
	defer adminserver.Shutdown(adminServer)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()
	if err := watcher.Add(cfg.SourcePath); err != nil {
		return fmt.Errorf("watch %s: %w", cfg.SourcePath, err)
	}

	logger.InfoContext(ctx, "watching postfix maps",
		slog.String("source", cfg.SourcePath), slog.String("work", cfg.WorkPath),
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
			err := reload(ctx, cfg.SourcePath, cfg.WorkPath, execRunner)
			lastReloadOK.Store(err == nil)
			if err != nil {
				logger.ErrorContext(ctx, "postfix reload failed", slog.Any("error", err))
				continue
			}
			logger.InfoContext(ctx, "reloaded postfix maps")
		}
	}
}
